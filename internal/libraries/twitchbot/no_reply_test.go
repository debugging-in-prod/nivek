package twitchbot

import (
	"slices"
	"strings"
	"testing"

	"github.com/gempir/go-twitch-irc/v4"
)

func TestParseNoReplyLogins(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{" , ,", nil},
		{"Nightbot", []string{"nightbot"}},
		{" StreamElements , MooBot,,fossabot ", []string{"streamelements", "moobot", "fossabot"}},
	}
	for _, c := range cases {
		if got := ParseNoReplyLogins(c.raw); !slices.Equal(got, c.want) {
			t.Errorf("ParseNoReplyLogins(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestDefaultNoReplyLoginsWellFormed(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, l := range defaultNoReplyLogins {
		if l != strings.ToLower(strings.TrimSpace(l)) || l == "" {
			t.Errorf("default %q must be lowercased, trimmed and non-empty", l)
		}
		if seen[l] {
			t.Errorf("default %q is listed twice", l)
		}
		seen[l] = true
	}
}

func TestDefaultNoReplyLoginsExcludeOwnerAccounts(t *testing.T) {
	t.Parallel()
	// The owner's channel, chat bot and dev-app accounts must never be ignored
	// by default: that would silence the bot's own channel/commands.
	for _, owner := range []string{"timallenfanclubofficial", "peanutbudderbot", "peanutbudder321"} {
		if slices.Contains(defaultNoReplyLogins, owner) {
			t.Errorf("owner account %q must not be in defaultNoReplyLogins", owner)
		}
	}
}

func TestBuildNoReplySet(t *testing.T) {
	t.Parallel()

	// Env empty: every default is present, plus the bot itself.
	def := buildNoReplySet(ParseNoReplyLogins(""), "PeanutBudderBot")
	for _, l := range defaultNoReplyLogins {
		if _, ok := def[l]; !ok {
			t.Errorf("default %q missing when env is empty", l)
		}
	}
	if _, ok := def["peanutbudderbot"]; !ok {
		t.Error("bot's own login (lowercased) must be in the set")
	}
	if want := len(defaultNoReplyLogins) + 1; len(def) != want {
		t.Errorf("env empty: set size = %d, want %d", len(def), want)
	}
	if NoReplyLoginCount(nil, "PeanutBudderBot") != len(def) {
		t.Error("NoReplyLoginCount must match the built set size")
	}

	// Env adds to the defaults; overlaps with defaults/bot and case variants
	// are deduped.
	extra := buildNoReplySet([]string{" SomeCustomBot ", "somecustombot", "NightBot", "peanutbudderbot", ""}, "peanutbudderbot")
	if want := len(defaultNoReplyLogins) + 2; len(extra) != want {
		t.Errorf("env union: set size = %d, want %d", len(extra), want)
	}
	for _, l := range []string{"somecustombot", "nightbot", "streamelements", "peanutbudderbot"} {
		if _, ok := extra[l]; !ok {
			t.Errorf("env union: %q missing", l)
		}
	}
	for l := range extra {
		if l != strings.ToLower(l) {
			t.Errorf("set entry %q is not lowercased", l)
		}
	}

	// No bot username configured: defaults only.
	if got := buildNoReplySet(nil, " "); len(got) != len(defaultNoReplyLogins) {
		t.Errorf("blank bot username: set size = %d, want %d", len(got), len(defaultNoReplyLogins))
	}
}

func TestIsNoReplyLogin(t *testing.T) {
	t.Parallel()

	def := &Bot{config: Config{BotUsername: "PeanutBudderBot"}}
	def.noReply = buildNoReplySet(nil, def.config.BotUsername)
	for _, login := range []string{"streamelements", "MooBot", " sery_bot ", "Nightbot", "commanderroot", "peanutbudderbot", "PEANUTBUDDERBOT"} {
		if !def.isNoReplyLogin(login) {
			t.Errorf("default set: %q should be ignored", login)
		}
	}
	for _, login := range []string{"", "alice", "timallenfanclubofficial", "peanutbudder321"} {
		if def.isNoReplyLogin(login) {
			t.Errorf("default set: %q should not be ignored", login)
		}
	}

	custom := &Bot{config: Config{BotUsername: "peanutbudderbot"}}
	custom.noReply = buildNoReplySet(ParseNoReplyLogins("SomeCustomBot, fossabot"), custom.config.BotUsername)
	if !custom.isNoReplyLogin("somecustombot") || !custom.isNoReplyLogin("FOSSABOT") {
		t.Error("custom set: configured logins should be ignored")
	}
	if !custom.isNoReplyLogin("streamelements") {
		t.Error("custom set must keep the defaults (union, not replacement)")
	}
	if !custom.isNoReplyLogin("peanutbudderbot") {
		t.Error("bot's own login must always be ignored")
	}

	// A Bot built without NewBot (nil set) still ignores itself.
	bare := &Bot{config: Config{BotUsername: "peanutbudderbot"}}
	if !bare.isNoReplyLogin("PeanutBudderBot") || bare.isNoReplyLogin("alice") {
		t.Error("nil set: only the bot's own login should be ignored")
	}
}

func TestHandleMessageIgnoresNoReplyLogins(t *testing.T) {
	t.Parallel()
	// handleMessage records an ID in seenIRCIDs only after the sender filter, so
	// an unrecorded ID proves the message was dropped before any processing.
	for _, c := range []struct {
		login   string
		ignored bool
	}{{"streamelements", true}, {"Sery_Bot", true}, {"alice", false}} {
		b := &Bot{
			config:     Config{BotUsername: "peanutbudderbot"},
			noReply:    buildNoReplySet(nil, "peanutbudderbot"),
			seenIRCIDs: newMessageIDCache(8),
		}
		b.handleMessage(twitch.PrivateMessage{
			ID:      "irc-1",
			Channel: "somechannel",
			User:    twitch.User{ID: "42", Name: c.login},
			Message: "hello",
		})
		if processed := b.seenIRCIDs.seen("irc-1"); processed == c.ignored {
			t.Errorf("login %q: processed=%v, want ignored=%v", c.login, processed, c.ignored)
		}
	}
}

func TestHandleWebhookMessageIgnoresNoReplyLogins(t *testing.T) {
	t.Parallel()
	// The stalk recorder runs right after the sender filter; if the chatter is
	// the stalk target and the message is not recorded, it was dropped.
	for _, c := range []struct {
		login   string
		ignored bool
	}{{"MooBot", true}, {"streamelements", true}, {"alice", false}} {
		target := strings.ToLower(c.login)
		b := &Bot{
			config:  Config{BotUsername: "peanutbudderbot", BotId: "1"},
			noReply: buildNoReplySet(nil, "peanutbudderbot"),
			stalk:   map[string]*stalkWatch{"somechannel": {target: target}},
		}
		event := `{"broadcaster_user_login":"somechannel","broadcaster_user_id":"9","chatter_user_id":"42",` +
			`"chatter_user_login":"` + c.login + `","chatter_user_name":"` + c.login + `","message_id":"m1","message":{"text":"hello"}}`
		b.handleWebhookMessage(&EventSubSubscriptionResponse{Event: []byte(event)})
		if processed := b.stalk["somechannel"].last != ""; processed == c.ignored {
			t.Errorf("login %q: processed=%v, want ignored=%v", c.login, processed, c.ignored)
		}
	}
}
