# Architecture

This document describes how the repo is organised now and turns that
organisation into rules. Paths are relative to the repo root.

## 1. Processes and boundaries

| Process | Entry point | Runs as | Owns | Talks to |
| --- | --- | --- | --- | --- |
| **twitch-bot** | `cmd/twitch-bot/main.go` → `internal/libraries/twitchbot` | host systemd unit `twitch-bot.service` (not Docker) | Twitch chat I/O (EventSub + IRC), command dispatch, cooldowns, timers (promos), in-memory chat state | core-api (HMAC, `internal/libraries/api`), Twitch Helix/EventSub (`internal/libraries/twitcheventsub`), DeepL, Discord webhook, df-executor (`overseer`) |
| **core-api** | `cmd/core-api/main.go` → `cmd/core-api/routes` → `cmd/core-api/endpoints/*` | Docker (compose) behind Traefik at `/api` | **The only process that touches Postgres.** Auth (Twitch OAuth, JWT cookie, CSRF), dashboard APIs, bot APIs, overlay relay, public stats | Postgres (`conman`), Twitch Helix/OAuth, Windy, IPInfo, twitch-bot listener (`internal/libraries/botclient`) |
| **nivek-nuxt** | `nivek-nuxt/app` | Docker (SSR Nuxt 4) | UI only | core-api through relative `/api` (same origin, cookie session) |
| df-executor, df-snapshot-pusher | `cmd/df-*` | DF host | Dwarf Fortress bridge | **Deprecated, kept on purpose. Don't modify.** |
| tools | `cmd/twitch-bot-auth`, `cmd/eventsub-healthcheck`, `cmd/eventsub-backfill` | run by hand | one-off ops | core-api / Twitch |

Boundary rules:

1. **Only core-api touches the database.** The bot never imports `conman`
   or opens a DB session. It reads and writes state only through
   `api.CoreAPIClient` (`internal/libraries/api/core_api_client.go`). New
   persistent bot state means a new core-api bot endpoint plus a new
   `CoreAPIClient` method.
2. **Bot ↔ core-api calls are HMAC-signed both ways.** Bot → core-api uses
   `api.CoreAPIClient` and `nivekmiddleware.NewHMACMiddleware("BOT_API_HMAC_KEY")`.
   core-api → bot uses `botclient.Client` and the bot's listener in
   `twitchbot/bot_api.go`. Don't add a third channel.
3. **The frontend talks only to core-api**, through same-origin `/api`. There
   are no Nuxt server routes. The browser never calls Twitch, the bot, or the
   DB directly. The only third-party exception today is `ipapi.co` in
   `Weather.vue`.
4. **The bot must survive any other process being down.** core-api, Helix,
   DeepL, Discord and df-executor failures degrade one feature. They never stop
   chat handling. See [merge-criteria.md](merge-criteria.md).

## 2. Layers

Code flows inward. An outer layer may import an inner one, never the reverse.

```
            ┌────────────────────────────────────────────────────────┐
 transport  │ cmd/core-api/endpoints/*   (Echo handlers)              │
            │ internal/libraries/twitchbot/cmd_*.go (chat handlers)   │
            │ nivek-nuxt pages/components                             │
            ├────────────────────────────────────────────────────────┤
 routing    │ cmd/core-api/routes/register.go  + api/constants.go     │
            │ twitchbot builtinRegistry + bot_read_message.go dispatch│
            ├────────────────────────────────────────────────────────┤
 service    │ internal/libraries/<feature>/service.go                 │
 (domain)   │ (business rules, validation, DB access via upper/db)    │
            ├────────────────────────────────────────────────────────┤
 model      │ internal/libraries/<feature>/const.go                   │
            │ (table name consts, row/DTO structs, domain constants)  │
            ├────────────────────────────────────────────────────────┤
 clients    │ internal/libraries/api, botclient, twitcheventsub,      │
 (adapters) │ deepl, weather/windy_client.go, twitchauth, alerting    │
            ├────────────────────────────────────────────────────────┤
 platform   │ nivek (NivekService), abstractservice, conman, config,  │
            │ nivekmiddleware, jwt, utilities                         │
            └────────────────────────────────────────────────────────┘
```

