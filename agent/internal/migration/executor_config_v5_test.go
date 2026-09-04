package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestConfigV5MigrationMarksCompleteAndConverges proves the fix for the Windows
// re-trigger loop: config_v5_migration must be recorded as completed and the
// on-disk config version must be bumped, so a second detection pass does not
// re-add it. Before the fix, config_v5_migration had no executor phase and was
// never marked complete, so it re-triggered on every boot.
func TestConfigV5MigrationMarksCompleteAndConverges(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "agent")
	stateDir := filepath.Join(tmp, "state")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")

	// Seed a config at an old schema version with no migration state.
	seed := map[string]interface{}{
		"version":       4,
		"agent_version": "0.2.6.5",
		"agent_id":      "7ba67463-b796-4b20-be7d-780d37f2b239",
		"token":         "x",
	}
	writeJSON(t, configPath, seed)

	detectionCfg := &FileDetectionConfig{
		OldConfigPath:    filepath.Join(tmp, "old-config"),
		OldStatePath:     filepath.Join(tmp, "old-state"),
		NewConfigPath:    configDir,
		NewStatePath:     stateDir,
		BackupDirPattern: filepath.Join(tmp, "backups", "%s"),
	}

	detection := &MigrationDetection{
		CurrentAgentVersion:  "0.2.6.5",
		CurrentConfigVersion: 4,
		RequiresMigration:    true,
		RequiredMigrations:   []string{"config_v5_migration"},
		Inventory:            &AgentFileInventory{},
	}

	plan := &MigrationPlan{
		Detection:     detection,
		TargetVersion: "0.2.6.7",
		Config:        detectionCfg,
		BackupPath:    filepath.Join(tmp, "backup"),
	}

	executor := NewMigrationExecutor(plan, configPath)
	result, err := executor.ExecuteMigration()
	if err != nil {
		t.Fatalf("ExecuteMigration returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("migration did not succeed: %+v", result.Errors)
	}

	// 1. config_v5_migration must now be recorded as completed.
	sm := NewStateManager(configPath)
	completed, err := sm.IsMigrationCompleted("config_v5_migration")
	if err != nil {
		t.Fatalf("IsMigrationCompleted error: %v", err)
	}
	if !completed {
		t.Fatal("config_v5_migration was not marked complete after migration")
	}

	// 2. The on-disk config version must be bumped to the target schema (7).
	// It is normalized to a string, the type config.Config.Version expects.
	got := readJSON(t, configPath)
	if v, _ := got["version"].(string); v != "7" {
		t.Fatalf("config version not bumped: got %v (%T) want \"7\"", got["version"], got["version"])
	}

	// 3. A fresh detection pass must NOT re-add config_v5_migration (convergence).
	migrations := determineRequiredMigrations(detection, detectionCfg)
	for _, m := range migrations {
		if m == "config_v5_migration" {
			t.Fatalf("config_v5_migration re-added after completion; migrations=%v", migrations)
		}
	}
}

// TestValidateMigrationCreatesMissingDirs proves validateMigration no longer
// fails when the new state/config directories do not yet exist — it creates
// them rather than reporting a false "not found" (the Windows symptom).
func TestValidateMigrationCreatesMissingDirs(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "agent")     // intentionally not pre-created
	stateDir := filepath.Join(tmp, "agent", "state") // intentionally not pre-created

	plan := &MigrationPlan{
		Detection: &MigrationDetection{Inventory: &AgentFileInventory{}},
		Config: &FileDetectionConfig{
			NewConfigPath: configDir,
			NewStatePath:  stateDir,
		},
	}
	executor := NewMigrationExecutor(plan, filepath.Join(configDir, "config.json"))

	if err := executor.validateMigration(); err != nil {
		t.Fatalf("validateMigration failed on missing dirs: %v", err)
	}
	for _, d := range []string{configDir, stateDir} {
		if _, err := os.Stat(d); err != nil {
			t.Fatalf("expected dir %s to exist after validation: %v", d, err)
		}
	}
}

func writeJSON(t *testing.T, path string, v map[string]interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var v map[string]interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return v
}
