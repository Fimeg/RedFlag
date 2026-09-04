package services

import (
	"testing"
	"time"
)

// TestEvaluatePackageAgeGate exercises the gate's three-way decision matrix:
// off / warn / block × (above-threshold / below-threshold / unknown).
//
// Pure logic — no network. The registry-fetching functions are tested
// against real endpoints by integration tests when they exist; the
// EvaluatePackageAgeGate function is the policy core that determines
// approval behavior and must be deterministic.

func TestEvaluatePackageAgeGate_BelowThresholdBlock(t *testing.T) {
	age := &PackageAgeResult{
		PublishedAt: time.Now().Add(-2 * time.Hour),
		Source:      "registry.npmjs.org",
	}
	dec := EvaluatePackageAgeGate(age, PackageAgeGatePolicy{MinAgeHours: 24.0, Enforcement: "block"})
	if !dec.ShouldBlock {
		t.Errorf("expected block, got pass; decision=%+v", dec)
	}
	if dec.BlockedUnknown {
		t.Error("a freshness block must not be flagged as a blocked-unknown")
	}
	if dec.WarnMessage == "" {
		t.Error("expected warn message even on block path; got empty")
	}
	if dec.Unknown {
		t.Error("unknown should be false when age is provided")
	}
}

func TestEvaluatePackageAgeGate_BelowThresholdWarn(t *testing.T) {
	age := &PackageAgeResult{
		PublishedAt: time.Now().Add(-2 * time.Hour),
		Source:      "registry.npmjs.org",
	}
	dec := EvaluatePackageAgeGate(age, PackageAgeGatePolicy{MinAgeHours: 24.0, Enforcement: "warn"})
	if dec.ShouldBlock {
		t.Errorf("warn enforcement must not block; decision=%+v", dec)
	}
	if dec.WarnMessage == "" {
		t.Error("expected warn message under warn enforcement; got empty")
	}
}

func TestEvaluatePackageAgeGate_AboveThreshold(t *testing.T) {
	age := &PackageAgeResult{
		PublishedAt: time.Now().Add(-72 * time.Hour),
		Source:      "pypi.org",
	}
	dec := EvaluatePackageAgeGate(age, PackageAgeGatePolicy{MinAgeHours: 24.0, Enforcement: "block"})
	if dec.ShouldBlock {
		t.Errorf("above-threshold package must not be blocked; decision=%+v", dec)
	}
	if dec.WarnMessage != "" {
		t.Errorf("above-threshold must produce no warning; got %q", dec.WarnMessage)
	}
}

func TestEvaluatePackageAgeGate_UnknownAgeIsAllowByDefault(t *testing.T) {
	// Fail-open by default: when the registry probe couldn't tell us when the
	// package was published (network, ecosystem unsupported, version missing),
	// and the operator has NOT opted into fail-closed, we must NOT block —
	// infrastructure issues should never gate legitimate approvals. Sovereignty.
	dec := EvaluatePackageAgeGate(nil, PackageAgeGatePolicy{MinAgeHours: 24.0, Enforcement: "block"})
	if !dec.Unknown {
		t.Error("expected unknown=true when age is nil")
	}
	if dec.ShouldBlock {
		t.Error("unknown age must not trigger block when block_unknown_age is off")
	}
	if dec.WarnMessage != "" {
		t.Errorf("allowed-unknown must not produce a warning message; got %q", dec.WarnMessage)
	}
}

func TestEvaluatePackageAgeGate_UnknownAgeFailsClosedWhenOptedIn(t *testing.T) {
	// Opt-in fail-closed: block enforcement + block_unknown_age + an ecosystem
	// that *should* have answered (npm/PyPI). A dark recency source for a
	// registry-backed package is itself a worm signal.
	dec := EvaluatePackageAgeGate(nil, PackageAgeGatePolicy{
		MinAgeHours:     24.0,
		Enforcement:     "block",
		BlockUnknownAge: true,
		EcosystemAged:   true,
	})
	if !dec.Unknown {
		t.Error("expected unknown=true when age is nil")
	}
	if !dec.ShouldBlock {
		t.Error("unknown age must fail closed when block_unknown_age is on for an aged ecosystem")
	}
	if !dec.BlockedUnknown {
		t.Error("expected BlockedUnknown=true on the fail-closed path")
	}
	if dec.WarnMessage == "" {
		t.Error("fail-closed block must carry a reason message")
	}
}