| Layer | Does | Doesn't |
| --- | --- | --- |
| **Endpoint** (`cmd/core-api/endpoints/<area>/<verb>.go`) | Bind and validate the request shape, read the user from context (`utilities.GetUserFromContext`), call **one** service, map the result or error to an HTTP status and JSON body, log server-side errors via `svc.Logger()` | Run SQL, call external APIs directly, contain business rules, put internal error text in 5xx bodies |
| **Chat handler** (`twitchbot/cmd_<name>.go`) | Parse args, check role, call core-api/Helix through an interface, format **one** reply and send it via `say`/`announce` after sanitising | Get dispatched without the cooldown gate, block the message loop on network I/O without a timeout, talk to Postgres |
| **Route registration** (`cmd/core-api/routes/register.go`) | Bind a path constant from `api/constants.go` to an endpoint constructor and choose its auth: public, `authenticated` (JWT cookie), `botAuth` (HMAC) | Hold logic |
| **Service** (`internal/libraries/<feature>/service.go`) | Hold domain rules and persistence for one feature. It is built with `NewService(nivek.NivekService)` and exposes an interface (`Nivek<Feature>Service`) | Import Echo, `twitchbot`, or HTTP request types |
| **Model / const** (`internal/libraries/<feature>/const.go`) | Hold the `Table<Name>` constants, the row structs with `db:` and `json:` tags, and domain constants and limits | Hold logic beyond trivial methods |
| **Client / adapter** | Speak one external protocol and translate it into nivek types (§4) | Leak vendor types to callers, read config from globals when it can be injected |
| **Platform** | Provide the service lifecycle, DB connections, config, logging and auth middleware | Know about features |

The `api` package is a special case. It is both the bot's client for core-api
(`core_api_client.go`) and the single registry of route paths
(`constants.go`) that `routes/register.go` and the client share. Keep it that
way: a new route constant goes in `api/constants.go` and is used by both sides.

## 3. Where each kind of thing lives

| Thing | Location | Example |
| --- | --- | --- |
| Binary entry point, env wiring | `cmd/<binary>/main.go` | `cmd/twitch-bot/main.go` |
| core-api config (validated before start) | `cmd/core-api/coreconfig/coreconfig.go` | `cfg.Validate()` |
| Shared env config (Postgres, Windy, IPInfo) | `internal/libraries/config/config.go` (`envconfig` struct tags with defaults) | `config.WindyConfig` |
| Bot config | `twitchbot.Config` populated in `cmd/twitch-bot/main.go` via `getEnv(key, default)` | `TWITCH_BOT_NO_REPLY_LOGINS` |
| HTTP route paths | `internal/libraries/api/constants.go` as `<Verb><Thing>` consts | `GetPublicStats = "/stats/public"` |
| Route → handler + auth binding | `cmd/core-api/routes/register.go` | `e.GET(apilib.GetStalk, ..., authenticated)` |
| Dashboard endpoint | `cmd/core-api/endpoints/<feature>/<verb>.go`, constructor `New<Verb><Thing>Endpoint(svc) echo.HandlerFunc` | `endpoints/dad/get.go` |
| Bot-only endpoint | `cmd/core-api/endpoints/bot/<feature>.go` | `endpoints/bot/lurk.go` |
| Domain service | `internal/libraries/<feature>/service.go` | `internal/libraries/quote/service.go` |
| Table names, row structs, domain consts | `internal/libraries/<feature>/const.go` | `lurk/const.go` (`TableLurk`, `Lurker`) |
| Pure validation | `internal/libraries/<feature>/validate.go` + `validate_test.go` | `promo/validate.go`, `commands/validate.go` |
| Hand-written SQL | next to the store that runs it (`query.go` builders, or `.psql` files) | `stats/query.go`, `task/queries.psql` |
| Store abstraction (testable DB seam) | `internal/libraries/<feature>/store.go`: `Store` interface + `dbStore` | `stats/store.go` |
| External API client | its own package (`internal/libraries/<vendor>`), or `<vendor>_client.go` inside the consuming feature package | `deepl/`, `twitcheventsub/helix_info.go`, `weather/windy_client.go` |
| Chat command handler | `internal/libraries/twitchbot/cmd_<name>.go` | `cmd_quote.go` |
| Builtin registration | `builtinRegistry` in `internal/libraries/twitchbot/bot.go` + seed row in `database/init.sql` + `database/prod-apply-<name>.sql` | `"quote": (*Bot).handleQuoteCommand` |
| Cooldown gate | `internal/libraries/twitchbot/cooldown.go` (`commandAllowed`, `ircCommandAllowed`) | |
| No-reply list | `internal/libraries/twitchbot/no_reply.go` | |
| Chat reply sanitiser | `sanitizeChatReply` (currently `twitchbot/cmd_info.go`) | |
| Schema | `database/init.sql` (CRLF), plus `database/prod-apply-<name>.sql` per change | `prod-apply-command-usage.sql` |
| Frontend API client | `nivek-nuxt/app/utils/api.ts` | |
| Frontend route constants | `nivek-nuxt/app/utils/constants.ts` (`API_ROUTES`) | |
| Frontend shared types | `nivek-nuxt/app/types/` | `types/df.ts` |

## 4. The swappable-client rule (anti-corruption layer)

**An external client turns the vendor's data into a type nivek defines. Code
that uses the client depends only on those nivek types, through an interface
it declares, so the vendor can be replaced without changing the consumer.**

Concretely:

