-- Production apply for the !quote global builtin (issue #162).
-- Safe to re-run: CREATE TABLE IF NOT EXISTS, INSERT ... ON CONFLICT DO NOTHING.
-- Purely additive: a new table and one new command row; nothing is altered or
-- dropped.
--
-- Deploy order (important):
--   1. The CREATE TABLE can run at any time — nothing reads nivek.quote until
--      the new core-api is live.
--   2. Deploy / merge this branch FIRST so the running bot binary has the
--      'quote' handler in builtinRegistry. The bot fails boot on an unknown
--      handler_key, so inserting the !quote row while an older binary is still
--      running would crash that binary on its next restart.
--   3. Then run the INSERT below. The bot picks the row up on its next restart;
--      until then (and if the row is never inserted) the handler simply stays
--      inactive.

CREATE TABLE IF NOT EXISTS nivek.quote (
    id SERIAL PRIMARY KEY,
    channel_twitch_id VARCHAR(64) NOT NULL,
    quote_number      INTEGER NOT NULL CHECK (quote_number > 0),
    text              TEXT NOT NULL,
    added_by          VARCHAR(50) NOT NULL DEFAULT '',
    game              TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT quote_channel_number_uk UNIQUE (channel_twitch_id, quote_number)
);

INSERT INTO nivek.command (trigger, kind, handler_key, min_role, cooldown_secs, description)
VALUES (
    '!quote',
    'builtin',
    'quote',
    'everyone',
    5,
    'Channel quotes. "!quote" for a random one, "!quote <n>" for a specific one. Mods/broadcaster add with "!quote add <text>" (max 300 chars) and remove with "!quote del <n>".'
)
ON CONFLICT (trigger) WHERE scope = 'global' DO NOTHING;
