package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/config"
)

// MigrationState is imported from config package to avoid duplication

// StateManager manages migration state persistence.
//
// It operates on the config file as a generic JSON map rather than unmarshaling
// into the typed config.Config. This is deliberate: StateManager only owns the
// `migration_state`, `version`, and `agent_version` keys, and must stay robust
// against field-type drift elsewhere in config.json. In particular the install
// template writes `"version"` as a JSON number while config.Config types it as a
// string — unmarshaling the whole struct here would fail and silently prevent
// migration completion from ever persisting (the Windows re-trigger loop).
type StateManager struct {
	configPath string
}

// NewStateManager creates a new state manager
func NewStateManager(configPath string) *StateManager {
	return &StateManager{
		configPath: configPath,
	}
}

// LoadState loads migration state from config file
func (sm *StateManager) LoadState() (*config.MigrationState, error) {
	raw, err := sm.loadConfigMap()
	if err != nil {
		if os.IsNotExist(err) {
			// Fresh install - no migration state yet
			return newEmptyState(), nil
		}
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	msRaw, ok := raw["migration_state"]
	if !ok || msRaw == nil {
		// No migration state recorded yet - seed from what we can read.
		state := newEmptyState()
		if v, ok := raw["agent_version"].(string); ok {
			state.AgentVersion = v
		}
		if v, ok := raw["version"].(string); ok {
			state.ConfigVersion = v
		}
		return state, nil
	}

	// Re-marshal the migration_state subtree and decode into the typed struct.
	data, err := json.Marshal(msRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal migration state: %w", err)
	}
	var state config.MigrationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to decode migration state: %w", err)
	}
	if state.LastCompleted == nil {
		state.LastCompleted = make(map[string]time.Time)
	}
	return &state, nil
}

// SaveState saves migration state to config file
func (sm *StateManager) SaveState(state *config.MigrationState) error {
	raw, err := sm.loadConfigMap()
	if err != nil {
		return fmt.Errorf("failed to load config for state save: %w", err)
	}

	state.Timestamp = time.Now().UTC()
	raw["migration_state"] = state

	return sm.saveConfigMap(raw)
}

// IsMigrationCompleted checks if a specific migration was completed
func (sm *StateManager) IsMigrationCompleted(migrationType string) (bool, error) {
	state, err := sm.LoadState()
	if err != nil {
		return false, err
	}

	// Check completed migrations list
	for _, completed := range state.CompletedMigrations {
		if completed == migrationType {
			return true, nil
		}
	}

	// Also check legacy last_completed map for backward compatibility
	if timestamp, exists := state.LastCompleted[migrationType]; exists {
		return !timestamp.IsZero(), nil
	}

	return false, nil
}

// MarkMigrationCompleted marks a migration as completed
func (sm *StateManager) MarkMigrationCompleted(migrationType string, rollbackPath string, agentVersion string) error {
	state, err := sm.LoadState()
	if err != nil {
		return err
	}

	// Update completed migrations list
	found := false
	for _, completed := range state.CompletedMigrations {
		if completed == migrationType {
			found = true
			break
		}
	}

	if !found {
		state.CompletedMigrations = append(state.CompletedMigrations, migrationType)
	}

	if state.LastCompleted == nil {
		state.LastCompleted = make(map[string]time.Time)
	}
	state.LastCompleted[migrationType] = time.Now().UTC()
	state.AgentVersion = agentVersion
	state.Success = true
	if rollbackPath != "" {
		state.RollbackPath = rollbackPath
	}

	return sm.SaveState(state)
}

// BumpConfigVersion persists the config schema version (and agent version) to the
// config file so migration detection converges on the next boot. Without this, a
// config-schema migration whose actual schema work is done lazily on load (by the
// config package's mergeConfigPreservingDefaults/migrateConfig) leaves the on-disk
// `version` field stale, and detection keeps re-triggering the same migration.
func (sm *StateManager) BumpConfigVersion(targetConfigVersion, agentVersion string) error {
	raw, err := sm.loadConfigMap()
	if err != nil {
		return fmt.Errorf("failed to load config for version bump: %w", err)
	}

	raw["version"] = targetConfigVersion
	if agentVersion != "" {
		raw["agent_version"] = agentVersion
	}

	return sm.saveConfigMap(raw)
}

// CleanupOldDirectories removes old migration directories after successful migration
func (sm *StateManager) CleanupOldDirectories() error {
	oldDirs := []string{
		"/etc/aggregator",
		"/var/lib/aggregator",
	}

	for _, oldDir := range oldDirs {
		if _, err := os.Stat(oldDir); err == nil {
			fmt.Printf("[MIGRATION] Cleaning up old directory: %s\n", oldDir)
			if err := os.RemoveAll(oldDir); err != nil {
				fmt.Printf("[MIGRATION] Warning: Failed to remove old directory %s: %v\n", oldDir, err)
			}
		}
	}

	return nil
}

// newEmptyState returns a zero-value migration state ready for use.
func newEmptyState() *config.MigrationState {
	return &config.MigrationState{
		LastCompleted:       make(map[string]time.Time),
		AgentVersion:        "",
		ConfigVersion:       "",
		Timestamp:           time.Now().UTC(),
		Success:             false,
		CompletedMigrations: []string{},
	}
}

// loadConfigMap reads the config file as a generic JSON object. Returns an error
// satisfying os.IsNotExist when the file is absent so callers can treat a fresh
// install distinctly.
func (sm *StateManager) loadConfigMap() (map[string]interface{}, error) {
	data, err := os.ReadFile(sm.configPath)
	if err != nil {
		return nil, err
	}

	raw := make(map[string]interface{})
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config json: %w", err)
	}
	return raw, nil
}

// saveConfigMap writes the config map back, preserving every key it didn't touch.
func (sm *StateManager) saveConfigMap(raw map[string]interface{}) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sm.configPath, data, 0644)
}
