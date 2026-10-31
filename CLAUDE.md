# CLAUDE.md

## Coding standards and merge criteria (required)

Before changing code in this repo, read [`docs/standards/`](docs/standards/README.md)
and follow it:

- [`docs/standards/merge-criteria.md`](docs/standards/merge-criteria.md) is a
  **hard gate**. Don't propose, commit, or open a PR for a change that breaks
  any item in it. In particular:
  - the change can't stop `twitch-bot` from starting, connecting, or handling
    messages;
  - it can't make the bot spam (every command goes through `commandAllowed`,
    no replies to itself, other bots, or the no-reply list, timers only when
    the channel is live and chat is active, replies sanitised to 450 chars
    with no leading `/` or `.`);
  - migrations are additive and idempotent;
  - builtin handlers ship before their seed rows;
  - CI is green;
  - one PR per issue;
  - no secrets.
- [`architecture.md`](docs/standards/architecture.md),
  [`go.md`](docs/standards/go.md), and
  [`typescript.md`](docs/standards/typescript.md) define where code lives and
  how it's written. Follow the existing dominant patterns they describe. Don't
  invent new layers.
- Known deviations are in
  [`inconsistencies.md`](docs/standards/inconsistencies.md). Fix them only
  through the separate PRs in
  [`refactor-plan.md`](docs/standards/refactor-plan.md), not as drive-bys in
  unrelated work.

Before finishing any code change, run `gofmt -l cmd internal` (must print
nothing), `go build ./...`, `go vet ./...`, and `go test -race ./...`. If
`nivek-nuxt/` changed, also run `npm ci && npm run build` there.
