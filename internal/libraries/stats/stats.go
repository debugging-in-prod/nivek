// Package stats is the usage-analytics backend (issue #177).
//
// Two halves:
//
//   - Ingest: the bot counts dispatched commands in memory and flushes them
//     every ~60s to core-api (POST /api/bot/stats/usage, HMAC). core-api
//     upserts them into nivek.command_usage, one row per
//     (channel, trigger, UTC day).
//   - Public read: GET /api/stats/public returns a small aggregate (active
//     chats, top channels by command use, top fishers). It is cached in
//     core-api memory for 60s, so the database sees at most one round of these
//     queries per minute no matter how much traffic the endpoint gets.
//
// The public payload only carries public Twitch logins and display names, plus
// counts. It never includes Twitch ids, emails, or tokens.
package stats

import (
	"strings"
	"time"
)

const (
	// DayLayout is the wire and SQL format of UsageCount.Day (UTC date).
	DayLayout = "2006-01-02"

	// MaxUsageBatch caps the rows in one ingest request. The bot's in-memory
	// tracker holds at most this many distinct keys per flush window, so one
	// flush always fits in one request.
	MaxUsageBatch = 1000

	// MaxTriggerLen / MaxLoginLen bound stored strings. Triggers are a single
	// chat word; Twitch logins are at most 25 chars.
	MaxTriggerLen = 50
	MaxLoginLen   = 50
	MaxChannelID  = 64

	// MaxUsesPerRow rejects absurd counts. The bot's cooldown floor caps one
	// trigger in one channel at ~30 uses a minute, so this is generous.
	MaxUsesPerRow = 1_000_000

	// TopChannelsLimit / TopFishersLimit are the public leaderboard sizes.
	TopChannelsLimit = 5
	TopFishersLimit  = 10

	// TopChannelsWindowDays is the top_channels window: the last 30 UTC days,
	// including today.
	TopChannelsWindowDays = 30

	// ActiveChatsDefinition is returned verbatim in the public payload.
	ActiveChatsDefinition = "Channels the bot is serving: every channel that has opted in to the bot (bot_opt_in = true), plus the bot's permanent home channels. Each channel is counted once."
)

// HomeChannelLogins are the channels the bot always joins, opted in or not
// (botCreatorChannel and the bot's own channel; see twitchbot.Bot.Start).
// They count toward active_chats and can appear in top_channels even without
// an opted-in users row.
var HomeChannelLogins = []string{"timallenfanclubofficial", "peanutbudderbot"}

// UsageCount is one aggregated (channel, trigger, day) usage row: the bot ->
// core-api ingest wire format.
type UsageCount struct {
	ChannelTwitchID string `json:"channel_twitch_id"`
	ChannelLogin    string `json:"channel_login"`
	Trigger         string `json:"trigger"`
	Day             string `json:"day"` // DayLayout, UTC
	Uses            int    `json:"uses"`
}

// UsageBatch is the ingest request body.
type UsageBatch struct {
	Counts []UsageCount `json:"counts"`
}

// ChannelUses is one top_channels entry.
type ChannelUses struct {
	DisplayName string `json:"display_name"`
	Uses        int64  `json:"uses"`
}

// Fisher is one top_fishers entry. Score is the sum of the fisher's !fish
// score across every channel; Catches is the number of fish (not trash) they
// have caught across every channel.
type Fisher struct {
	DisplayName string `json:"display_name"`
	Score       int64  `json:"score"`
	Catches     int64  `json:"catches"`
}

// PublicStats is the GET /api/stats/public response body.
type PublicStats struct {
	ActiveChats           int           `json:"active_chats"`
	ActiveChatsDefinition string        `json:"active_chats_definition"`
	TopChannels           []ChannelUses `json:"top_channels"`
	TopChannelsWindowDays int           `json:"top_channels_window_days"`
	TopFishers            []Fisher      `json:"top_fishers"`
	GeneratedAt           time.Time     `json:"generated_at"`
}

// Normalize trims and lowercases the string fields of c in place and reports
// whether the row is valid to store. now bounds Day to a small window around
// the current UTC date so a bad clock or a crafted body can't scatter rows
// across arbitrary dates.
func (c *UsageCount) Normalize(now time.Time) bool {
	c.ChannelTwitchID = strings.TrimSpace(c.ChannelTwitchID)
	c.ChannelLogin = strings.ToLower(strings.TrimSpace(c.ChannelLogin))
	c.Trigger = strings.ToLower(strings.TrimSpace(c.Trigger))
	c.Day = strings.TrimSpace(c.Day)

	if c.ChannelTwitchID == "" || len(c.ChannelTwitchID) > MaxChannelID {
		return false
	}
	for _, r := range c.ChannelTwitchID {
		if r < '0' || r > '9' {
			return false
		}
	}
	if len(c.ChannelLogin) > MaxLoginLen {
		return false
	}
	if c.Trigger == "" || len(c.Trigger) > MaxTriggerLen || strings.ContainsAny(c.Trigger, " \t\r\n") {
		return false
	}
	if c.Uses <= 0 || c.Uses > MaxUsesPerRow {
		return false
	}
	day, err := time.Parse(DayLayout, c.Day)
	if err != nil {
		return false
	}
	today := now.UTC().Truncate(24 * time.Hour)
	if day.Before(today.AddDate(0, 0, -7)) || day.After(today.AddDate(0, 0, 1)) {
		return false
	}
	return true
}

// MergeUsage sums rows that share (channel, trigger, day). A Postgres
// multi-row INSERT ... ON CONFLICT DO UPDATE fails if it touches the same row
// twice, so batches must be de-duplicated before they are upserted. Order of
// first appearance is kept so output is deterministic.
func MergeUsage(in []UsageCount) []UsageCount {
	type key struct{ ch, trig, day string }
	idx := make(map[key]int, len(in))
	out := make([]UsageCount, 0, len(in))
	for _, c := range in {
		k := key{c.ChannelTwitchID, c.Trigger, c.Day}
		if i, ok := idx[k]; ok {
			out[i].Uses += c.Uses
			if out[i].Uses > MaxUsesPerRow {
				out[i].Uses = MaxUsesPerRow
			}
			if c.ChannelLogin != "" {
				out[i].ChannelLogin = c.ChannelLogin
			}
			continue
		}
		idx[k] = len(out)
		out = append(out, c)
	}
	return out
}
