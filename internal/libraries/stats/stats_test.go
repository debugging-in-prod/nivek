package stats

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 31, 15, 4, 5, 0, time.UTC)

func TestNormalize(t *testing.T) {
	t.Parallel()
	ok := UsageCount{ChannelTwitchID: " 123 ", ChannelLogin: " Foo ", Trigger: " !FISH ", Day: "2026-10-31", Uses: 3}
	if !ok.Normalize(fixedNow) {
		t.Fatal("valid row rejected")
	}
	if ok.ChannelTwitchID != "123" || ok.ChannelLogin != "foo" || ok.Trigger != "!fish" {
		t.Fatalf("not normalized: %+v", ok)
	}

	base := UsageCount{ChannelTwitchID: "1", Trigger: "!x", Day: "2026-10-31", Uses: 1}
	cases := map[string]func(*UsageCount){
		"empty channel":     func(c *UsageCount) { c.ChannelTwitchID = "" },
		"non-numeric id":    func(c *UsageCount) { c.ChannelTwitchID = "12a" },
		"long id":           func(c *UsageCount) { c.ChannelTwitchID = strings.Repeat("1", MaxChannelID+1) },
		"long login":        func(c *UsageCount) { c.ChannelLogin = strings.Repeat("a", MaxLoginLen+1) },
		"empty trigger":     func(c *UsageCount) { c.Trigger = " " },
		"long trigger":      func(c *UsageCount) { c.Trigger = "!" + strings.Repeat("a", MaxTriggerLen) },
		"space in trigger":  func(c *UsageCount) { c.Trigger = "!a b" },
		"zero uses":         func(c *UsageCount) { c.Uses = 0 },
		"negative uses":     func(c *UsageCount) { c.Uses = -1 },
		"huge uses":         func(c *UsageCount) { c.Uses = MaxUsesPerRow + 1 },
		"bad day":           func(c *UsageCount) { c.Day = "31/10/2026" },
		"day too old":       func(c *UsageCount) { c.Day = "2026-10-23" },
		"day in the future": func(c *UsageCount) { c.Day = "2026-11-02" },
	}
	for name, mut := range cases {
		c := base
		mut(&c)
		if c.Normalize(fixedNow) {
			t.Errorf("%s: accepted %+v", name, c)
		}
	}
	for _, d := range []string{"2026-10-24", "2026-11-01"} {
		c := base
		c.Day = d
		if !c.Normalize(fixedNow) {
			t.Errorf("edge day %s rejected", d)
		}
	}
}

func TestMergeUsage(t *testing.T) {
	t.Parallel()
	in := []UsageCount{
		{ChannelTwitchID: "1", Trigger: "!a", Day: "2026-10-31", Uses: 2},
		{ChannelTwitchID: "2", Trigger: "!a", Day: "2026-10-31", Uses: 1},
		{ChannelTwitchID: "1", ChannelLogin: "one", Trigger: "!a", Day: "2026-10-31", Uses: 3},
		{ChannelTwitchID: "1", Trigger: "!a", Day: "2026-10-30", Uses: 4},
		{ChannelTwitchID: "2", Trigger: "!a", Day: "2026-10-31", Uses: MaxUsesPerRow},
	}
	got := MergeUsage(in)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}
	if got[0].Uses != 5 || got[0].ChannelLogin != "one" {
		t.Errorf("merged row = %+v", got[0])
	}
	if got[1].ChannelTwitchID != "2" || got[1].Uses != MaxUsesPerRow {
		t.Errorf("clamped row = %+v", got[1])
	}
	if got[2].Day != "2026-10-30" || got[2].Uses != 4 {
		t.Errorf("distinct day row = %+v", got[2])
	}
}

// maxPlaceholder returns the highest $n in q.
func maxPlaceholder(t *testing.T, q string) int {
	t.Helper()
	n := 0
	for _, m := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(q, -1) {
		v, _ := strconv.Atoi(m[1])
		n = max(n, v)
	}
	return n
}

