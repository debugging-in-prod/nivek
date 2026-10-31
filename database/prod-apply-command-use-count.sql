-- Production apply for the custom-command response variables change (#158).
-- Safe to re-run: ADD COLUMN IF NOT EXISTS is idempotent and purely additive.
--
-- Deploy order: any. The column backs the ${count} response variable. Until it
-- exists, core-api's POST /bot/commands/use returns 500 and the bot leaves
-- ${count} literal in the reply (it still replies). Nothing else reads it, and
-- no seed rows / handler_keys are added, so the bot's boot check is unaffected.

ALTER TABLE nivek.command ADD COLUMN IF NOT EXISTS use_count INTEGER NOT NULL DEFAULT 0;
