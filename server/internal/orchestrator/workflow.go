package orchestrator

import (
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// sweepAutoApprove advances eligible pending packages without an operator. Policy
// is consulted per package; a qualifying package is approved and its dry-run is
// enqueued (approved -> checking_dependencies). Auto-approval stops there: the
// install phase still requires the dry-run result and the maintenance window
// (enforced at InstallUpdate), and on the capability path the supply-chain gate
// still mints and verifies the token. Auto-approval never reaches installing on
// its own, and never runs when dry-runs are disabled by policy.
func (o *Orchestrator) sweepAutoApprove() {
	maxSeverity := o.settings.GetPolicyString("auto_approve_max_severity", "off")
	if autoApproveCeiling(maxSeverity) == 0 {
		return // disabled — the safe default
	}
	if !o.settings.GetPolicyBool("allow_dry_runs", true) {
		// The operator has taken manual control of the dry-run step; auto-approval
		// would strand packages in approved with no way to advance. Stay out.
		return
	}

	pending, err := o.store.GetPackagesInStatus(models.StatusPending)
	if err != nil {
		log.Printf("[ERROR] [server] [orchestrator] sweep_auto_approve_failed error=%v", err)
		return
	}
	approved := 0
	for _, pkg := range pending {
		if !shouldAutoApprove(pkg, maxSeverity) {
			continue
		}
		if err := o.advanceApproved(pkg); err != nil {
			log.Printf("[ERROR] [server] [orchestrator] auto_approve_failed update_id=%s error=%v", pkg.ID, err)
			continue
		}
		approved++
	}
	if approved > 0 {
		log.Printf("[INFO] [server] [orchestrator] auto_approved count=%d max_severity=%s", approved, maxSeverity)
	}
}

// advanceApproved approves a package and enqueues its dry-run. Both steps are
// individually idempotent: the guarded transition no-ops if the row already
// moved, and a duplicate dry_run_update command is harmless (the agent reports
// against update_id, and DryRun is read-only).
func (o *Orchestrator) advanceApproved(pkg models.UpdateState) error {
	meta := models.JSONB{"approved_by": "orchestrator", "auto_approved": true}
	if err := o.store.TransitionByID(pkg.ID, models.StatusApproved, meta); err != nil {
		return fmt.Errorf("approve: %w", err)
	}
	p := pkg
	p.Status = models.StatusApproved
	if err := o.enqueuer.EnqueueDryRun(&p); err != nil {
		return fmt.Errorf("enqueue dry-run: %w", err)
	}
	aid := pkg.AgentID
	o.emitEvent(&aid, "orchestrator", "auto_approved", "info", "orchestrator",
		"Package auto-approved by orchestrator",
		map[string]interface{}{
			"update_id":    pkg.ID.String(),
			"package_type": pkg.PackageType,
			"package_name": pkg.PackageName,
		})
	return nil
}

// sweepAutoConfirm advances capability-gated packages waiting in
// pending_dependencies to installing and mints their capability token, without
// an operator click — the dependency-confirmation half of the auto-approve
// lane. It is intentionally stateless: rather than relying on a persisted
// "auto-approved" marker (the approve-time history metadata is not carried on
// the row), it re-applies the same auto-approval policy here. A package is
// auto-confirmed only when:
//
//   - auto-approval policy is enabled and the package is at or below the
//     severity ceiling with no top-level supply-chain vulns (shouldAutoApprove), and
//   - the resolved dependency closure has been OSV-checked and came back clean
//     (closureCleared) — fail-closed: an unchecked or vulnerable closure is
//     never auto-minted, since the closure is the exact artifact set the token
//     authorizes, and
//   - dry-runs are not under manual operator control (allow_dry_runs), and
//   - the current time is inside a maintenance window (installs respect the
//     window even though the read-only dry-run that preceded this did not), and
//   - the package is capability-gated (dnf/apt) — legacy command packages are
//     left for the operator, since the server can't verify a legacy dependency.
//
// The mint itself is idempotent (an active token short-circuits the mint), and
// the install transition is guarded, so a redundant pass is a no-op.
func (o *Orchestrator) sweepAutoConfirm(now time.Time) {
	if o.confirmer == nil || o.window == nil {
		return // not wired (e.g. minter disabled) — closures wait for an operator
	}
	maxSeverity := o.settings.GetPolicyString("auto_approve_max_severity", "off")
	if autoApproveCeiling(maxSeverity) == 0 {
		return // auto-approval disabled — the safe default
	}
	if !o.settings.GetPolicyBool("allow_dry_runs", true) {
		return // operator has taken manual control of the dry-run/confirm lane
	}

	open, err := o.window.IsWithinMaintenanceWindow(now)
	if err != nil {
		// Fail closed: do not auto-confirm installs when the window can't be evaluated.
		log.Printf("[ERROR] [server] [orchestrator] auto_confirm_window_check_failed error=%v", err)
		return
	}
	if !open {
		return // outside the maintenance window — installs wait, dry-run already done
	}

	waiting, err := o.store.GetPackagesInStatus(models.StatusPendingDependencies)
	if err != nil {
		log.Printf("[ERROR] [server] [orchestrator] sweep_auto_confirm_failed error=%v", err)
		return
	}
	confirmed := 0
	for _, pkg := range waiting {
		if !shouldAutoApprove(pkg, maxSeverity) {
			continue // too severe, or top-level vulns — a human confirms this one
		}
		if !closureCleared(pkg) {
			// Closure not yet OSV-checked (will retry), or it carries vulns in a
			// transitive artifact — never auto-mint over an unvetted/vulnerable
			// closure. The dependency vulns are recorded on the row for the operator.
			continue
		}
		p := pkg
		handled, err := o.confirmer.ConfirmDependenciesAuto(&p)
		if err != nil {
			// The confirmer has already marked the package failed; just record it.
			log.Printf("[ERROR] [server] [orchestrator] auto_confirm_failed update_id=%s package=%s/%s error=%v",
				pkg.ID, pkg.PackageType, pkg.PackageName, err)
			continue
		}
		if !handled {
			continue // legacy command path — left for the operator
		}
		confirmed++
		log.Printf("[INFO] [server] [orchestrator] auto_confirmed update_id=%s package=%s/%s",
			pkg.ID, pkg.PackageType, pkg.PackageName)
		aid := pkg.AgentID
		o.emitEvent(&aid, "orchestrator", "auto_confirmed", "info", "orchestrator",
			"Dependencies auto-confirmed; capability token minted",
			map[string]interface{}{
				"update_id":    pkg.ID.String(),
				"package_type": pkg.PackageType,
				"package_name": pkg.PackageName,
			})
	}
	if confirmed > 0 {
		log.Printf("[INFO] [server] [orchestrator] auto_confirm_minted count=%d max_severity=%s", confirmed, maxSeverity)
	}
}

// AdvanceLifecycle is a synchronous, targeted entry point: given a freshly
// discovered package id, it auto-approves immediately if policy allows, instead
// of waiting for the next timer sweep. Best-effort and idempotent — it no-ops
// unless the package is still pending and qualifies. Report-driven transitions
// (dependency results, capability results) remain in their handlers, which the
// LIFECYCLE-001 guarded transitions already make safe; folding those into the
// orchestrator is tracked LIFECYCLE-003 follow-up.
func (o *Orchestrator) AdvanceLifecycle(updateID uuid.UUID) {
	pkg, err := o.store.GetUpdateByID(updateID)
	if err != nil {
		log.Printf("[ERROR] [server] [orchestrator] advance_load_failed update_id=%s error=%v", updateID, err)
		return
	}
	if pkg.Status != models.StatusPending {
		return
	}
	if !o.settings.GetPolicyBool("allow_dry_runs", true) {
		return
	}
	maxSeverity := o.settings.GetPolicyString("auto_approve_max_severity", "off")
	if !shouldAutoApprove(*pkg, maxSeverity) {
		return
	}
	if err := o.advanceApproved(*pkg); err != nil {
		log.Printf("[ERROR] [server] [orchestrator] advance_auto_approve_failed update_id=%s error=%v", updateID, err)
	}
}
