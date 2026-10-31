# Merge criteria

A PR may merge into `main` only if **every** item below holds. Merging to
`main` deploys to production automatically, and production is the only test
environment. Treat each item as a hard gate, not as guidance.

This document is written so that Claude and other automated agents can check a
change against it. Each section says what the rule is, why it exists, and how
to check it.

---

## 1. The bot must keep working

**Rule.** A PR must not be able to prevent `cmd/twitch-bot` from **starting**,
**connecting to Twitch**, or **handling chat messages**.

In practice:

- **Fail soft.** Errors from core-api, Helix/EventSub, DeepL, Discord,
  df-executor, or the DB (through core-api) are logged and the bot keeps
  running. New code never calls `panic`, `log.Fatal`, or `os.Exit`, and never
  lets an error escape a goroutine without being caught.
- **Startup tolerates core-api being down.** New boot-time calls to core-api
  must retry or degrade, like `loadCommandsAtBoot` followed by
  `retryCommandsInBackground` in `twitchbot/command_loader.go`. They must not
  exit. (Existing exception, scheduled for removal: `getLiveChannels` in
  `cmd/twitch-bot/main.go` still exits if core-api is unreachable at boot. See
  [inconsistencies.md](inconsistencies.md). Don't add more of these.)
- **New bot-side tracking is non-blocking, bounded, and panic-recovered.** It
  runs off the message path (goroutine or buffered channel), has a cap on
  memory (map size, queue length) with pruning, drops data instead of
  blocking when core-api is slow or down, and every goroutine starts with
  `defer recover()`. See `usage_tracker.go` as the reference.
- **Network calls reachable from the message path have a timeout**
  (`context.WithTimeout` or an `http.Client.Timeout`).
- **No new required env vars.** Any new config is optional and defaults to
  safe behaviour, which is usually "feature off".
- **The bot never talks to Postgres.** All state goes through
  `api.CoreAPIClient`.

**Check:** read every new bot code path and ask what happens if core-api
returns 500, times out, or is unreachable. The answer must be "that feature
replies with a soft error or nothing, and everything else carries on."

## 2. The bot must never spam a chat

**Rule.** A PR must not be able to make the bot send unwanted, repeated, or
looping messages to any channel.

- **Every command goes through the central cooldown gate.** All dispatch,
  builtin or custom, EventSub or IRC, calls `commandAllowed` (or
  `ircCommandAllowed` for IRC) in `internal/libraries/twitchbot/cooldown.go`
  before its handler runs. Put role checks *before* the gate so unauthorised
  users can't burn the cooldown.
- **Never reply to itself or to other bots.** Messages from the bot's own
  user ID/login, and from logins on the no-reply list (`no_reply.go`,
  `TWITCH_BOT_NO_REPLY_LOGINS`, defaults `streamelements,moobot,sery_bot`), are
  dropped before any dispatch or reply. New inbound paths must apply the same
  filter.
- **No loops.** Bot output must never trigger more bot output, whether
  directly, through another bot, or through a command whose reply contains a
  trigger. Argument-taking commands are dispatched up front and then
  `return`, so their arguments are never scanned for more triggers.
- **Timers only post when the channel is live and chat is active.** Scheduled
  messages (promos, or any future timer) post only when the channel is
  currently live (EventSub state, or a cached Helix lookup, with unknown or
  error counting as **offline**) and enough real chat lines have arrived since
  the last post (`promo_gate.go`). Never post to an offline or inactive
  channel from a timer.
- **Replies are sanitised.** Every outgoing reply passes through
  `sanitizeChatReply` (or an equivalent shared sanitiser) before
  `say`/`announce`:
  - it is capped at **450 characters** (runes);
  - it never starts with `/` or `.`, so user-controlled text can't become a
    Twitch chat command such as `/ban` or `.timeout`;
  - control characters and newlines are flattened to single spaces.
- **One reply per command invocation.** Per-word reactions that aren't gated
  aren't allowed for new code.

**Check:** for each new `b.say(` / `b.announce(` / `ircsay(` call, trace back
to the gate (`commandAllowed`), the self/no-reply filter, and the sanitiser.
For each new timer, confirm the live check and the chat-activity check.

## 3. Database changes

- **Additive only.** Allowed: `CREATE TABLE IF NOT EXISTS`,
  `ADD COLUMN IF NOT EXISTS ... DEFAULT ...`, `CREATE INDEX IF NOT EXISTS`, and
  `INSERT ... ON CONFLICT DO NOTHING`. Not allowed: dropping, renaming, or
  changing the type of anything that's in use, or adding a `NOT NULL` column
  with no default.
- **Idempotent.** Running the file twice gives the same result. CI job
  `db-migrations` applies `init.sql` and then every `prod-apply-*.sql` **twice**.
- **Shipped in two places.**
  1. `database/init.sql` for fresh databases. **This file uses CRLF line
     endings. Preserve them.** `git diff --stat` must show only the lines you
     really added.
  2. A new `database/prod-apply-<name>.sql` for production. Its header comment
     says what it does, that it is safe to re-run, and the **deploy order**.
- **Applied by hand, per [deploy-runbook.md](../deploy-runbook.md).** Back up,
  apply with `ON_ERROR_STOP=1`, **restart core-api** and wait for healthy,
  **restart twitch-bot**, then check the journal. The deploy pipeline does not
  apply SQL.
- **New builtin handlers ship before their seed rows.** A `nivek.command` row
  whose `handler_key` isn't in the running binary's `builtinRegistry` must
  never exist in prod.
  - The Go handler and its `builtinRegistry` entry merge and deploy **first**.
  - The seed row arrives afterwards through `prod-apply-<name>.sql` with
    `ON CONFLICT ... DO NOTHING`.
  - A handler that has no row yet simply stays inactive. That state is safe.

  The current loader logs and skips an unknown `handler_key` instead of
  crashing (`buildCommandMaps`). Older binaries crashed on it, and
  `deploy-runbook.md` still treats it as fatal, so keep the order anyway.

## 4. CI must pass

All jobs in `.github/workflows/ci.yml` must be green:

| Job | Runs |
| --- | --- |
| `go` | `gofmt -l cmd internal` (must print nothing), `go test -race ./...`, `go vet ./...` |
| `nuxt` | `npm ci && npm run build` in `nivek-nuxt` |
| `db-migrations` | `init.sql` plus every `prod-apply-*.sql` applied twice, then `go vet -tags integration` and the integration tests |

Run the same commands locally before pushing. New logic ships with unit tests.

## 5. Process

- **Every feature, infra, or maintenance change has a GitHub issue.** One PR
  per issue. The PR body says `Closes #<n>`. Keep the PR to that issue. Put
  drive-by refactors in their own issue/PR (see
  [refactor-plan.md](refactor-plan.md)).
- **Security and host-hardening issues stay vague in public.** This repo is
  open source. Public issues and PRs about host or infra hardening describe
  the goal, not the weakness. Exploitable code bugs are reported and fixed
  through **GitHub private security advisories**, not public issues. See
  [SECURITY.md](../../SECURITY.md).
- **No secrets** in the repo, in logs, in test fixtures, or in issue/PR text.
  That covers tokens, HMAC keys, webhook URLs, passwords, and `.env` contents.
- **Don't touch, unless the issue is specifically about them:** security
  hardening (auth, CSRF, CORS, headers, HMAC verification), deploy workflows
  (`.github/workflows/build.yml`, `deploy-prod.yml`), systemd units, and
  secrets handling.

## 6. Deployment awareness

- **A merge to `main` deploys to prod automatically** (GitHub Actions on a
  self-hosted runner rebuilds core-api, the web frontend, and the bot binary).
- **The deploy does not restart twitch-bot.** Restarting it, and restarting
  core-api after any schema change, is part of deploying the change, and it
  follows [deploy-runbook.md](../deploy-runbook.md) (core-api healthy first,
  then the bot). The PR description says whether a restart or a SQL apply is
  needed.
- **Things that stay on purpose. Don't remove or "modernise" them:**
  - the deprecated `df-*` services (`cmd/df-executor`, `cmd/df-snapshot-pusher`,
    `internal/libraries/overseer`, `cmd/core-api/endpoints/df`);
  - the `gempir/go-twitch-irc` IRC library (still used for `!banish` and the
    chat-read nudge).

---

## Reviewer checklist (copy into the PR)

```
- [ ] Bot cannot fail to start/connect/handle messages because of this change (fail-soft, no new Fatal/panic, startup tolerates core-api down)
- [ ] No new required env vars; new config optional with safe default
- [ ] Every new command dispatch goes through commandAllowed / ircCommandAllowed
- [ ] No replies to self / other bots / no-reply list; no loops
- [ ] Timers post only when live AND chat active
- [ ] Every reply sanitised (≤450 chars, no leading '/' or '.')
- [ ] New bot tracking is non-blocking, bounded, panic-recovered
- [ ] DB changes additive + idempotent; init.sql (CRLF kept) + prod-apply-*.sql; deploy order in header
- [ ] Builtin handler ships before its seed row
- [ ] gofmt / go vet / go test -race / nuxt build / db-migrations green
- [ ] Linked issue, one PR per issue, "Closes #n"
- [ ] No secrets; security details not disclosed publicly
- [ ] Deploy notes: SQL to apply? core-api restart? bot restart?
- [ ] df-* services and gempir IRC untouched
```
