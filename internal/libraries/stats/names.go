package stats

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DisplayNameResolver maps lowercase Twitch logins to Twitch display names.
// Logins it can't resolve are absent from the map. Implementations may return
// a partial map together with an error. The stats service depends only on
// this interface; in production it is the twitcheventsub Helix client, whose
// DisplayNames batches Get Users calls (stats can't import twitcheventsub:
// that would be an import cycle through api).
type DisplayNameResolver interface {
	DisplayNames(ctx context.Context, logins []string) (map[string]string, error)
}

const (
	// nameCacheTTL is how long a resolved display name is reused. Display
	// names change rarely, and a stale one is still the right person.
	nameCacheTTL = 24 * time.Hour
	// nameMissTTL is how long a login Twitch didn't return (renamed, banned,
	// deleted) is served as the login before it is looked up again.
	nameMissTTL = time.Hour
	// nameCacheMax bounds the cache. The leaderboard has TopFishersLimit
	// entries, so this only fills through leaderboard churn.
	nameCacheMax = 1000
	// nameLookupTimeout bounds one resolution round. On timeout the logins
	// are shown instead, so the endpoint is never slowed beyond this.
	nameLookupTimeout = 3 * time.Second
	// maxDisplayNameLen rejects implausible display names from upstream.
	maxDisplayNameLen = 64
)

// twitchLogin matches a valid Twitch login. Anything else (legacy rows,
// junk) is shown as stored and never sent upstream.
var twitchLogin = regexp.MustCompile(`^[a-z0-9_]{1,25}$`)

type nameEntry struct {
	name    string
	expires time.Time
}

// nameCache resolves display names through a DisplayNameResolver with a
// bounded TTL cache. It never fails: any login it can't resolve maps to
// itself. Safe for concurrent use.
type nameCache struct {
	resolver DisplayNameResolver
	now      func() time.Time
	timeout  time.Duration

	mu      sync.Mutex // guards entries
	entries map[string]nameEntry
}

func newNameCache(r DisplayNameResolver, now func() time.Time) *nameCache {
	return &nameCache{resolver: r, now: now, timeout: nameLookupTimeout, entries: map[string]nameEntry{}}
}

// resolve returns a display name for every login in logins, falling back to
// the login itself. A non-nil error means the upstream lookup failed (the
// result is still complete, with fallbacks); callers only log it.
func (c *nameCache) resolve(ctx context.Context, logins []string) (map[string]string, error) {
	out := make(map[string]string, len(logins))
	now := c.now()

	var missing []string
	seen := map[string]bool{}
	c.mu.Lock()
	for _, l := range logins {
		key := strings.ToLower(l)
		if seen[key] {
			continue
		}
		seen[key] = true
		out[l] = l
		if !twitchLogin.MatchString(key) {
			continue
		}
		if e, ok := c.entries[key]; ok && now.Before(e.expires) {
			out[l] = e.name
			continue
		}
		missing = append(missing, key)
	}
	c.mu.Unlock()
	if len(missing) == 0 || c.resolver == nil {
		return out, nil
	}

	// Detached from the caller so one hung-up client can't cancel a lookup
	// that the whole cached payload shares; bounded by c.timeout.
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
	defer cancel()
	got, err := c.resolver.DisplayNames(lctx, missing)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)
	for _, key := range missing {
		name, ok := got[key]
		name = strings.TrimSpace(name)
		switch {
		case ok && name != "" && len(name) <= maxDisplayNameLen:
			c.entries[key] = nameEntry{name: name, expires: now.Add(nameCacheTTL)}
		case err == nil:
			// Twitch answered and doesn't know this login: cache the miss
			// briefly so it isn't re-requested every refresh.
			c.entries[key] = nameEntry{name: key, expires: now.Add(nameMissTTL)}
		default:
			// Lookup failed: fall back without caching, retry next refresh.
			continue
		}
	}
	for _, l := range logins {
		if e, ok := c.entries[strings.ToLower(l)]; ok && now.Before(e.expires) {
			out[l] = e.name
		}
	}
	return out, err
}

// pruneLocked drops expired entries, then arbitrary ones, until there is room
// for a full leaderboard's worth of new entries.
func (c *nameCache) pruneLocked(now time.Time) {
	if len(c.entries) < nameCacheMax-TopFishersLimit {
		return
	}
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	for k := range c.entries {
		if len(c.entries) < nameCacheMax-TopFishersLimit {
			break
		}
		delete(c.entries, k)
	}
}
