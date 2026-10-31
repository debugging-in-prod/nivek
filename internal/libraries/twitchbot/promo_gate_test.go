package twitchbot

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/promo"
)

func intPtr(n int) *int { return &n }

// newPromoTestBot builds a minimal Bot for scheduler tests. lookup is the
// Helix fallback (nil = no Helix client).
func newPromoTestBot(lookup func(context.Context, string) (bool, error)) *Bot {
	b := &Bot{
		config:    Config{BotUsername: "peanutbudderbot", BotId: "999"},
		live:      make(map[string]bool),
		chatLines: newChatLineCounter(),
		helixLive: newHelixLiveCache(lookup),
		sayQueue:  make(chan sayRequest, 64),
	}
	b.noReply = buildNoReplySet(nil, b.config.BotUsername)
	return b
}

func drainSayRequests(b *Bot) []sayRequest {
	var out []sayRequest
	for {
		select {
		case r := <-b.sayQueue:
			out = append(out, r)
		default:
			return out
		}
	}
}

func chat(b *Bot, channel, login string, n int) {
	for range n {
		b.recordChatLine(channel, login)
	}
}

var testPromo = promo.Promo{
	Id:              1,
	Channelname:     "streamer",
	BroadcasterId:   "42",
	Message:         "follow me",
	IntervalSeconds: 60,
	Enabled:         true,
}

