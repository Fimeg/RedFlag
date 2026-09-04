package orchestrator

import (
	"strings"

	"github.com/Fimeg/RedFlag/server/internal/models"
)

// severityRank orders package severities from least to most urgent. A package is
// auto-approvable only when its severity is at or below the configured ceiling.
var severityRank = map[string]int{
	"low":       1,
	"moderate":  2,
	"important": 3,
	"high":      4,
	"critical":  5,
}

// severityUnknown ranks above critical so an unrecognized severity is never
// auto-approved — unknown means "a human looks at it".
const severityUnknown = 99

func rankOf(severity string) int {
	if r, ok := severityRank[strings.ToLower(strings.TrimSpace(severity))]; ok {
		return r
	}
	return severityUnknown
}

// autoApproveCeiling resolves policy.auto_approve_max_severity into a rank. The
// sentinels "off" / "none" / "disabled" / "" return 0 — nothing is at or below
// rank 0, so auto-approval is disabled. This is the safe default.
func autoApproveCeiling(setting string) int {
	switch strings.ToLower(strings.TrimSpace(setting)) {
	case "", "off", "none", "disabled":
		return 0
	}
	r := rankOf(setting)
	if r == severityUnknown {
		// An unrecognized policy value must not silently approve everything;
		// fail safe to disabled.
		return 0
	}
	return r
}

// shouldAutoApprove decides whether a pending package may be advanced without an
// operator. It is intentionally conservative: severity must be at or below the
// configured ceiling, and the package must not carry supply-chain vulnerabilities.
// It never bypasses dry-run or the capability gate — it only collapses the
// pending -> approved -> checking_dependencies wait that an operator would
// otherwise click through.
func shouldAutoApprove(pkg models.UpdateState, maxSeverity string) bool {
	ceiling := autoApproveCeiling(maxSeverity)
	if ceiling == 0 {
		return false
	}
	if rankOf(pkg.Severity) > ceiling {
		return false
	}
	if hasBlockingVulns(pkg) {
		return false
	}
	return true
}

// hasBlockingVulns reports whether the package metadata records supply-chain
// vulnerabilities on the top-level package. Auto-approval defers to the operator
// whenever vulns are present, regardless of the severity ceiling — the vuln
// review is a human call. Delegates to the shared gate predicate so auto-confirm
// and the manual approval path enforce the same rule.
func hasBlockingVulns(pkg models.UpdateState) bool {
	return models.MetadataHasVulns(pkg, "supply_chain_vulns")
}

// closureCleared reports whether the resolved dependency closure has been
// OSV-checked AND came back clean. Auto-confirmation requires both. A closure
// that was never checked (closure_checked_at absent — e.g. OSV was unreachable
// at report time) is treated as NOT cleared: fail-closed, so the capability
// token is never minted over transitive artifacts OSV did not vet. A closure
// carrying vulns is left for an operator regardless of the severity ceiling.
func closureCleared(pkg models.UpdateState) bool {
	return models.ClosureCleared(pkg)
}
