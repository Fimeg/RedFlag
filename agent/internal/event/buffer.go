package event

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/uuid/v5"

	"sync"

	"github.com/Fimeg/RedFlag/agent/internal/models"
)

const (
	defaultMaxBufferSize = 1000 // Max events to buffer
)

// Buffer handles local event buffering for offline resilience
type Buffer struct {
	filePath string
	maxSize  int
	mu       sync.Mutex
}

// NewBuffer creates a new event buffer with the specified file path
func NewBuffer(filePath string) *Buffer {
	return &Buffer{
		filePath: filePath,
		maxSize:  defaultMaxBufferSize,
	}
}

// BufferEvent saves an event to the local buffer file
func (b *Buffer) BufferEvent(event *models.SystemEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Ensure event has an ID
	if event.ID == uuid.Nil {
		return fmt.Errorf("event ID cannot be nil")
	}

	// Create directory if needed
	dir := filepath.Dir(b.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create buffer directory: %w", err)
	}

	// Read existing buffer
	var events []*models.SystemEvent
	if data, err := os.ReadFile(b.filePath); err == nil {
		if err := json.Unmarshal(data, &events); err != nil {
			// If we can't unmarshal, start fresh
			events = []*models.SystemEvent{}
		}
	}

	// Append new event
	events = append(events, event)

	// Keep only last N events if buffer too large (circular buffer)
	if len(events) > b.maxSize {
		events = events[len(events)-b.maxSize:]
	}

	// Write back to file
	data, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	if err := os.WriteFile(b.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write buffer file: %w", err)
	}

	return nil
}

// GetBufferedEvents retrieves and clears the buffer atomically.
func (b *Buffer) GetBufferedEvents() ([]*models.SystemEvent, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var events []*models.SystemEvent
	data, err := os.ReadFile(b.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read buffer file: %w", err)
	}

	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("failed to unmarshal events: %w", err)
	}

	if err := os.Remove(b.filePath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return events, nil
}

// ReadBufferedEvents retrieves buffered events without clearing them.
func (b *Buffer) ReadBufferedEvents() ([]*models.SystemEvent, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Read buffer file
	var events []*models.SystemEvent
	data, err := os.ReadFile(b.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No buffer file means no events
		}
		return nil, fmt.Errorf("failed to read buffer file: %w", err)
	}

	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("failed to unmarshal events: %w", err)
	}

	return events, nil
}

// Clear removes the current buffer file.
func (b *Buffer) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := os.Remove(b.filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// SetMaxSize sets the maximum number of events to buffer
func (b *Buffer) SetMaxSize(size int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxSize = size
}

// GetStats returns buffer statistics
func (b *Buffer) GetStats() (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	data, err := os.ReadFile(b.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var events []*models.SystemEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return 0, err
	}

	return len(events), nil
}
