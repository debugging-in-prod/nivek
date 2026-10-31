package twitchbot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitcheventsub"
)

type fakeInfoHelix struct {
	mu sync.Mutex

	stream    twitcheventsub.StreamInfo
	streamErr error
	channel   twitcheventsub.ChannelInfo
	chanErr   error
	users     map[string]twitcheventsub.UserInfo
	userErr   error
	follow    twitcheventsub.FollowInfo
	followErr error
	block     bool // block until ctx is done

	calls     map[string]int
	lastToken string
}

func (f *fakeInfoHelix) record(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[name]++
}

func (f *fakeInfoHelix) callCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *fakeInfoHelix) wait(ctx context.Context) error {
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (f *fakeInfoHelix) GetStream(ctx context.Context, _ string) (twitcheventsub.StreamInfo, error) {
	f.record("stream")
	if err := f.wait(ctx); err != nil {
		return twitcheventsub.StreamInfo{}, err
	}
	return f.stream, f.streamErr
}

func (f *fakeInfoHelix) GetChannelInfo(ctx context.Context, _ string) (twitcheventsub.ChannelInfo, error) {
	f.record("channel")
	if err := f.wait(ctx); err != nil {
		return twitcheventsub.ChannelInfo{}, err
	}
	return f.channel, f.chanErr
}

func (f *fakeInfoHelix) GetUserByLogin(ctx context.Context, login string) (twitcheventsub.UserInfo, bool, error) {
	f.record("user")
	if err := f.wait(ctx); err != nil {
		return twitcheventsub.UserInfo{}, false, err
	}
	if f.userErr != nil {
		return twitcheventsub.UserInfo{}, false, f.userErr
	}
	u, ok := f.users[login]
	return u, ok, nil
}

func (f *fakeInfoHelix) GetChannelFollow(ctx context.Context, tok, _, _ string) (twitcheventsub.FollowInfo, error) {
	f.record("follow")
	f.mu.Lock()
	f.lastToken = tok
	f.mu.Unlock()
	if err := f.wait(ctx); err != nil {
		return twitcheventsub.FollowInfo{}, err
	}
	return f.follow, f.followErr
}

func newInfoBot(h infoHelix) *Bot {
	cd, _ := newTestCooldowns()
	return &Bot{
		config:    Config{BotUsername: "peanutbudderbot", BotId: "1", BotOAuth: "oauth:usertok"},
		noReply:   buildNoReplySet(nil, "peanutbudderbot"),
		cooldowns: cd,
		sayQueue:  make(chan sayRequest, 32),
		info:      h,
		infoCache: newInfoCache(infoCacheTTL, infoCacheMax),
		// infoSlots nil: lookups run inline so tests are deterministic.
	}
}

func infoMsg(trigger, text string) *chatMessageEvent {
	return &chatMessageEvent{
		BroadcasterUserId:    "9",
		BroadcasterUserLogin: "somechannel",
		BroadcasterUserName:  "SomeChannel",
		ChatterUserId:        "42",
		ChatterUserLogin:     "alice",
		ChatterUserName:      "Alice",
		Message:              chatMessageBody{Text: text},
		matchedTrigger:       trigger,
	}
}

func drainSays(b *Bot) []string {
	var out []string
	for {
		select {
		case r := <-b.sayQueue:
			out = append(out, r.message)
		default:
			return out
		}
	}
}

func onlySay(t *testing.T, b *Bot) string {
	t.Helper()
	says := drainSays(b)
	if len(says) != 1 {
		t.Fatalf("want exactly 1 reply, got %d: %q", len(says), says)
	}
	return says[0]
}

func TestFormatClockDuration(t *testing.T) {
	t.Parallel()
	cases := map[time.Duration]string{
		30 * time.Second:                 "less than a minute",
		time.Minute:                      "1 min",
		45 * time.Minute:                 "45 mins",
		time.Hour:                        "1 hour",
		time.Hour + 2*time.Minute:        "1 hour 2 mins",
		26*time.Hour + 1*time.Minute:     "26 hours 1 min",
		3*time.Hour + 59*time.Second:     "3 hours",
		2*time.Hour + 30*time.Minute + 5: "2 hours 30 mins",
	}
	for d, want := range cases {
		if got := formatClockDuration(d); got != want {
			t.Errorf("formatClockDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestFormatCalendarAge(t *testing.T) {
	t.Parallel()
	d := func(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 0, 0, 0, time.UTC) }
	cases := []struct {
		then, now time.Time
		want      string
	}{
		{d(2025, 6, 18, 12), d(2026, 10, 31, 12), "1 year 4 months 13 days"},
		{d(2024, 10, 31, 12), d(2026, 10, 31, 12), "2 years"},
		{d(2026, 1, 31, 0), d(2026, 3, 1, 0), "29 days"},
		{d(2024, 2, 29, 0), d(2025, 2, 28, 0), "11 months 30 days"},
		{d(2026, 10, 30, 13), d(2026, 10, 31, 12), "23 hours"},
		{d(2026, 9, 30, 13), d(2026, 10, 31, 12), "1 month"},
		{d(2026, 10, 31, 12), d(2026, 10, 31, 12), "less than a minute"},
		{d(2027, 1, 1, 0), d(2026, 10, 31, 12), "less than a minute"}, // clock skew
	}
	for _, tc := range cases {
		if got := formatCalendarAge(tc.then, tc.now); got != tc.want {
			t.Errorf("formatCalendarAge(%v, %v) = %q, want %q", tc.then, tc.now, got, tc.want)
		}
	}
}

func TestSanitizeChatReply(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"/ban someone":        "ban someone",
		"./ /.me hi":          "me hi",
		"  .timeout x":        "timeout x",
		"line1\nline2\r\tend": "line1 line2 end",
		"SomeChannel is live": "SomeChannel is live",
		".timeout x 1":        "timeout x 1",
		"ok\n/ban x":          "ok /ban x",
		"#1: \"/ban x\"":      "#1: \"/ban x\"",
		"a\x00b\x1bc\u0085d":  "a b c d",
		"bad\xffutf8":         "bad utf8",
		"\n\t/ban x":          "ban x",
	}
	for in, want := range cases {
		if got := sanitizeChatReply(in); got != want {
			t.Errorf("sanitizeChatReply(%q) = %q, want %q", in, got, want)
		}
	}
	long := sanitizeChatReply(strings.Repeat("é", 1000))
	if n := utf8.RuneCountInString(long); n != maxChatReplyLen {
		t.Fatalf("long reply has %d runes, want %d", n, maxChatReplyLen)
	}
}

func TestCommandArgAndTarget(t *testing.T) {
	t.Parallel()
	if got := commandArg("hey !FollowAge @Bob extra", "!followage"); got != "@Bob" {
		t.Fatalf("commandArg = %q", got)
	}
	if got := commandArg("!followage", "!followage"); got != "" {
		t.Fatalf("commandArg with no arg = %q", got)
	}
	if got, ok := infoTargetLogin(infoMsg("!followage", "!followage @Bob")); !ok || got != "bob" {
		t.Fatalf("infoTargetLogin = %q, %v", got, ok)
	}
	if got, ok := infoTargetLogin(infoMsg("!followage", "!followage")); !ok || got != "alice" {
		t.Fatalf("infoTargetLogin self = %q, %v", got, ok)
	}
	if _, ok := infoTargetLogin(infoMsg("!followage", "!followage /ban")); ok {
		t.Fatal("a non-login argument must be rejected")
	}
}

func TestUptimeCommand(t *testing.T) {
	t.Parallel()
	h := &fakeInfoHelix{stream: twitcheventsub.StreamInfo{Live: true, StartedAt: time.Now().Add(-(time.Hour + 2*time.Minute + 10*time.Second))}}
	b := newInfoBot(h)
	b.handleUptimeCommand(infoMsg("!uptime", "!uptime"))
	if got := onlySay(t, b); got != "SomeChannel has been live for 1 hour 2 mins" {
		t.Fatalf("got %q", got)
	}

	// Cached: a second call inside the TTL doesn't hit Helix.
	b.handleUptimeCommand(infoMsg("!uptime", "!uptime"))
	drainSays(b)
	if n := h.callCount("stream"); n != 1 {
		t.Fatalf("Helix called %d times, want 1 (cached)", n)
	}

	off := newInfoBot(&fakeInfoHelix{})
	off.handleUptimeCommand(infoMsg("!uptime", "!uptime"))
	if got := onlySay(t, off); got != "SomeChannel is offline" {
		t.Fatalf("got %q", got)
	}
}

func TestInfoErrorsAreNotCachedAndReplyOnce(t *testing.T) {
	t.Parallel()
	h := &fakeInfoHelix{streamErr: errors.New("boom")}
	b := newInfoBot(h)
	b.handleUptimeCommand(infoMsg("!uptime", "!uptime"))
	if got := onlySay(t, b); !strings.Contains(got, "couldn't") {
		t.Fatalf("got %q", got)
	}
	b.handleUptimeCommand(infoMsg("!uptime", "!uptime"))
	drainSays(b)
	if n := h.callCount("stream"); n != 2 {
		t.Fatalf("errors must not be cached; Helix called %d times", n)
	}
}

func TestNilHelixFailsSoft(t *testing.T) {
	t.Parallel()
	b := newInfoBot(nil)
	b.handleGameCommand(infoMsg("!game", "!game"))
	if got := onlySay(t, b); !strings.Contains(got, "couldn't") {
		t.Fatalf("got %q", got)
	}
}

func TestGameAndTitleCommands(t *testing.T) {
	t.Parallel()
	h := &fakeInfoHelix{channel: twitcheventsub.ChannelInfo{GameName: "Dwarf Fortress", Title: "/ban everyone\nlol"}}
	b := newInfoBot(h)
	b.handleGameCommand(infoMsg("!game", "!game"))
	if got := onlySay(t, b); got != "SomeChannel is playing Dwarf Fortress" {
		t.Fatalf("got %q", got)
	}
	b.handleTitleCommand(infoMsg("!title", "!title"))
	if got := onlySay(t, b); got != "SomeChannel's title: /ban everyone lol" {
		t.Fatalf("got %q", got)
	}
	if n := h.callCount("channel"); n != 1 {
		t.Fatalf("!game and !title should share one cached lookup, got %d", n)
	}

	// A channel display name can't turn the reply into a chat command either.
	msg := infoMsg("!title", "!title")
	msg.BroadcasterUserName = ""
	msg.BroadcasterUserLogin = ""
	b2 := newInfoBot(&fakeInfoHelix{channel: twitcheventsub.ChannelInfo{}})
	b2.handleTitleCommand(msg)
	if got := onlySay(t, b2); strings.HasPrefix(got, "/") || strings.HasPrefix(got, ".") {
		t.Fatalf("reply starts with a command char: %q", got)
	}
}

func TestAccountAgeCommand(t *testing.T) {
	t.Parallel()
	created := time.Now().UTC().AddDate(-3, -2, -1).Add(-time.Hour)
	h := &fakeInfoHelix{users: map[string]twitcheventsub.UserInfo{
		"bob":   {ID: "7", Login: "bob", DisplayName: "Bob", CreatedAt: created},
		"alice": {ID: "42", Login: "alice", DisplayName: "Alice", CreatedAt: created},
	}}
	b := newInfoBot(h)
	b.handleAccountAgeCommand(infoMsg("!accountage", "!accountage @Bob"))
	if got := onlySay(t, b); got != "@alice, Bob created their account 3 years 2 months 1 day ago" {
		t.Fatalf("got %q", got)
	}
	b.handleAccountAgeCommand(infoMsg("!accountage", "!accountage"))
	if got := onlySay(t, b); !strings.HasPrefix(got, "@alice, Alice created") {
		t.Fatalf("got %q", got)
	}
	b.handleAccountAgeCommand(infoMsg("!accountage", "!accountage nobody"))
	if got := onlySay(t, b); got != "@alice couldn't find a user named nobody" {
		t.Fatalf("got %q", got)
	}
}

func TestFollowAgeCommand(t *testing.T) {
	t.Parallel()
	followed := time.Now().UTC().AddDate(-1, -4, -13).Add(-time.Hour)
	h := &fakeInfoHelix{
		users:  map[string]twitcheventsub.UserInfo{"bob": {ID: "7", Login: "bob", DisplayName: "Bob"}},
		follow: twitcheventsub.FollowInfo{Following: true, FollowedAt: followed},
	}
	b := newInfoBot(h)

	b.handleFollowAgeCommand(infoMsg("!followage", "!followage bob"))
	if got := onlySay(t, b); got != "@alice, Bob has been following SomeChannel for 1 year 4 months 13 days" {
		t.Fatalf("got %q", got)
	}
	if h.lastToken != "usertok" {
		t.Fatalf("followers lookup must use the bot user token, got %q", h.lastToken)
	}

	// Self lookup uses the chatter id directly (no user lookup).
	before := h.callCount("user")
	b.handleFollowAgeCommand(infoMsg("!followage", "!followage"))
	if got := onlySay(t, b); !strings.HasPrefix(got, "@alice, Alice has been following SomeChannel for") {
		t.Fatalf("got %q", got)
	}
	if h.callCount("user") != before {
		t.Fatal("self !followage should not look the chatter up")
	}

	nf := newInfoBot(&fakeInfoHelix{follow: twitcheventsub.FollowInfo{}})
	nf.handleFollowAgeCommand(infoMsg("!followage", "!followage"))
	if got := onlySay(t, nf); got != "@alice, Alice isn't following SomeChannel" {
		t.Fatalf("got %q", got)
	}
}

func TestFollowAgeUnavailableRepliesOncePerWindow(t *testing.T) {
	t.Parallel()
	h := &fakeInfoHelix{followErr: twitcheventsub.ErrFollowsUnavailable}
	b := newInfoBot(h)
	cd, clk := newTestCooldowns()
	b.cooldowns = cd

	b.handleFollowAgeCommand(infoMsg("!followage", "!followage"))
	if got := onlySay(t, b); got != followageUnavailableMsg {
		t.Fatalf("got %q", got)
	}
	b.handleFollowAgeCommand(infoMsg("!followage", "!followage"))
	if says := drainSays(b); len(says) != 0 {
		t.Fatalf("repeat unavailable notice inside the window: %q", says)
	}
	clk.advance(followageUnavailableWindow)
	b.handleFollowAgeCommand(infoMsg("!followage", "!followage"))
	if got := onlySay(t, b); got != followageUnavailableMsg {
		t.Fatalf("got %q after the window", got)
	}
}

func TestInfoLookupTimesOut(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("waits for infoLookupTimeout")
	}
	b := newInfoBot(&fakeInfoHelix{block: true})
	start := time.Now()
	b.handleUptimeCommand(infoMsg("!uptime", "!uptime"))
	if el := time.Since(start); el > infoLookupTimeout+2*time.Second {
		t.Fatalf("lookup blocked %v", el)
	}
	if got := onlySay(t, b); !strings.Contains(got, "couldn't") {
		t.Fatalf("got %q", got)
	}
}

func TestRunInfoLookupIsAsyncBoundedAndRecovers(t *testing.T) {
	t.Parallel()
	b := &Bot{infoSlots: make(chan struct{}, 1)}
	release := make(chan struct{})
	started := make(chan struct{})
	b.runInfoLookup("x", func(context.Context) { close(started); <-release })
	<-started // first lookup runs off the caller's goroutine and holds the slot

	var ran atomic.Bool
	b.runInfoLookup("x", func(context.Context) { ran.Store(true) }) // no slot: dropped, returns immediately
	close(release)

	done := make(chan struct{})
	deadline := time.After(5 * time.Second)
	for len(b.infoSlots) != 0 {
		select {
		case <-deadline:
			t.Fatal("slot never released")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	b.runInfoLookup("x", func(context.Context) { defer close(done); panic("boom") })
	waitOrFail(t, done, "panicking lookup")
	if ran.Load() {
		t.Fatal("a lookup over the in-flight cap must be dropped")
	}
}

func TestInfoCacheTTLAndCap(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	c := newInfoCache(time.Minute, 2)
	c.now = clk.now
	c.put("a", 1)
	if v, ok := c.get("a"); !ok || v.(int) != 1 {
		t.Fatal("expected hit")
	}
	clk.advance(time.Minute)
	if _, ok := c.get("a"); ok {
		t.Fatal("expected expiry after TTL")
	}
	c.put("a", 1)
	c.put("b", 2)
	c.put("c", 3)
	if n := len(c.entries); n > 2 {
		t.Fatalf("cache exceeded cap: %d", n)
	}
	var nilCache *infoCache
	nilCache.put("x", 1)
	if _, ok := nilCache.get("x"); ok {
		t.Fatal("nil cache must not hit")
	}
}

// The info commands dispatch through the central cooldown gate with their
// seeded 10s cooldown.
func TestInfoCommandsDispatchThroughCooldown(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, clk := dispatchBot(&calls)
	h := &fakeInfoHelix{}
	b.info = h
	b.infoCache = nil // force a Helix call per allowed run
	b.commands["!uptime"] = builtinRegistry["uptime"]
	b.commandCooldownSecs["!uptime"] = 10

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!uptime", false))
	b.handleWebhookMessage(chatEvent("m2", "43", "bob", "!uptime !uptime", false))
	clk.advance(5 * time.Second)
	b.handleWebhookMessage(chatEvent("m3", "44", "carol", "!uptime", false))
	if n := h.callCount("stream"); n != 1 {
		t.Fatalf("!uptime ran %d times inside its 10s cooldown, want 1", n)
	}
	clk.advance(5 * time.Second)
	b.handleWebhookMessage(chatEvent("m4", "44", "carol", "!uptime", false))
	if n := h.callCount("stream"); n != 2 {
		t.Fatalf("!uptime ran %d times, want 2 after the cooldown", n)
	}
	// The bot's own messages never trigger it.
	clk.advance(time.Minute)
	b.handleWebhookMessage(chatEvent("m5", "1", "peanutbudderbot", "!uptime", false))
	if n := h.callCount("stream"); n != 2 {
		t.Fatal("bot must not answer its own !uptime")
	}
}

// Every builtin seeded in init.sql / prod-apply-*.sql must have a compiled
// handler, so a seed row can never reach prod without the code that serves it
// (an unknown handler_key fails bot boot).
func TestSeededBuiltinsHaveHandlers(t *testing.T) {
	t.Parallel()
	files, _ := filepath.Glob("../../../database/*.sql")
	if len(files) == 0 {
		t.Fatal("no database/*.sql files found")
	}
	re := regexp.MustCompile(`\(\s*'(![a-z0-9_]+)'\s*,\s*'builtin'\s*,\s*'([a-z0-9_]+)'`)
	seen := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			seen[m[2]] = true
			if _, ok := builtinRegistry[m[2]]; !ok {
				t.Errorf("%s seeds %s with handler_key %q, which is not in builtinRegistry", filepath.Base(f), m[1], m[2])
			}
		}
	}
	for _, k := range []string{"uptime", "followage", "accountage", "game", "title"} {
		if !seen[k] {
			t.Errorf("handler_key %q is not seeded in database/*.sql", k)
		}
	}
}
