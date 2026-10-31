-- Production apply for the info-command builtins (issue #160):
-- !uptime, !followage, !accountage, !game, !title.
-- Safe to re-run: INSERT ... ON CONFLICT DO NOTHING only. Additive — no schema
-- change, no existing row is modified.
--
-- Deploy order (important):
--   1. Deploy / merge this branch FIRST so the running bot binary has the
--      uptime/followage/accountage/game/title handlers in builtinRegistry.
--      The bot fails boot on an unknown handler_key, so inserting these rows
--      while an older binary is running would crash it on its next restart.
--   2. Then run this file. Until it runs the new handlers simply stay inactive
--      (no seed row -> not in the dispatch map). The bot loads commands at
--      boot, so restart it afterwards to pick the rows up.
--
-- Each command is seeded with cooldown_secs = 10 (central cooldown gate).

INSERT INTO nivek.command (trigger, kind, handler_key, min_role, cooldown_secs, description) VALUES
    ('!uptime',     'builtin', 'uptime',     'everyone', 10, 'Shows how long the stream has been live, or that the channel is offline.'),
    ('!followage',  'builtin', 'followage',  'everyone', 10, 'Shows how long you (or "!followage <user>") have followed this channel. Needs the bot to be a mod here.'),
    ('!accountage', 'builtin', 'accountage', 'everyone', 10, 'Shows how old your Twitch account (or "!accountage <user>") is.'),
    ('!game',       'builtin', 'game',       'everyone', 10, 'Shows the category the channel is currently streaming.'),
    ('!title',      'builtin', 'title',      'everyone', 10, 'Shows the channel''s current stream title.')
ON CONFLICT (trigger) WHERE scope = 'global' DO NOTHING;
