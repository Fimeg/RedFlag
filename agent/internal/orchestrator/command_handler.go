package orchestrator

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/crypto"
	"github.com/Fimeg/RedFlag/agent/internal/logging"
	"github.com/gofrs/uuid/v5"
)

// ConfirmedTracker is a disk-persisted set of command IDs the server has confirmed
// as completed (via ReportLog). Used to distinguish between "command re-sent by
// server before agent confirmed" vs "command truly duplicate."
type ConfirmedTracker struct {
	confirmed map[string]time.Time
	mu        sync.RWMutex
	filePath  string
}

// NewConfirmedTracker creates a tracker that persists state under statePath/confirmed_completed.json.
func NewConfirmedTracker(statePath string) *ConfirmedTracker {
	return &ConfirmedTracker{
		confirmed: make(map[string]time.Time),
		filePath:  filepath.Join(statePath, "confirmed_completed.json"),
	}
}

// Load restores confirmed completions from disk. Missing file is not an error (fresh start).
func (t *ConfirmedTracker) Load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, err := os.Stat(t.filePath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(t.filePath)
	if err != nil {
		return fmt.Errorf("read confirmed_completed: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var loaded map[string]time.Time
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("parse confirmed_completed: %w", err)
	}
	t.confirmed = loaded
	return nil
}

// Save persists the confirmed set to disk.
func (t *ConfirmedTracker) Save() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(t.filePath), 0o755); err != nil {
		return fmt.Errorf("create confirmed dir: %w", err)
	}

	data, err := json.MarshalIndent(t.confirmed, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal confirmed_completed: %w", err)
	}

	if err := os.WriteFile(t.filePath, data, 0o600); err != nil {
		return fmt.Errorf("write confirmed_completed: %w", err)
	}
	return nil
}

// Add records that the server confirmed a command as completed. Safe to call
// multiple times with the same ID — duplicates are a no-op (idempotent, ETHOS #4).
func (t *ConfirmedTracker) Add(commandID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.confirmed[commandID]; exists {
		return
	}
	t.confirmed[commandID] = time.Now().UTC()
}

// GetPending returns the current confirmed set as a slice (snapshot — safe to mutate).
func (t *ConfirmedTracker) GetPending() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ids := make([]string, 0, len(t.confirmed))
	for id := range t.confirmed {
		ids = append(ids, id)
	}
	return ids
}

// Confirm removes the IDs the server confirmed. IDs not in the confirmed
// set are silently ignored — server might confirm an ID we already dropped after
// a successful prior round-trip.
func (t *ConfirmedTracker) Confirm(commandIDs []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range commandIDs {
		delete(t.confirmed, id)
	}
}

// Len returns the current size of the confirmed set.
func (t *ConfirmedTracker) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.confirmed)
}

const (
	// keyRefreshInterval is how often the agent proactively re-checks the server's primary key
	keyRefreshInterval = 6 * time.Hour
	// commandMaxAge is the maximum age of a signed command (F-4 fix: reduced from 24h to 4h)
	commandMaxAge = 4 * time.Hour
	// commandClockSkew is the allowed future clock skew for signed commands
	commandClockSkew = 5 * time.Minute
)

// CommandHandler handles command processing with signature verification
type CommandHandler struct {
	verifier       *crypto.CommandVerifier
	securityLogger *logging.SecurityLogger
	keyCache       map[string]ed25519.PublicKey // key_id -> public key
	keyCacheMu     sync.RWMutex
	executedIDs    map[string]time.Time // cmd UUID -> execution time (F-2 fix: dedup)
	executedIDsMu  sync.Mutex
	executedIDsPath string              // Migration 033 §5: disk-persisted dedup
	// confirmedCompleted tracks command IDs the server has confirmed as completed
	// (via ReceiptConfirmedIDs/AcknowledgedIDs in response). Commands in this set
	// are NOT rejected as duplicates even if they appear in the executed set,
	// because the server may not have processed the ReportLog() yet.
	confirmedCompleted map[string]time.Time // cmd UUID -> confirmation time
	confirmedCompletedMu sync.Mutex
	confirmedCompletedPath string // disk-persisted confirmation set
	lastKeyRefresh time.Time
	logger         *log.Logger
}

// CommandSigningConfig holds configuration for command signing
type CommandSigningConfig struct {
	Enabled         bool   `json:"enabled" env:"REDFLAG_AGENT_COMMAND_SIGNING_ENABLED" default:"true"`
	EnforcementMode string `json:"enforcement_mode" env:"REDFLAG_AGENT_COMMAND_ENFORCEMENT_MODE" default:"strict"`
}

