package promo

import "time"

const TablePromo = "promo"

// Interval bounds shared by the service (clamp on write) and the bot's
// scheduler (floor at read). Kept here so the DB CHECK, the API validation, and
// the chat command all agree on the same limits.
const (
	MinIntervalSeconds = 60    // 1 minute — anything tighter is spam
	MaxIntervalSeconds = 86400 // 24 hours
)

// Chat-activity gate bounds. A promo only posts once at least MinChatLines
// non-bot chat lines have been seen in its channel since its last post. 0
// disables the activity requirement (the live requirement always applies).
const (
	DefaultMinChatLines = 5
	MaxMinChatLines     = 1000
)

// ClampMinChatLines keeps a chat-line threshold inside [0, MaxMinChatLines].
func ClampMinChatLines(n int) int {
	if n < 0 {
		return 0
	}
	if n > MaxMinChatLines {
		return MaxMinChatLines
	}
	return n
}

// EffectiveMinChatLines resolves a promo's chat-line threshold. A nil value
// (column not yet migrated, or an older core-api that doesn't send the field)
// falls back to DefaultMinChatLines so the gate is never accidentally off.
func (p Promo) EffectiveMinChatLines() int {
	if p.MinChatLines == nil {
		return DefaultMinChatLines
	}
	return ClampMinChatLines(*p.MinChatLines)
}

// Promo is one recurring message. One row per channel (broadcaster_id UNIQUE).
// channelname is the lowercased Twitch login of the channel the message is
// posted in, so a promo created from chat and one created from the dashboard
// land on the same row.
type Promo struct {
	Id              int    `db:"id,omitempty" json:"id"`
	Channelname     string `db:"channelname" json:"channelname"`
	BroadcasterId   string `db:"broadcaster_id" json:"broadcaster_id"`
	Message         string `db:"message" json:"message"`
	IntervalSeconds int    `db:"interval_seconds" json:"interval_seconds"`
	Enabled         bool   `db:"enabled" json:"enabled"`
	// MinChatLines is how many non-bot chat lines must be seen in the channel
	// since the promo's last post before it may post again. Pointer + omitempty
	// so inserts leave it to the DB default (5) and a missing column/field reads
	// as nil → DefaultMinChatLines.
	MinChatLines *int      `db:"min_chat_lines,omitempty" json:"min_chat_lines,omitempty"`
	UpdatedAt    time.Time `db:"updated_at,omitempty" json:"updated_at"`
	CreatedAt    time.Time `db:"created_at,omitempty" json:"created_at"`
}

// MaxMessageLen matches Twitch's 500-character chat message limit; a longer
// promo could never be posted.
const MaxMessageLen = 500