func TestEffectiveMinChatLines(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   *int
		want int
	}{
		{nil, promo.DefaultMinChatLines},
		{intPtr(0), 0},
		{intPtr(12), 12},
		{intPtr(-3), 0},
		{intPtr(promo.MaxMinChatLines + 50), promo.MaxMinChatLines},
	}
	for _, c := range cases {
		p := promo.Promo{MinChatLines: c.in}
		if got := p.EffectiveMinChatLines(); got != c.want {
			t.Errorf("EffectiveMinChatLines(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPromoPostsOnlyWhenLiveAndChatActive(t *testing.T) {
	t.Parallel()
	b := newPromoTestBot(nil)
	b.setLive("streamer", true)
	st := newPromoSchedule()
	t0 := time.Unix(1_000_000, 0)
	ctx := context.Background()
	promos := []promo.Promo{testPromo}

	// First sighting starts the clock only.
	if n := b.evaluatePromos(ctx, t0, promos, st); n != 0 {
		t.Fatalf("first tick posted %d", n)
	}

	// Interval elapsed but only 4 lines (< default 5): hold.
	chat(b, "streamer", "alice", 4)
	if n := b.evaluatePromos(ctx, t0.Add(61*time.Second), promos, st); n != 0 {
		t.Fatalf("posted with too little chat")
	}

	// Fifth line arrives → posts on the next tick.
	chat(b, "Streamer", "bob", 1)
	if n := b.evaluatePromos(ctx, t0.Add(91*time.Second), promos, st); n != 1 {
		t.Fatalf("expected a post once 5 lines seen")
	}
	says := drainSayRequests(b)
	if len(says) != 1 || says[0].broadcasterId != "42" || says[0].message != "follow me" {
		t.Fatalf("unexpected says: %+v", says)
	}

	// Chat-line window resets after posting: lots of time but no new chat → no post.
	if n := b.evaluatePromos(ctx, t0.Add(10*time.Minute), promos, st); n != 0 {
		t.Fatalf("posted again without new chat")
	}

	// New chat but interval not yet elapsed since the last post → no post.
	chat(b, "streamer", "carol", 10)
	st.lastPosted[testPromo.Id] = t0.Add(10 * time.Minute)
	if n := b.evaluatePromos(ctx, t0.Add(10*time.Minute+30*time.Second), promos, st); n != 0 {
		t.Fatalf("posted before interval elapsed")
	}
	if n := b.evaluatePromos(ctx, t0.Add(11*time.Minute+1*time.Second), promos, st); n != 1 {
		t.Fatalf("expected post after interval + chat")
	}
}

func TestPromoNeverPostsOffline(t *testing.T) {
	t.Parallel()
	b := newPromoTestBot(nil)
	b.setLive("streamer", false)
	st := newPromoSchedule()
	t0 := time.Unix(1_000_000, 0)
	ctx := context.Background()
	promos := []promo.Promo{testPromo}

	for i := range 10 {
		chat(b, "streamer", "alice", 10)
		if n := b.evaluatePromos(ctx, t0.Add(time.Duration(i)*time.Hour), promos, st); n != 0 {
			t.Fatalf("posted to offline channel")
		}
	}

	// Going live doesn't flush a backlog: offline chat doesn't count and the
	// clock restarts, so nothing posts on the first live tick.
	b.setLive("streamer", true)
	if n := b.evaluatePromos(ctx, t0.Add(10*time.Hour), promos, st); n != 0 {
		t.Fatalf("burst-posted on go-live")
	}
}

func TestPromoPermanentChannelNoLongerBypassesLive(t *testing.T) {
	t.Parallel()
	b := newPromoTestBot(nil) // no Helix client, no EventSub state → unknown
	st := newPromoSchedule()
	p := testPromo
	p.Channelname = botCreatorChannel
	t0 := time.Unix(1_000_000, 0)
	ctx := context.Background()

	for i := range 5 {
		chat(b, botCreatorChannel, "alice", 10)
		if n := b.evaluatePromos(ctx, t0.Add(time.Duration(i)*time.Hour), []promo.Promo{p}, st); n != 0 {
			t.Fatalf("posted to channel with unknown live status")
		}
	}
}

func TestPromoMinChatLinesZeroDisablesActivityGate(t *testing.T) {
	t.Parallel()
	b := newPromoTestBot(nil)
	b.setLive("streamer", true)
	st := newPromoSchedule()
	p := testPromo
	p.MinChatLines = intPtr(0)
	t0 := time.Unix(1_000_000, 0)
	ctx := context.Background()

	b.evaluatePromos(ctx, t0, []promo.Promo{p}, st)
	if n := b.evaluatePromos(ctx, t0.Add(61*time.Second), []promo.Promo{p}, st); n != 1 {
		t.Fatalf("min_chat_lines=0 should post on interval alone while live")
	}
}

func TestRecordChatLineExcludesBots(t *testing.T) {
	t.Parallel()
	b := newPromoTestBot(nil)
	chat(b, "streamer", "peanutbudderbot", 3)
	chat(b, "streamer", "PeanutBudderBot", 3)
	chat(b, "streamer", "streamelements", 3)
	chat(b, "streamer", "", 3)
	if got := b.chatLines.get("streamer"); got != 0 {
		t.Fatalf("bot/no-reply lines counted: %d", got)
	}
	chat(b, "streamer", "alice", 2)
	if got := b.chatLines.get("STREAMER"); got != 2 {
		t.Fatalf("got %d lines, want 2", got)
	}
}

func TestIRCChatLinesCountedOnlyWithoutEventSub(t *testing.T) {
	t.Parallel()
	b := newPromoTestBot(nil)
	login := "streamer"
	other := "eventsubchan"
	b.config.Channels = []BotUser{{hasPrivs: true}}
	b.config.Channels[0].TwitchLogin = &other

	msg := func(channel, user, text string) twitch.PrivateMessage {
		return twitch.PrivateMessage{
			Channel: channel,
			User:    twitch.User{Name: user},
			Message: text,
		}
	}
	b.handleMessage(msg(login, "alice", "hi"))
	b.handleMessage(msg(login, "streamelements", "ad"))
	b.handleMessage(msg(login, "peanutbudderbot", "promo"))
	b.handleMessage(msg(other, "alice", "hi"))

	if got := b.chatLines.get(login); got != 1 {
		t.Errorf("IRC-only channel: got %d lines, want 1", got)
	}
	if got := b.chatLines.get(other); got != 0 {
		t.Errorf("EventSub channel must not count IRC lines (double count): got %d", got)
	}
}

func TestHelixLiveFallbackCachedAndFailsClosed(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var live atomic.Bool
	var fail atomic.Bool
	lookup := func(ctx context.Context, id string) (bool, error) {
		calls.Add(1)
		if fail.Load() {
			return true, errors.New("helix down")
		}
		return live.Load(), nil
	}
	b := newPromoTestBot(lookup)
	ctx := context.Background()
	t0 := time.Unix(1_000_000, 0)

	live.Store(true)
	if !b.promoChannelLive(ctx, "unknownchan", "7", t0) {
		t.Fatal("helix says live")
	}
	// Cached within TTL: no second call even though Helix would now say offline.
	live.Store(false)
	if !b.promoChannelLive(ctx, "unknownchan", "7", t0.Add(30*time.Second)) {
		t.Fatal("expected cached live")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	// TTL expired: refreshed.
	if b.promoChannelLive(ctx, "unknownchan", "7", t0.Add(61*time.Second)) {
		t.Fatal("expected refreshed offline")
	}
	// Error → offline, and the failure is cached too.
	fail.Store(true)
	if b.promoChannelLive(ctx, "unknownchan", "7", t0.Add(130*time.Second)) {
		t.Fatal("helix error must read as offline")
	}
	before := calls.Load()
	b.promoChannelLive(ctx, "unknownchan", "7", t0.Add(140*time.Second))
	if calls.Load() != before {
		t.Fatal("error result should be cached")
	}
	// Missing broadcaster id → offline without a call.
	if b.promoChannelLive(ctx, "otherchan", "", t0) {
		t.Fatal("empty id must be offline")
	}

	// EventSub state wins over Helix.
	b.setLive("knownchan", false)
	fail.Store(false)
	live.Store(true)
	if b.promoChannelLive(ctx, "knownchan", "8", t0) {
		t.Fatal("EventSub offline must win over Helix")
	}
}

func TestChatLineCounterConcurrent(t *testing.T) {
	t.Parallel()
	b := newPromoTestBot(nil)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				b.recordChatLine("streamer", "alice")
				_ = b.chatLines.get("streamer")
			}
		}()
	}
	wg.Wait()
	if got := b.chatLines.get("streamer"); got != 2000 {
		t.Fatalf("got %d, want 2000", got)
	}
}

func TestLinesSinceNoUnderflow(t *testing.T) {
	t.Parallel()
	if linesSince(3, 5) != 0 || linesSince(10, 4) != 6 {
		t.Fatal("linesSince wrong")
	}
}

func TestPromoStateForgottenWhenRemoved(t *testing.T) {
	t.Parallel()
	b := newPromoTestBot(nil)
	b.setLive("streamer", true)
	st := newPromoSchedule()
	t0 := time.Unix(1_000_000, 0)
	b.evaluatePromos(context.Background(), t0, []promo.Promo{testPromo}, st)
	b.evaluatePromos(context.Background(), t0.Add(time.Minute), nil, st)
	if len(st.lastPosted) != 0 || len(st.lineMark) != 0 {
		t.Fatalf("state not cleared: %+v", st)
	}
}