func TestEvaluatePackageAgeGate_UnknownAgeAllowedForNonAgedEcosystem(t *testing.T) {
	// Even with block_unknown_age on, an ecosystem with no recency source
	// (apt/dnf) must never be blocked on unknown — there's nothing dark about
	// a distro package having no upstream publish timestamp.
	dec := EvaluatePackageAgeGate(nil, PackageAgeGatePolicy{
		MinAgeHours:     24.0,
		Enforcement:     "block",
		BlockUnknownAge: true,
		EcosystemAged:   false,
	})
	if dec.ShouldBlock {
		t.Error("non-aged ecosystem must not fail closed on unknown even when opted in")
	}
}

func TestEvaluatePackageAgeGate_UnknownAgeAllowedUnderWarnEvenIfOptedIn(t *testing.T) {
	// block_unknown_age only bites under "block" enforcement — under "warn" a
	// dark source is surfaced via logs/metadata, never a hard stop.
	dec := EvaluatePackageAgeGate(nil, PackageAgeGatePolicy{
		MinAgeHours:     24.0,
		Enforcement:     "warn",
		BlockUnknownAge: true,
		EcosystemAged:   true,
	})
	if dec.ShouldBlock {
		t.Error("warn enforcement must not fail closed on unknown")
	}
}

func TestEvaluatePackageAgeGate_OffMeansOff(t *testing.T) {
	// "off" enforcement should not be reached by the gate (caller checks
	// config and skips), but if it is, behavior must be safe: no block, no warn.
	age := &PackageAgeResult{
		PublishedAt: time.Now().Add(-1 * time.Hour),
		Source:      "registry.npmjs.org",
	}
	dec := EvaluatePackageAgeGate(age, PackageAgeGatePolicy{MinAgeHours: 24.0, Enforcement: "off"})
	if dec.ShouldBlock {
		t.Error("off enforcement must not block even below threshold")
	}
}

func TestEcosystemSupportsPackageAge(t *testing.T) {
	for _, eco := range []string{"npm", "PyPI"} {
		if !EcosystemSupportsPackageAge(eco) {
			t.Errorf("%s should support package age", eco)
		}
	}
	for _, eco := range []string{"Debian", "AlmaLinux", "", "crates.io"} {
		if EcosystemSupportsPackageAge(eco) {
			t.Errorf("%s should not support package age", eco)
		}
	}
}

func TestPackageAgeGateConfig_Defaults(t *testing.T) {
	// Env vars cleared in this test process should yield defaults.
	t.Setenv("REDFLAG_SUPPLY_CHAIN_MIN_PACKAGE_AGE_HOURS", "")
	t.Setenv("REDFLAG_SUPPLY_CHAIN_GATE_ENFORCEMENT", "")
	minAge, enf := PackageAgeGateConfig()
	if minAge != 24.0 {
		t.Errorf("default min age should be 24, got %v", minAge)
	}
	if enf != "warn" {
		t.Errorf("default enforcement should be warn, got %q", enf)
	}
}

func TestPackageAgeGateConfig_EnvOverride(t *testing.T) {
	t.Setenv("REDFLAG_SUPPLY_CHAIN_MIN_PACKAGE_AGE_HOURS", "48")
	t.Setenv("REDFLAG_SUPPLY_CHAIN_GATE_ENFORCEMENT", "block")
	minAge, enf := PackageAgeGateConfig()
	if minAge != 48.0 {
		t.Errorf("env override min age should be 48, got %v", minAge)
	}
	if enf != "block" {
		t.Errorf("env override enforcement should be block, got %q", enf)
	}
}

func TestPackageAgeGateConfig_InvalidEnforcementFallsBack(t *testing.T) {
	t.Setenv("REDFLAG_SUPPLY_CHAIN_GATE_ENFORCEMENT", "panic")
	_, enf := PackageAgeGateConfig()
	if enf != "warn" {
		t.Errorf("invalid enforcement should fall back to warn, got %q", enf)
	}
}
