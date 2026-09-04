package orchestrator

import (
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
)

// dryRunRetryKey is the metadata counter the orchestrator uses to bound dry-run
// re-enqueue attempts on a stuck package, without holding state of its own.
const dryRunRetryKey = "orchestrator_dry_run_retries"

// sweepCheckingDependencies recovers packages stuck waiting on a dry-run report.
// A package that has sat in checking_dependencies past the threshold had its
// dry_run_update command lost or its agent-side dry-run fail silently. Re-enqueue
// the dry-run up to the retry budget; once spent, fail the package so it stops
// occupying an active state (ETHOS §3 — every active state has a timeout path).
func (o *Orchestrator) sweepCheckingDependencies(now time.Time) {
	threshold := time.Duration(o.settings.GetOperationalInt("orchestrator_checking_timeout_minutes", 30)) * time.Minute
	maxRetries := o.settings.GetOperationalInt("orchestrator_dry_run_max_retries", 2)

	stuck, err := o.store.GetPackagesInStatus(models.StatusCheckingDependencies)
	if err != nil {
		log.Printf("[ERROR] [server] [orchestrator] sweep_checking_failed error=%v", err)
		return
	}
	for _, pkg := range stuck {
		if now.Sub(pkg.LastUpdatedAt) < threshold {
			continue
		}
		attempts, err := o.store.BumpRetryCounter(pkg.ID, dryRunRetryKey)
		if err != nil {
			log.Printf("[ERROR] [server] [orchestrator] dry_run_retry_bump_failed update_id=%s error=%v", pkg.ID, err)
			continue
		}
		if attempts > maxRetries {
			meta := models.JSONB{
				"failure_reason":   "dry-run never reported; retry budget exhausted",
				"failed_by":        "orchestrator",
				"dry_run_attempts": attempts - 1,
			}
			if err := o.store.TransitionByID(pkg.ID, models.StatusFailed, meta); err != nil {
				log.Printf("[ERROR] [server] [orchestrator] checking_fail_transition_failed update_id=%s error=%v", pkg.ID, err)
				continue
			}
			log.Printf("[INFO] [server] [orchestrator] checking_dependencies_failed update_id=%s package=%s/%s attempts=%d",
				pkg.ID, pkg.PackageType, pkg.PackageName, attempts-1)
			aid := pkg.AgentID
			o.emitEvent(&aid, "orchestrator", "checking_dependencies_timeout", "error", "orchestrator",
				"Dry-run never reported; retry budget exhausted", map[string]interface{}{
					"update_id":      pkg.ID.String(),
					"package_type":   pkg.PackageType,
					"package_name":   pkg.PackageName,
					"dry_run_attempts": attempts - 1,
				})
			continue
		}
		p := pkg
		if err := o.enqueuer.EnqueueDryRun(&p); err != nil {
			log.Printf("[ERROR] [server] [orchestrator] dry_run_requeue_failed update_id=%s error=%v", pkg.ID, err)
			continue
		}
		log.Printf("[INFO] [server] [orchestrator] dry_run_requeued update_id=%s package=%s/%s attempt=%d",
			pkg.ID, pkg.PackageType, pkg.PackageName, attempts)
	}
}

// sweepInstalling fails packages that entered installing but never reported a
// terminal result within the window (helper crashed mid-install, agent died, or
// the token was consumed but the result was lost). The 1h default matches the
// command-layer sent-timeout so the two layers don't fight over the same row.
func (o *Orchestrator) sweepInstalling(now time.Time) {
	threshold := time.Duration(o.settings.GetOperationalInt("orchestrator_installing_timeout_minutes", 60)) * time.Minute
	stuck, err := o.store.GetPackagesInStatus(models.StatusInstalling)
	if err != nil {
		log.Printf("[ERROR] [server] [orchestrator] sweep_installing_failed error=%v", err)
		return
	}
	for _, pkg := range stuck {
		if now.Sub(pkg.LastUpdatedAt) < threshold {
			continue
		}
		meta := models.JSONB{
			"failure_reason": fmt.Sprintf("install did not report a result within %s", threshold),
			"failed_by":      "orchestrator",
		}
		if err := o.store.TransitionByID(pkg.ID, models.StatusFailed, meta); err != nil {
			log.Printf("[ERROR] [server] [orchestrator] installing_fail_transition_failed update_id=%s error=%v", pkg.ID, err)
			continue
		}
		log.Printf("[INFO] [server] [orchestrator] installing_timed_out update_id=%s package=%s/%s threshold=%s",
			pkg.ID, pkg.PackageType, pkg.PackageName, threshold)
		aid := pkg.AgentID
		o.emitEvent(&aid, "orchestrator", "installing_timeout", "error", "orchestrator",
			"Install did not report a result within threshold",
			map[string]interface{}{
				"update_id":    pkg.ID.String(),
				"package_type": pkg.PackageType,
				"package_name": pkg.PackageName,
				"threshold":    threshold.String(),
			})
	}
}

// sweepPendingDependencies surfaces packages an operator left unconfirmed for a
// long time. Per design this is NOT a state change — the operator deliberately
// deferred, and the package ages out on the next scan. We only log the count so
// the condition is visible; the operator-facing warning indicator is LIFECYCLE-004.
func (o *Orchestrator) sweepPendingDependencies(now time.Time) {
	threshold := time.Duration(o.settings.GetOperationalInt("orchestrator_pending_deps_warn_hours", 24)) * time.Hour
	stuck, err := o.store.GetPackagesInStatus(models.StatusPendingDependencies)
	if err != nil {
		log.Printf("[ERROR] [server] [orchestrator] sweep_pending_deps_failed error=%v", err)
		return
	}
	stale := 0
	for _, pkg := range stuck {
		if now.Sub(pkg.LastUpdatedAt) >= threshold {
			stale++
		}
	}
	if stale > 0 {
		log.Printf("[INFO] [server] [orchestrator] pending_dependencies_stale count=%d threshold=%s", stale, threshold)
	}
}
