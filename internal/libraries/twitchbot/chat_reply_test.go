package twitchbot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// sinkBot has the say/announce and IRC queues the send sinks write to.
func sinkBot() *Bot {
	return &Bot{
		sayQueue:    make(chan sayRequest, 8),
		ircSayQueue: make(chan ircSayRequest, 8),
	}
}

// TestSendSinksSanitize drives every send sink (say, announce, ircsay) with
// the same inputs and checks what reaches the queue: sanitised text, nothing at
// all for an empty result, and never a line starting with '/' or '.' or longer
// than maxChatReplyLen runes.
func TestSendSinksSanitize(t *testing.T) {
	t.Parallel()
	longMulti := strings.Repeat("é", 1000)
	longWant := strings.Repeat("é", maxChatReplyLen-1) + "…"

	cases := []struct {
		name string
		in   string
		want []string // nil: nothing queued
	}{
		{"normal text", "hello chat, how's it going?", []string{"hello chat, how's it going?"}},
		{"leading slash", "/ban someone", []string{"ban someone"}},
		{"leading dot", ".timeout someone 600", []string{"timeout someone 600"}},
		{"whitespace then slash", "  \t /ban x", []string{"ban x"}},
		{"mixed prefix", "./ /.me hi", []string{"me hi"}},
		{"newline and CR", "line one\r\nline two\n/ban x", []string{"line one line two /ban x"}},
		{"newline then slash", "\n/ban x", []string{"ban x"}},
		{"over 450 runes multibyte", longMulti, []string{longWant}},
		{"empty", "", nil},
		{"whitespace only", " \t\r\n ", nil},
		{"only prefixes", "/./ .", nil},
	}

	sinks := []struct {
		name  string
		send  func(b *Bot, msg string)
		drain func(t *testing.T, b *Bot) []string
	}{
		{"say", func(b *Bot, m string) { b.say("9", m) }, drainSinkSay(false)},
		{"announce", func(b *Bot, m string) { b.announce("9", m) }, drainSinkSay(true)},
		{"ircsay", func(b *Bot, m string) { b.ircsay("somechannel", m) }, drainSinkIRC},
	}

	for _, sink := range sinks {
		for _, c := range cases {
			b := sinkBot()
			sink.send(b, c.in)
			got := sink.drain(t, b)
			if len(got) != len(c.want) {
				t.Fatalf("%s/%s: queued %q, want %q", sink.name, c.name, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("%s/%s: queued %q, want %q", sink.name, c.name, got[i], c.want[i])
				}
				if strings.HasPrefix(got[i], "/") || strings.HasPrefix(got[i], ".") {
					t.Fatalf("%s/%s: queued %q starts with a command prefix", sink.name, c.name, got[i])
				}
				if n := utf8.RuneCountInString(got[i]); n > maxChatReplyLen {
					t.Fatalf("%s/%s: queued %d runes, max %d", sink.name, c.name, n, maxChatReplyLen)
				}
			}
		}
	}
}

func drainSinkSay(announce bool) func(t *testing.T, b *Bot) []string {
	return func(t *testing.T, b *Bot) []string {
		t.Helper()
		var out []string
		for {
			select {
			case r := <-b.sayQueue:
				if r.announce != announce || r.broadcasterId != "9" {
					t.Fatalf("queued %+v, want announce=%v to channel 9", r, announce)
				}
				out = append(out, r.message)
			default:
				return out
			}
		}
	}
}

func drainSinkIRC(t *testing.T, b *Bot) []string {
	t.Helper()
	var out []string
	for {
		select {
		case r := <-b.ircSayQueue:
			if r.channel != "somechannel" {
				t.Fatalf("queued IRC %+v, want channel somechannel", r)
			}
			out = append(out, r.message)
		default:
			return out
		}
	}
}

// TestSanitizeChatReplyIdempotent: the sinks re-sanitise text handlers have
// already sanitised, so a second pass must not change anything.
func TestSanitizeChatReplyIdempotent(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"", "   ", "hello", "/ban x", ". /.x", "a\r\nb\tc", "nul\x00byte", "bad\xffutf8",
		" /ban x", "…", "x  ·  y", strings.Repeat("é", 1000),
		strings.Repeat("a ", 300), strings.Repeat("ab", 224) + " " + strings.Repeat("c", 10),
	}
	for _, in := range inputs {
		once := sanitizeChatReply(in)
		if twice := sanitizeChatReply(once); twice != once {
			t.Errorf("sanitizeChatReply not idempotent for %q: %q -> %q", in, once, twice)
		}
	}
}
