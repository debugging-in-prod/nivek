-- Production apply for the !cmd global builtin (issue #161: manage custom
-- commands from chat with "!cmd add|edit|del").
-- Safe to re-run: INSERT ... ON CONFLICT DO NOTHING. Additive only.
--
-- Deploy order (important):
--   1. Deploy / merge this branch FIRST so the running bot binary has the
--      'cmd' handler in builtinRegistry. The bot fails boot on an unknown
--      handler_key, so inserting this row while an older binary is running
--      would crash that bot on its next restart.
--   2. Then run the INSERT below and restart the bot (it loads global
--      commands at boot). Until the row exists the new handler simply stays
--      inactive.

INSERT INTO nivek.command (trigger, kind, handler_key, min_role, cooldown_secs, description)
VALUES (
    '!cmd',
    'builtin',
    'cmd',
    'mod',
    3,
    'Manage this channel''s custom commands from chat (broadcaster/mods only). "!cmd add !trigger <response>", "!cmd edit !trigger <response>", "!cmd del !trigger".'
)
ON CONFLICT (trigger) WHERE scope = 'global' DO NOTHING;
