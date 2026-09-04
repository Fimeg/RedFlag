package models_test

// reconcile_test.go — Unit tests for RECONCILE-001: scan-set closure.
//
// Coverage:
//   - ReconcileFromScan state rules (all eight states)
//   - ValidateTransition: installed → pending is now permitted
//   - ValidateTransition: ignored is still locked (cannot reopen)
//   - IsTerminal: resting states do not auto-advance from scan signals

import (
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/models"
)

// TestReconcileFromScan verifies the re-discovery rules defined in
// RECONCILE-001 Target Model. This is the Go twin of the SQL CASE in
// UpdateCurrentStateInTx — they must agree.
func TestReconcileFromScan(t *testing.T) {
	cases := []struct {
		current  models.PackageStatus
		expected models.PackageStatus
		label    string
	}{
		// Terminal-resting states that scan must NOT auto-advance.
		{models.StatusIgnored, models.StatusIgnored, "ignored is preserved (operator decision)"},
		{models.StatusFailed, models.StatusFailed, "failed is preserved (operator recovery path)"},

		// installed reappears in scan → reopen (new version available).
		// This is the deliberate reactivation path introduced by RECONCILE-001.
		{models.StatusInstalled, models.StatusPending, "installed reappears → reopen to pending"},

		// All waiting/active states reset to pending on re-discovery.
		{models.StatusPending, models.StatusPending, "pending → pending (no-op)"},
		{models.StatusApproved, models.StatusPending, "approved → pending on re-discovery"},
		{models.StatusCheckingDependencies, models.StatusPending, "checking_dependencies → pending on re-discovery"},
		{models.StatusPendingDependencies, models.StatusPending, "pending_dependencies → pending on re-discovery"},
		{models.StatusInstalling, models.StatusPending, "installing → pending on re-discovery"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := models.ReconcileFromScan(tc.current)
			if got != tc.expected {
				t.Errorf("ReconcileFromScan(%q) = %q, want %q", tc.current, got, tc.expected)
			}
		})
	}
}

// TestInstalledToPendingTransitionAllowed verifies that the state machine now
// permits installed → pending (RECONCILE-001: deliberate reactivation when a
// package reappears in a later scan). This is load-bearing: if the machine
// blocks this move, the reopen code path in UpdateCurrentStateInTx will fail.
func TestInstalledToPendingTransitionAllowed(t *testing.T) {
	if err := models.ValidateTransition(models.StatusInstalled, models.StatusPending); err != nil {
		t.Errorf("installed → pending must be permitted after RECONCILE-001: %v", err)
	}
}

// TestWaitingStatesResolveOutOfBand verifies the out-of-band resolution edge added by
// RECONCILE-001: a waiting update (pending/approved) that vanishes from a successful scan
// can be closed to installed. This is load-bearing — transitionStatus validates against
// the map, so without this edge closeScanAbsentRows hits the validation wall and silently
// skips every out-of-band patch (the exact case it exists to handle).
func TestWaitingStatesResolveOutOfBand(t *testing.T) {
	for _, from := range []models.PackageStatus{models.StatusPending, models.StatusApproved} {
		if err := models.ValidateTransition(from, models.StatusInstalled); err != nil {
			t.Errorf("%q → installed must be permitted (out-of-band resolution): %v", from, err)
		}
	}

	// The in-flight states are deliberately NOT given a direct edge to installed: they
	// reach installed only via the receipt path (installing → installed) or are recovered
	// by the orchestrator. The scan-set reconciler excludes them (GetTrackedNonResting).
	for _, from := range []models.PackageStatus{
		models.StatusCheckingDependencies, models.StatusPendingDependencies,
	} {
		if err := models.ValidateTransition(from, models.StatusInstalled); err == nil {
			t.Errorf("%q → installed must NOT be a direct edge (orchestrator/receipt-owned)", from)
		}
	}
}

// TestIgnoredTransitionLocked verifies that the ignored state remains fully
// terminal. A scan cannot reopen an operator-rejected package — that decision
// is preserved indefinitely.
func TestIgnoredTransitionLocked(t *testing.T) {
	targets := []models.PackageStatus{
		models.StatusPending,
		models.StatusApproved,
		models.StatusInstalled,
		models.StatusFailed,
		models.StatusInstalling,
	}
	for _, to := range targets {
		if err := models.ValidateTransition(models.StatusIgnored, to); err == nil {
			t.Errorf("ignored → %q must be forbidden, but ValidateTransition returned nil", to)
		}
	}
}

// TestInstalledRemainsTerminalForScanClosure verifies IsTerminal semantics
// (scan-stability): installed/failed/ignored are terminal, the rest are not.
// NOTE: IsTerminal is about scan-stability, NOT closure scope. The scan-set
// reconciler's actual closure candidates are narrower still — only the waiting
// states (pending, approved); see GetTrackedNonResting. In-flight states are
// non-terminal here but are excluded from closure (orchestrator/receipt-owned).
func TestInstalledRemainsTerminalForScanClosure(t *testing.T) {
	for _, s := range []models.PackageStatus{
		models.StatusInstalled, models.StatusFailed, models.StatusIgnored,
	} {
		if !s.IsTerminal() {
			t.Errorf("status %q should be terminal (scan-set reconciler must skip it)", s)
		}
	}

	for _, s := range []models.PackageStatus{
		models.StatusPending, models.StatusApproved,
		models.StatusCheckingDependencies, models.StatusPendingDependencies,
		models.StatusInstalling,
	} {
		if s.IsTerminal() {
			t.Errorf("status %q must NOT be terminal (scan-set reconciler must consider it)", s)
		}
	}
}