func TestActiveChatsQuery(t *testing.T) {
	t.Parallel()
	q, args := ActiveChatsQuery([]string{"Home1", "home2"})
	if maxPlaceholder(t, q) != len(args) || len(args) != 2 {
		t.Fatalf("placeholders/args mismatch: %d vs %v\n%s", maxPlaceholder(t, q), args, q)
	}
	if args[0] != "home1" {
		t.Errorf("home logins not lowercased: %v", args)
	}
	for _, want := range []string{"bot_opt_in = TRUE", "UNION", "nivek.users", "($1::text), ($2::text)"} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q:\n%s", want, q)
		}
	}
	q, args = ActiveChatsQuery(nil)
	if len(args) != 0 || strings.Contains(q, "UNION") || strings.Contains(q, "$") {
		t.Errorf("no-home query: %v\n%s", args, q)
	}
}

func TestTopChannelsQuery(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	q, args := TopChannelsQuery(since, []string{"a", "b"}, 5)
	if maxPlaceholder(t, q) != len(args) || len(args) != 4 {
		t.Fatalf("placeholders/args mismatch: %v\n%s", args, q)
	}
	if args[0] != "2026-10-02" || args[1] != "a" || args[3] != 5 {
		t.Errorf("args = %v", args)
	}
	for _, want := range []string{
		"day >= $1::date",
		"u.bot_opt_in IS TRUE OR t.channel_login IN ($2, $3)",
		"ORDER BY t.uses DESC",
		"LIMIT $4",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q:\n%s", want, q)
		}
	}
	// Only public name columns are selected — never ids.
	sel := q[strings.Index(q, "SELECT COALESCE"):strings.Index(q, "FROM totals")]
	if strings.Contains(sel, "twitch_id") || strings.Contains(sel, "email") {
		t.Errorf("select list leaks identifiers: %s", sel)
	}

	q, args = TopChannelsQuery(since, nil, 5)
	if !strings.Contains(q, "OR FALSE") || len(args) != 2 || !strings.Contains(q, "LIMIT $2") {
		t.Errorf("no-home query: %v\n%s", args, q)
	}
}

func TestTopFishersQuery(t *testing.T) {
	t.Parallel()
	q, args := TopFishersQuery(10)
	if len(args) != 1 || args[0] != 10 || maxPlaceholder(t, q) != 1 {
		t.Fatalf("args = %v\n%s", args, q)
	}
	for _, want := range []string{"nivek.fish_score", "GROUP BY chattername", "SUM(score)", "jsonb_array_length(fish)", "LIMIT $1"} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q:\n%s", want, q)
		}
	}
}

func TestUpsertUsageQuery(t *testing.T) {
	t.Parallel()
	rows := []UsageCount{
		{ChannelTwitchID: "1", ChannelLogin: "a", Trigger: "!x", Day: "2026-10-31", Uses: 2},
		{ChannelTwitchID: "2", ChannelLogin: "", Trigger: "!y", Day: "2026-10-31", Uses: 1},
	}
	q, args := UpsertUsageQuery(rows)
	if len(args) != 10 || maxPlaceholder(t, q) != 10 {
		t.Fatalf("args = %v\n%s", args, q)
	}
	if args[5] != "2" || args[9] != 1 {
		t.Errorf("args order = %v", args)
	}
	for _, want := range []string{
		"($1, $2, $3, $4::date, $5)",
		"($6, $7, $8, $9::date, $10)",
		"ON CONFLICT (channel_twitch_id, trigger, day) DO UPDATE",
		"LEAST(cu.uses::bigint + EXCLUDED.uses, 2147483647)",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q:\n%s", want, q)
		}
	}
}

func TestTopChannelsSince(t *testing.T) {
	t.Parallel()
	got := TopChannelsSince(fixedNow)
	want := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC) // 30 days incl. today
	if !got.Equal(want) {
		t.Errorf("since = %s, want %s", got, want)
	}
}

type fakeStore struct {
	mu       sync.Mutex
	calls    atomic.Int32
	err      error
	channels []ChannelUses
	fishers  []Fisher
	upserted []UsageCount
	since    time.Time
}

func (f *fakeStore) ActiveChats(context.Context) (int, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return 7, f.err
}

