-- Production apply for the promo chat-activity gate (issue #163).
-- Safe to re-run: ADD COLUMN IF NOT EXISTS with a default, purely additive.
--
-- Deploy order: any order works. The bot treats a missing min_chat_lines as the
-- default (5), and core-api leaves the column out of inserts/updates unless it
-- already holds a value, so neither binary depends on this having run first.
-- Apply it before setting a promo's threshold via the dashboard API.
--
-- What this does:
--   Adds nivek.promo.min_chat_lines — how many non-bot chat lines must be seen
--   in the channel since the promo's last post before it may post again
--   (0 = no activity requirement; the channel must still be live). Existing
--   rows get the default of 5.

ALTER TABLE nivek.promo ADD COLUMN IF NOT EXISTS min_chat_lines INTEGER NOT NULL DEFAULT 5
    CHECK (min_chat_lines >= 0 AND min_chat_lines <= 1000);
