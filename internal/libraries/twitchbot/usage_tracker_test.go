package twitchbot

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/stats"
)

func newTestUsage(start time.Time) (*usageTracker, *fakeClock) {
	clk := &fakeClock{t: start}
	u := newUsageTracker()
	u.now = clk.now
	return u, clk
}

func sortedUsage(c []stats.UsageCount) []stats.UsageCount {
	sort.Slice(c, func(i, j int) bool {
		if c[i].ChannelTwitchID != c[j].ChannelTwitchID {
			return c[i].ChannelTwitchID < c[j].ChannelTwitchID
		}
		if c[i].Trigger != c[j].Trigger {
			return c[i].Trigger < c[j].Trigger
		}
		return c[i].Day < c[j].Day
	})
	return c
}

func TestUsageTrackerRecordAndDrain(t *testing.T) {
	t.Parallel()
	u, clk := newTestUsage(time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC))
	u.record("9", "SomeChannel", "!Fish")
	u.record("9", "somechannel", "!fish")
	u.record("9", "somechannel", "!lurk")
	u.record("10", "other", "!fish")
	u.record("", "x", "!fish")   // ignored: no channel
	u.record("9", "x", "")       // ignored: no trigger
	clk.advance(2 * time.Minute) // crosses UTC midnight -> new day row
	u.record("9", "somechannel", "!fish")

	got, dropped := u.drain()
	got = sortedUsage(got)
	if dropped != 0 || len(got) != 4 {
		t.Fatalf("drain = %+v dropped=%d", got, dropped)
	}
	want := []stats.UsageCount{
		{ChannelTwitchID: "10", ChannelLogin: "other", Trigger: "!fish", Day: "2026-10-31", Uses: 1},
		{ChannelTwitchID: "9", ChannelLogin: "somechannel", Trigger: "!fish", Day: "2026-10-31", Uses: 2},
		{ChannelTwitchID: "9", ChannelLogin: "somechannel", Trigger: "!fish", Day: "2026-11-01", Uses: 1},
		{ChannelTwitchID: "9", ChannelLogin: "somechannel", Trigger: "!lurk", Day: "2026-10-31", Uses: 1},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if again, _ := u.drain(); len(again) != 0 {
		t.Errorf("drain did not clear: %+v", again)
	}
}

func TestUsageTrackerBoundedMemory(t *testing.T) {
	t.Parallel()
	u, _ := newTestUsage(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	u.maxKeys = 3
	for _, trig := range []string{"!a", "!b", "!c", "!d", "!e"} {
		u.record("9", "c", trig)
	}
	u.record("9", "c", "!a") // existing key still counts at the cap
	got, dropped := u.drain()
	if len(got) != 3 || dropped != 2 {
		t.Fatalf("len=%d dropped=%d, want 3/2", len(got), dropped)
	}
	for _, c := range got {
		if c.Trigger == "!a" && c.Uses != 2 {
			t.Errorf("!a uses = %d, want 2", c.Uses)
		}
	}
	// Oversized trigger is ignored, not truncated into a different key.
	u.record("9", "c", "!"+string(make([]byte, stats.MaxTriggerLen)))
	if got, _ := u.drain(); len(got) != 0 {
		t.Errorf("long trigger recorded: %+v", got)
	}
}

func TestUsageTrackerNilSafe(t *testing.T) {
	t.Parallel()
	var u *usageTracker
	u.record("9", "c", "!fish")
	if got, d := u.drain(); got != nil || d != 0 {
		t.Fatal("nil tracker returned data")
	}
	u.flush(context.Background(), nil)
	u.run(context.Background(), time.Millisecond, nil) // returns immediately

	b := &Bot{} // bare fixture: no tracker, no cooldowns
	if !b.commandAllowed(&chatMessageEvent{BroadcasterUserId: "9"}, "!fish", 0) {
		t.Fatal("bare bot must allow")
	}
}

func TestUsageTrackerConcurrentRecord(t *testing.T) {
	t.Parallel()
	u, _ := newTestUsage(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	var total atomic.Int64
	var wg sync.WaitGroup
	stop := make(chan struct{})
	// A concurrent drainer, like the flush loop racing dispatch workers.
	drained := make(chan int64)
	go func() {
		var sum int64
		for {
			select {
			case <-stop:
				got, _ := u.drain()
				for _, c := range got {
					sum += int64(c.Uses)
				}
				drained <- sum
				return
			default:
				got, _ := u.drain()
				for _, c := range got {
					sum += int64(c.Uses)
				}
			}
		}
	}()
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 500 {
				u.record("9", "c", []string{"!a", "!b"}[(w+i)%2])
				total.Add(1)
			}
		}()
	}
	wg.Wait()
	close(stop)
	if sum := <-drained; sum != total.Load() {
		t.Fatalf("drained %d uses, recorded %d", sum, total.Load())
	}
}

