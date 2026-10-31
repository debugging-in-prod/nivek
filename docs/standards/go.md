# Go standards

These rules apply to `cmd/**` and `internal/**`. Go version and module path
come from `go.mod`. The module path is still
`github.com/tim-the-toolman-taylor/nivek` after the GitHub org rename. Keep it;
GitHub redirects the old path and changing it touches every file.

Formatting and tooling: `gofmt -l cmd internal` must print nothing, and
`go vet ./...` and `go test -race ./...` must pass (CI job `go` in
`.github/workflows/ci.yml`).

## 1. Packages and files

- One feature, one package: `internal/libraries/<feature>` (lowercase, no
  underscores: `dadusage`, `overlayrelay`, `twitcheventsub`).
- Standard file names inside a feature package:

  | File | Contents |
  | --- | --- |
  | `const.go` | `Table<Name>` constants, row structs, domain constants and limits |
  | `service.go` | Service interface, implementation, `NewService` |
  | `validate.go` | Pure validation and normalisation (no I/O) |
  | `store.go` | `Store` interface + DB implementation, when the service needs a test seam |
  | `query.go` / `*.psql` | Hand-written SQL |
  | `<vendor>_client.go` | External client living inside the feature package |
  | `*_test.go` | Unit tests; `*_integration_test.go` with `//go:build integration` for DB tests |

  Use `const.go`, not `constants.go`, for new packages. `api/constants.go` and
  `twitcheventsub/constants.go`, `hangman/constants.go` predate this.
- Chat handlers: `twitchbot/cmd_<name>.go`, one command per file, with tests
  in `cmd_<name>_test.go`.
- core-api endpoints: `cmd/core-api/endpoints/<feature>/<verb>.go` (`get.go`,
  `create.go`, `update.go`, `delete.go`). Bot-facing endpoints go in
  `endpoints/bot/<feature>.go`. Shared code goes in `helpers.go` in the same
  package.
- When an import alias is needed because an endpoint package and a service
  package share a name, use `<name>Svc` for the service and `<name>Ep` for the
  endpoint package (as `register.go` and the endpoints already do).

## 2. Models and constants

- The row struct lives in `const.go` with both tags:
  `db:"column"` and `json:"field"`.
- **SERIAL ids and DB-defaulted timestamp columns need `,omitempty` on the `db`
  tag** (`db:"id,omitempty"`, `db:"created,omitempty"`). Otherwise upper/db
  inserts zero values and fails. This has broken signup before.
- Nullable columns are pointer fields (`*string`, `*int`).
- Table names are constants (`const TableLurk = "lurk"`). Don't write string
  literals in `Collection(...)`.
- Domain limits (max lengths, min intervals, default cooldowns) are named
  constants in the package that owns the rule, with a comment saying why. The
  bot reuses them by importing the package (for example `promo` validation),
  not by copying the number.
- Unexported tunables that tests need to change are `var`, not `const`, with a
  comment (`helixBaseURL`, `bootCommandBackoff`).

## 3. Services

The dominant pattern (lurk, promo, quote, stalk, dad, bread, autoshout, user,
commands):

```go
type NivekQuoteService interface {          // exported interface: Nivek<Feature>Service
    Get(channelID string, n int) (Quote, bool, error)
}

type nivekQuoteServiceImpl struct {          // unexported impl
    nivek nivek.NivekService
    table db.Collection
}

func NewService(svc nivek.NivekService) NivekQuoteService { ... }  // constructor named NewService
```

- The constructor is called `NewService`. If a package has a second service,
  use `New<Thing>Service` (`commands.NewChannelEditorService`).
- It returns the interface, not the impl pointer.
- It takes `nivek.NivekService` and nothing request-scoped. Pass per-call data
  (channel, user) as method arguments, not constructor arguments.
- Build services **once, outside the request closure**, in the endpoint
  constructor (as `endpoints/bot/lurk.go` does), not on every request.
- Services return `(value, error)`. A missing row is reported as
  `found bool` or a package sentinel error (`ErrNotFound`), not as
  `db.ErrNoMoreRows` leaking to the caller.
