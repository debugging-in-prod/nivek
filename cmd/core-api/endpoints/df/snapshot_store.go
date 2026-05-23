package df

import (
	"sync"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer"
)

// snapshotStore holds the most recently received MapSnapshot in memory.
// The store starts empty — the GET endpoint returns 404 until the DFHost
// pusher delivers its first POST. Single-snapshot (no history, no
// persistence) so memory footprint stays bounded.
type snapshotStore struct {
	mu      sync.RWMutex
	current *overseer.MapSnapshot
}

var store = &snapshotStore{}

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
