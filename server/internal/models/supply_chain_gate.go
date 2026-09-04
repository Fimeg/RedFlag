package models

import "strings"

// Supply-chain gate predicates. These are the single source of truth for "is this
// package's supply-chain posture clean enough to mint a capability token over it?"
// Both the automatic confirmation sweep (orchestrator) and the manual operator
// approval path (handlers) call them, so the two enforcement points can never drift.

// MetadataHasVulns reports whether metadata[key] records a non-empty vulnerability
// set. An unexpected shape is treated as vulnerable, conservatively.
func MetadataHasVulns(pkg UpdateState, key string) bool {
	if pkg.Metadata == nil {
		return false
	}
	v, ok := pkg.Metadata[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case []interface{}:
		return len(t) > 0
	case string:
		s := strings.TrimSpace(t)
		return s != "" && s != "[]" && s != "null"
	default:
		return true
	}
}

// ClosureChecked reports whether the resolved dependency closure was OSV-checked at
// all (closure_checked_at present and non-empty). False means OSV could not vet the
// closure — the "trust the void" case the operator must consciously accept.
func ClosureChecked(pkg UpdateState) bool {
	if pkg.Metadata == nil {
		return false
	}
	v, ok := pkg.Metadata["closure_checked_at"]
	if !ok || v == nil {
		return false
	}
	if s, isStr := v.(string); isStr && strings.TrimSpace(s) == "" {
		return false
	}
	return true
}

// ClosureCleared reports whether the resolved dependency closure was OSV-checked
// AND came back clean. A token is auto-minted over the closure only when this holds;
// the manual path requires an explicit operator override to proceed without it.
func ClosureCleared(pkg UpdateState) bool {
	return ClosureChecked(pkg) && !MetadataHasVulns(pkg, "closure_vulns")
}