// NewCommandHandler creates a new command handler. stateDir is the agent's persistent
// state directory; passing an empty string disables disk-persisted dedup (test path).
func NewCommandHandler(cfg *config.Config, stateDir string, securityLogger *logging.SecurityLogger, logger *log.Logger) (*CommandHandler, error) {
	handler := &CommandHandler{
		securityLogger:          securityLogger,
		logger:                  logger,
		verifier:                crypto.NewCommandVerifier(),
		keyCache:                make(map[string]ed25519.PublicKey),
		executedIDs:             make(map[string]time.Time),
		confirmedCompleted:      make(map[string]time.Time),
		confirmedCompletedMu:    sync.Mutex{},
		confirmedCompletedPath:  "",
	}
	if stateDir != "" {
		handler.executedIDsPath = filepath.Join(stateDir, "executed_commands.json")
		handler.confirmedCompletedPath = filepath.Join(stateDir, "confirmed_completed.json")
	}

	// Migration 033 §5: rebuild dedup set from disk so a restart can't re-execute a
	// command issued within commandMaxAge. Missing file is a fresh start, not an error.
	if handler.executedIDsPath != "" {
		if err := handler.loadExecutedIDs(); err != nil {
			logger.Printf("[WARNING] [agent] [cmd_handler] load_executed_ids_failed path=%q error=%v", handler.executedIDsPath, err)
		} else {
			logger.Printf("[INFO] [agent] [cmd_handler] executed_ids_loaded count=%d path=%q", len(handler.executedIDs), handler.executedIDsPath)
			// Trim anything that's already aged out — keeps the file from growing forever.
			handler.CleanupExecutedIDs()
		}
	}

	// Load confirmed completion tracker (server-acked commands)
	if handler.confirmedCompletedPath != "" {
		if err := handler.loadConfirmedCompleted(); err != nil {
			logger.Printf("[WARNING] [agent] [cmd_handler] load_confirmed_completed_failed path=%q error=%v", handler.confirmedCompletedPath, err)
		} else {
			logger.Printf("[INFO] [agent] [cmd_handler] confirmed_completed_loaded count=%d", len(handler.confirmedCompleted))
		}
	}

	// Pre-load cached public key if command signing is enabled
	if cfg.CommandSigning.Enabled {
		if pubKey, err := crypto.LoadCachedPublicKey(); err == nil {
			// Store under empty key_id for backward-compat lookup
			handler.keyCacheMu.Lock()
			handler.keyCache[""] = pubKey
			handler.keyCacheMu.Unlock()
			logger.Printf("[INFO] [agent] [cmd_handler] primary_public_key_loaded")
		} else {
			logger.Printf("[WARNING] [agent] [cmd_handler] primary_key_not_cached error=\"%v\"", err)
		}
	}

	return handler, nil
}

