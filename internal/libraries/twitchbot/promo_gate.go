package twitchbot

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/promo"
)

// Promo gate (issue #163): a recurring message only posts when
//
//  1. its channel is currently live — EventSub stream.online/offline state
//     (b.live) when we have it, otherwise a Helix streams lookup cached for
//     promoLiveCacheTTL. Unknown or errored status counts as OFFLINE; and
//  2. at least the promo's MinChatLines non-bot chat lines were seen in the
//     channel since the promo last posted (or since its clock started).
//
// Both checks fail closed: anything uncertain means "don't post".

const (
	// promoLiveCacheTTL is how long a Helix live lookup (or its failure) is
	// trusted for a channel we have no EventSub state for.
	promoLiveCacheTTL = 60 * time.Second
	// promoLiveLookupTimeout bounds one Helix lookup so a slow Twitch API can't
	// stall the scheduler.
	promoLiveLookupTimeout = 5 * time.Second
)

// chatLineCounter is a per-channel monotonic count of non-bot chat lines.
// Promos snapshot it when they post and compare later, so one counter serves
// any number of promos per channel. Safe for concurrent use.
type chatLineCounter struct {
	mu     sync.Mutex
	counts map[string]uint64
}

func newChatLineCounter() *chatLineCounter {
	return &chatLineCounter{counts: make(map[string]uint64)}
}

func (c *chatLineCounter) inc(login string) {
	if c == nil || login == "" {
		return
	}
	login = strings.ToLower(login)
	c.mu.Lock()
	c.counts[login]++
	c.mu.Unlock()
}

func (c *chatLineCounter) get(login string) uint64 {
	if c == nil {
		return 0
	}
	login = strings.ToLower(login)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[login]
}

// liveCacheEntry is one cached Helix live lookup. Errors are cached as
// live=false so a failing API is retried at most once per TTL per channel.
type liveCacheEntry struct {
	live bool
	at   time.Time
}

// helixLiveCache memoizes Helix "is this channel live" lookups. Safe for
// concurrent use.
type helixLiveCache struct {
	mu      sync.Mutex
	entries map[string]liveCacheEntry
	ttl     time.Duration
	// lookup is the Helix call; nil means no Helix client → always offline.
	lookup func(ctx context.Context, broadcasterID string) (bool, error)
}

func newHelixLiveCache(lookup func(context.Context, string) (bool, error)) *helixLiveCache {
	return &helixLiveCache{
		entries: make(map[string]liveCacheEntry),
		ttl:     promoLiveCacheTTL,
		lookup:  lookup,
	}
}

// isLive returns the cached or freshly looked-up live status for a broadcaster.
// Any error, missing id, or missing client yields false (offline).
func (c *helixLiveCache) isLive(ctx context.Context, broadcasterID string, now time.Time) bool {
	if c == nil || c.lookup == nil || broadcasterID == "" {
		return false
	}

	c.mu.Lock()
	if e, ok := c.entries[broadcasterID]; ok && now.Sub(e.at) < c.ttl {
		c.mu.Unlock()
		return e.live
	}
	c.mu.Unlock()

	lctx, cancel := context.WithTimeout(ctx, promoLiveLookupTimeout)
	live, err := c.lookup(lctx, broadcasterID)
	cancel()
	if err != nil {
		log.Printf("[PROMO] helix live lookup for %s failed, treating as offline: %v", broadcasterID, err)
		live = false
	}

	c.mu.Lock()
	c.entries[broadcasterID] = liveCacheEntry{live: live, at: now}
	c.mu.Unlock()
	return live
}

// recordChatLine counts one chat line toward the promo activity gate. Callers
// must already have filtered out the bot itself and no-reply logins; this
// re-checks the no-reply list defensively so a bot line can never count.
func (b *Bot) recordChatLine(channel, chatterLogin string) {
	if b.chatLines == nil || chatterLogin == "" || b.isNoReplyLogin(chatterLogin) {
		return
	}
	b.chatLines.inc(channel)
}

// promoChannelLive reports whether a channel is live for promo purposes.
// EventSub/boot state wins when present; otherwise fall back to a cached Helix
// lookup. Unknown/errored → false. Unlike the pre-#163 behavior, permanent
// home channels get no bypass: timers never post to an offline chat.
func (b *Bot) promoChannelLive(ctx context.Context, login, broadcasterID string, now time.Time) bool {
	login = strings.ToLower(login)
	b.liveMu.Lock()
	live, known := b.live[login]
	b.liveMu.Unlock()
	if known {
		return live
	}
	return b.helixLive.isLive(ctx, broadcasterID, now)
}

// promoSchedule is the scheduler's per-promo state. Owned by the scheduler
// goroutine only (not shared), so it needs no lock.
type promoSchedule struct {
	// lastPosted is when each promo (by id) last posted or had its clock reset.
	lastPosted map[int]time.Time
	// lineMark is the channel's chat-line count at that same moment.
	lineMark map[int]uint64
}

func newPromoSchedule() *promoSchedule {
	return &promoSchedule{
		lastPosted: make(map[int]time.Time),
		lineMark:   make(map[int]uint64),
	}
}

// linesSince returns chat lines seen since mark, tolerating a counter that
// went backwards (never expected, but must not underflow into "huge").
func linesSince(total, mark uint64) uint64 {
	if total < mark {
		return 0
	}
	return total - mark
}

// evaluatePromos runs one scheduler tick over the active promo set: posts
// each promo that is due, live, and has seen enough chat, and resets clocks
// for offline channels. Returns the number of promos posted.
func (b *Bot) evaluatePromos(ctx context.Context, now time.Time, promos []promo.Promo, st *promoSchedule) int {
	posted := 0
	seen := make(map[int]bool, len(promos))
	for _, p := range promos {
		seen[p.Id] = true
		lines := b.chatLines.get(p.Channelname)

		if !b.promoChannelLive(ctx, p.Channelname, p.BroadcasterId, now) {
			// Clock (and chat-line window) only run while live: reset each
			// offline tick so the first post lands one full interval after going
			// live, never a backlog the instant the stream starts.
			st.lastPosted[p.Id] = now
			st.lineMark[p.Id] = lines
			continue
		}

		last, known := st.lastPosted[p.Id]
		if !known {
			// First sighting (just created, or bot just started): start the
			// clock and the chat-line window now, don't post yet.
			st.lastPosted[p.Id] = now
			st.lineMark[p.Id] = lines
			continue
		}

		interval := p.IntervalSeconds
		if interval < promo.MinIntervalSeconds {
			interval = promo.MinIntervalSeconds
		}
		if now.Sub(last) < time.Duration(interval)*time.Second {
			continue
		}

		// Interval elapsed but chat is quiet: hold (don't reset the clock) so it
		// posts as soon as enough people have talked.
		if linesSince(lines, st.lineMark[p.Id]) < uint64(p.EffectiveMinChatLines()) {
			continue
		}

		b.say(p.BroadcasterId, p.Message)
		st.lastPosted[p.Id] = now
		st.lineMark[p.Id] = lines
		posted++
		log.Printf("[PROMO] posted #%d to %s", p.Id, p.Channelname)
	}

	// Forget promos that were deleted or disabled so their state can't linger.
	for id := range st.lastPosted {
		if !seen[id] {
			delete(st.lastPosted, id)
			delete(st.lineMark, id)
		}
	}
	return posted
}
