package handlers

import (
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/models"
)

func TestPackageVersionFromOSVMetaRecordsCheckedVersion(t *testing.T) {
	meta := map[string]interface{}{
		"supply_chain_checked_version": "2.4.0",
		"supply_chain_vulns":           nil,
	}

	pv := packageVersionFromOSVMeta("npm", "demo", meta)
	if pv == nil {
		t.Fatal("packageVersionFromOSVMeta returned nil")
	}
	if pv.Version != "2.4.0" {
		t.Fatalf("version = %q, want %q", pv.Version, "2.4.0")
	}
	if pv.OSVStatus == nil || *pv.OSVStatus != "clean" {
		t.Fatalf("OSVStatus = %v, want clean", pv.OSVStatus)
	}
	if pv.OSVVulns != nil {
		t.Fatalf("OSVVulns = %v, want nil for clean result", *pv.OSVVulns)
	}
}

func TestPackageVersionFromOSVMetaRecordsVulnerableInstalledVersion(t *testing.T) {
	raw := `[{"id":"CVE-2026-0001"}]`
	meta := map[string]interface{}{
		"installed_checked_version": "1.9.0",
		"installed_vulns":           raw,
	}

	pv := packageVersionFromOSVMeta("dnf", "demo", meta)
	if pv == nil {
		t.Fatal("packageVersionFromOSVMeta returned nil")
	}
	if pv.Version != "1.9.0" {
		t.Fatalf("version = %q, want %q", pv.Version, "1.9.0")
	}
	if pv.OSVStatus == nil || *pv.OSVStatus != "vulnerable" {
		t.Fatalf("OSVStatus = %v, want vulnerable", pv.OSVStatus)
	}
	if pv.OSVVulns == nil || *pv.OSVVulns != raw {
		t.Fatalf("OSVVulns = %v, want %s", pv.OSVVulns, raw)
	}
}

func TestPersistedSupplyChainVulnsRequiresMatchingCheckedVersion(t *testing.T) {
	update := &models.UpdateState{
		Metadata: models.JSONB{
			"supply_chain_checked_version": "2.4.0",
			"supply_chain_vulns":           `[{"id":"CVE-2026-0001"}]`,
		},
	}

	if got := persistedSupplyChainVulns(update, "2.5.0"); len(got) != 0 {
		t.Fatalf("wrong-version vulns len = %d, want 0", len(got))
	}
	if got := persistedSupplyChainVulns(update, "2.4.0"); len(got) != 1 || got[0].ID != "CVE-2026-0001" {
		t.Fatalf("matching-version vulns = %#v, want CVE-2026-0001", got)
	}
}

func TestMetadataHasTargetSupplyChainVulnsRejectsUnversionedMetadata(t *testing.T) {
	update := &models.UpdateState{
		Metadata: models.JSONB{
			"supply_chain_vulns": `[{"id":"CVE-2026-0001"}]`,
		},
	}

	if metadataHasTargetSupplyChainVulns(update, "2.4.0") {
		t.Fatal("unversioned supply_chain_vulns must not match a target version")
	}
}

func TestTopLevelClosureRequiresPackageNameMatch(t *testing.T) {
	closure := []models.ClosureItem{
		{Name: "dep", Version: "1.0", SHA256: "dep-hash"},
		{Name: "demo", Version: "2.4.0", SHA256: "demo-hash"},
	}

	top, ok := topLevelClosure(closure, "demo")
	if !ok {
		t.Fatal("topLevelClosure did not find named package")
	}
	if top.Name != "demo" || top.Version != "2.4.0" || top.SHA256 != "demo-hash" {
		t.Fatalf("topLevelClosure = %#v, want demo 2.4.0 demo-hash", top)
	}

	if _, ok := topLevelClosure(closure, "missing"); ok {
		t.Fatal("topLevelClosure must not fall back to the first closure entry")
	}
}

