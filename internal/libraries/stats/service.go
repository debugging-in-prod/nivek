package stats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// CacheTTL is how long a computed PublicStats is served from memory.
	CacheTTL = 60 * time.Second
	// errorBackoff stops a failing database from being hit on every request:
	// after a failed refresh, no new attempt is made for this long.
	errorBackoff = 10 * time.Second
	// queryTimeout bounds one refresh's queries (all three). Display-name
	// resolution has its own, separate nameLookupTimeout.
	queryTimeout = 5 * time.Second
)

// ErrUnavailable is returned when stats were never computed successfully and
// the last attempt failed.
var ErrUnavailable = errors.New("stats unavailable")

// PublicService serves the cached public stats payload. Safe for concurrent use.
type PublicService struct {
	store   Store
	now     func() time.Time
	names   *nameCache
	nameErr func(error)

	mu        sync.Mutex
	cached    *PublicStats
	fetchedAt time.Time
	failedAt  time.Time
}

// NewPublicService builds the cached public-stats reader over store.
//
// names resolves top_fishers logins to Twitch display names; nil disables
// resolution and logins are shown. Resolution never fails the payload: on any
// error the affected fishers keep their login and nameErr (optional) is
// called so the transport layer can log it.
func NewPublicService(store Store, names DisplayNameResolver, nameErr func(error)) *PublicService {
	s := &PublicService{store: store, now: time.Now, nameErr: nameErr}
	if names != nil {
		s.names = newNameCache(names, func() time.Time { return s.now() })
	}
	return s
}

// Get returns the public stats, recomputing them at most once per CacheTTL.
// The mutex is held across the refresh so concurrent requests on a cold or
// expired cache share one round of queries instead of stampeding the
// database. If a refresh fails, the previous payload (if any) keeps being
// served.
func (s *PublicService) Get(ctx context.Context) (PublicStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if s.cached != nil && now.Sub(s.fetchedAt) < CacheTTL {
		return *s.cached, nil
	}
	if !s.failedAt.IsZero() && now.Sub(s.failedAt) < errorBackoff {
		if s.cached != nil {
			return *s.cached, nil
		}
		return PublicStats{}, ErrUnavailable
	}

	fresh, err := s.compute(ctx, now)
	if err != nil {
		s.failedAt = now
		if s.cached != nil {
			return *s.cached, nil
		}
		return PublicStats{}, err
	}
	s.cached = &fresh
	s.fetchedAt = now
	s.failedAt = time.Time{}
	return fresh, nil
}

func (s *PublicService) compute(ctx context.Context, now time.Time) (PublicStats, error) {
	// Detached from the caller: one client hanging up must not fail the
	// refresh that every waiting request shares.
	base := context.WithoutCancel(ctx)
	qctx, cancel := context.WithTimeout(base, queryTimeout)
	defer cancel()

	active, err := s.store.ActiveChats(qctx)
	if err != nil {
		return PublicStats{}, err
	}
	channels, err := s.store.TopChannels(qctx, TopChannelsLimit)
	if err != nil {
		return PublicStats{}, err
	}
	fishers, err := s.store.TopFishers(qctx, TopFishersLimit)
	if err != nil {
		return PublicStats{}, err
	}
	fishers = s.resolveFisherNames(base, fishers)
	if channels == nil {
		channels = []ChannelUses{}
	}
	if fishers == nil {
		fishers = []Fisher{}
	}
	if len(channels) > TopChannelsLimit || len(fishers) > TopFishersLimit {
		return PublicStats{}, fmt.Errorf("stats store returned more rows than requested")
	}
	return PublicStats{
		ActiveChats:           active,
		ActiveChatsDefinition: ActiveChatsDefinition,
		TopChannels:           channels,
		TopChannelsWindow:     TopChannelsWindow,
		TopChannelsWindowDays: TopChannelsWindowDays,
		TopFishers:            fishers,
		GeneratedAt:           now.UTC().Truncate(time.Second),
	}, nil
}

// resolveFisherNames returns a copy of fishers with each login replaced by
// the Twitch display name. Bounded by nameLookupTimeout; failures keep the
// login.
func (s *PublicService) resolveFisherNames(ctx context.Context, in []Fisher) []Fisher {
	if s.names == nil || len(in) == 0 {
		return in
	}
	fishers := append([]Fisher(nil), in...)
	logins := make([]string, len(fishers))
	for i, f := range fishers {
		logins[i] = f.DisplayName
	}
	names, err := s.names.resolve(ctx, logins)
	if err != nil && s.nameErr != nil {
		s.nameErr(err)
	}
	for i := range fishers {
		if n := names[fishers[i].DisplayName]; n != "" {
			fishers[i].DisplayName = n
		}
	}
	return fishers
}

// IngestUsage validates, normalizes, and merges a bot usage batch, then
// upserts it. Invalid rows are skipped, not fatal. Returns how many rows were
// accepted.
func IngestUsage(ctx context.Context, store Store, batch []UsageCount, now time.Time) (int, error) {
	valid := make([]UsageCount, 0, len(batch))
	for _, c := range batch {
		if c.Normalize(now) {
			valid = append(valid, c)
		}
	}
	valid = MergeUsage(valid)
	if len(valid) == 0 {
		return 0, nil
	}
	if err := store.UpsertUsage(ctx, valid); err != nil {
		return 0, err
	}
	return len(valid), nil
}
