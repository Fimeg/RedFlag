package services

import "testing"

// GetSupplyChainGateConfig layers env > config > DB > default via GetSetting.
// The env path short-circuits before any DB access, so we can exercise the
// resolver — and the fact that it makes the DB-backed settings *live* at all —
// without a database by setting the REDFLAG_SUPPLY_CHAIN_* env vars, which
// getEnvironmentValue maps to the supply_chain.* keys.
func TestGetSupplyChainGateConfig_EnvDrivesGate(t *testing.T) {
	t.Setenv("REDFLAG_SUPPLY_CHAIN_MIN_PACKAGE_AGE_HOURS", "48")
	t.Setenv("REDFLAG_SUPPLY_CHAIN_GATE_ENFORCEMENT", "block")
	t.Setenv("REDFLAG_SUPPLY_CHAIN_BLOCK_UNKNOWN_AGE", "true")

	s := &SecuritySettingsService{} // settingsQueries nil: env path must not touch it
	minAge, enf, blockUnknown := s.GetSupplyChainGateConfig()

	if minAge != 48.0 {
		t.Errorf("min age: want 48, got %v", minAge)
	}
	if enf != "block" {
		t.Errorf("enforcement: want block, got %q", enf)
	}
	if !blockUnknown {
		t.Error("block_unknown_age: want true")
	}
}

func TestGetSupplyChainGateConfig_InvalidEnforcementIgnored(t *testing.T) {
	// A bad enforcement env value falls back to the baked default rather than
	// driving the gate into an undefined state. (min/block set to valid env
	// values so every key resolves on the env path and never touches the DB.)
	t.Setenv("REDFLAG_SUPPLY_CHAIN_GATE_ENFORCEMENT", "panic")
	t.Setenv("REDFLAG_SUPPLY_CHAIN_MIN_PACKAGE_AGE_HOURS", "24")
	t.Setenv("REDFLAG_SUPPLY_CHAIN_BLOCK_UNKNOWN_AGE", "false")

	s := &SecuritySettingsService{}
	_, enf, _ := s.GetSupplyChainGateConfig()
	if enf != "warn" {
		t.Errorf("invalid enforcement should fall back to warn, got %q", enf)
	}
}

func TestGetSoakGateConfig_EnvDrivesGate(t *testing.T) {
	t.Setenv("REDFLAG_SUPPLY_CHAIN_SOAK_WINDOW_DAYS", "7")
	t.Setenv("REDFLAG_SUPPLY_CHAIN_SOAK_ENFORCEMENT", "warn")

	s := &SecuritySettingsService{} // env path must not touch the nil DB
	days, enf := s.GetSoakGateConfig()
	if days != 7.0 {
		t.Errorf("soak window: want 7, got %v", days)
	}
	if enf != "warn" {
		t.Errorf("soak enforcement: want warn, got %q", enf)
	}
}

func TestGetSoakGateConfig_InvalidEnforcementIgnored(t *testing.T) {
	t.Setenv("REDFLAG_SUPPLY_CHAIN_SOAK_WINDOW_DAYS", "14")
	t.Setenv("REDFLAG_SUPPLY_CHAIN_SOAK_ENFORCEMENT", "nonsense")

	s := &SecuritySettingsService{}
	_, enf := s.GetSoakGateConfig()
	if enf != "block" {
		t.Errorf("invalid soak enforcement should fall back to the block default, got %q", enf)
	}
}