func TestUsageFlushErrorsAndPanicsAreDropped(t *testing.T) {
	t.Parallel()
	u, _ := newTestUsage(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))

	u.record("9", "c", "!fish")
	var calls int
	u.flush(context.Background(), func(context.Context, []stats.UsageCount) error {
		calls++
		return errors.New("core-api down")
	})
	if got, _ := u.drain(); len(got) != 0 {
		t.Fatalf("failed batch was retained: %+v", got)
	}

	u.record("9", "c", "!fish")
	u.flush(context.Background(), func(context.Context, []stats.UsageCount) error {
		calls++
		panic("boom")
	})
	if calls != 2 {
		t.Fatalf("sink calls = %d", calls)
	}

	// Empty window: no request at all.
	u.flush(context.Background(), func(context.Context, []stats.UsageCount) error {
		t.Error("sink called with nothing to send")
		return nil
	})
}

func TestUsageRunFlushesOnTickAndShutdown(t *testing.T) {
	t.Parallel()
	u := newUsageTracker()
	got := make(chan []stats.UsageCount, 4)
	sink := func(_ context.Context, c []stats.UsageCount) error {
		got <- c
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		u.run(ctx, 10*time.Millisecond, sink)
		close(done)
	}()

	u.record("9", "c", "!fish")
	select {
	case c := <-got:
		if len(c) != 1 || c[0].Uses != 1 {
			t.Fatalf("tick flush = %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no flush on tick")
	}

	u.record("9", "c", "!lurk")
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}
	// Either the last tick or the shutdown flush shipped !lurk.
	var seen bool
	for len(got) > 0 {
		for _, c := range <-got {
			seen = seen || c.Trigger == "!lurk"
		}
	}
	if !seen {
		t.Fatal("pending counts not flushed on shutdown")
	}
}

// Only commands that pass the cooldown gate are counted.
func TestDispatchRecordsUsageOnlyWhenAllowed(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, clk := dispatchBot(&calls)
	b.usage, _ = newTestUsage(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!fish", false))
	b.handleWebhookMessage(chatEvent("m2", "43", "bob", "!fish", false)) // global cooldown
	b.handleWebhookMessage(chatEvent("m3", "44", "carol", "hello chat", false))
	b.handleWebhookMessage(chatEvent("m4", "45", "dave", "!hi", false)) // custom command
	clk.advance(defaultGlobalCooldown)
	b.handleWebhookMessage(chatEvent("m5", "43", "bob", "!fish", false))
	// The bot's own messages never dispatch, so never count.
	b.handleWebhookMessage(chatEvent("m6", "1", "peanutbudderbot", "!fish", false))

	got, _ := b.usage.drain()
	counts := map[string]int{}
	for _, c := range got {
		if c.ChannelTwitchID != "9" || c.ChannelLogin != "somechannel" {
			t.Errorf("row channel = %+v", c)
		}
		counts[c.Trigger] += c.Uses
	}
	if counts["!fish"] != int(calls.Load()) || counts["!fish"] != 2 || counts["!hi"] != 1 || len(counts) != 2 {
		t.Fatalf("usage = %v, handler calls = %d", counts, calls.Load())
	}
	// Recording must never queue chat.
	if n := len(b.sayQueue); n != 1 { // just the one !hi reply
		t.Errorf("sayQueue = %d, want 1", n)
	}
}

func TestIRCDispatchRecordsUsage(t *testing.T) {
	t.Parallel()
	cd, _ := newTestCooldowns()
	b := &Bot{cooldowns: cd, usage: newUsageTracker()}
	msg := twitch.PrivateMessage{Channel: "chan", RoomID: "9", User: twitch.User{ID: "50", IsMod: true}}
	b.ircCommandAllowed(msg, banishTrigger)
	b.ircCommandAllowed(msg, banishTrigger) // blocked by the floor: not counted
	got, _ := b.usage.drain()
	if len(got) != 1 || got[0].Uses != 1 || got[0].ChannelLogin != "chan" || got[0].Trigger != banishTrigger {
		t.Fatalf("usage = %+v", got)
	}
}

// usageAPI captures RecordCommandUsage calls; every other method is the nil
// embedded interface.
type usageAPI struct {
	api.CoreAPIClient
	got chan []stats.UsageCount
}

func (f *usageAPI) RecordCommandUsage(_ context.Context, c []stats.UsageCount) error {
	f.got <- c
	return nil
}

func TestUsageSinkUsesCoreAPI(t *testing.T) {
	t.Parallel()
	if (&Bot{}).usageSinkFn() != nil {
		t.Fatal("no client -> nil sink")
	}
	fake := &usageAPI{got: make(chan []stats.UsageCount, 1)}
	b := &Bot{coreAPI: fake, usage: newUsageTracker()}
	b.usage.record("9", "c", "!fish")
	b.usage.flush(context.Background(), b.usageSinkFn())
	if c := <-fake.got; len(c) != 1 {
		t.Fatalf("sent %+v", c)
	}

	// A client whose RecordCommandUsage panics (nil embedded interface) is
	// recovered inside flush.
	type bare struct{ api.CoreAPIClient }
	b2 := &Bot{coreAPI: bare{}, usage: newUsageTracker()}
	b2.usage.record("9", "c", "!fish")
	b2.usage.flush(context.Background(), b2.usageSinkFn())
}
