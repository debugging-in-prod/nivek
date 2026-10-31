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
		{"", DefaultNoReplyLogins},
		{"   ", DefaultNoReplyLogins},
		{" , ,", DefaultNoReplyLogins},
		{"Nightbot", []string{"nightbot"}},
		{" StreamElements , MooBot,,fossabot ", []string{"streamelements", "moobot", "fossabot"}},
	}
	for _, c := range cases {
		if got := ParseNoReplyLogins(c.raw); !slices.Equal(got, c.want) {
			t.Errorf("ParseNoReplyLogins(%q) = %v, want %v", c.raw, got, c.want)
		}
	}

	// The returned default slice must not alias the package default.
	got := ParseNoReplyLogins("")
	got[0] = "mutated"
	if DefaultNoReplyLogins[0] != "streamelements" {
		t.Fatal("ParseNoReplyLogins must copy DefaultNoReplyLogins")
	}
}

func TestIsNoReplyLogin(t *testing.T) {
	t.Parallel()

	def := &Bot{config: Config{BotUsername: "PeanutBudderBot"}}
	def.noReply = buildNoReplySet(nil, def.config.BotUsername)
	for _, login := range []string{"streamelements", "MooBot", " sery_bot ", "peanutbudderbot", "PEANUTBUDDERBOT"} {
		if !def.isNoReplyLogin(login) {
			t.Errorf("default set: %q should be ignored", login)
		}
	}
	for _, login := range []string{"", "alice", "nightbot"} {
		if def.isNoReplyLogin(login) {
			t.Errorf("default set: %q should not be ignored", login)
		}
	}

	custom := &Bot{config: Config{BotUsername: "peanutbudderbot"}}
	custom.noReply = buildNoReplySet(ParseNoReplyLogins("Nightbot, fossabot"), custom.config.BotUsername)
	if !custom.isNoReplyLogin("nightbot") || !custom.isNoReplyLogin("FOSSABOT") {
		t.Error("custom set: configured logins should be ignored")
	}
	if custom.isNoReplyLogin("streamelements") {
		t.Error("custom set replaces the defaults")
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
