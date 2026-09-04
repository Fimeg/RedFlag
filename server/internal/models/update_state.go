package models

import (
	"fmt"
)

// PackageStatus is the lifecycle status of a package update, backed by the
// current_package_state.status SQL column with a CHECK constraint.
type PackageStatus string

const (
	StatusPending              PackageStatus = "pending"
	StatusApproved             PackageStatus = "approved"
	StatusCheckingDependencies PackageStatus = "checking_dependencies"
	StatusPendingDependencies  PackageStatus = "pending_dependencies"
	StatusInstalling           PackageStatus = "installing"
	StatusInstalled            PackageStatus = "installed"
	StatusFailed               PackageStatus = "failed"
	StatusIgnored              PackageStatus = "ignored"
)

// PackageStatusTransitions encodes every permitted (from -> to) move in the
// state machine. The map is read-only after init; access through ValidateTransition.
var PackageStatusTransitions = map[PackageStatus][]PackageStatus{
	// pending/approved -> installed is the out-of-band resolution edge (RECONCILE-001):
	// a *waiting* update that gets patched outside RedFlag (dnf-automatic, a sysadmin) is
	// closed by the scan-set reconciler when it vanishes from a successful scan. The
	// in-flight states (checking_dependencies, pending_dependencies, installing) are NOT
	// given this edge — they are owned by the orchestrator + capability-receipt path, and
	// the reconciler deliberately excludes them (see GetTrackedNonResting closure scope).
	StatusPending:              {StatusApproved, StatusInstalled, StatusIgnored},
	StatusApproved:             {StatusCheckingDependencies, StatusInstalling, StatusInstalled, StatusIgnored, StatusFailed},
	StatusCheckingDependencies: {StatusPendingDependencies, StatusInstalling, StatusFailed},
	StatusPendingDependencies:  {StatusInstalling, StatusFailed},
	StatusInstalling: {StatusInstalled, StatusFailed, StatusPendingDependencies},
	// installed is scan-stable but not transition-locked. When a scan-set reconciler
	// closes a row by absence (out-of-band patch) and that same package reappears in
	// a later scan (new version available), the row reopens to pending. This matches
	// the DefectDojo reimport model: a resolved finding that resurfaces is reactivated,
	// not silently ignored. The reopen path is: scan-set reconciler → UpdateCurrentStateInTx.
	StatusInstalled: {StatusPending},
	// Failed is a *recoverable* resting state, not a dead end. An operator can
	// re-open it (failed -> pending) to re-run the lifecycle, or close it out
	// (failed -> installed when it turns out the update no longer applies /
	// was resolved out of band, failed -> ignored to stop tracking it). The
	// failure itself is preserved in update_version_history regardless.
	StatusFailed:  {StatusPending, StatusInstalled, StatusIgnored},
	StatusIgnored: {}, // terminal
}

var allPackageStatuses = func() map[PackageStatus]struct{} {
	m := make(map[PackageStatus]struct{}, len(PackageStatusTransitions))
	for s := range PackageStatusTransitions {
		m[s] = struct{}{}
	}
	return m
}()

// ValidateTransition checks whether moving from `from` to `to` is permitted
// by the state machine. It is idempotent (from == to always passes).
// Returns an error naming both states when the transition is invalid.
func ValidateTransition(from, to PackageStatus) error {
	if from == to {
		return nil
	}
	allowed, ok := PackageStatusTransitions[from]
	if !ok {
		return fmt.Errorf("unknown source state %q", from)
	}
	for _, a := range allowed {
		if a == to {
			return nil
		}
	}
	return fmt.Errorf("transition %q -> %q is not permitted", from, to)
}

// IsTerminal returns true for resting states that scan-set reconciliation must
// not automatically advance. Note:
//   - `installed` is scan-stable by default but CAN reopen to pending when the
//     same package reappears in a later scan (new version available). That reopen
//     happens deliberately in UpdateCurrentStateInTx — not here. IsTerminal being
//     true simply means the scan-set reconciler will not close an already-installed
//     row a second time.
//   - `failed` is resting but recoverable — operator transitions it explicitly.
//   - `ignored` is transition-locked.
func (s PackageStatus) IsTerminal() bool {
	switch s {
	case StatusInstalled, StatusFailed, StatusIgnored:
		return true
	}
	return false
}

// IsActive returns true for states that represent an in-flight operation.
func (s PackageStatus) IsActive() bool {
	switch s {
	case StatusCheckingDependencies, StatusPendingDependencies, StatusInstalling:
		return true
	}
	return false
}

// StatusFromString parses a raw status string into a PackageStatus constant.
// Returns an error if the string is not a known state.
func StatusFromString(s string) (PackageStatus, error) {
	st := PackageStatus(s)
	if _, ok := allPackageStatuses[st]; ok {
		return st, nil
	}
	return "", fmt.Errorf("unknown package status %q", s)
}

// ReconcileFromScan returns the status to set after an agent re-discovers a
// package that already has a row in current_package_state. This is the
// SQL twin of the CASE in UpdateCurrentStateInTx — keep them in sync.
//
// Rules (Target Model, RECONCILE-001):
//   - ignored: preserved (operator decision, scan cannot override)
//   - failed: preserved (operator recovery path; auto-reopen would mask failures)
//   - installed + reappears in scan: reopen to pending (a new version is available;
//     this is deliberate reactivation, not a silent terminal-preserve)
//   - all active/waiting states: reset to pending (stale in-flight state)
func ReconcileFromScan(current PackageStatus) PackageStatus {
	switch current {
	case StatusIgnored, StatusFailed:
		return current
	default:
		// installed → pending is now allowed (state machine updated).
		// All other states (pending, approved, checking_dependencies,
		// pending_dependencies, installing) also reset to pending.
		return StatusPending
	}
}

// HistoryStatus are the valid values for update_version_history.update_status.
type HistoryStatus string

const (
	HistoryInstalled = HistoryStatus("installed")
	HistoryFailed    = HistoryStatus("failed")
	HistoryRollback  = HistoryStatus("rollback")
)

