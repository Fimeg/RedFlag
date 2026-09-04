package acknowledgment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PendingResult represents a command result awaiting acknowledgment
type PendingResult struct {
	CommandID  string    `json:"command_id"`
	SentAt     time.Time `json:"sent_at"`
	RetryCount int       `json:"retry_count"`
}

// Tracker manages pending acknowledgments for command results
type Tracker struct {
	pending  map[string]*PendingResult
	mu       sync.RWMutex
	filePath string
	maxAge   time.Duration // Max time to keep pending (default 24h)
	maxRetries int         // Max retries before giving up (default 10)
}

// NewTracker creates a new acknowledgment tracker
func NewTracker(statePath string) *Tracker {
	return &Tracker{
		pending:    make(map[string]*PendingResult),
		filePath:   filepath.Join(statePath, "pending_acks.json"),
		maxAge:     24 * time.Hour,
		maxRetries: 10,
	}
}

// Load restores pending acknowledgments from disk
func (t *Tracker) Load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If file doesn't exist, that's fine (fresh start)
	if _, err := os.Stat(t.filePath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(t.filePath)
	if err != nil {
		return fmt.Errorf("failed to read pending acks: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file
	}

	var pending map[string]*PendingResult
	if err := json.Unmarshal(data, &pending); err != nil {
		return fmt.Errorf("failed to parse pending acks: %w", err)
	}

	t.pending = pending
	return nil
}

// Save persists pending acknowledgments to disk
func (t *Tracker) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(t.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create ack directory: %w", err)
	}

	data, err := json.MarshalIndent(t.pending, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal pending acks: %w", err)
	}

	if err := os.WriteFile(t.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write pending acks: %w", err)
	}

	return nil
}

// Add marks a command result as pending acknowledgment
func (t *Tracker) Add(commandID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.pending[commandID] = &PendingResult{
		CommandID:  commandID,
		SentAt:     time.Now().UTC(),
		RetryCount: 0,
	}
}

// Acknowledge marks command results as acknowledged and removes them
func (t *Tracker) Acknowledge(commandIDs []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, id := range commandIDs {
		delete(t.pending, id)
	}
}

// GetPending returns list of command IDs awaiting acknowledgment
func (t *Tracker) GetPending() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ids := make([]string, 0, len(t.pending))
	for id := range t.pending {
		ids = append(ids, id)
	}
	return ids
}

// IncrementRetry increments retry count for a command
func (t *Tracker) IncrementRetry(commandID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if result, exists := t.pending[commandID]; exists {
		result.RetryCount++
	}
}

// DroppedResult describes a pending result-ack that Cleanup abandoned. The agent
// only ever redelivers the command ID (not the result payload), so a result the
// server never recorded can sit here unrecoverable until it ages out — and a drop
// is the silent loss of an auditable event. Cleanup returns these so the caller
// journals each one inward (ETHOS #1) rather than discarding it to /dev/null.
type DroppedResult struct {
	CommandID  string
	Reason     string // "max_age" | "max_retries"
	RetryCount int
	AgeSeconds int
}

// Cleanup removes old or over-retried pending results and returns what it dropped
// so the loss can be recorded as history. Returns an empty slice when nothing aged out.
func (t *Tracker) Cleanup() []DroppedResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UTC()
	var dropped []DroppedResult

	for id, result := range t.pending {
		age := now.Sub(result.SentAt)
		switch {
		case age > t.maxAge:
			dropped = append(dropped, DroppedResult{CommandID: id, Reason: "max_age", RetryCount: result.RetryCount, AgeSeconds: int(age.Seconds())})
			delete(t.pending, id)
		case result.RetryCount >= t.maxRetries:
			dropped = append(dropped, DroppedResult{CommandID: id, Reason: "max_retries", RetryCount: result.RetryCount, AgeSeconds: int(age.Seconds())})
			delete(t.pending, id)
		}
	}

	return dropped
}

// Stats returns statistics about pending acknowledgments
func (t *Tracker) Stats() Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := Stats{
		Total: len(t.pending),
	}

	now := time.Now().UTC()
	for _, result := range t.pending {
		age := now.Sub(result.SentAt)

		if age > 1*time.Hour {
			stats.OlderThan1Hour++
		}
		if result.RetryCount > 0 {
			stats.WithRetries++
		}
		if result.RetryCount >= 5 {
			stats.HighRetries++
		}
	}

	return stats
}

// Stats holds statistics about pending acknowledgments
type Stats struct {
	Total          int
	OlderThan1Hour int
	WithRetries    int
	HighRetries    int
}
