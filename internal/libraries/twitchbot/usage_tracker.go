package twitchbot

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/stats"
)

// Per-channel command-usage analytics (issue #177).
//
// commandAllowed (cooldown.go) calls usage.record once a command has passed
// the cooldown gate, i.e. only for commands that actually dispatch. record is
// an O(1) map bump under a mutex: no I/O, no goroutine, never blocks dispatch.
// A background loop (run) drains the counters every usageFlushInterval and
// ships them to core-api in one HMAC request. Errors are logged and the batch
// is dropped (analytics are best-effort), every flush is panic-recovered, and
// nothing here ever sends chat.
const (
	usageFlushInterval = 60 * time.Second
	usageFlushTimeout  = 10 * time.Second
	// usageMaxKeys bounds memory between flushes. Matches the server's batch
	// cap so a full window always fits in one request. New keys past the cap
	// are dropped (and counted) until the next flush; existing keys still
	// increment.
	usageMaxKeys = stats.MaxUsageBatch
)

type usageKey struct {
	channelID string
	trigger   string
	day       string
}

type usageEntry struct {
	login string
	uses  int
}

// usageTracker accumulates (channel, trigger, UTC day) use counts in memory.
// Safe for concurrent use. A nil *usageTracker ignores everything (bare
// &Bot{} test fixtures).
type usageTracker struct {
	mu      sync.Mutex
	now     func() time.Time
	counts  map[usageKey]*usageEntry
	maxKeys int
	dropped int
}

func newUsageTracker() *usageTracker {
	return &usageTracker{
		now:     time.Now,
		counts:  make(map[usageKey]*usageEntry),
		maxKeys: usageMaxKeys,
	}
}

// record counts one dispatched use of trigger in the channel with Twitch id
// channelID (channelLogin is kept for display when no users row exists).
func (u *usageTracker) record(channelID, channelLogin, trigger string) {
	if u == nil || channelID == "" || trigger == "" {
		return
	}
	trigger = strings.ToLower(trigger)
	if len(trigger) > stats.MaxTriggerLen {
		return
	}
	login := strings.ToLower(channelLogin)
	if len(login) > stats.MaxLoginLen {
		login = ""
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	k := usageKey{channelID: channelID, trigger: trigger, day: u.now().UTC().Format(stats.DayLayout)}
	if e, ok := u.counts[k]; ok {
		if e.uses < stats.MaxUsesPerRow {
			e.uses++
		}
		if e.login == "" {
			e.login = login
		}
		return
	}
	if len(u.counts) >= u.maxKeys {
		u.dropped++
		return
	}
	u.counts[k] = &usageEntry{login: login, uses: 1}
}

// drain returns and clears the accumulated counts, plus how many new keys
// were dropped at the cap since the last drain.
func (u *usageTracker) drain() ([]stats.UsageCount, int) {
	if u == nil {
		return nil, 0
	}
	u.mu.Lock()
	counts := u.counts
	dropped := u.dropped
	u.counts = make(map[usageKey]*usageEntry, len(counts))
	u.dropped = 0
	u.mu.Unlock()

	out := make([]stats.UsageCount, 0, len(counts))
	for k, e := range counts {
		out = append(out, stats.UsageCount{
			ChannelTwitchID: k.channelID,
			ChannelLogin:    e.login,
			Trigger:         k.trigger,
			Day:             k.day,
			Uses:            e.uses,
		})
	}
	return out, dropped
}

// usageSink ships one drained batch (normally CoreAPIClient.RecordCommandUsage).
type usageSink func(ctx context.Context, counts []stats.UsageCount) error

// flush drains and ships the counters once. Never panics: a panicking sink
// is recovered and logged, and the batch is dropped.
func (u *usageTracker) flush(ctx context.Context, sink usageSink) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[USAGE] flush panic recovered: %v", r)
		}
	}()
	counts, dropped := u.drain()
	if dropped > 0 {
		log.Printf("[USAGE] tracker full: dropped %d new keys this window", dropped)
	}
	if len(counts) == 0 || sink == nil {
		return
	}
	fctx, cancel := context.WithTimeout(ctx, usageFlushTimeout)
	defer cancel()
	if err := sink(fctx, counts); err != nil {
		log.Printf("[USAGE] dropped %d usage rows: %s", len(counts), err.Error())
	}
}

// run flushes every interval until ctx is done, then makes one last
// best-effort flush. Runs on its own goroutine.
func (u *usageTracker) run(ctx context.Context, interval time.Duration, sink usageSink) {
	if u == nil {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			u.flush(context.WithoutCancel(ctx), sink)
			return
		case <-t.C:
			u.flush(ctx, sink)
		}
	}
}

// recordCommandUse is the commandAllowed hook. Kept separate so the gate stays
// readable.
func (b *Bot) recordCommandUse(channelID, channelLogin, trigger string) {
	b.usage.record(channelID, channelLogin, trigger)
}

// usageSinkFn adapts the core-api client to a usageSink (nil when the bot has
// no client, e.g. tests). A closure rather than a method value so that any
// panic from the client happens inside flush, where it is recovered.
func (b *Bot) usageSinkFn() usageSink {
	if b.coreAPI == nil {
		return nil
	}
	return func(ctx context.Context, counts []stats.UsageCount) error {
		return b.coreAPI.RecordCommandUsage(ctx, counts)
	}
}
