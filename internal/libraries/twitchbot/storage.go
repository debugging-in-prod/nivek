package twitchbot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func (cm *CounterManager) Save() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(cm.storagePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Marshal data to JSON
	data, err := json.MarshalIndent(cm.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Write to file atomically (write to temp, then rename)
	tempPath := cm.storagePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, cm.storagePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (cm *CounterManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Read file
	data, err := os.ReadFile(cm.storagePath)
	if err != nil {
		return fmt.Errorf("failed to read storage file: %w", err)
	}

	// Unmarshal JSON
	if err := json.Unmarshal(data, cm.data); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	// Initialize maps if nil (shouldn't happen, but defensive)
	if cm.data.BreadCounts == nil {
		cm.data.BreadCounts = make(map[string]int)
	}
	if cm.data.PissCounts == nil {
		cm.data.PissCounts = make(map[string]int)
	}

	return nil
}
