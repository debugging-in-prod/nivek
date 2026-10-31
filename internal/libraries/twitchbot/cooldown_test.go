package twitchbot

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

// fakeClock is a manually-advanced clock for the cooldown guard.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	f.t = f.t.Add(d)
	f.mu.Unlock()
}

func newTestCooldowns() (*commandCooldowns, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	c := newCommandCooldowns()
	c.now = clk.now
	return c, clk
}

func TestCooldownWindows(t *testing.T) {
	t.Parallel()
	cases := []struct {
		secs         int
		global, user time.Duration
	}{
		{0, defaultGlobalCooldown, defaultPerUserCooldown},
		{-3, defaultGlobalCooldown, defaultPerUserCooldown},
		{1, cooldownFloor, cooldownFloor},
		{2, 2 * time.Second, 2 * time.Second},
		{30, 30 * time.Second, 30 * time.Second},
	}
	for _, c := range cases {
		g, u := cooldownWindows(c.secs)
		if g != c.global || u != c.user {
			t.Errorf("cooldownWindows(%d) = (%v,%v), want (%v,%v)", c.secs, g, u, c.global, c.user)
		}
	}
}

func TestCooldownDefaultGlobalAndPerUser(t *testing.T) {
	t.Parallel()
	c, clk := newTestCooldowns()

	if !c.allow("chan", "!fish", "alice", 0, false) {
		t.Fatal("first use must be allowed")
	}
	if c.allow("chan", "!fish", "bob", 0, false) {
		t.Fatal("global cooldown: another chatter within 5s must be blocked")
	}
	if c.allow("chan", "!FISH", "bob", 0, false) {
		t.Fatal("trigger matching must be case-insensitive")
	}
	// Other commands and other channels are independent.
	if !c.allow("chan", "!bread", "bob", 0, false) {
		t.Fatal("a different command must not share the cooldown")
	}
	if !c.allow("other", "!fish", "alice", 0, false) {
		t.Fatal("a different channel must not share the cooldown")
	}

	clk.advance(defaultGlobalCooldown)
	if !c.allow("chan", "!fish", "bob", 0, false) {
		t.Fatal("global window elapsed: bob must be allowed")
	}
	clk.advance(defaultGlobalCooldown)
	if c.allow("chan", "!fish", "alice", 0, false) {
		t.Fatal("per-user window (15s) still running for alice at 10s")
	}
	clk.advance(defaultPerUserCooldown - 2*defaultGlobalCooldown)
	if !c.allow("chan", "!fish", "alice", 0, false) {
		t.Fatal("per-user window elapsed: alice must be allowed")
	}
}

func TestCooldownBlockedAttemptDoesNotExtend(t *testing.T) {
	t.Parallel()
	c, clk := newTestCooldowns()
	c.allow("chan", "!fish", "alice", 0, false)
	clk.advance(4 * time.Second)
	c.allow("chan", "!fish", "bob", 0, false) // blocked; must not re-stamp
	clk.advance(time.Second)
	if !c.allow("chan", "!fish", "bob", 0, false) {
		t.Fatal("window is measured from the last successful run, not the last attempt")
	}
}

func TestCooldownExplicitSecs(t *testing.T) {
	t.Parallel()
	c, clk := newTestCooldowns()
	c.allow("chan", "!hangman", "alice", 3, false)
	clk.advance(2 * time.Second)
	if c.allow("chan", "!hangman", "bob", 3, false) {
		t.Fatal("explicit 3s cooldown must block at 2s")
	}
	clk.advance(time.Second)
	if !c.allow("chan", "!hangman", "alice", 3, false) {
		t.Fatal("explicit cooldown replaces the 15s per-user default")
	}
}

func TestCooldownPrivilegedBypassKeepsFloor(t *testing.T) {
	t.Parallel()
	c, clk := newTestCooldowns()

	c.allow("chan", "!so", "viewer", 0, false)
	clk.advance(time.Second)
	if c.allow("chan", "!so", "mod", 0, true) {
		t.Fatal("mods must still respect the 2s floor")
	}
	clk.advance(cooldownFloor - time.Second)
	if !c.allow("chan", "!so", "mod", 0, true) {
		t.Fatal("mods bypass the 5s global window once the floor has passed")
	}
	clk.advance(cooldownFloor)
	if !c.allow("chan", "!so", "mod", 0, true) {
		t.Fatal("mods bypass their own per-user window")
	}
	// A mod's run still counts toward the global window for viewers.
	if c.allow("chan", "!so", "viewer2", 0, false) {
		t.Fatal("a viewer right after a mod must hit the global window")
	}
	// Floor holds even with a configured cooldown shorter than the floor.
	clk.advance(cooldownFloor)
	c.allow("chan", "!x", "mod", 1, true)
	clk.advance(time.Second)
	if c.allow("chan", "!x", "mod", 1, true) {
		t.Fatal("cooldown_secs below the floor is clamped up")
	}
}

func TestCooldownNilGuardAllows(t *testing.T) {
	t.Parallel()
	var c *commandCooldowns
	for range 3 {
		if !c.allow("chan", "!fish", "alice", 0, false) {
			t.Fatal("nil guard must allow (fixtures without a guard)")
		}
	}
}

