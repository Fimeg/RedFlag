package services

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sync"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services/upstream"
)

// Reconciler automatically matches agent-reported packages to tracked_software
// entries using the cascade: Repology aliases -> container image patterns ->
// exact source_ref name matching.
//
// mu + running together form a "single-flight" run-guard: mu protects the
// running flag only. ReconcileAll acquires mu briefly to check/set running,
// then releases it for the slow per-item work, then re-acquires to clear the
// flag. This prevents overlapping ReconcileAll runs without holding the lock
// across HTTP calls, DB writes, or the repology rate-limit sleep.
//
// Periodic execution is managed by the taskrunner; register ReconcileAll with
// bgRunner.Every so the runner owns the ticker, shutdown, and panic isolation.
type Reconciler struct {
	agentSWQueries *queries.AgentTrackedSoftwareQueries
	upstreamQ      *queries.UpstreamQueries
	reconcileQ     *queries.ReconciliationQueries
	aliasCache     *upstream.RepologyCache
	mu             sync.Mutex
	running        bool
}

func NewReconciler(
	atsQ *queries.AgentTrackedSoftwareQueries,
	uq *queries.UpstreamQueries,
	rq *queries.ReconciliationQueries,
	ac *upstream.RepologyCache,
) *Reconciler {
	return &Reconciler{
		agentSWQueries: atsQ,
		upstreamQ:      uq,
		reconcileQ:     rq,
		aliasCache:     ac,
	}
}

// ReconcileAll runs the full reconciliation cascade across all tracked software.
// It is single-flight: if a run is already in progress the new call returns
// immediately without blocking. mu is held only for the brief flag check/set,
// not across the slow per-item work (Repology HTTP + DB + rate-limit sleep).
func (r *Reconciler) ReconcileAll(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		log.Printf("[INFO] [reconciler] reconcile_all skipped: already running")
		return
	}
	r.running = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	rows, err := r.upstreamQ.List()
	if err != nil {
		log.Printf("[ERROR] [reconciler] list tracked_software failed: %v", err)
		return
	}

	matched := 0
	for _, sw := range rows {
		n := r.reconcileOne(ctx, sw)
		matched += n
	}
	if matched > 0 {
		log.Printf("[INFO] [reconciler] reconcile_all matched=%d tracked=%d", matched, len(rows))
	}
}

// ReconcileOne runs reconciliation for a single tracked_software entry.
// It does not participate in the ReconcileAll run-guard: on-demand single-item
// reconciliation is always allowed regardless of whether a background run is
// in progress, and does not block the background run.
func (r *Reconciler) ReconcileOne(ctx context.Context, sw models.TrackedSoftware) int {
	return r.reconcileOne(ctx, sw)
}

func (r *Reconciler) reconcileOne(ctx context.Context, sw models.TrackedSoftware) int {
	count := 0

	// Method 1: Repology alias matching
	if sw.RepologySlug != nil && *sw.RepologySlug != "" {
		r.ensureAliasCache(ctx, *sw.RepologySlug)
		aliases, err := r.aliasCache.GetAliases(*sw.RepologySlug)
		if err != nil {
			log.Printf("[WARN] [reconciler] get_aliases slug=%s err=%v", *sw.RepologySlug, err)
		} else if len(aliases) > 0 {
			matches, err := r.reconcileQ.MatchByRepology(*sw.RepologySlug, aliases)
			if err != nil {
				log.Printf("[WARN] [reconciler] match_repology slug=%s err=%v", *sw.RepologySlug, err)
			} else {
				for _, m := range matches {
					if err := r.upsertReconciled(m, "repology"); err != nil {
						log.Printf("[WARN] [reconciler] upsert_repology agent=%s pkg=%s err=%v", m.AgentID, m.PackageName, err)
					} else {
						count++
					}
				}
			}
		}
	}

	// Method 2: Container image pattern matching
	if sw.ContainerImagePattern != nil && *sw.ContainerImagePattern != "" {
		matches, err := r.reconcileQ.MatchByContainer(*sw.ContainerImagePattern)
		if err != nil {
			log.Printf("[WARN] [reconciler] match_container pattern=%s err=%v", *sw.ContainerImagePattern, err)
		} else {
			for _, m := range matches {
				if err := r.upsertReconciled(m, "container"); err != nil {
					log.Printf("[WARN] [reconciler] upsert_container agent=%s pkg=%s err=%v", m.AgentID, m.PackageName, err)
				} else {
					count++
				}
			}
		}
	}

	// Method 3: Exact source_ref matching (for npm/pypi/gem)
	if sw.Source == "npm" || sw.Source == "pypi" || sw.Source == "rubygems" {
		matches, err := r.reconcileQ.MatchByExactName(sw.SourceRef)
		if err != nil {
			log.Printf("[WARN] [reconciler] match_exact ref=%s err=%v", sw.SourceRef, err)
		} else {
			for _, m := range matches {
				if err := r.upsertReconciled(m, "exact_name"); err != nil {
					log.Printf("[WARN] [reconciler] upsert_exact agent=%s pkg=%s err=%v", m.AgentID, m.PackageName, err)
				} else {
					count++
				}
			}
		}
	}

	return count
}

func (r *Reconciler) ensureAliasCache(ctx context.Context, slug string) {
	aliases, err := r.aliasCache.GetAliases(slug)
	if err != nil || len(aliases) == 0 {
		if err := r.aliasCache.Refresh(ctx, slug); err != nil {
			log.Printf("[WARN] [reconciler] ensure_alias_cache slug=%s err=%v", slug, err)
		}
	}
}

func (r *Reconciler) upsertReconciled(m models.MatchResult, method string) error {
	input := models.AgentTrackedSoftwareInput{
		TrackedSoftwareID: m.TrackedID,
		InstalledVersion:  m.Version,
	}
	_, err := r.agentSWQueries.UpsertReconciled(m.AgentID, input, method, m.PackageName)
	// ON CONFLICT WHERE match_method='manual' skips the UPDATE and RETURNING returns
	// zero rows → sql.ErrNoRows. Treat as a no-op: the manual binding is intentional.
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
