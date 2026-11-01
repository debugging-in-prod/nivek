# Refactor plan

This is a list of small PRs that each remove items from
[inconsistencies.md](inconsistencies.md) and can merge on their own.

- PRs are **ordered by risk**: docs first, then tests/CI, then the frontend,
  then core-api, then services and clients, with paths that are critical to
  the bot last.
- Each PR needs its own GitHub issue (one PR per issue) and must pass
  [merge-criteria.md](merge-criteria.md).
- No PR here adds a DB migration, a required env var, or a change in
  behaviour beyond the one it states.
- Every PR also needs CI green (`go`, `nuxt`, `db-migrations`).

Phase 6 PRs are listed last because they're bot-critical, not because they
matter least. See the status table below for what has landed.

---

## Status (2026-11-01)

| Item | Status | PR(s) |
| --- | --- | --- |
| R0.1 | done | #199 |
| R0.2 | done | #198 |
| R1.1 | done | #271 |
| R1.2 | done | #201 (+ #203, clock-independent info tests) |
| R1.3 | done | #200 |
| R2.1 | done | #208 |
| R2.2 | done | #211 |
| R2.3 | done | #221 |
| R2.4 | done | #237 |
| R2.5 | done | #236 |
| R2.6 | **on hold** (awaiting owner approval) | — |
| R3.1 | done | #213 |
| R3.2 | done | #218 |
| R3.3 | done | #207 |
| R3.4 | done | #210 |
| R3.5 | done | #217 (PUT routes), #247 (POST aliases removed) |
| R4.1 | done | #220 |
| R4.2 | done | #227 |
| R4.3 | done | #225 #232 #234 #235 #239 #242 #243 #245 #253 #255 |
| R4.4 | not pursued (opportunistic only) | — |
| R4.5 | done (GORM removed, pgx kept) | #223 |
| R5.1 | done | #258 |
| R5.2 | done | #260 |
| R6.1 | done | #189 (issue #188) |
| R6.2 | done | #262 |
| R6.3 | done | #264 |
| R6.4 | done | #193 (issue #191) |
| R6.5 | done | #266 |

**Deploy note.** While this plan was rolling out, a race between the two
prod deploy workflows deleted `bin/twitch-bot` from the shared checkout and
the bot crash-looped on restart. #249 fixed it: the binary now lives outside
the checkout and both workflows share one concurrency group. Issue #256
tracked a remaining gap: when a pending "Prod Service Deployment" run was
cancelled by that group, the installed binary could be left stale. Fixed by
having `build.yml` (Deploy Production) also rebuild the binary for its own
commit on every run.

---

## Phase 0: Docs (no runtime risk)

