# nivek coding standards

These documents describe how code in this repo is laid out and written. They
take the patterns most of the repo already follows and make them the rule.
They do not introduce a new architecture. Where the code disagrees with
itself, the standard picks the pattern that most of the code uses, or the one
newer code is converging on. The rest is recorded in
[inconsistencies.md](inconsistencies.md) and scheduled in
[refactor-plan.md](refactor-plan.md).

| Document | What it covers |
| --- | --- |
| [merge-criteria.md](merge-criteria.md) | **Hard gate.** What a PR must satisfy before it can merge. Read this first. |
| [architecture.md](architecture.md) | Processes, layers, dependency direction, where each kind of thing lives, the swappable-client (anti-corruption layer) rule, and the bot / core-api / frontend boundaries. |
| [go.md](go.md) | Go conventions: packages, services, models/constants, endpoints, clients, config, errors, logging, concurrency, tests. |
| [typescript.md](typescript.md) | Nuxt 4 frontend conventions: directories, API calls, types, state, components, styling, errors. |
| [inconsistencies.md](inconsistencies.md) | Every place the code currently deviates from these standards, with file paths. |
| [refactor-plan.md](refactor-plan.md) | Small, separately mergeable PRs that remove those deviations, ordered from lowest to highest risk. |

Related existing docs: [LOGGING.md](../LOGGING.md),
[deploy-runbook.md](../deploy-runbook.md),
[AUTHENTICATION.md](../AUTHENTICATION.md),
[OVERLAY_RELAY.md](../OVERLAY_RELAY.md).

## How to use these docs

- **New code** follows the standards. If a standard conflicts with the code
  around it, follow the standard. Don't fix the surrounding code in the same
  PR unless the issue asks for that. Open or link an issue for it instead
  (one PR per issue).
- **Touching old code**: you may bring the lines you change into line with the
  standards. Repo-wide cleanups go through [refactor-plan.md](refactor-plan.md)
  as their own PRs. Don't mix them into feature work.
- **Reviewing**: check [merge-criteria.md](merge-criteria.md) item by item.
  The other documents are guidance. The merge criteria are not optional.
- **Changing a standard**: edit the doc in its own PR, with a reason. If a
  standard and the dominant code pattern disagree, fix one of them. Don't
  leave both in place.
- **Claude / automated agents**: the root [`CLAUDE.md`](../../CLAUDE.md)
  requires following these documents, and `merge-criteria.md` in particular.

## Source of truth when docs disagree

1. The code, for what actually happens (docs can go stale; see
   [inconsistencies.md](inconsistencies.md) §Docs).
2. [merge-criteria.md](merge-criteria.md), for what is allowed to merge.
3. These standards, for how new code should look.
