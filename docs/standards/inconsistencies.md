# Inconsistencies (survey, 2026-10-31)

This lists every place the code currently differs from the
[standards](README.md) or from itself, found in a survey of `cmd/`,
`internal/`, `database/`, `docs/`, and `nivek-nuxt/`.

- Each item has an ID (used by [refactor-plan.md](refactor-plan.md)), the
  dominant pattern it breaks, and file paths. Line numbers are approximate and
  were correct at the time of the survey.
- **Severity:** **H** = touches the merge criteria (spam or bot uptime),
  **M** = correctness or maintainability, **L** = style.

Items about the deprecated `df-*` code are listed for completeness only. They
are out of scope for fixes (see merge-criteria §6).

---

## Bot (`internal/libraries/twitchbot`, `cmd/twitch-bot`)

| ID | Sev | Inconsistency | Where |
| --- | --- | --- | --- |
| B1 | **H** | **Done (#188).** ~~Per-word `^` echo isn't cooldown-gated.~~ The `^` reply is intentional (any word containing `^` makes the bot `say("^")`) and only needed the cooldown. It now fires at most once per message, gated by `caretEchoAllowed` (shared `commandCooldowns`, key `^echo`, 10 s global window per channel, no per-user window, no mod/broadcaster bypass) and isn't counted as command usage. | `internal/libraries/twitchbot/bot_read_message.go` (word loop), `cooldown.go` (`caretEchoAllowed`) |
| B2 | **H** | **Bot boot exits if core-api is unreachable.** `getLiveChannels` calls `log.Fatalf` when `GetActiveChannels` fails or returns nothing, so systemd sees a crash loop while core-api is down. Command loading next to it already degrades and retries (`command_loader.go`). | `cmd/twitch-bot/main.go` (`getLiveChannels`) |
| B3 | **H** | **Reply sanitisation happens per call site, not at the sink.** `say`/`announce` don't sanitise. Only some handlers call `sanitizeChatReply` first (`cmd_info.go`, `cmd_quote.go`, `cmd_cmd.go`, `custom_commands.go`). Others pass interpolated or core-api-supplied text straight to `b.say` (DB-stored responses, translated text, remembered chat lines, error strings), with no 450-char cap or leading `/`/`.` strip. | `cmd_dad.go` (`sayRandomDad`), `cmd_fish.go`, `cmd_stalk.go` (`"%s said: %s"`), `cmd_translate.go`, `cmd_newpromo.go` (`err.Error()` in reply), `cmd_lurk.go`, `cmd_shoutout.go`, `cmd_banish.go`, `cmd_pbcommands.go`, `cmd_df.go` |
| B4 | M | `sanitizeChatReply` and `maxChatReplyLen` are general reply rules but live in the info-commands file. | `internal/libraries/twitchbot/cmd_info.go` |
| B5 | M | `say`/`announce`/`ircsay` do a blocking send on a 64-slot channel. If the sender falls behind (Helix slow; 1.5 s spacing), the EventSub message handler blocks, which breaks the "never block the message path" rule. | `bot_send_message.go` (`say`, `announce`), `bot.go` (`ircsay`, `make(chan sayRequest, 64)`) |
| B6 | M | Library code reads env directly instead of taking it from `twitchbot.Config`: `WEBHOOK_LISTEN_ADDRESS`, `TWITCH_EVENTSUB_SECRET` (read per request), `DEEPL_API_KEY` (read per `!translate`), `TWITCH_BOT_DISCORD_WEBHOOK`. | `bot_api.go`, `cmd_translate.go`, `discord.go` |
| B7 | L | Identity constants are hardcoded in code rather than config: `BotId: "1322716097"` and `botCreatorChannel`. | `cmd/twitch-bot/main.go`, `internal/libraries/twitchbot/bot.go` |
| B8 | L | Commented-out DF dispatch blocks (DF is suspended). | `bot_read_message.go` (end of handler), `cmd_df.go` |
| B9 | L | Log tags are mostly `[TAG]`, but some lines have none (`cmd_fish.go`: `"error running fish for channel ..."`). | `cmd_fish.go` |
| B10 | L | `twitchbot` reaches Twitch through three shapes. Two follow the standard: the narrow consumer interface `infoHelix` (`cmd_info.go`) and an injected func (`liveLookup`, `promo_gate.go`). The third is the wide provider-side `twitcheventsub.TwitchEventSubClient` interface (`chat_privs.go`), which needs a large fake. `bot_send_message.go` also builds its own raw Helix requests. | `cmd_info.go`, `promo_gate.go`, `chat_privs.go`, `bot_send_message.go` |

## core-api endpoints (`cmd/core-api`)

| ID | Sev | Inconsistency | Where |
| --- | --- | --- | --- |
| A1 | M | **5xx bodies include internal error text** (`fmt.Sprintf("...: %s", err.Error())`) instead of a generic message plus a server-side log. | `endpoints/commands/get_commands.go`, `endpoints/promo/{get,create,update,delete}.go`, `endpoints/stalk/{get,set,clear}.go`, `endpoints/dad/get.go`, `endpoints/weather/get_weather.go`, `endpoints/autoshout/*` (df: `endpoints/df/post_snapshot.go`, out of scope) |
| A2 | M | Wrong status and message pairs: the dad endpoints return **401 with "internal server error"** for a context failure, and a 500 whose body includes the internal user ID for a missing `twitch_login`. | `endpoints/dad/get.go`, `dad/create.go`, `dad/delete.go` |
| A3 | M | Services are built **inside the request closure** (one per request) in dashboard endpoints, but **once in the constructor** in bot endpoints. | per-request: `endpoints/{stalk,promo,autoshout,dad}/*.go`, `commands/get_commands.go`, `weather/get_weather.go`; once: `endpoints/bot/*.go` |
| A4 | L | Two error styles: `c.JSON(status, map[string]string{"error": ...})` (dominant) and `echo.NewHTTPError` (in `channelcmd/channelcmd.go`, `user/get_by_token.go`, `user/bot_optin.go`, `user/auth/twitch.go`). | as listed |
| A5 | L | `utility.RejectBadRequest` exists but nothing calls it. | `cmd/core-api/utility/toolbox.go` |
| A6 | L | Update verb mismatch: `POST /promo/:id`, `POST /auto-shout/:id` vs `PUT /channel/commands/:id`, `PUT /channel/command-settings/:id`. | `internal/libraries/api/constants.go`, `cmd/core-api/routes/register.go` |
| A7 | M | `weather.getIP` logs the client IP at Info on every request, and silently uses an IP from the request body when one is present. | `endpoints/weather/get_weather.go` |
| A8 | L | `endpoints/bot/` files are organised by feature, all other endpoint dirs by verb. This is accepted (bot endpoints are many small handlers), but `user/get_by_token.go` follows neither pattern. | `cmd/core-api/endpoints/user/get_by_token.go` |

## Services / models (`internal/libraries/<feature>`)

| ID | Sev | Inconsistency | Where |
| --- | --- | --- | --- |
| S1 | L | Constructor naming: the dominant name is `NewService(nivek.NivekService) Nivek<X>Service`. Outliers: `task.NewNivekTaskService`, `weather.NewWeatherReportService` (returns concrete `*ReportService`), `jwt.NewJWTService` (concrete), `stats.NewPublicService(store)` (concrete, store-injected, the newer testable style). | `task/service.go`, `weather/service.go`, `jwt/service.go`, `stats/service.go` |
| S2 | L | Interface naming: `Nivek<X>Service` (most) vs `CommandsService`, `ChannelEditorService` (commands), `Service` (overlayrelay). | `commands/service.go`, `commands/editor.go`, `overlayrelay/service.go` |
| S3 | M | `fishing.NewService(svc, channel)` takes request-scoped data in its constructor. | `internal/libraries/fishing/service.go` |
| S4 | L | Constants file name: `const.go` (most) vs `constants.go`. | `api/constants.go`, `hangman/constants.go`, `twitcheventsub/constants.go` |
| S5 | M | Error wrapping: about 231 `%w` vs about 70 `fmt.Errorf("...%s", err.Error())`, which breaks `errors.Is`. Heaviest in `weather/`, `twitcheventsub/client.go`, `fishing/`, `user/`. | `grep -rn 'Errorf(.*err.*\.Error()' cmd internal` |
| S6 | L | DB access comes in four flavours: upper/db `Collection` (lurk, bread, promo…), upper/db `SQL()` raw queries (commands, quote, dad, stats…), a `.psql` file (task), and `conman` exposing GORM and pgx connectors that no feature uses. | `internal/libraries/conman/{gorm.go,postgres.go,postgres_pgx.go}`, `task/queries.psql` |
| S7 | L | Only `stats` has a `Store` interface seam. Other services hold a `db.Collection`/session directly and have no unit-test seam. | `internal/libraries/*/service.go` |
| S8 | L | `nivek.NivekService.CustomConfig() any` is a type-erased bag (core-api config is passed through it). | `internal/libraries/nivek/interface.go` |

## External clients (anti-corruption layer)

| ID | Sev | Inconsistency | Where |
| --- | --- | --- | --- |
| C1 | M | **Windy client breaks the swappable-client rule.** The consumer (`ReportService`) holds a concrete `*WindyClient` with no interface. The client returns a pre-formatted display string (`"72°F"`) instead of a nivek type. The vendor wire structs (`WindyResponseBody`, `WindyWeatherReportRequest`, `WindyResponseBodyUnits`) are exported. Value receivers. | `internal/libraries/weather/windy_client.go`, `weather/service.go` |
| C2 | M | Windy uses `&http.Client{}` with **no timeout** and no context. It's the only outbound client without one. | `weather/windy_client.go` (`NewWindyClient`) |
| C3 | M | IPInfo is used directly in the service: the vendor type `*ipinfo.Core` flows through `ReportService` methods, and a parse failure silently returns lat/lon `0,0`. | `weather/service.go` |
| C4 | L | Clients read config from inside request paths (`c.nivek.CommonConfig().Windy.Token`), `HMACMiddleware` reads its key env var on every request, and `jwt.newTokenService` reads `JWT_SECRET` from the env itself. None of them take config at construction. (Auth code is listed only. Changing it is security-hardening scope, see merge-criteria §5.) | `weather/windy_client.go`, `nivekmiddleware/HMACMiddleware.go`, `jwt/token.go` |
| C5 | L | `twitcheventsub` holds both the EventSub subscription client and the read-only Helix lookups, and the bot also builds its own Helix send calls. Helix knowledge is spread over `twitcheventsub/client.go`, `twitcheventsub/helix_info.go`, and `twitchbot/bot_send_message.go`. | as listed |
| C6 | L | Client shapes vary: `deepl.Translate(client, key, ...)` is a package function, the others are structs with constructors. Both are acceptable. The struct form is preferred for new clients. | `internal/libraries/deepl/deepl.go` |

## Config

| ID | Sev | Inconsistency | Where |
| --- | --- | --- | --- |
| K1 | L | Three config mechanisms: `envconfig` structs (`internal/libraries/config`), `coreconfig` (core-api, validated), and `getEnv` in `cmd/twitch-bot/main.go`, plus direct `os.Getenv` in libraries (B6, C4) and in `cmd/core-api/endpoints/df/post_snapshot.go`. | as listed |

## Database / migrations

| ID | Sev | Inconsistency | Where |
| --- | --- | --- | --- |
| D1 | L | `database/overlay-relay.sql` is a second base-schema file. It isn't in `init.sql` and isn't named `prod-apply-*`. CI applies it next to `init.sql`. | `database/overlay-relay.sql`, `.github/workflows/ci.yml` (`db-migrations`) |
| D2 | M | The prod-apply headers and the runbook say "the bot fails boot on an unknown `handler_key`", but since #174 `buildCommandMaps` logs and skips such rows (see the `builtinRegistry` comment in `bot.go`). The deploy-order rule still stands, but its stated reason is out of date. | `database/prod-apply-cmd-builtin.sql`, `database/prod-apply-info-commands.sql`, `docs/deploy-runbook.md` §0 and "If something is wrong" |

## Frontend (`nivek-nuxt/app`)

| ID | Sev | Inconsistency | Where |
| --- | --- | --- | --- |
| F1 | M | Four ways to fetch: `api()` + `API_ROUTES` (dominant); `api()` with a hardcoded path; raw `$fetch('/api/…')`, which skips the CSRF and 401 handling; `useFetch`. | hardcoded: `stores/auth.ts` (`'/profile'`, `'/logout'`), `pages/commands/index.vue` (`'/commands'`), `pages/overlay/index.vue` (`'/overlay/device'`); raw `$fetch`: `utils/dfService.ts`; `useFetch`: `components/BotStats.vue` |
| F2 | L | `API_ROUTES` is used unevenly: `TwitchStart`, `AuthLanding`, `Secure.Profile`, `Secure.Tasks.*` are unused, and `/api/auth/twitch/start` is hardcoded instead. `TwitchStart` includes `/api` while `Secure.*` entries don't. The `Task` interface is unused. | `utils/constants.ts`; hardcoded in `components/Navigation.vue`, `pages/index.vue`, `pages/auth/landing.vue`, `pages/commands/index.vue`; `/api/overlay/download` in `pages/overlay/index.vue` |
| F3 | M | API types are mostly written inline in components. There are duplicates and near-duplicates: `Draft` is defined twice with different shapes, `Command` vs `CustomCommand`, and `Coord` duplicates `Position`. `User` lives in `utils/constants.ts`, not `types/`. | `components/{AutoShout,CommandSettings,CustomCommands,DadResponses,FishScore,Promos,Stalk,Weather}.vue`, `pages/commands/index.vue`, `pages/overlay/index.vue`, `pages/df/index.vue`, `types/df.ts` |
| F4 | L | Duplicated logic: `errMessage()`, `ROLES`, `MIN_COOLDOWN`/`MAX_COOLDOWN` copied between two components; `toElev` and `POLL_INTERVAL_MS` copied between two DF pages. | `components/CommandSettings.vue`, `components/CustomCommands.vue`, `pages/df/index.vue`, `pages/df/citizens.vue` |
| F5 | M | Error UX varies: some panels fail silently (only `console.error`), others show a message or the raw `err.message`. Only three files use `role="alert"`. | silent: `AutoShout.vue`, `DadResponses.vue`, `FishScore.vue`, `Weather.vue`, `Promos.vue`; raw: `pages/df/index.vue`, `pages/df/citizens.vue` |
| F6 | L | Indentation: 4 spaces in some files, 2 elsewhere. There's no formatter. | 4-space: `AutoShout.vue`, `DadResponses.vue`, `FishScore.vue`, `Promos.vue`, `Weather.vue`, `pages/df/*`, `pages/devlog/index.vue`, `types/df.ts`, `utils/dfService.ts` |
| F7 | M | A non-page module sits in `pages/`, where Nuxt may scan it as a route. | `pages/df/renderer.ts` |
| F8 | L | The composable file name doesn't match its export: `useOverlayAccess.ts` exports `useHasOverlayAccess`. | `composables/useOverlayAccess.ts` |
| F9 | L | Styling mix: Bootstrap utilities in dashboard panels, custom classes in the DF pages and layout, about 130 hardcoded hex colours next to the `base.css` variables. | `components/*.vue`, `layouts/default.vue`, `assets/css/main.css` |
| F10 | L | Stale comments mention a JWT/Bearer token (auth is an HttpOnly cookie) and a nonexistent `DF.vue`. There's leftover code: an empty `onMounted` with a commented `console.log` in devlog, and an empty `<style scoped>` in Weather. | `components/Weather.vue`, `utils/dfService.ts`, `pages/df/citizens.vue`, `pages/devlog/index.vue` |
| F11 | L | A component mutates store state directly (`auth.user.bot_opt_in = …`) instead of calling a store action. | `components/BotOptInToggle.vue` |
| F12 | L | `definePageMeta({ ssr: false })` is set on only two pages. The others rely on fetching in `onMounted`. | `pages/auth/landing.vue`, `pages/overlay/index.vue` |
| F13 | L | No ESLint, Prettier, or typecheck. The only test (`tests/publicStats.test.ts`) isn't run in CI (job `nuxt` runs only `npm ci && npm run build`). `nivek-nuxt/README.md` is the unmodified Nuxt starter. | `nivek-nuxt/package.json`, `.github/workflows/ci.yml` |
| F14 | L | The only browser-side third-party call is `ipapi.co` (allowed through the CSP). | `components/Weather.vue`, `nuxt.config.ts` |

## Docs

| ID | Sev | Inconsistency | Where |
| --- | --- | --- | --- |
| X1 | L | See D2: the deploy-runbook reason for the handler-before-seed order is out of date. | `docs/deploy-runbook.md` |
| X2 | L | Local nginx strips `/api` before proxying (`proxy_pass http://core-api/;`) while Traefik in prod forwards `/api`. Not documented. | `docker/nginx-local/default.conf`, `docker-compose.yml` |