- Services don't log and then return the same error. Do one or the other. The
  caller at the transport layer logs.
- When the service needs a test seam over SQL, put it behind a `Store`
  interface (`stats/store.go`) and unit-test the service with a fake.

## 4. core-api endpoints

```go
// NewGetStalkEndpoint returns the logged-in channel's stalk target.
func NewGetStalkEndpoint(svc nivek.NivekService) echo.HandlerFunc {
    stalkService := stalkSvc.NewService(svc)          // built once
    return func(c echo.Context) error {
        user, err := utilities.GetUserFromContext(c, svc.Logger())
        if err != nil {
            return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
        }
        out, err := stalkService.Get(*user.TwitchLogin)
        if err != nil {
            svc.Logger().Errorf("[stalk] get for %s: %v", *user.TwitchLogin, err)
            return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
        }
        return c.JSON(http.StatusOK, out)
    }
}
```

- Constructor name: `New<Verb><Thing>Endpoint(svc nivek.NivekService, ...deps) echo.HandlerFunc`.
- Paths come from `api/constants.go` and auth is chosen in `register.go`.
  There are exactly three auth classes: public, `authenticated` (JWT cookie +
  CSRF), `botAuth` (HMAC).
- Error body: `map[string]string{"error": "<short client-safe message>"}`.
  - 4xx: a specific, user-facing message (validation text from `validate.go`
    is fine).
  - 5xx: a generic message. The detail goes to `svc.Logger().Errorf`, which
    reaches the Discord alert hook. Don't put `err.Error()` in a 5xx body.
- Request structs are unexported, live in the endpoint file, and have `json`
  tags. Validate required fields right after `c.Bind`.
- Update verbs: new endpoints use `PUT` for update and `DELETE` for delete.
  The existing `POST /promo/:id` and `POST /auto-shout/:id` stay until a
  dedicated PR migrates them together with the frontend.
- Handlers for public, unauthenticated routes that hit the DB need a cache or
  rate bound (see `stats.PublicService`: a 60 s TTL plus an error backoff).

## 5. Clients (outbound HTTP)