func TestResolveInstallTargetHonorsManualSelectedVersion(t *testing.T) {
	selected := "2.4.0"
	update := &models.UpdateState{
		SelectedVersion:  &selected,
		AvailableVersion: "2.5.0",
		Status:           models.StatusPending,
		Metadata: models.JSONB{
			selectedVersionSourceKey: selectedVersionSourceManual,
		},
	}

	target, explicit := (&UpdateHandler{}).resolveInstallTarget(update)
	if target != selected || !explicit {
		t.Fatalf("target=%q explicit=%t, want %q true", target, explicit, selected)
	}
}

func TestResolveInstallTargetKeepsInstalledHoldWhenNoGatedTarget(t *testing.T) {
	selected := "2.4.0"
	update := &models.UpdateState{
		SelectedVersion:  &selected,
		AvailableVersion: "2.5.0",
		Status:           models.StatusPending,
		Metadata: models.JSONB{
			selectedVersionSourceKey: selectedVersionSourceInstalledHold,
		},
	}

	target, explicit := (&UpdateHandler{}).resolveInstallTarget(update)
	if target != selected || !explicit {
		t.Fatalf("target=%q explicit=%t, want held installed version", target, explicit)
	}
}

func TestResolveInstallTargetTreatsLegacyPendingMismatchAsInstalledHold(t *testing.T) {
	selected := "2.4.0"
	update := &models.UpdateState{
		SelectedVersion:  &selected,
		AvailableVersion: "2.5.0",
		Status:           models.StatusPending,
	}

	target, explicit := (&UpdateHandler{}).resolveInstallTarget(update)
	if target != selected || !explicit {
		t.Fatalf("target=%q explicit=%t, want legacy installed hold to stay held", target, explicit)
	}
}

func TestGatedTargetAheadUsesPackageManagerVersionShape(t *testing.T) {
	if !gatedTargetAhead("dnf", "3:29.5.2-1.fc43", "29.5.1-1.fc43") {
		t.Fatal("dnf epoch-bearing target should not be rejected by semver comparison")
	}
	if gatedTargetAhead("dnf", "3:29.5.2-1.fc43", "3:29.5.2-1.fc43") {
		t.Fatal("dnf exact same EVR should be treated as a no-op")
	}
	if gatedTargetAhead("dnf", "2:99.0.0-1.fc43", "3:1.0.0-1.fc43") {
		t.Fatal("dnf lower epoch should not be treated as newer")
	}
	// Epoch normalisation: "0:2.0-1" and "2.0-1" are semantically identical because
	// splitRPMEVR defaults a missing epoch to 0. The string-equality guard is bypassed
	// by the textual difference, so the component-wise comparison must return false.
	if gatedTargetAhead("dnf", "0:2.0-1", "2.0-1") {
		t.Fatal("dnf explicit-epoch-0 vs implicit-epoch-0 should be treated as a no-op")
	}
	if gatedTargetAhead("dnf", "2.0-1", "0:2.0-1") {
		t.Fatal("dnf implicit-epoch-0 vs explicit-epoch-0 should be treated as a no-op")
	}
	if !gatedTargetAhead("apt", "1.2.3-1ubuntu1~22.04", "1.2.3-1ubuntu1") {
		t.Fatal("apt package versions should not be rejected by semver comparison")
	}
	if gatedTargetAhead("apt", "1.2.3-1ubuntu1", "1.2.3-1ubuntu1") {
		t.Fatal("apt exact same version should be treated as a no-op")
	}
}

func TestGatedTargetAheadKeepsSemverGuardForOtherTypes(t *testing.T) {
	if !gatedTargetAhead("npm", "2.1.0", "2.0.9") {
		t.Fatal("semver-like package should allow a higher target")
	}
	if gatedTargetAhead("npm", "2.0.0", "2.0.0") {
		t.Fatal("semver-like package should reject equal target")
	}
	if gatedTargetAhead("npm", "1.9.9", "2.0.0") {
		t.Fatal("semver-like package should reject lower target")
	}
}
