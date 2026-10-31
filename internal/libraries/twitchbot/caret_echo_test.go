package twitchbot

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
)

func TestCaretEchoOncePerMessageAndCooldown(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, clk := dispatchBot(&calls)
	b.usage, _ = newTestUsage(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))

	// Several "^" words in one message reply exactly once.
	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "^ ^ ^ wow^", false))
	if got := drainSay(b); len(got) != 1 || got[0] != "^" {
		t.Fatalf("first message queued %q, want one \"^\"", got)
	}

	// Within the window: silent for another chatter and for a mod.
	clk.advance(caretEchoCooldownSecs*time.Second - time.Second)
	b.handleWebhookMessage(chatEvent("m2", "43", "bob", "^", false))
	b.handleWebhookMessage(chatEvent("m3", "50", "mod", "^^", true))
	if got := drainSay(b); len(got) != 0 {
		t.Fatalf("queued %q inside the cooldown, want nothing", got)
	}

	// After the window it replies again (once).
	clk.advance(time.Second)
	b.handleWebhookMessage(chatEvent("m4", "43", "bob", "^ ^", false))
	if got := drainSay(b); len(got) != 1 || got[0] != "^" {
		t.Fatalf("after the window queued %q, want one \"^\"", got)
	}

	// The echo is not a command, so it is never counted as usage.
	if rows, _ := b.usage.drain(); len(rows) != 0 {
		t.Errorf("usage rows = %+v, want none", rows)
	}
}

func TestCaretEchoDoesNotBlockCommands(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, _ := dispatchBot(&calls)

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!fish ^", false))
	if calls.Load() != 1 {
		t.Fatalf("!fish ran %d times, want 1", calls.Load())
	}
	if got := drainSay(b); len(got) != 1 || got[0] != "^" {
		t.Fatalf("queued %q, want one \"^\"", got)
	}
}

func TestCaretEchoPerChannel(t *testing.T) {
	t.Parallel()
	cd, _ := newTestCooldowns()
	b := &Bot{cooldowns: cd}
	if !b.caretEchoAllowed(&chatMessageEvent{BroadcasterUserId: "1"}) {
		t.Fatal("channel 1 first echo blocked")
	}
	if !b.caretEchoAllowed(&chatMessageEvent{BroadcasterUserId: "2"}) {
		t.Fatal("channel 2 blocked by channel 1's cooldown")
	}
	if b.caretEchoAllowed(&chatMessageEvent{BroadcasterUserId: "1"}) {
		t.Fatal("channel 1 second echo allowed inside the window")
	}
}

func TestCaretEchoOneReplyForManyCaretWords(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, _ := dispatchBot(&calls)

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "a^b ^ hello ^^ x^ ^y world^", false))
	if got := drainSay(b); len(got) != 1 || got[0] != "^" {
		t.Fatalf("queued %q, want exactly one \"^\"", got)
	}
}

func TestCaretEchoIgnoresNoReplyAndSelf(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, chatterID, login string }{
		{"no-reply login", "77", "streamelements"},
		{"no-reply login mixed case", "78", "MooBot"},
		{"bot by id", "1", "someoneelse"},
		{"bot by login", "99", "peanutbudderbot"},
	} {
		var calls atomic.Int32
		b, _ := dispatchBot(&calls)

		b.handleWebhookMessage(chatEvent("m1", c.chatterID, c.login, "^ ^^ wow^", false))
		if got := drainSay(b); len(got) != 0 {
			t.Fatalf("%s: queued %q, want nothing", c.name, got)
		}

		// The ignored message must not have consumed the cooldown: a regular
		// chatter right after (same instant) still gets the echo.
		b.handleWebhookMessage(chatEvent("m2", "42", "alice", "^", false))
		if got := drainSay(b); len(got) != 1 || got[0] != "^" {
			t.Fatalf("%s: follow-up queued %q, want one \"^\" (cooldown consumed?)", c.name, got)
		}
	}
}

// The IRC path (handleMessage) must not echo "^": only the EventSub path does,
// behind caretEchoAllowed. A caret-only PRIVMSG queues nothing on either send
// queue and leaves the echo cooldown untouched.
func TestHandleMessageHasNoCaretEcho(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, _ := dispatchBot(&calls)
	b.ircSayQueue = make(chan ircSayRequest, 16)

	b.handleMessage(twitch.PrivateMessage{
		Channel: "somechannel",
		RoomID:  "9",
		User:    twitch.User{ID: "42", Name: "alice"},
		Message: "^ ^^ wow^",
	})
	if got := drainSay(b); len(got) != 0 {
		t.Fatalf("EventSub say queue got %q from IRC, want nothing", got)
	}
	select {
	case r := <-b.ircSayQueue:
		t.Fatalf("IRC say queue got %q, want nothing", r.message)
	default:
	}
	if !b.caretEchoAllowed(&chatMessageEvent{BroadcasterUserId: "9"}) {
		t.Fatal("IRC message consumed the ^ echo cooldown")
	}
}