Follow [architecture.md §4](architecture.md#4-the-swappable-client-rule-anti-corruption-layer).
In addition:

- Every `http.Client` has a `Timeout`, or every request carries a context
  deadline. `&http.Client{}` with neither is a bug.
- Request paths take a `context.Context` as their first parameter.
- Make the base URL overridable for tests: an unexported
  `var <vendor>BaseURL` or a constructor argument, exercised with `httptest`
  (`deepl.translateAt`, `twitcheventsub.helixBaseURL`).
- On a non-2xx response, read a **bounded** amount of the body into the error
  and never log credentials or tokens.
- HMAC-signed internal clients (`api.CoreAPIClient`, `botclient.Client`) use the
  canonical string `<METHOD>\n<PATH>\n<RAW_QUERY>\n<TIMESTAMP>\n<BODY>`. Don't
  add a different signing scheme.

## 6. Configuration

- **No new required env vars.** New settings are optional and the default must
  be safe: the feature is off, or behaves conservatively. Document them in
  `README.md` → *Optional bot settings* (bot) or `.env.example`.
- Where config is read:
  - core-api: `cmd/core-api/coreconfig` (validated before the server starts)
    and `internal/libraries/config` (`envconfig` tags with `default:`).
  - bot: `cmd/twitch-bot/main.go` with `getEnv(key, default)`, passed into
    `twitchbot.Config`.
  - New library code receives config through its constructor. It doesn't call
    `os.Getenv` (see [inconsistencies.md](inconsistencies.md) §Config for the
    existing exceptions).
- A missing optional integration (DeepL key, Discord webhook, app-token
  credentials) logs one line at startup ("X disabled (set Y to enable)") and
  turns the feature off. It never stops the process.

## 7. Errors

- Wrap with `%w` and context: `fmt.Errorf("increment lurk [%s/%s]: %w", ch, who, err)`.
  Don't use `%s` with `err.Error()`, because it breaks `errors.Is/As`.
- Name the operation, not "error": `"fetch commands: %w"`, not
  `"error error fetching: %s"`.
- Sentinel errors are package-level `var ErrX = errors.New(...)`.
- **No `panic`, `log.Fatal`, or `os.Exit` outside `main()` startup
  validation.** Library and handler code returns errors. In the bot, even
  startup should degrade rather than exit when the failure is a dependency
  being down and not misconfiguration (see
  [inconsistencies.md](inconsistencies.md) §Bot boot).
- Goroutines started by the bot, and long-lived ones in core-api, begin with a
  `defer func(){ if r := recover(); r != nil { log... } }()`.

## 8. Logging

[LOGGING.md](../LOGGING.md) is the rule. In short:

- If the code holds a `nivek.NivekService`, use `svc.Logger()` (logrus, which
  carries the Discord hook). Package-level `logrus.*` is banned.
- Bot, tools, `twitcheventsub`, and `overseer` use stdlib `log`.
- Bot log lines start with an upper-case tag in brackets: `[COMMANDS]`,
  `[PROMO]`, `[SAY]`, `[INFO]`, `[EVENTSUB]`, `[USAGE]`. core-api service lines
  use `[Feature]`.
- Never log secrets, tokens, HMAC keys, cookies, or full request bodies.

## 9. Concurrency and resource bounds

- Shared maps in the bot are behind a `sync.Mutex`/`RWMutex` or are
  atomic-swapped (`setCommands`). Document which fields a mutex guards.
- Every in-memory map that grows with chat input has a cap and a prune
  (`cooldown.go capLocked/pruneLocked`, `eventsub_dedupe.go`,
  `usage_tracker.go`).
- Network calls on the message path run in a goroutine or with a short
  `context.WithTimeout` (`runInfoLookup`, `promoLiveLookupTimeout`).
- Inject time as `now func() time.Time` in types that need deterministic tests
  (`stats.PublicService`).

## 10. Twitch bot specifics

These are summarised here and defined in [merge-criteria.md](merge-criteria.md).

- New builtin flow:
  1. Write `func (b *Bot) handle<Name>Command(m *chatMessageEvent)` in
     `cmd_<name>.go`.
  2. Add it to `builtinRegistry` in `bot.go` under a stable `handler_key`.
  3. Add the seed row to `database/init.sql` (CRLF) and to a new
     `database/prod-apply-<name>-builtin.sql` using
     `ON CONFLICT (trigger) WHERE scope = 'global' DO NOTHING`. Its header
     comment states the deploy order: the binary first, then the SQL, then a
     bot restart.
  4. If the command takes free-form arguments, add an up-front dispatch block
     in `bot_read_message.go`. It goes through `commandAllowed`, then
     `return`s.
- Every reply goes through `sanitizeChatReply` (strips control characters,
  collapses whitespace, strips leading `/` and `.`, caps at 450 runes) before
  `say`/`announce`. User-controlled text is never formatted into a reply
  unsanitised.
- Triggers in the DB are lowercase. `buildCommandMaps` rejects mixed-case ones.
- Role checks (`isModOrBroadcaster`, `meetsMinRole`) run **before**
  `commandAllowed`, so unauthorised users can't burn the cooldown.
- Bot-side tracking (usage, chat-line counts, stalk memory) is in-memory,
  bounded, flushed asynchronously, and drops data when core-api is down. It
  never blocks a reply.

## 11. Tests

- Table-driven tests with `t.Run`. Use the stdlib `testing` package. There is
  no assertion library in `go.mod`.
- Use fakes through consumer-side interfaces (`infoHelix`, `stats.Store`,
  `api.CoreAPIClient`) and `httptest.Server` for HTTP clients. Don't touch the
  network.
- Bot handler tests build a `Bot{}` with a buffered `sayQueue` and assert on
  what was queued (`cmd_info_test.go`, `cooldown_test.go`).
- DB tests are `//go:build integration` (`*_integration_test.go`) and run in
  CI job `db-migrations` against a real Postgres.
- New logic ships with unit tests in the same PR.