1. **Vendor wire types stay private to the client.** Request and response
   structs that mirror the vendor's JSON (e.g. `WindyResponseBody`,
   `WindyWeatherReportRequest`) are unexported, or at least never appear in a
   method signature used outside the client file.
2. **The client returns nivek domain types.** These are small structs named
   for what nivek needs, not for what the vendor sends. Unit conversion and
   reshaping happen inside the client. Good examples:
   - `twitcheventsub/helix_info.go` returns `StreamInfo`, `ChannelInfo`,
     `UserInfo`, `FollowInfo`, which are subsets shaped for the bot's info
     commands, not raw Helix JSON.
   - `deepl.Translate` returns a plain `string`. The DeepL envelope never
     leaves the package.
3. **The consumer declares the interface it needs**, with only the methods it
   calls, next to the consumer. Production wiring passes the real client and
   tests pass a fake. Good examples:
   - `twitchbot/cmd_info.go`: `type infoHelix interface { GetStream(...);
     GetChannelInfo(...); ... }`, implemented by the real Twitch client and by
     a fake in `cmd_info_test.go`.
   - `stats/store.go`: `type Store interface`, with `dbStore` in production
     and a fake in `stats_test.go`; `stats.PublicService` depends only on
     `Store`.
   - `twitchbot` depends on `api.CoreAPIClient` (an interface), not on the
     concrete HTTP client.
4. **Config and HTTP clients are injected.** The constructor takes the API key
   (or a config struct) and an `*http.Client` with a **timeout**. The client
   doesn't read `os.Getenv` or `nivek.CommonConfig()` inside request paths.
5. **Errors are wrapped with `%w` and are vendor-neutral** at the boundary
   (`fmt.Errorf("windy: fetch forecast: %w", err)`). Sentinel errors that
   callers branch on are exported from the client package
   (`twitcheventsub.ErrFollowsUnavailable`).

Worked example: weather (the target shape. Today's code differs; see
[inconsistencies.md](inconsistencies.md) §Clients):

```go
// internal/libraries/weather/const.go: nivek's type
type Forecast struct{ TempF int }

// internal/libraries/weather/service.go: consumer-side interface
type forecastProvider interface {
    Forecast(ctx context.Context, lat, lon float64) (Forecast, error)
}

// internal/libraries/weather/windy_client.go: the only file that knows Windy
type windyResponse struct{ TempSurface []float64 `json:"temp-surface"` } // unexported
func (c *WindyClient) Forecast(ctx context.Context, lat, lon float64) (Forecast, error) {
    // POST, decode windyResponse, Kelvin→°F, return Forecast
}
```

`ReportService` holds a `forecastProvider`. To swap Windy for another weather
API, you add one new file that implements `Forecast` and nothing else changes.

## 5. Dependency direction (enforced in review)

- `cmd/*` → `internal/libraries/*`. Never the reverse.
- `cmd/core-api/endpoints/*` → services/clients. An endpoint never imports
  another endpoint package. Shared helpers go in that package's `helpers.go`
  or in `cmd/core-api/utility`.
- `internal/libraries/twitchbot` → `api`, `twitcheventsub`, `deepl`,
  `overseer`, and feature packages **for their types and pure functions only**
  (e.g. `commands.Commands`, `promo` validation). It must never call a feature
  service's `NewService(nivek)`, because that needs Postgres.
- Feature service packages never import `twitchbot`, `cmd/*`, or Echo.
- `twitcheventsub` is imported by both bot and core-api, so it depends only on
  the stdlib (and logs with stdlib `log`, per [LOGGING.md](../LOGGING.md)).
- Frontend: `pages` → `components` → `composables`/`stores` → `utils`/`types`.
  `utils` and `types` import nothing from Vue components.

## 6. Bot runtime model (what new bot code must respect)

- **Inbound chat**: EventSub `channel.chat.message` → `bot_read_message.go`
  is the command path. IRC (`bot.go handleMessage`) runs only `!banish` and the
  chat-read nudge for channels that haven't granted chat-read.
- **Dispatch**: drop the bot's own messages and no-reply logins → dedupe
  message ID → argument-taking commands dispatched first → word scan against
  `builtinRegistry`-backed `b.commands` and per-channel custom commands. Every
  dispatch goes through `commandAllowed`.
- **Outbound**: `say` / `announce` enqueue to `sayQueue` (cap 64), drained by
  `senderLoop` with 1.5 s spacing. Helix Send Chat Message is used, with an app
  token when configured (bot badge).
- **Timers**: `runPromotionMessageLoop` posts promos only when the channel is
  live (EventSub state or cached Helix lookup; unknown counts as offline) and
  enough chat lines have arrived since the last post (`promo_gate.go`).
- **Background work** (usage flush, command reload, Helix lookups) runs in
  goroutines with `recover()`, bounded memory, and context timeouts.
- **Boot**: commands load with retries and then retry in the background, so
  the bot starts even when `/commands` fails (`command_loader.go`).
