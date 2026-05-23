package df

import (
	"sync"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer"
)

// snapshotStore holds the most recently received MapSnapshot in memory.
// Phase 1 is read-mostly (the GET endpoint reads, executor push will write
// once Phase 2 lands). The store is intentionally single-snapshot — no
// history, no persistence — so memory footprint stays bounded.
type snapshotStore struct {
	mu      sync.RWMutex
	current *overseer.MapSnapshot
}

// store is the package-level singleton. Initialized with the Phase 1 fixture
// in init() so the GET endpoint has something to return before any real
// executor push has happened.
var store = &snapshotStore{}

func init() {
	fixture := buildFixtureSnapshot()
	store.set(&fixture)
}

func (s *snapshotStore) get() *overseer.MapSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *snapshotStore) set(snap *overseer.MapSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = snap
}
