package twitchbot

import (
	"context"
	"log"
	"sync"
	"time"
)

type CounterData struct {
	BreadCounts map[string]int        `json:"bread_counts"`
	PissCounts  map[string]int        `json:"piss_counts"`
	FishCounts  map[string]*FishScore `json:"fish_counts"`
	LastReset   time.Time             `json:"last_reset"`
}

type CounterManager struct {
	data        *CounterData
	storagePath string
	location    *time.Location
	mu          sync.RWMutex
}

func NewCounterManager(storagePath string, location *time.Location) (*CounterManager, error) {
	cm := &CounterManager{
		storagePath: storagePath,
		location:    location,
		data: &CounterData{
			BreadCounts: make(map[string]int),
			PissCounts:  make(map[string]int),
			FishCounts:  make(map[string]*FishScore),
			LastReset:   time.Now().In(location),
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

func (cm *CounterManager) IncrementBread(username string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.data.BreadCounts[username]++
	count := cm.data.BreadCounts[username]

	// Save after each increment
	go cm.Save()

	return count
}

func (cm *CounterManager) IncrementPiss(username string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.data.PissCounts[username]++
	count := cm.data.PissCounts[username]

	// Save after each increment
	go cm.Save()

	return count
}

func (cm *CounterManager) IncrementFish(username string) *FishScore {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Initialize FishScore if user doesn't exist
	if cm.data.FishCounts[username] == nil {
		cm.data.FishCounts[username] = &FishScore{
			Score:       0,
			Fish:        []fish{},
			TrashCaught: 0,
			TimesFished: 0,
		}
	}

	// Now safely increment
	cm.data.FishCounts[username].TimesFished++
	score := cm.data.FishCounts[username]

	// Save after each increment
	go cm.Save()

	return score
}

func (cm *CounterManager) GetTotalBread() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	total := 0
	for _, count := range cm.data.BreadCounts {
		total += count
	}
	return total
}

func (cm *CounterManager) GetTotalPiss() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	total := 0
	for _, count := range cm.data.PissCounts {
		total += count
	}
	return total
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
	cm.data.BreadCounts = make(map[string]int)
	cm.data.PissCounts = make(map[string]int)
	cm.data.LastReset = time.Now().In(cm.location)

	log.Printf("Counters reset at %s", cm.data.LastReset.Format("2006-01-02 15:04:05 MST"))
}