func (f *fakeStore) TopChannels(_ context.Context, since time.Time, _ int) ([]ChannelUses, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.since = since
	return f.channels, f.err
}

func (f *fakeStore) TopFishers(context.Context, int) ([]Fisher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fishers, f.err
}

func (f *fakeStore) UpsertUsage(_ context.Context, rows []UsageCount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserted = append(f.upserted, rows...)
	return f.err
}

func (f *fakeStore) setErr(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func TestPublicServiceCaches(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		channels: []ChannelUses{{DisplayName: "Foo", Uses: 9}},
		fishers:  []Fisher{{DisplayName: "bar", Score: 50, Catches: 2}},
	}
	svc := NewPublicService(store)
	now := fixedNow
	svc.now = func() time.Time { return now }

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveChats != 7 || got.ActiveChatsDefinition != ActiveChatsDefinition ||
		got.TopChannelsWindowDays != 30 || len(got.TopChannels) != 1 || len(got.TopFishers) != 1 ||
		!got.GeneratedAt.Equal(fixedNow) {
		t.Fatalf("payload = %+v", got)
	}
	if !store.since.Equal(TopChannelsSince(fixedNow)) {
		t.Errorf("since = %s", store.since)
	}

	// Concurrent requests inside the TTL share the cached value.
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Get(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := store.calls.Load(); n != 1 {
		t.Fatalf("store hit %d times inside TTL, want 1", n)
	}

	now = now.Add(CacheTTL)
	if _, err := svc.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := store.calls.Load(); n != 2 {
		t.Fatalf("store hit %d times after TTL, want 2", n)
	}
}

func TestPublicServiceStaleOnErrorAndBackoff(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	svc := NewPublicService(store)
	now := fixedNow
	svc.now = func() time.Time { return now }

	// Cold cache + failure -> error, then backoff without hitting the store.
	store.setErr(errors.New("db down"))
	if _, err := svc.Get(context.Background()); err == nil {
		t.Fatal("want error on cold failure")
	}
	if _, err := svc.Get(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable during backoff, got %v", err)
	}
	if n := store.calls.Load(); n != 1 {
		t.Fatalf("store hit %d times during backoff, want 1", n)
	}

	// Recover, cache, then fail again: the stale payload keeps being served.
	now = now.Add(errorBackoff)
	store.setErr(nil)
	first, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	store.setErr(errors.New("db down again"))
	now = now.Add(CacheTTL)
	got, err := svc.Get(context.Background())
	if err != nil || !got.GeneratedAt.Equal(first.GeneratedAt) {
		t.Fatalf("want stale payload, got %+v err %v", got, err)
	}
	// Empty results serialize as [] not null.
	if got.TopChannels == nil || got.TopFishers == nil {
		t.Errorf("nil slices in payload: %+v", got)
	}
}

func TestIngestUsage(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	n, err := IngestUsage(context.Background(), store, []UsageCount{
		{ChannelTwitchID: "1", ChannelLogin: "A", Trigger: "!Fish", Day: "2026-10-31", Uses: 1},
		{ChannelTwitchID: "1", ChannelLogin: "a", Trigger: "!fish", Day: "2026-10-31", Uses: 2},
		{ChannelTwitchID: "", Trigger: "!fish", Day: "2026-10-31", Uses: 1}, // invalid, skipped
	}, fixedNow)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(store.upserted) != 1 || store.upserted[0].Uses != 3 || store.upserted[0].Trigger != "!fish" {
		t.Fatalf("upserted = %+v", store.upserted)
	}

	// All invalid -> no store call.
	empty := &fakeStore{}
	if n, err := IngestUsage(context.Background(), empty, []UsageCount{{}}, fixedNow); n != 0 || err != nil || empty.upserted != nil {
		t.Fatalf("n=%d err=%v upserted=%v", n, err, empty.upserted)
	}

	failing := &fakeStore{err: errors.New("boom")}
	if _, err := IngestUsage(context.Background(), failing, []UsageCount{
		{ChannelTwitchID: "1", Trigger: "!x", Day: "2026-10-31", Uses: 1},
	}, fixedNow); err == nil {
		t.Fatal("want store error")
	}
}
