# Deploy runbook: applying `prod-apply-*.sql`

Schema and seed changes ship as hand-run `database/prod-apply-*.sql` files.
They are **not** applied by the deploy pipeline. This runbook is the procedure
for applying one safely.

Why it exists: incident [#172](https://github.com/debugging-in-prod/nivek/issues/172).
An additive `ALTER TABLE ... ADD COLUMN` was applied while core-api was running.
core-api's pooled connections had cached prepared statements for whole-row
selects, and those statements started failing intermittently with
`bind message has N result formats but query has M columns (SQLSTATE 08P01)`.
Then twitch-bot crashed at boot because `/commands` failed. A restart of
core-api drops the pool and fixes it. This runbook makes that restart
mandatory, not something you do after the alert fires.

> CI already proves every `prod-apply-*.sql` applies cleanly on top of
> `init.sql` and is idempotent (job `db-migrations` in `.github/workflows/ci.yml`).
> Don't apply a file whose PR didn't pass that job.

## Where things are

| Thing | Location |
|---|---|
| Host | `ssh vultr` (user `deploy`) |
| Compose project (postgres, core-api, web) | `/home/deploy/actions-runner/_work/nivek/nivek` |
| twitch-bot | host systemd unit `twitch-bot.service` (not in Docker) |
| core-api health | `GET /api/healthz` (container healthcheck + public `https://peanutbudderbot.com/api/healthz`) |

All `docker compose` commands below run from the compose project directory:

```sh
cd /home/deploy/actions-runner/_work/nivek/nivek
```

## 0. Check the deploy order first

Read the header comment of the `prod-apply-*.sql` file. Some files depend on
code being deployed first. For example, a new builtin's seed row needs the bot
binary with that handler. The current bot doesn't crash on an unknown
`handler_key`: `buildCommandMaps` (`internal/libraries/twitchbot/command_loader.go`)
logs `skipping invalid command row: ... unknown handler_key` and skips the row.
The order still matters, for two reasons:

- binaries from before #174 do fail boot on an unknown `handler_key`;
- on the current binary, a row applied before its handler ships stays
  inactive (the trigger does nothing) until the binary is deployed and the
  bot restarted.

Older `prod-apply-*.sql` headers still give the old "fails boot" reason. Leave
them as they are (they're applied history); the order they ask for is still
right. Make sure the merge/deploy that the file depends on is done (GitHub
Actions run is green) before you continue.

## 1. Back up the database (always)

```sh
mkdir -p ~/backups
docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  > ~/backups/nivek-$(date -u +%Y%m%dT%H%M%SZ).dump
ls -lh ~/backups | tail -n 3   # non-zero size, fresh timestamp
```

The command runs inside the postgres container, so no credentials touch the
host shell or its history. Restore, if you ever need it:

```sh
docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists' \
  < ~/backups/<file>.dump
```

## 2. Apply the SQL

```sh
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < database/prod-apply-<name>.sql
```

`ON_ERROR_STOP=1` aborts on the first error, so a failure doesn't leave the
rest of the file half-applied. The files are written to be idempotent, so
re-running one after fixing the cause is safe.

## 3. ALWAYS restart core-api and wait until it is healthy

Do this even when the change "only adds a column" or "only inserts rows".
Additive changes are exactly what broke #172.

```sh
docker compose restart core-api

# Wait for every core-api container to report healthy (up to ~2 min).
for i in $(seq 1 24); do
  s=$(docker compose ps -q core-api | xargs docker inspect --format '{{.State.Health.Status}}' | sort -u)
  echo "core-api: $s"
  [ "$s" = "healthy" ] && break
  sleep 5
done

curl -fsS https://peanutbudderbot.com/api/healthz && echo
# DB-backed public route (the one that 500'd in #172). Hit it a few times so
# it lands on more than one pooled connection.
for i in 1 2 3 4 5; do curl -fsS -o /dev/null -w '%{http_code}\n' https://peanutbudderbot.com/api/commands; done
```

Don't continue until core-api reports `healthy` and every request returns `200`.

## 4. Restart twitch-bot

The bot loads global commands only at boot, so it has to restart after
core-api is healthy, never before.

```sh
sudo systemctl restart twitch-bot
systemctl status twitch-bot --no-pager
```

## 5. Verify the bot in journalctl

```sh
journalctl -u twitch-bot --since "5 min ago" --no-pager
```

Look for:

- a clean boot: no `panic`, no `unable to load commands`, no restart loop
  (`systemctl status` shows one recent start, not several);
- no `SQLSTATE`, `08P01`, or `5xx` lines;
- the bot joining its channels.

Also check core-api for 5xx since the restart:

```sh
docker compose logs --since 5m core-api | grep -E '5xx|SQLSTATE' || echo "clean"
```

## If something is wrong

- **core-api 500s with `08P01` after the apply:** you skipped or raced step 3.
  Restart core-api again (step 3), then the bot (step 4).
- **The SQL itself failed:** nothing after the failing statement ran
  (`ON_ERROR_STOP`). Fix the cause and re-run the file (it's idempotent). If
  data is wrong, restore from the step 1 dump.
- **A new command does nothing, and the bot logs
  `skipping invalid command row: ... unknown handler_key`:** a seed row was
  applied before the binary that handles it was deployed (see step 0). The
  bot keeps running and skips that row. Deploy the binary, then restart the
  bot (step 4) so it reloads commands.
- **twitch-bot crash-loops at boot:** check `journalctl -u twitch-bot` for the
  cause. An unknown `handler_key` only crashes binaries from before #174; on
  the current binary it is logged and skipped (see above).

## Automated post-deploy check

`.github/workflows/deploy-prod.yml` ends with a read-only smoke step. It sends
`GET /api/healthz` (retried through the rollout) and then `GET /api/commands`
five times (not retried, so an intermittent 500 isn't hidden). The deploy job
fails on any non-2xx. It doesn't restart anything and it doesn't apply SQL.
This runbook is still the only way prod-apply SQL reaches prod.

## twitch-bot binary location

The canonical twitch-bot binary is `/home/deploy/nivek-bin/twitch-bot`,
outside the checkout. Both `deploy-prod.yml` and `build.yml` rebuild it there
from their own commit on every deploy (build to `twitch-bot.new`, then an
atomic rename). The two workflows share the `production-deploy` concurrency
group, so a pending run can be cancelled; building in both means the binary
still matches the last deployed commit (issue
[#256](https://github.com/debugging-in-prod/nivek/issues/256)). If the build
fails in `build.yml`, the previous canonical binary is kept and still
installed, and the run goes red. The last step of the deploy job in
both `deploy-prod.yml` and `build.yml` copies it into
`/home/deploy/actions-runner/_work/nivek/nivek/bin/twitch-bot`, the path the
`twitch-bot.service` `ExecStart` runs. The copy happens on every deploy, even
when an earlier step failed. It's needed because `actions/checkout` cleans the
gitignored `bin/` in the shared workspace (issue
[#248](https://github.com/debugging-in-prod/nivek/issues/248)). The two
workflows share the `production-deploy` concurrency group, so they never run
at the same time. Deploys never restart the bot.

If the bot fails to start with `status=203/EXEC` (binary missing), reinstall
the canonical copy and restart. Run this as `deploy`, not root:

```sh
cd /home/deploy/actions-runner/_work/nivek/nivek
mkdir -p bin
cp -f /home/deploy/nivek-bin/twitch-bot bin/twitch-bot.new \
  && chmod 755 bin/twitch-bot.new && mv -f bin/twitch-bot.new bin/twitch-bot
sudo systemctl restart twitch-bot
```

If `/home/deploy/nivek-bin/twitch-bot` is missing too, build it first (as
`deploy`, from the checkout):

```sh
mkdir -p /home/deploy/nivek-bin
/usr/local/go/bin/go build -o /home/deploy/nivek-bin/twitch-bot.new ./cmd/twitch-bot \
  && mv -f /home/deploy/nivek-bin/twitch-bot.new /home/deploy/nivek-bin/twitch-bot
```
