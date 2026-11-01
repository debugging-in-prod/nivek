package twitchbot

import (
	"sync/atomic"
	"testing"

	"github.com/gempir/go-twitch-irc/v4"
)

// R5.2: the bot's Twitch user id comes from Config (env TWITCH_BOT_ID). The
// zero value must fall back to the id that used to be hard-coded so the
// self-reply guard behaves exactly as before.

func TestConfigBotIDDefault(t *testing.T) {
	t.Parallel()
	if DefaultBotID != "1322716097" {
		t.Fatalf("DefaultBotID = %q, want 1322716097", DefaultBotID)
	}
	cases := []struct{ in, want string }{
		{"", DefaultBotID},
		{"   ", DefaultBotID},
		{"1322716097", "1322716097"},
		{" 555 ", "555"},
	}
	for _, c := range cases {
		if got := (Config{BotId: c.in}).botID(); got != c.want {
			t.Errorf("Config{BotId: %q}.botID() = %q, want %q", c.in, got, c.want)
		}
	}
}

// A chat message from the default bot id gets no reply and runs no command,
// even when the login doesn't match the bot's (so only the id check catches it).
func TestWebhookMessageFromDefaultBotIDNoReply(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"", DefaultBotID} {
		var calls atomic.Int32
		b, _ := dispatchBot(&calls)
		b.config.BotId = id

		b.handleWebhookMessage(chatEvent("m1", DefaultBotID, "someone", "!fish", false))
		b.handleWebhookMessage(chatEvent("m2", DefaultBotID, "someone", "!hi", false))
		if calls.Load() != 0 || len(b.sayQueue) != 0 {
			t.Fatalf("BotId=%q: handler calls=%d, queued=%d; want no reply to the bot's own id",
				id, calls.Load(), len(b.sayQueue))
		}

		// Sanity: the same message from another user does reply.
		b.handleWebhookMessage(chatEvent("m3", "42", "alice", "!hi", false))
		if len(b.sayQueue) != 1 {
			t.Fatalf("BotId=%q: queued %d replies for alice's !hi, want 1", id, len(b.sayQueue))
		}
	}
}

func TestIRCMessageFromDefaultBotIDIsBot(t *testing.T) {
	t.Parallel()
	b := &Bot{config: Config{BotUsername: "peanutbudderbot"}} // zero BotId
	if !b.isBotIRCUser(twitch.PrivateMessage{User: twitch.User{ID: DefaultBotID, Name: "someone"}}) {
		t.Error("zero BotId: message from DefaultBotID must count as the bot")
	}
	if b.isBotIRCUser(twitch.PrivateMessage{User: twitch.User{ID: "99", Name: "alice"}}) {
		t.Error("zero BotId: other users must not count as the bot")
	}
}