// loadExecutedIDs restores the dedup set from disk. Caller holds no lock.
func (h *CommandHandler) loadExecutedIDs() error {
	if _, err := os.Stat(h.executedIDsPath); os.IsNotExist(err) {
		return nil
	}
	data, err := os.ReadFile(h.executedIDsPath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	var loaded map[string]time.Time
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	h.executedIDsMu.Lock()
	h.executedIDs = loaded
	h.executedIDsMu.Unlock()
	return nil
}

// loadConfirmedCompleted restores the server-confirmed completion set from disk.
// Called once during construction. Caller holds no lock.
func (h *CommandHandler) loadConfirmedCompleted() error {
	if _, err := os.Stat(h.confirmedCompletedPath); os.IsNotExist(err) {
		return nil
	}
	data, err := os.ReadFile(h.confirmedCompletedPath)
	if err != nil {
		return fmt.Errorf("read confirmed_completed: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	var loaded map[string]time.Time
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("parse confirmed_completed: %w", err)
	}
	h.confirmedCompletedMu.Lock()
	h.confirmedCompleted = loaded
	h.confirmedCompletedMu.Unlock()
	return nil
}

// saveConfirmedCompleted persists the server-confirmed completion set to disk.
// Caller MUST hold confirmedCompletedMu.
func (h *CommandHandler) saveConfirmedCompletedLocked() error {
	if h.confirmedCompletedPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(h.confirmedCompletedPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(h.confirmedCompleted, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := h.confirmedCompletedPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := os.Rename(tmp, h.confirmedCompletedPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// saveExecutedIDs persists the current dedup set. Caller MUST hold executedIDsMu.
// Errors are logged by the caller — best-effort persistence; the in-memory set is
// the authoritative source within a single process lifetime.
func (h *CommandHandler) saveExecutedIDsLocked() error {
	if h.executedIDsPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(h.executedIDsPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(h.executedIDs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := h.executedIDsPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := os.Rename(tmp, h.executedIDsPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// getKeyForCommand returns the appropriate public key for verifying a command.
// Uses key_id-aware lookup with lazy fetch for unknown keys.
func (h *CommandHandler) getKeyForCommand(cmd client.Command, serverURL string) (ed25519.PublicKey, error) {
	keyID := cmd.KeyID

	// Check in-memory cache first
	h.keyCacheMu.RLock()
	if key, ok := h.keyCache[keyID]; ok {
		h.keyCacheMu.RUnlock()
		return key, nil
	}
	h.keyCacheMu.RUnlock()

	// Not in memory — check disk cache via CheckKeyRotation
	key, isNew, err := h.verifier.CheckKeyRotation(keyID, serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve key %q: %w", keyID, err)
	}

	if isNew {
		h.logger.Printf("[INFO] [agent] [cmd_handler] new_signing_key_cached key_id=%q", keyID)
		if h.securityLogger != nil {
			h.securityLogger.LogKeyRotationDetected(keyID)
		}
	}

	// Store in memory cache
	h.keyCacheMu.Lock()
	h.keyCache[keyID] = key
	h.keyCacheMu.Unlock()

	return key, nil
}

// ProcessCommand processes a command with signature verification
func (h *CommandHandler) ProcessCommand(cmd client.Command, cfg *config.Config, agentID uuid.UUID) error {
	// F-2 fix: Check deduplication BEFORE verification.
	// Persistence is implemented (see loadExecutedIDs/saveExecutedIDsLocked, Migration 033 §5):
	// the set survives restarts within commandMaxAge.
	h.executedIDsMu.Lock()
	if execTime, found := h.executedIDs[cmd.ID]; found {
		h.executedIDsMu.Unlock()
		h.logger.Printf("[WARNING] [agent] [cmd_handler] duplicate_command_rejected command_id=%q already_executed_at=%v", cmd.ID, execTime)
		if h.securityLogger != nil {
			h.securityLogger.LogCommandVerificationFailure(cmd.ID, fmt.Sprintf("duplicate command rejected, already executed at %v", execTime))
		}
		return fmt.Errorf("duplicate command %s rejected, already executed at %v", cmd.ID, execTime)
	}
	h.executedIDsMu.Unlock()

	signingCfg := cfg.CommandSigning

	if !signingCfg.Enabled {
		if cmd.Signature != "" {
			h.logger.Printf("[INFO] [agent] [cmd_handler] command_has_signature_but_signing_disabled command_id=%q", cmd.ID)
		}
		h.markExecuted(cmd.ID)
		return nil
	}

	// Resolve the correct public key for this command
	pubKey, err := h.getKeyForCommand(cmd, cfg.ServerURL)
	if err != nil {
		h.logger.Printf("[ERROR] [agent] [cmd_handler] key_resolution_failed command_id=%q error=%q", cmd.ID, err)
		if h.securityLogger != nil {
			h.securityLogger.LogCommandVerificationFailure(cmd.ID, "key resolution failed: "+err.Error())
		}
		if signingCfg.EnforcementMode == "strict" {
			return fmt.Errorf("command verification failed: %w", err)
		}
		return nil
	}

	verifyFunc := func() error {
		if cmd.SignedAt != nil {
			// New format: timestamp-aware verification
			return h.verifier.VerifyCommandWithTimestamp(cmd, pubKey, commandMaxAge, commandClockSkew)
		}
		// Old format: no timestamp (backward compat)
		return h.verifier.VerifyCommand(cmd, pubKey)
	}

	switch signingCfg.EnforcementMode {
	case "strict":
		if cmd.Signature == "" {
			h.logger.Printf("[ERROR] [agent] [cmd_handler] command_not_signed command_id=%q", cmd.ID)
			if h.securityLogger != nil {
				h.securityLogger.LogCommandVerificationFailure(cmd.ID, "missing signature")
			}
			return fmt.Errorf("command verification failed: strict enforcement requires signed commands")
		}
		if err := verifyFunc(); err != nil {
			h.logger.Printf("[ERROR] [agent] [cmd_handler] command_verification_failed command_id=%q error=%q", cmd.ID, err)
			if h.securityLogger != nil {
				h.securityLogger.LogCommandVerificationFailure(cmd.ID, err.Error())
			}
			return fmt.Errorf("command verification failed: %w", err)
		}
		h.logger.Printf("[INFO] [agent] [cmd_handler] command_verified command_id=%q", cmd.ID)
		if h.securityLogger != nil {
			h.securityLogger.LogCommandVerificationSuccess(cmd.ID)
		}
		h.markExecuted(cmd.ID)
	case "warning":
		if cmd.Signature != "" {
			if err := verifyFunc(); err != nil {
				h.logger.Printf("[WARNING] [agent] [cmd_handler] verification_failed_warning_mode command_id=%q error=%q", cmd.ID, err)
				if h.securityLogger != nil {
					h.securityLogger.LogCommandVerificationFailure(cmd.ID, err.Error())
				}
			} else {
				if h.securityLogger != nil {
					h.securityLogger.LogCommandVerificationSuccess(cmd.ID)
				}
			}
		} else {
			h.logger.Printf("[WARNING] [agent] [cmd_handler] unsigned_command_warning_mode command_id=%q", cmd.ID)
		}
		h.markExecuted(cmd.ID)
	// "disabled" or any other value: skip verification
	default:
		h.markExecuted(cmd.ID)
	}

	return nil
}

// markExecuted records a command ID in the deduplication set (F-2 fix).
// Persists the updated set to disk so a restart-within-commandMaxAge cannot re-execute
// the same command (Migration 033 §5).
func (h *CommandHandler) markExecuted(cmdID string) {
	h.executedIDsMu.Lock()
	h.executedIDs[cmdID] = time.Now().UTC()
	if err := h.saveExecutedIDsLocked(); err != nil {
		h.logger.Printf("[ERROR] [agent] [cmd_handler] persist_executed_ids_failed cmd=%s error=%v", cmdID, err)
	}
	h.executedIDsMu.Unlock()
}

// CleanupExecutedIDs evicts entries older than commandMaxAge from the dedup set.
// Should be called when ShouldRefreshKey() fires (every 6h). Persists the trimmed
// set if anything was evicted.
func (h *CommandHandler) CleanupExecutedIDs() {
	h.executedIDsMu.Lock()
	defer h.executedIDsMu.Unlock()

	cutoff := time.Now().UTC().Add(-commandMaxAge)
	evicted := 0
	for id, execTime := range h.executedIDs {
		if execTime.Before(cutoff) {
			delete(h.executedIDs, id)
			evicted++
		}
	}
	if evicted > 0 {
		h.logger.Printf("[INFO] [agent] [cmd_handler] cleanup_executed_ids evicted=%d remaining=%d", evicted, len(h.executedIDs))
		if err := h.saveExecutedIDsLocked(); err != nil {
			h.logger.Printf("[ERROR] [agent] [cmd_handler] persist_after_cleanup_failed error=%v", err)
		}
	}
}

// RefreshPrimaryKey proactively re-fetches the server's primary key.
// Should be called every keyRefreshInterval to detect rotations early.
func (h *CommandHandler) RefreshPrimaryKey(serverURL string) error {
	h.logger.Printf("[INFO] [agent] [cmd_handler] refreshing_primary_key")
	pubKey, err := crypto.FetchAndCacheServerPublicKey(serverURL)
	if err != nil {
		return fmt.Errorf("failed to refresh primary key: %w", err)
	}

	h.keyCacheMu.Lock()
	h.keyCache[""] = pubKey
	h.keyCacheMu.Unlock()
	h.lastKeyRefresh = time.Now()

	h.logger.Printf("[INFO] [agent] [cmd_handler] primary_key_refreshed")
	return nil
}

// ShouldRefreshKey returns true if enough time has passed to warrant a proactive key refresh
func (h *CommandHandler) ShouldRefreshKey() bool {
	return time.Since(h.lastKeyRefresh) >= keyRefreshInterval
}

// UpdateServerPublicKey updates the primary cached public key (kept for backward compat)
func (h *CommandHandler) UpdateServerPublicKey(serverURL string) error {
	return h.RefreshPrimaryKey(serverURL)
}
