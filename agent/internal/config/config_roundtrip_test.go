package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestConfigRoundtripPreservesUnknownKeys is the canonical roundtrip test.
// The config file is the source of truth — keys unknown to the Go struct
// (machine_id, operator-added fields, future features) MUST survive a
// full load+save cycle intact. Regression test for the upgrade-path gap
// where marshal-unmarshal through the struct would silently drop them.
func TestConfigRoundtripPreservesUnknownKeys(t *testing.T) {
	// Craft a config file with keys the struct does not know about.
	// machine_id is the load-bearing example; "operator_note" simulates
	// any future field an admin or config-manager adds outside the agent.
	raw := `{
		"server_url": "http://test:8080",
		"agent_id": "00000000-0000-0000-0000-000000000001",
		"check_in_interval": 300,
		"machine_id": "host-abc-123-xyz",
		"operator_note": "do not remove",
		"desktop": {"enabled": true, "max_restarts": 3}
	}`

	td := t.TempDir()
	configPath := filepath.Join(td, "config.json")
	if err := os.WriteFile(configPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	// Load — this triggers loadFromFile + mergeConfigPreservingDefaults
	// + persistNewDefaults writes back.
	cfg, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify the struct got the known keys.
	if cfg.ServerURL != "http://test:8080" {
		t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "http://test:8080")
	}
	if cfg.CheckInInterval != 300 {
		t.Errorf("CheckInInterval = %d, want 300", cfg.CheckInInterval)
	}
	if !cfg.Desktop.Enabled {
		t.Error("Desktop.Enabled = false, want true (was missing from old config, should be injected by persistNewDefaults)")
	}

	// Read the written file back raw. Parse as map[string]interface{} to
	// see keys the struct does not know about.
	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var rawWritten map[string]interface{}
	if err := json.Unmarshal(written, &rawWritten); err != nil {
		t.Fatalf("written config is not valid JSON: %v\n%s", err, string(written))
	}

	// DID NOT LOSE machine_id — the load-bearing unknown key.
	if _, exists := rawWritten["machine_id"]; !exists {
		t.Errorf("FATAL: machine_id lost in config roundtrip — unknown keys are silently dropped")
	}
	// DID NOT LOSE operator_note — arbitrary operator-added key.
	if _, exists := rawWritten["operator_note"]; !exists {
		t.Errorf("operator_note lost in config roundtrip — operator-added keys are silently dropped")
	}
	// DID ADD desktop — new struct key not in the original file.
	if _, exists := rawWritten["desktop"]; !exists {
		t.Errorf("desktop missing from written config — persistNewDefaults did not inject it")
	}

	// Verify machine_id retained its value exactly.
	machineID, _ := rawWritten["machine_id"].(string)
	if machineID != "host-abc-123-xyz" {
		t.Errorf("machine_id = %q, want %q", machineID, "host-abc-123-xyz")
	}

	// Verify operator_note retained its value exactly.
	note, _ := rawWritten["operator_note"].(string)
	if note != "do not remove" {
		t.Errorf("operator_note = %q, want %q", note, "do not remove")
	}

	// Verify desktop sub-fields were preserved (max_restarts from the file).
	desktop, ok := rawWritten["desktop"].(map[string]interface{})
	if !ok {
		t.Error("desktop is not a map in written config")
	} else {
		if mr, ok := desktop["max_restarts"].(float64); !ok || mr != 3 {
			t.Errorf("desktop.max_restarts = %v, want 3 (preserved from file)", desktop["max_restarts"])
		}
	}

	t.Logf("roundtrip OK — %d keys in written config", len(rawWritten))
}

// TestPersistNewDefaultsInjectsNestedSubFields verifies that new sub-fields
// introduced in a version upgrade are written even when the parent key already
// exists on disk. E.g. an old config has desktop:{"enabled":true} and the new
// version adds max_restarts / restart_delay_sec — those must land on disk.
func TestPersistNewDefaultsInjectsNestedSubFields(t *testing.T) {
	// Old config: desktop with only "enabled" — missing max_restarts and
	// restart_delay_sec that a newer agent version carries as defaults.
	raw := `{
		"server_url": "http://test:8080",
		"agent_id": "00000000-0000-0000-0000-000000000003",
		"check_in_interval": 300,
		"desktop": {"enabled": true}
	}`

	td := t.TempDir()
	configPath := filepath.Join(td, "config.json")
	if err := os.WriteFile(configPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// The merged struct must have the new sub-fields present (defaults are 0
	// for both: 0 = unlimited restarts, 0 = default restart delay).
	if cfg.Desktop.MaxRestarts != 0 {
		t.Logf("Desktop.MaxRestarts = %d (non-zero default — ok)", cfg.Desktop.MaxRestarts)
	}
	if cfg.Desktop.RestartDelaySec != 0 {
		t.Logf("Desktop.RestartDelaySec = %d (non-zero default — ok)", cfg.Desktop.RestartDelaySec)
	}

	// Read the written file — the new sub-fields must be on disk.
	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var rawWritten map[string]interface{}
	if err := json.Unmarshal(written, &rawWritten); err != nil {
		t.Fatalf("written config is not valid JSON: %v", err)
	}

	desktop, ok := rawWritten["desktop"].(map[string]interface{})
	if !ok {
		t.Fatal("desktop missing from written config entirely")
	}
	if e, ok := desktop["enabled"].(bool); !ok || !e {
		t.Errorf("desktop.enabled = %v, want true (original value lost)", desktop["enabled"])
	}
	if _, exists := desktop["max_restarts"]; !exists {
		t.Error("desktop.max_restarts missing — persistNewDefaults did not inject new nested sub-field")
	}
	if _, exists := desktop["restart_delay_sec"]; !exists {
		t.Error("desktop.restart_delay_sec missing — persistNewDefaults did not inject new nested sub-field")
	}

	t.Logf("nested sub-field injection OK — %d top-level keys, desktop has %d sub-keys",
		len(rawWritten), len(desktop))
}

// TestConfigLoadDoesNotRemoveKeys checks that loading a config with extra
// keys (from a future agent version) does not strip them in memory.
// The file is the authority; the load path must not filter by struct tags.
func TestConfigLoadDoesNotRemoveKeys(t *testing.T) {
	raw := `{
		"server_url": "http://test:8080",
		"agent_id": "00000000-0000-0000-0000-000000000002",
		"check_in_interval": 300,
		"machine_id": "should-survive",
		"future_feature": {"enabled": true}
	}`

	td := t.TempDir()
	configPath := filepath.Join(td, "config.json")
	if err := os.WriteFile(configPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Known fields populated.
	if cfg.ServerURL != "http://test:8080" {
		t.Errorf("ServerURL = %q", cfg.ServerURL)
	}
	if cfg.CheckInInterval != 300 {
		t.Errorf("CheckInInterval = %d", cfg.CheckInInterval)
	}

	// The file on disk was NOT changed if it already had the struct keys.
	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var rawWritten map[string]interface{}
	if err := json.Unmarshal(written, &rawWritten); err != nil {
		t.Fatalf("written config invalid: %v", err)
	}

	if _, exists := rawWritten["machine_id"]; !exists {
		t.Error("machine_id removed by load")
	}
	if _, exists := rawWritten["future_feature"]; !exists {
		t.Error("future_feature removed by load")
	}
}
