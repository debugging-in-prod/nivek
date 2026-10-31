package twitchbot

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/quote"
)

// fakeQuoteAPI implements the quote methods of api.CoreAPIClient in memory.
// Any other method panics via the nil embedded interface.
type fakeQuoteAPI struct {
	api.CoreAPIClient
	mu     sync.Mutex
	quotes map[int]quote.Quote
	next   int
	err    error
	adds   []string
}

func newFakeQuoteAPI() *fakeQuoteAPI {
	return &fakeQuoteAPI{quotes: map[int]quote.Quote{}, next: 1}
}

func (f *fakeQuoteAPI) GetQuote(_ string, number int) (quote.Quote, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return quote.Quote{}, false, f.err
	}
	if number <= 0 {
		for _, q := range f.quotes {
			return q, true, nil
		}
		return quote.Quote{}, false, nil
	}
	q, ok := f.quotes[number]
	return q, ok, nil
}

func (f *fakeQuoteAPI) AddQuote(channel, text, addedBy string) (quote.Quote, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return quote.Quote{}, f.err
	}
	q := quote.Quote{ChannelTwitchId: channel, QuoteNumber: f.next, Text: text, AddedBy: addedBy,
		CreatedAt: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	f.quotes[f.next] = q
	f.next++
	f.adds = append(f.adds, text)
	return q, nil
}

func (f *fakeQuoteAPI) DeleteQuote(_ string, number int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.quotes[number]
	delete(f.quotes, number)
	return ok, nil
}

// quoteBot is dispatchBot with the !quote seed row loaded and a fake core-api.
func quoteBot(t *testing.T) (*Bot, *fakeQuoteAPI, *fakeClock) {
	t.Helper()
	var calls atomic.Int32
	b, clk := dispatchBot(&calls)
	fake := newFakeQuoteAPI()
	b.coreAPI = fake
	b.location = time.UTC
	b.commands[quoteTrigger] = builtinRegistry["quote"]
	b.commandCooldownSecs[quoteTrigger] = 5
	return b, fake, clk
}

func TestQuoteRegistered(t *testing.T) {
	t.Parallel()
	if _, ok := builtinRegistry["quote"]; !ok {
		t.Fatal(`builtinRegistry is missing "quote"; the seed row would fail bot boot`)
	}
}

func TestQuoteFlow(t *testing.T) {
	t.Parallel()
	b, fake, clk := quoteBot(t)

	// Empty channel.
	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!quote", false))
	if got := drainSays(b); len(got) != 1 || got[0] != "@alice, no quote found" {
		t.Fatalf("empty !quote replies = %q", got)
	}

	// Mod adds; text keeps its case and other triggers are not dispatched.
	clk.advance(cooldownFloor)
	b.handleWebhookMessage(chatEvent("m2", "50", "mod", "!quote add He said !fish  Loudly", true))
	if got := drainSays(b); len(got) != 1 || got[0] != "Quote #1 added" {
		t.Fatalf("add replies = %q", got)
	}
	if len(fake.adds) != 1 || fake.adds[0] != "He said !fish Loudly" {
		t.Fatalf("stored = %q", fake.adds)
	}

	// Fetch by number.
	clk.advance(defaultPerUserCooldown)
	b.handleWebhookMessage(chatEvent("m3", "42", "alice", "!quote 1", false))
	want := `#1: "He said !fish Loudly" [2026-10-31]`
	if got := drainSays(b); len(got) != 1 || got[0] != want {
		t.Fatalf("!quote 1 replies = %q, want %q", got, want)
	}

	// Missing number.
	clk.advance(defaultPerUserCooldown)
	b.handleWebhookMessage(chatEvent("m4", "42", "alice", "!quote #7", false))
	if got := drainSays(b); len(got) != 1 || got[0] != "@alice, no quote found" {
		t.Fatalf("!quote #7 replies = %q", got)
	}

	// Mod deletes.
	clk.advance(cooldownFloor)
	b.handleWebhookMessage(chatEvent("m5", "50", "mod", "!quote del 1", true))
	if got := drainSays(b); len(got) != 1 || got[0] != "Quote #1 deleted" {
		t.Fatalf("del replies = %q", got)
	}
	if len(fake.quotes) != 0 {
		t.Fatal("quote not deleted")
	}
}