func TestCooldownPrunesExpiredEntries(t *testing.T) {
	t.Parallel()
	c, clk := newTestCooldowns()
	for i := range 100 {
		c.allow("chan", fmt.Sprintf("!c%d", i), fmt.Sprintf("u%d", i), 0, false)
	}
	if g, u := c.size(); g != 100 || u != 100 {
		t.Fatalf("before prune: got (%d,%d) entries, want (100,100)", g, u)
	}
	clk.advance(cooldownPruneInterval)
	c.allow("chan", "!bread", "x", 0, false)
	if g, u := c.size(); g != 1 || u != 1 {
		t.Fatalf("after prune: got (%d,%d) entries, want (1,1) (only the fresh use)", g, u)
	}
}

func TestCooldownHardCap(t *testing.T) {
	t.Parallel()
	c, _ := newTestCooldowns()
	c.maxSize = 50
	for i := range 500 {
		c.allow("chan", fmt.Sprintf("!c%d", i), fmt.Sprintf("u%d", i), 0, false)
	}
	if g, u := c.size(); g > 50 || u > 50 {
		t.Fatalf("cap exceeded: (%d,%d) > 50", g, u)
	}
}

func TestCooldownConcurrentSingleWinner(t *testing.T) {
	t.Parallel()
	c := newCommandCooldowns() // real clock; all calls land well within 5s
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.allow("chan", "!fish", fmt.Sprintf("u%d", i), 0, false) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("exactly one concurrent caller may win the global window, got %d", wins.Load())
	}
}

func TestBuildCommandCooldowns(t *testing.T) {
	t.Parallel()
	got := buildCommandCooldowns([]commands.Commands{
		{Trigger: "!fish", Kind: "builtin", Enabled: true, CooldownSecs: 30},
		{Trigger: "!bread", Kind: "builtin", Enabled: true},
		{Trigger: "!off", Kind: "builtin", Enabled: false, CooldownSecs: 9},
		{Trigger: "!custom", Kind: "custom", Enabled: true, CooldownSecs: 9},
	})
	if len(got) != 1 || got["!fish"] != 30 {
		t.Fatalf("buildCommandCooldowns = %v, want map[!fish:30]", got)
	}
}

// dispatchBot is a minimal Bot for driving handleWebhookMessage.
func dispatchBot(calls *atomic.Int32) (*Bot, *fakeClock) {
	cd, clk := newTestCooldowns()
	b := &Bot{
		config:  Config{BotUsername: "peanutbudderbot", BotId: "1"},
		noReply: buildNoReplySet(nil, "peanutbudderbot"),
		commands: map[string]commandHandler{
			"!fish": func(*Bot, *chatMessageEvent) { calls.Add(1) },
		},
		commandCooldownSecs: map[string]int{},
		cooldowns:           cd,
		customCommands: map[string]map[string]commands.Commands{
			"somechannel": {"!hi": {Trigger: "!hi", ResponseTmpl: ptr("hello"), MinRole: "everyone"}},
		},
		sayQueue: make(chan sayRequest, 16),
	}
	return b, clk
}

func chatEvent(id, chatterID, login, text string, mod bool) *EventSubSubscriptionResponse {
	badges := "[]"
	if mod {
		badges = `[{"set_id":"moderator","id":"1","info":""}]`
	}
	ev := fmt.Sprintf(`{"broadcaster_user_login":"somechannel","broadcaster_user_id":"9",`+
		`"chatter_user_id":%q,"chatter_user_login":%q,"chatter_user_name":%q,"message_id":%q,`+
		`"badges":%s,"message":{"text":%q}}`, chatterID, login, login, id, badges, text)
	return &EventSubSubscriptionResponse{Event: []byte(ev)}
}

func TestDispatchEnforcesCooldownOnBuiltins(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, clk := dispatchBot(&calls)

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!fish", false))
	b.handleWebhookMessage(chatEvent("m2", "43", "bob", "!fish", false))
	b.handleWebhookMessage(chatEvent("m3", "42", "alice", "!fish !fish !fish", false))
	if calls.Load() != 1 {
		t.Fatalf("handler ran %d times inside the cooldown, want 1", calls.Load())
	}
	clk.advance(defaultGlobalCooldown)
	b.handleWebhookMessage(chatEvent("m4", "43", "bob", "!fish", false))
	if calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2 after the global window", calls.Load())
	}
}

func TestDispatchEnforcesCooldownOnCustomCommands(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, _ := dispatchBot(&calls)

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!hi", false))
	b.handleWebhookMessage(chatEvent("m2", "43", "bob", "!hi", false))
	b.handleWebhookMessage(chatEvent("m3", "44", "carol", "!hi !hi", false))
	if n := len(b.sayQueue); n != 1 {
		t.Fatalf("custom command replied %d times inside the cooldown, want 1", n)
	}
}

func TestDispatchModBypassesGlobalWithFloor(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, clk := dispatchBot(&calls)

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!fish", false))
	b.handleWebhookMessage(chatEvent("m2", "50", "mod", "!fish", true)) // within floor
	clk.advance(cooldownFloor)
	b.handleWebhookMessage(chatEvent("m3", "50", "mod", "!fish", true))
	if calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2 (mod blocked only by the floor)", calls.Load())
	}
}

func TestIRCBanishCooldown(t *testing.T) {
	t.Parallel()
	cd, _ := newTestCooldowns()
	b := &Bot{cooldowns: cd}
	msg := twitch.PrivateMessage{RoomID: "9", User: twitch.User{ID: "50", IsMod: true}}
	if !b.ircCommandAllowed(msg, banishTrigger) {
		t.Fatal("first IRC !banish must be allowed")
	}
	if b.ircCommandAllowed(msg, strings.ToUpper(banishTrigger)) {
		t.Fatal("a repeat IRC !banish inside the floor must be blocked")
	}
}
