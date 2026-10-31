-- Production apply for per-channel command-usage analytics (issue #177).
-- Safe to re-run: CREATE TABLE / CREATE INDEX IF NOT EXISTS. Purely additive:
-- one new table and one index; nothing is altered or dropped, and no seed
-- rows / handler_keys are added, so the bot's boot check is unaffected.
--
-- Deploy order: any.
--   - Before this runs, core-api's POST /api/bot/stats/usage returns 500; the
--     bot logs "[USAGE] dropped N usage rows" once a minute and carries on
--     (counters are best-effort, chat is never affected). GET
--     /api/stats/public returns 503 until the table exists.
--   - After applying, follow docs/deploy-runbook.md (restart core-api, wait
--     for healthy). No bot restart is needed for this file.

CREATE TABLE IF NOT EXISTS nivek.command_usage (
    channel_twitch_id VARCHAR(64) NOT NULL,
    channel_login     VARCHAR(50) NOT NULL DEFAULT '',
    trigger           TEXT NOT NULL,
    day               DATE NOT NULL,
    uses              INTEGER NOT NULL DEFAULT 0 CHECK (uses >= 0),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_twitch_id, trigger, day)
);
CREATE INDEX IF NOT EXISTS command_usage_day_idx ON nivek.command_usage (day);