func TestQuoteAddDelRequireMod(t *testing.T) {
	t.Parallel()
	b, fake, clk := quoteBot(t)

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!quote add sneaky", false))
	clk.advance(defaultPerUserCooldown)
	b.handleWebhookMessage(chatEvent("m2", "42", "alice", "!quote del 1", false))
	if len(fake.adds) != 0 {
		t.Fatal("non-mod was able to add a quote")
	}
	for _, s := range drainSays(b) {
		if !strings.Contains(s, "only a mod") {
			t.Errorf("unexpected reply %q", s)
		}
	}
}

func TestQuoteAddTooLong(t *testing.T) {
	t.Parallel()
	b, fake, _ := quoteBot(t)
	b.handleWebhookMessage(chatEvent("m1", "9", "somechannel", "!quote add "+strings.Repeat("a", quote.MaxTextLen+1), false))
	if len(fake.adds) != 0 {
		t.Fatal("over-long quote was stored")
	}
	if got := drainSays(b); len(got) != 1 || !strings.Contains(got[0], "too long") {
		t.Fatalf("replies = %q", got)
	}
}

func TestQuoteCooldownLimitsReplies(t *testing.T) {
	t.Parallel()
	b, _, _ := quoteBot(t)
	for i, who := range []string{"a", "b", "c", "d"} {
		b.handleWebhookMessage(chatEvent("m"+string(rune('0'+i)), "4"+string(rune('0'+i)), who, "!quote", false))
	}
	if n := len(drainSays(b)); n != 1 {
		t.Fatalf("got %d replies inside the 5s window, want 1", n)
	}
}

func TestQuoteInactiveWithoutSeedRow(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, _ := dispatchBot(&calls)
	b.coreAPI = newFakeQuoteAPI()
	b.handleWebhookMessage(chatEvent("m1", "9", "somechannel", "!quote add hi !fish", false))
	if got := drainSays(b); len(got) != 0 {
		t.Fatalf("!quote replied without a seed row: %q", got)
	}
	// Falls through to the normal scan, so the other trigger still works.
	if calls.Load() != 1 {
		t.Fatalf("!fish ran %d times, want 1", calls.Load())
	}
}

func TestQuoteCoreAPIErrorFailsSoft(t *testing.T) {
	t.Parallel()
	b, fake, clk := quoteBot(t)
	fake.err = errors.New("boom")
	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!quote", false))
	if got := drainSays(b); len(got) != 0 {
		t.Fatalf("read error should be silent, got %q", got)
	}
	clk.advance(cooldownFloor)
	b.handleWebhookMessage(chatEvent("m2", "50", "mod", "!quote add x", true))
	if got := drainSays(b); len(got) != 1 || !strings.Contains(got[0], "couldn't save") {
		t.Fatalf("add error replies = %q", got)
	}
}

func TestQuoteMidMessageIsRandomOnly(t *testing.T) {
	t.Parallel()
	b, fake, _ := quoteBot(t)
	b.handleWebhookMessage(chatEvent("m1", "50", "mod", "lol !quote add sneaky", true))
	if len(fake.adds) != 0 {
		t.Fatal("mid-message !quote must not add")
	}
}

func TestFormatQuoteSanitizes(t *testing.T) {
	t.Parallel()
	game := "Just Chatting"
	q := quote.Quote{QuoteNumber: 12, Text: "line\none\x00", Game: &game,
		CreatedAt: time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC)}
	if got, want := formatQuote(q, time.UTC), `#12: "line one" [Just Chatting, 2026-10-31]`; got != want {
		t.Errorf("formatQuote = %q, want %q", got, want)
	}
	q.Game = nil
	if got, want := formatQuote(q, nil), `#12: "line one" [2026-10-31]`; got != want {
		t.Errorf("formatQuote = %q, want %q", got, want)
	}
}

func TestParseQuoteNumber(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]int{"1": 1, "#12": 12, " 3 ": 3} {
		if n, ok := parseQuoteNumber(in); !ok || n != want {
			t.Errorf("parseQuoteNumber(%q) = (%d, %v)", in, n, ok)
		}
	}
	for _, in := range []string{"", "0", "-1", "abc", "#", "1.5"} {
		if _, ok := parseQuoteNumber(in); ok {
			t.Errorf("parseQuoteNumber(%q) should fail", in)
		}
	}
}
