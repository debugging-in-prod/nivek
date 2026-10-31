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
	// queryTimeout bounds one refresh (all three queries).
	queryTimeout = 5 * time.Second
)

// ErrUnavailable is returned when stats were never computed successfully and
// the last attempt failed.
var ErrUnavailable = errors.New("stats unavailable")

// PublicService serves the cached public stats payload. Safe for concurrent use.
type PublicService struct {
	store Store
	now   func() time.Time

	mu        sync.Mutex
	cached    *PublicStats
	fetchedAt time.Time
	failedAt  time.Time
}

// NewPublicService builds the cached public-stats reader over store.
func NewPublicService(store Store) *PublicService {
	return &PublicService{store: store, now: time.Now}
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
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), queryTimeout)
	defer cancel()

	active, err := s.store.ActiveChats(ctx)
	if err != nil {
		return PublicStats{}, err
	}
	since := TopChannelsSince(now)
	channels, err := s.store.TopChannels(ctx, since, TopChannelsLimit)
	if err != nil {
		return PublicStats{}, err
	}
	fishers, err := s.store.TopFishers(ctx, TopFishersLimit)
	if err != nil {
		return PublicStats{}, err
	}
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
		TopChannelsWindowDays: TopChannelsWindowDays,
		TopFishers:            fishers,
		GeneratedAt:           now.UTC().Truncate(time.Second),
	}, nil
}

// TopChannelsSince is the first UTC day of the top_channels window.
func TopChannelsSince(now time.Time) time.Time {
	today := now.UTC().Truncate(24 * time.Hour)
	return today.AddDate(0, 0, -(TopChannelsWindowDays - 1))
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