### R0.1 Fix the stated reason for the handler-before-seed order (D2, X1) — done (#199)
- **Scope:** `docs/deploy-runbook.md` §0 and "If something is wrong". Say
  that an unknown `handler_key` is now logged and skipped
  (`buildCommandMaps`), and that the order still matters for older binaries
  and to keep the command active. Leave the old `prod-apply-*.sql` header
  comments alone (they're applied history). New files use the corrected
  wording.
- **Risk:** none (docs).
- **Verify:** reviewer reads `command_loader.go buildCommandMaps` against the
  new wording.

### R0.2 Document the local vs prod `/api` prefix handling (X2) — done (#198)
- **Scope:** a short note in `README.md` or `docs/` about how nginx-local
  strips `/api` and how Traefik in prod keeps it.
- **Risk:** none.
- **Verify:** the note matches `docker/nginx-local/default.conf`.

## Phase 1: Tests and CI (no runtime risk)

### R1.1 Run frontend unit tests in CI (F13)
- **Status:** done (#271). Owner approved the `.github/workflows/ci.yml`
  edit.
- **Scope:** add `- run: npm test` to the `nuxt` job in
  `.github/workflows/ci.yml`. This edits a CI workflow, not a deploy
  workflow, but still needs explicit maintainer approval.
- **Risk:** very low. CI can only get stricter.
- **Verify:** the CI run shows `tests/publicStats.test.ts` passing.

### R1.2 Pin the current bot reply behaviour with characterisation tests (before B1/B3/B5) — done (#201, plus #203)
- **Scope:** `internal/libraries/twitchbot/*_test.go` only. Add tests that
  pin down, for each handler, what gets queued. That covers the `^` echo, dad
  random, stalk recall, translate, and newpromo error replies. These tests
  lock behaviour in place before Phase 6 changes it.
- **Risk:** none (tests only).
- **Verify:** `go test -race ./internal/libraries/twitchbot/...`.

### R1.3 Weather tests through `httptest` (before C1–C3) — done (#200)
- **Scope:** `internal/libraries/weather/*_test.go`. Cover Kelvin→°F
  formatting, an empty series, and a decode error, using an `httptest`
  server. This needs the base URL to be overridable, so turn the const URL
  into a `var` in the same PR (no behaviour change).
- **Risk:** very low.
- **Verify:** `go test -race ./internal/libraries/weather/...`.

## Phase 2: Frontend (no bot or core-api risk)

Each PR: `npm ci && npm run build && npm test` in `nivek-nuxt`, then click
through the affected panels on prod after deploy.

### R2.1 Move the non-page module out of `pages/` (F7) — done (#208)
- **Scope:** move `pages/df/renderer.ts` to `utils/dfRenderer.ts` and update
  imports. The DF pages are deprecated but this is a pure move: no logic
  change.
- **Risk:** low. Build verifies the imports.

### R2.2 One fetch path: `api()` + `API_ROUTES` everywhere (F1, F2) — done (#211)
- **Scope:** replace hardcoded paths in `stores/auth.ts`,
  `pages/commands/index.vue`, `pages/overlay/index.vue`, `Navigation.vue`,
  `pages/index.vue`, `pages/auth/landing.vue` with `API_ROUTES` constants.
  Make `utils/dfService.ts` use `api()`. Remove unused `API_ROUTES` entries
  and the unused `Task` type. Leave `BotStats.vue`'s `useFetch` in place
  (it's allowed) but point it at a constant.
- **Risk:** low. A wrong path shows up as a 404 in that one panel.
- **Verify:** build, sign in, open every dashboard panel, the commands page,
  overlay page, and DF page. Sign out.

### R2.3 Shared API types in `app/types/` (F3, F4) — done (#221)
- **Scope:** move the inline response types into `types/<feature>.ts`, merge
  the duplicates (`Draft` → `CommandDraft`/`CustomCommandDraft`, `Coord` →
  `Position`), and move `ROLES`/cooldown limits into `utils/constants.ts`
  with a comment pointing at the Go source. Types only. No runtime change.
- **Risk:** very low (the type checker catches mismatches during build).

### R2.4 Uniform loading and error UX (F5, F4) — done (#237)
- **Scope:** add a shared `utils/errorMessage.ts` (+ test). Apply the
  `loading`/`error`/`role="alert"` pattern from
  [typescript.md §6](typescript.md#6-loading-and-error-states) to the panels
  that fail silently, and drop the duplicated `errMessage()` helpers.
- **Risk:** low (UI only).

### R2.5 Small cleanups (F8, F10, F11, F12) — done (#236)
- **Scope:** rename the composable file to match its export, remove the stale
  JWT/Bearer comments and leftover code, and add an auth store action for
  `bot_opt_in`.
- **Risk:** very low.

### R2.6 Formatter and linter (F6, F9) — ON HOLD (awaiting owner approval)
- **Status:** on hold, awaiting owner approval (large mechanical diff).
  Not started.
- **Scope:** add Prettier (2-space) and ESLint (`@nuxt/eslint`) with a
  `lint` script. Reformat in one mechanical commit with nothing else in it.
  Turning on the CI lint step is a separate follow-up PR. Replacing colour
  literals with CSS variables is its own follow-up.
- **Risk:** low (formatting only), but the diff is large. Review with
  `git diff -w`.

## Phase 3: core-api endpoints (no bot boot risk, but the bot uses some of these routes)

Each PR: `go test -race ./cmd/core-api/...`, then after deploy
`curl https://peanutbudderbot.com/api/healthz` plus the affected dashboard
panel. **Don't change response shapes or status codes the bot depends on**
(`endpoints/bot/*`).

### R3.1 Generic 5xx bodies, details to the logger (A1, A2) — done (#213)
- **Scope:** dashboard endpoints only: `commands/get_commands.go`,
  `promo/*`, `stalk/*`, `dad/*`, `autoshout/*`, `weather/get_weather.go`.
  Replace `err.Error()` in 5xx bodies with `"internal server error"` plus
  `svc.Logger().Errorf(...)`. Fix the dad 401/500 pairs. Leave `df/*` alone.
- **Risk:** low. The frontend shows its own messages for 5xx.
- **Verify:** unit tests per endpoint (`httptest` + fake user context), and
  check that a forced failure produces an Error log line (alerting).

### R3.2 Build services once per endpoint constructor (A3) — done (#218)
- **Scope:** move `xSvc.NewService(svc)` out of the request closures in the
  dashboard endpoints. Services are stateless over a shared DB session, so
  this changes no behaviour.
- **Risk:** low.
- **Verify:** `go test`, plus a smoke test of each panel.

### R3.3 Weather endpoint logging (A7) — done (#207)
- **Scope:** drop the per-request client-IP Info log in `getIP` (or move it
  to Debug).
- **Risk:** very low.

### R3.4 Remove the dead helper (A5) — done (#210)
- **Scope:** delete `utility.RejectBadRequest`, or start using it. Pick one.
- **Risk:** none.

### R3.5 PUT for updates (A6), optional, coordinated — done (#217, POST aliases removed in #247)
- **Scope:** add `PUT /promo/:id` and `PUT /auto-shout/:id` alongside the
  existing POST routes, switch the frontend to them, and remove the POSTs in
  a later PR once the new frontend is deployed.
- **Risk:** low if the routes overlap for one deploy.

## Phase 4: Services, models, clients (core-api side)

### R4.1 Windy behind a nivek interface (C1, C2, C4) — done (#220)
- **Scope:** `internal/libraries/weather` only.
  - Add `type Forecast struct{ TempF int }` in `weather/const.go`.
  - Unexport the Windy wire types.
  - `WindyClient.Forecast(ctx, lat, lon) (Forecast, error)` with
    `http.Client{Timeout: 10 * time.Second}` and the key injected at
    construction.
  - `ReportService` depends on an unexported `forecastProvider` interface and
    formats `"%d°F"` itself.

  The JSON response shape of `POST /api/weather` stays the same.
- **Risk:** low (dashboard-only feature, behind auth).
- **Verify:** R1.3 tests plus a new fake-provider test for `ReportService`,
  then the Weather panel on prod.

### R4.2 IPInfo behind the same pattern (C3) — done (#227)
- **Scope:** add a `locator` interface returning `Location{City string; Lat,
  Lon float64}`. A parse failure returns an error, not `0,0`.
- **Risk:** low.

### R4.3 `%w` wrapping sweep, one package per PR (S5) — done (#225 #232 #234 #235 #239 #242 #243 #245 #253 #255)
- **Status:** one PR per package: `fishing` #225, `twitcheventsub` #232,
  `user` #234, `weather` #235, `api` #239, `bread` #242, `dadusage` #243,
  `autoshout` #245, `nivek` #253, `twitchbot` #255. `lurk`, `conman`, and
  the rest of `fishing` were already clean. One `%s`/`err.Error()` leftover
  remains outside the swept packages, in
  `cmd/core-api/endpoints/bot/hangman.go`.
- **Scope:** replace `fmt.Errorf("...%s", err.Error())` with
  `fmt.Errorf("...: %w", err)` package by package, starting with `weather`,
  `fishing`, `user`. Error text stays essentially the same.
- **Risk:** very low. Check no caller string-matches error text first
  (`grep -rn 'strings.Contains(err.Error()'`).

### R4.4 Naming alignment (S1, S2, S3, S4), only when a package is already being touched — not pursued
- **Status:** not pursued. No PR in this round touched these constructors
  for another reason, so per the "only when already touching" rule nothing
  was renamed: `task.NewNivekTaskService`,
  `weather.NewWeatherReportService`, and `fishing.NewService(svc, channel)`
  are unchanged.
- **Scope:** rename `task.NewNivekTaskService` → `NewService`, and make
  `fishing.NewService` take the channel as a method argument. Don't rename
  `api/constants.go` or the exported interfaces just for style, because that
  churns too many files for no behavioural gain. Record any accepted
  exceptions in [go.md](go.md).
- **Risk:** low (compile-checked).

### R4.5 Dead connectors in `conman` (S6) — done (#223, GORM only)
- **Status:** #223 removed the GORM connector and its `go.mod` deps. pgx
  was kept: `postgres_pgx.go` is live (every upper/db session opens through
  pgx in `describe_exec` mode, the incident #172 fix).
- **Scope:** confirm GORM and pgx connectors are unused (`grep -rn
  '\.Gorm(\|pgx' cmd internal`), then remove them and their `go.mod`
  dependencies in their own PR.
- **Risk:** low (compile-checked). Needs a careful `go mod tidy` review.

## Phase 5: Config (touches bot startup wiring, so it's medium risk)

### R5.1 Bot libraries take config from `twitchbot.Config` (B6, K1) — done (#258)
- **Scope:** read `DEEPL_API_KEY`, `TWITCH_BOT_DISCORD_WEBHOOK`,
  `WEBHOOK_LISTEN_ADDRESS`, and `TWITCH_EVENTSUB_SECRET` once in
  `cmd/twitch-bot/main.go` (same names, same defaults: all optional) and pass
  them through `Config`. No env var is added or made required.
- **Risk:** medium. A wiring mistake would disable translate, Discord, or the
  EventSub listener.
- **Verify:** unit tests that the zero value of `Config` disables each
  feature, as today. After deploy: restart the bot, check the journal for the
  same "enabled/disabled" lines as before, check that `/eventsub` still
  receives (`eventsub-healthcheck`), and run `!translate` once.

### R5.2 Move bot identity into config with today's values as defaults (B7) — done (#260)
- **Scope:** add `TWITCH_BOT_ID` (optional, default `"1322716097"`).
- **Risk:** low to medium. The default keeps behaviour identical.

## Phase 6: Bot-critical paths (highest risk, smallest diffs, one at a time)

Every PR here:

- lands on its own, with R1.2's characterisation tests updated in the same PR
  to show the intended behaviour change;
- passes `go test -race ./internal/libraries/twitchbot/...`;
- after deploy, gets a manual `sudo systemctl restart twitch-bot` and a check
  of `journalctl -u twitch-bot` for a clean boot and channel joins;
- is smoke-tested in the home channel.

### R6.1 Gate the `^` echo (B1) — done (#189, issue #188)
- **Decision:** the `^` reply is intentional and stays. It only needed the
  cooldown.
- **Scope:** `bot_read_message.go`, `cooldown.go`. Fires at most once per
  message, through `caretEchoAllowed` (the shared `commandCooldowns`, key
  `^echo`, 10 s global window per channel, no per-user window, no
  mod/broadcaster bypass). Silent on cooldown; not counted as command usage.
- **Risk:** low code risk. It only reduces output.
- **Verify:** `caret_echo_test.go`: `"^ ^ ^"` queues one reply, a second
  message within the cooldown queues none, and it replies again after the
  window.

### R6.2 Sanitise at the sink (B3, B4) — done (#262)
- **Scope:** move `sanitizeChatReply`/`maxChatReplyLen` from `cmd_info.go`
  into `chat_reply.go`, and call it inside `say`/`announce`/`ircsay`, dropping
  empty results. Existing per-handler calls become harmless no-ops; remove
  them in the same PR or a later one.
- **Risk:** medium. Every reply passes through this code. Sanitising is
  idempotent, so already-sanitised text doesn't change.
- **Verify:** a table test covering the leading `/`, leading `.`, newline,
  >450-rune, and empty cases through `say`, plus the existing handler tests.

### R6.3 Non-blocking send queue (B5) — done (#264)
- **Scope:** `say`/`announce`/`ircsay` use `select { case q <- r: default:
  log.Printf("[SAY] queue full, dropping reply to %s", id) }`.
- **Risk:** medium. Under extreme load this drops replies instead of
  stalling, which is the intended fail-soft behaviour.
- **Verify:** a unit test with a full queue returns immediately and logs.

### R6.4 Boot tolerates core-api being down (B2) — done (#193)
- **Scope:** `cmd/twitch-bot/main.go` and `twitchbot`. If `GetActiveChannels`
  fails at boot, start with the permanent home channel(s) and retry the
  channel list in the background, using the same backoff pattern as
  `command_loader.go`. Then join the channels once it succeeds. Keep
  `log.Fatal` for real misconfiguration (missing `CORE_API_URL`, HMAC key,
  credentials).
- **Risk:** **highest in the plan.** It changes the boot sequence and IRC
  join flow.
- **Verify:**
  - unit test with a fake `CoreAPIClient` that fails N times and then
    succeeds;
  - a manual staging-style check on prod in a quiet window: stop core-api
    (`docker compose stop core-api`), restart the bot, and check that it
    stays up, logs the retry, and joins the home channel;
  - start core-api again and check that the remaining channels are joined and
    commands load;
  - follow [deploy-runbook.md](../deploy-runbook.md) for the restart order.

### R6.5 Remove commented-out DF dispatch (B8) — done (#266)
- **Scope:** delete the commented blocks only. Keep the `cmd_df.go`
  functions (DF is deprecated but stays).
- **Risk:** none at runtime, but it touches the message handler file, so it
  goes after the other R6 PRs to avoid conflicts.
