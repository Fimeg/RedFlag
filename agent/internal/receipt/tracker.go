// Package receipt persists the set of command IDs the agent has received from the
// server but not yet seen the server confirm as transitioned to status='received'.
//
// Doctrine: TODO-full-command-lifecycle.md §2. The agent reports these in
// SystemMetrics.ReceivedCommandIDs each check-in. The server flips matching rows from
// 'sent' to 'received' and echoes the confirmed subset back in
// CommandsResponse.ReceiptConfirmedIDs; the agent then drops only the confirmed IDs.
// The unsent ones stay buffered so a dropped check-in doesn't lose receipt confirmation.
//
// This is distinct from pending_acks.json, which tracks command RESULTS awaiting
// server confirmation. Receipt happens BEFORE dispatch; result happens AFTER.
package receipt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// pendingReceipt captures when the agent first received a command — used to age out
// entries the server has somehow forgotten so they don't accumulate forever.
type pendingReceipt struct {
	CommandID  string    `json:"command_id"`
	ReceivedAt time.Time `json:"received_at"`
}

// Tracker is a disk-persisted set of command IDs awaiting server-side receipt confirmation.
type Tracker struct {
	pending  map[string]*pendingReceipt
	mu       sync.RWMutex
	filePath string
	maxAge   time.Duration // discard buffered receipts older than this so a permanent server-side loss doesn't leak forever
}

// NewTracker creates a tracker that persists state under statePath/pending_receipts.json.
func NewTracker(statePath string) *Tracker {
	return &Tracker{
		pending:  make(map[string]*pendingReceipt),
		filePath: filepath.Join(statePath, "pending_receipts.json"),
		maxAge:   24 * time.Hour,
	}
}

// Load restores pending receipts from disk. Missing file is not an error (fresh start).
func (t *Tracker) Load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, err := os.Stat(t.filePath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(t.filePath)
	if err != nil {
		return fmt.Errorf("read pending_receipts: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var pending map[string]*pendingReceipt
	if err := json.Unmarshal(data, &pending); err != nil {
		return fmt.Errorf("parse pending_receipts: %w", err)
	}
	t.pending = pending
	return nil
}

// Save persists the pending set to disk.
func (t *Tracker) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(t.filePath), 0o755); err != nil {
		return fmt.Errorf("create receipt dir: %w", err)
	}

	data, err := json.MarshalIndent(t.pending, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pending_receipts: %w", err)
	}

	if err := os.WriteFile(t.filePath, data, 0o600); err != nil {
		return fmt.Errorf("write pending_receipts: %w", err)
	}
	return nil
}

// Add records that the agent has received a command from the server. Safe to call
// multiple times with the same ID — duplicates are a no-op (idempotent, ETHOS #4).
func (t *Tracker) Add(commandID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.pending[commandID]; exists {
		return
	}
	t.pending[commandID] = &pendingReceipt{
		CommandID:  commandID,
		ReceivedAt: time.Now().UTC(),
	}
}

// GetPending returns the current pending set as a slice (snapshot — safe to mutate).
// Order is not stable.
func (t *Tracker) GetPending() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ids := make([]string, 0, len(t.pending))
	for id := range t.pending {
		ids = append(ids, id)
	}
	return ids
}

// Confirm removes the IDs the server confirmed receipt of. IDs not in the pending
// set are silently ignored — server might confirm an ID we already dropped after
// a successful prior round-trip, or an old buffer survived a crash.
func (t *Tracker) Confirm(commandIDs []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range commandIDs {
		delete(t.pending, id)
	}
}

// DroppedReceipt describes a receipt confirmation abandoned by Cleanup: the agent
// received a command but the server never confirmed receipt, and the entry aged
// out. A drop means the command's lifecycle silently diverged between the two
// sides — the caller journals it inward (ETHOS #1) rather than discarding it.
type DroppedReceipt struct {
	CommandID  string
	AgeSeconds int
}

// Cleanup discards receipts older than maxAge and returns what it dropped so the
// loss can be recorded as history. Bounds disk/memory growth in the pathological
// case where the server forgets a command id permanently. Returns an empty slice
// when nothing aged out.
func (t *Tracker) Cleanup() []DroppedReceipt {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UTC()
	var dropped []DroppedReceipt
	for id, p := range t.pending {
		if age := now.Sub(p.ReceivedAt); age > t.maxAge {
			dropped = append(dropped, DroppedReceipt{CommandID: id, AgeSeconds: int(age.Seconds())})
			delete(t.pending, id)
		}
	}
	return dropped
}

// Len returns the current size of the pending set.
func (t *Tracker) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.pending)
}
