package twitchbot

import (
	"context"
	"log"
	"sync"
	"time"
)

type CounterData struct {
	LastReset time.Time `json:"last_reset"`
}

type CounterManager struct {
	data        *CounterData
	storagePath string
	location    *time.Location
	mu          sync.RWMutex
}

// @TODO::remove this file
// bread and piss counters need to be migrated to postgres

func NewCounterManager(storagePath string, location *time.Location) (*CounterManager, error) {
	cm := &CounterManager{
		storagePath: storagePath,
		location:    location,
		data: &CounterData{
			LastReset: time.Now().In(location),
		},
	}

	// Try to load existing data
	if err := cm.Load(); err != nil {
		log.Printf("No existing data found, starting fresh: %v", err)
		// Initialize with current time
		cm.data.LastReset = time.Now().In(location)
	}

	// Check if we need to reset (in case bot was offline during midnight)
	if cm.shouldReset() {
		log.Println("Counters were stale, resetting...")
		cm.reset()
	}

	return cm, nil
}

func (cm *CounterManager) StartResetTimer(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Println("Reset timer started, checking every minute for midnight")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cm.shouldReset() {
				log.Println("Midnight reached! Resetting all counters...")
				cm.reset()
				if err := cm.Save(); err != nil {
					log.Printf("Error saving after reset: %v", err)
				}
			}
		}
	}
}

func (cm *CounterManager) shouldReset() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	now := time.Now().In(cm.location)
	lastReset := cm.data.LastReset.In(cm.location)

	// Check if we've crossed midnight
	lastResetDay := lastReset.Truncate(24 * time.Hour)
	currentDay := now.Truncate(24 * time.Hour)

	return currentDay.After(lastResetDay)
}

func (cm *CounterManager) reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Clear all counters
	// @TODO::add db call to clear the counters on the db
	cm.data.LastReset = time.Now().In(cm.location)

	log.Printf("Counters reset at %s", cm.data.LastReset.Format("2006-01-02 15:04:05 MST"))
}
