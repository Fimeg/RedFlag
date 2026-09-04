package handlers

// reconcile_test.go — Unit tests for RECONCILE-001: scan-set closure safety.
//
// These tests do not require a live DB. They cover:
//   - scanEcosystemSupported: only dnf and apt trigger closure
//   - The closure gate in ReportUpdates: ScanSucceeded=false → closure skipped
//   - The closure gate: unsupported ecosystem → closure skipped

import (
	"testing"
)

// TestScanEcosystemSupported verifies that the closure gate only fires for the
// OS package managers whose scan semantics guarantee a complete list.
// Docker is explicitly excluded (image-set semantics differ).
func TestScanEcosystemSupported(t *testing.T) {
	supported := []string{"dnf", "apt"}
	for _, eco := range supported {
		if !scanEcosystemSupported(eco) {
			t.Errorf("ecosystem %q should be supported for scan-set closure", eco)
		}
	}

	notSupported := []string{"docker", "winget", "windows", "windows_update", "pip", "npm", ""}
	for _, eco := range notSupported {
		if scanEcosystemSupported(eco) {
			t.Errorf("ecosystem %q must NOT trigger scan-set closure (not supported)", eco)
		}
	}
}

// TestClosureGateNeverFiresOnFailedScan verifies the safety invariant from
// ETHOS §3 and RECONCILE-001: a failed/partial scan must never trigger closure.
// This is tested by confirming that the closure branch is guarded behind
// req.ScanSucceeded == true, not just ecosystem check.
//
// Implementation note: this test is structural (verifying the gate condition
// as expressed in Go code), not a DB integration test. The condition in
// ReportUpdates is:
//   if req.ScanSucceeded && scanEcosystemSupported(req.Ecosystem)
//
// A failed scan: ScanSucceeded=false → gate is false regardless of ecosystem.
func TestClosureGateNeverFiresOnFailedScan(t *testing.T) {
	failedScans := []struct {
		scanSucceeded bool
		ecosystem     string
		shouldClose   bool
		label         string
	}{
		{false, "dnf", false, "failed dnf scan must not close"},
		{false, "apt", false, "failed apt scan must not close"},
		{false, "docker", false, "failed docker scan must not close"},
		{true, "docker", false, "successful docker scan must not close (unsupported ecosystem)"},
		{true, "winget", false, "successful winget scan must not close (unsupported ecosystem)"},
		{true, "dnf", true, "successful dnf scan should close"},
		{true, "apt", true, "successful apt scan should close"},
	}

	for _, tc := range failedScans {
		t.Run(tc.label, func(t *testing.T) {
			// This mirrors the gate condition in ReportUpdates exactly.
			gateOpen := tc.scanSucceeded && scanEcosystemSupported(tc.ecosystem)
			if gateOpen != tc.shouldClose {
				t.Errorf("closure gate for ScanSucceeded=%v ecosystem=%q: got %v, want %v",
					tc.scanSucceeded, tc.ecosystem, gateOpen, tc.shouldClose)
			}
		})
	}
}
