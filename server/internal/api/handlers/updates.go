package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/capability"
	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/Fimeg/RedFlag/server/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// isValidResult checks if the result value complies with the database constraint.
// The constraint on update_logs.result is the authority (see migration 048); keep
// this map in sync.
func isValidResult(result string) bool {
	validResults := map[string]bool{
		"success": true,
		"failed":  true,
		"partial": true,
		"started": true,
		"running": true,
	}
	return validResults[result]
}

// UpdateHandler handles package update operations
// DEPRECATED: This handler is being consolidated - will be replaced by unified update handling
type UpdateHandler struct {
	updateQueries            *queries.UpdateQueries
	agentQueries             *queries.AgentQueries
	commandQueries           *queries.CommandQueries
	agentHandler             *AgentHandler
	maintenanceWindowQueries *queries.MaintenanceWindowQueries
	securitySettings         *services.SecuritySettingsService // optional; reads policy.allow_dry_runs
	config                   *config.Config                    // optional; for self-referential URLs
	minter                   *services.CapabilityMinter        // optional; mints capability tokens at approval
	tokenQueries             *queries.CapabilityTokenQueries   // optional; delivers minted tokens to agents
	orchestrator             LifecycleAdvancer                 // optional; auto-approve fast-path after a scan
	reconciler               ReconcilerTrigger                 // optional; auto-bind tracked software after a scan
	runner                   BackgroundRunner                  // optional; bounded fire-and-forget pool (SCALE-001 S2)
}

// BackgroundRunner is the bounded fire-and-forget pool (SCALE-001 S6/S2). The
// report path used to launch raw `go func()` calls with no backpressure; routing
// them through a runner caps steady-state concurrency and makes saturation
// observable. Implemented by *taskrunner.Runner.
type BackgroundRunner interface {
	Go(name string, fn func())
}

// LifecycleAdvancer is the orchestrator hook the update handlers fire after a
// scan report lands, so newly-discovered packages can be auto-approved
// immediately instead of waiting for the next orchestrator timer tick. It is
// declared here (not imported from the orchestrator package) so the dependency
// runs one way: orchestrator -> handlers, never back.
type LifecycleAdvancer interface {
	OnPackagesDiscovered()
	// OnDependenciesReported is fired after an agent reports a dependency
	// closure (the package has just entered pending_dependencies), so an
	// eligible capability-gated package is auto-confirmed and its token minted
	// without waiting for the next orchestrator timer tick.
	OnDependenciesReported(updateID uuid.UUID)
}

// ReconcilerTrigger is the hook the update handler fires after new packages
// are discovered so the reconciler can attempt automatic binding.
type ReconcilerTrigger interface {
	ReconcileAll(ctx context.Context)
}

func NewUpdateHandler(uq *queries.UpdateQueries, aq *queries.AgentQueries, cq *queries.CommandQueries, ah *AgentHandler, mwq *queries.MaintenanceWindowQueries, cfg *config.Config) *UpdateHandler {
	return &UpdateHandler{
		updateQueries:            uq,
		agentQueries:             aq,
		commandQueries:           cq,
		agentHandler:             ah,
		maintenanceWindowQueries: mwq,
		config:                   cfg,
	}
}

// SetSecuritySettings injects the settings service so this handler can read
// policy.allow_dry_runs at request time. Nil leaves the strict default (dry
// run required) in place.
func (h *UpdateHandler) SetSecuritySettings(s *services.SecuritySettingsService) {
	h.securitySettings = s
}

// SetCapabilityMinter wires the capability-token minter and token store. When
// both are set, approval mints a signed token over the package closure and the
// delivery endpoint serves it to the agent. Nil leaves the gate inactive.
func (h *UpdateHandler) SetCapabilityMinter(m *services.CapabilityMinter, tq *queries.CapabilityTokenQueries) {
	h.minter = m
	h.tokenQueries = tq
}

// SetOrchestrator wires the lifecycle orchestrator's synchronous discovery hook.
// Nil leaves auto-approval to the orchestrator's own timer sweep.
func (h *UpdateHandler) SetOrchestrator(o LifecycleAdvancer) {
	h.orchestrator = o
}

// SetReconciler wires the automatic reconciliation service.
func (h *UpdateHandler) SetReconciler(r ReconcilerTrigger) {
	h.reconciler = r
}

// SetTaskRunner wires the bounded background pool. Nil falls back to launching a
// plain goroutine per task (legacy unbounded behavior).
func (h *UpdateHandler) SetTaskRunner(r BackgroundRunner) {
	h.runner = r
}

// bg runs fn on the bounded pool when a runner is wired, else as a plain
// goroutine. Keeps the report path's fire-and-forget work bounded and named.
func (h *UpdateHandler) bg(name string, fn func()) {
	if h.runner != nil {
		h.runner.Go(name, fn)
		return
	}
	go fn()
}

// EnqueueDryRun creates and signs the dry_run_update command for a package and
// advances it to checking_dependencies, queuing a short heartbeat so the agent
// polls promptly. Shared by the operator-triggered dry-run endpoint and the
// orchestrator's auto-approve path. Policy (allow_dry_runs) and maintenance-window
// checks are the caller's responsibility — by the time this runs, the decision to
// dry-run has been made.
func (h *UpdateHandler) EnqueueDryRun(update *models.UpdateState) error {
	// GATE-005: use selected_version when set, else available_version.
	// "target_version" is the version to install; "available_version" carries the
	// latest upstream version so the agent can compare for freshness checks.
	params := map[string]interface{}{
		"update_id":         update.ID.String(),
		"package_name":      update.PackageName,
		"package_type":      update.PackageType,
		"available_version": update.AvailableVersion,
	}
	// Target precedence: manual selected_version wins; installed_hold keeps the
	// installed version pinned until resolveGatedTarget finds a newer version that
	// cleared soak. Absent either, RedFlag auto-pins the soak-gated version
	// (forward-only, block-enforced). [GATE-006 B] Falling through to no
	// target_version means the agent installs available_version, the legacy default.
	if targetVersion, explicit := h.resolveInstallTarget(update); explicit {
		params["target_version"] = targetVersion
	}
	command := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     update.AgentID,
		CommandType: models.CommandTypeDryRunUpdate,
		Params:      params,
		Status:      models.CommandStatusPending,
		Source:      models.CommandSourceManual,
		CreatedAt:   time.Now().UTC(),
	}

	// Collapse the agent's poll interval during the active phase (best-effort).
	if shouldEnable, err := h.shouldEnableHeartbeat(update.AgentID, 10); err == nil && shouldEnable {
		heartbeatCmd := &models.AgentCommand{
			ID:          uuid.Must(uuid.NewV4()),
			AgentID:     update.AgentID,
			CommandType: models.CommandTypeEnableHeartbeat,
			Params:      models.JSONB{"duration_minutes": 10},
			Status:      models.CommandStatusPending,
			Source:      models.CommandSourceSystem,
			CreatedAt:   time.Now().UTC(),
		}
		if err := h.agentHandler.signAndCreateCommand(heartbeatCmd); err != nil {
			log.Printf("[WARNING] [server] [updates] heartbeat_create_failed agent_id=%s error=%v", update.AgentID, err)
		}
	}

	if err := h.agentHandler.signAndCreateCommand(command); err != nil {
		return fmt.Errorf("create dry run command: %w", err)
	}
	if err := h.updateQueries.SetCheckingDependencies(update.ID); err != nil {
		return fmt.Errorf("set checking_dependencies: %w", err)
	}
	return nil
}

// shouldEnableHeartbeat checks if heartbeat is already active for an agent
// Returns true if heartbeat should be enabled (i.e., not already active or expired)
func (h *UpdateHandler) shouldEnableHeartbeat(agentID uuid.UUID, durationMinutes int) (bool, error) {
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		log.Printf("Warning: Failed to get agent %s for heartbeat check: %v", agentID, err)
		return true, nil // Enable heartbeat by default if we can't check
	}

	// Check if rapid polling is already enabled and not expired
	if enabled, ok := agent.Metadata["rapid_polling_enabled"].(bool); ok && enabled {
		if untilStr, ok := agent.Metadata["rapid_polling_until"].(string); ok {
			until, err := time.Parse(time.RFC3339, untilStr)
			if err == nil && until.After(time.Now().UTC().Add(5*time.Minute)) {
				// Heartbeat is already active for sufficient time
				log.Printf("[Heartbeat] Agent %s already has active heartbeat until %s (skipping)", agentID, untilStr)
				return false, nil
			}
		}
	}

	return true, nil
}

// ReportUpdates handles update reports from agents using event sourcing
func (h *UpdateHandler) ReportUpdates(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	var req models.UpdateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert update report items to events
	events := make([]models.UpdateEvent, 0, len(req.Updates))
	for _, item := range req.Updates {
		// Merge agent-reported top-level fields into metadata JSONB so they
		// survive through to current_package_state for UI enrichment.
		meta := item.Metadata
		if item.PackageDescription != "" || len(item.CVEList) > 0 || item.KBID != "" || item.SizeBytes > 0 {
			if meta == nil {
				meta = make(models.JSONB)
			}
			if item.PackageDescription != "" {
				meta["description"] = item.PackageDescription
			}
			if len(item.CVEList) > 0 {
				meta["cve_list"] = item.CVEList
			}
			if item.KBID != "" {
				meta["kb_id"] = item.KBID
			}
			if item.SizeBytes > 0 {
				meta["size_bytes"] = item.SizeBytes
			}
		}
		event := models.UpdateEvent{
			ID:               uuid.Must(uuid.NewV4()),
			AgentID:          agentID,
			PackageType:      item.PackageType,
			PackageName:      item.PackageName,
			VersionFrom:      item.CurrentVersion,
			VersionTo:        item.AvailableVersion,
			Severity:         item.Severity,
			RepositorySource: item.RepositorySource,
			Metadata:         meta,
			EventType:        "discovered",
			CreatedAt:        req.Timestamp,
		}
		events = append(events, event)
	}

	// Store events in batch with error isolation
	if err := h.updateQueries.CreateUpdateEventsBatch(events); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record update events"})
		return
	}

	// Best-effort: record each offered version in the timeline catalog so the package
	// detail pane and as-of-date resolution have history to read. A failure here must
	// not fail the scan report.
	for _, item := range req.Updates {
		pv := &models.PackageVersion{
			PackageType: item.PackageType,
			PackageName: item.PackageName,
			Version:     item.AvailableVersion,
		}
		if item.RepositorySource != "" {
			src := item.RepositorySource
			pv.Source = &src
		}
		if err := h.updateQueries.UpsertPackageVersion(pv); err != nil {
			log.Printf("[WARNING] [server] [versions] catalog_upsert_failed pkg=%s version=%s error=%v",
				item.PackageName, item.AvailableVersion, err)
		}
	}

	// Best-effort OSV.dev check at discovery time — runs async so it never
	// blocks the agent report. Deduped: same pkg+version is only queried once
	// per server lifetime. Results (vulns or clean) are written back to the
	// current_package_state metadata so the UI's "Known Vulnerabilities" card
	// shows them before the operator approves.
	h.bg("osv_checks", func() { enqueueOSVChecks(events, h.updateQueries) })

	// Fire the orchestrator's auto-approve fast-path so packages eligible by
	// policy advance without waiting for the next timer sweep. Non-blocking and
	// best-effort — the timer sweep is the backstop.
	if h.orchestrator != nil {
		h.orchestrator.OnPackagesDiscovered()
	}

	// Fire the reconciler so newly-reported packages get automatic bindings.
	if h.reconciler != nil {
		h.bg("reconcile_all", func() { h.reconciler.ReconcileAll(context.Background()) })
	}

	// RECONCILE-001: scan-set closure (close-by-absence).
	// Only fires when the agent confirmed a successful (exit 0), complete scan
	// of a scoped ecosystem (dnf, apt). A failed/partial scan MUST NOT close rows
	// — presence of the package in a failed scan means nothing about its current
	// install state. Docker is explicitly excluded: image-set semantics differ
	// from OS package-manager semantics and will be handled separately.
	if req.ScanSucceeded && scanEcosystemSupported(req.Ecosystem) {
		// Build the set of package names present in this scan (the reported set).
		reportedSet := make(map[string]struct{}, len(req.Updates))
		for _, item := range req.Updates {
			reportedSet[item.PackageName] = struct{}{}
		}
		// Run closure async so it does not delay the HTTP response to the agent.
		// Errors are logged; they do not fail the report (additive ingest already
		// committed). The reconciler is not on the critical path.
		h.bg("close_scan_absent", func() { h.closeScanAbsentRows(agentID, req.Ecosystem, reportedSet) })
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "update events recorded",
		"count":      len(events),
		"command_id": req.CommandID,
	})
}

// scanEcosystemSupported returns true for the OS package managers whose scan
// semantics guarantee a complete list of available updates on success.
// Docker is explicitly false: image sets have different update semantics
// (digest pinning vs. available-upgrade lists) and will be handled separately.
func scanEcosystemSupported(ecosystem string) bool {
	switch ecosystem {
	case "dnf", "apt", "pacman":
		return true
	default:
		return false
	}
}

// closeScanAbsentRows is the core of RECONCILE-001: scan-set closure.
// It computes waiting_rows(agent, ecosystem) − reported_set = to_close,
// then transitions each absent row to installed via the state machine.
//
// Closure scope is the *waiting* states only (pending, approved) — enforced by
// GetTrackedNonResting. In-flight states (checking_dependencies, pending_dependencies,
// installing) are owned by the orchestrator + capability-receipt path and are left alone,
// so the reconciler never races a RedFlag-driven install nor mislabels its provenance.
// A waiting row is out-of-band by construction (RedFlag had not begun executing it):
// capability tokens are only minted after checking_dependencies, a state a waiting row
// has not reached, so it can hold no active token for its current version. Closures here
// are therefore always stamped out_of_band — see the provenance note below.
//
// Safety invariants (ETHOS §3):
//   - Only called when req.ScanSucceeded is true (caller enforces).
//   - Only for supported ecosystems (caller enforces via scanEcosystemSupported).
//   - Each closure routes through transitionStatus (guarded UPDATE with WHERE
//     status = $current), so concurrent racing callers land on the constraint,
//     not a silent overwrite. pending/approved -> installed is a permitted edge.
//   - Idempotent: running 3× produces the same result (ETHOS §4).
func (h *UpdateHandler) closeScanAbsentRows(agentID uuid.UUID, ecosystem string, reportedSet map[string]struct{}) {
	// Load all non-resting rows for this (agent, ecosystem).
	tracked, err := h.updateQueries.GetTrackedNonResting(agentID, ecosystem)
	if err != nil {
		log.Printf("[ERROR] [server] [reconcile] get_tracked_non_resting_failed agent=%s ecosystem=%s error=%v",
			agentID, ecosystem, err)
		return
	}
	if len(tracked) == 0 {
		return
	}

	// Diff: tracked − reported = to_close.
	for _, row := range tracked {
		if _, present := reportedSet[row.PackageName]; present {
			// Still in scan — keep / version-bump handled by UpdateCurrentStateInTx.
			continue
		}

		// Package absent from scan: this closure is out_of_band by construction.
		// We deliberately do NOT consult consumed capability tokens here. The row ID
		// is preserved across the UPSERT on (agent_id, package_type, package_name), so
		// a consumed token from a *prior* lifecycle (when this row was last installed
		// via the receipt path) stays linked to the same ID. A waiting-state row cannot
		// have a token for its *current* version — tokens are minted only after
		// checking_dependencies, which a waiting row has not reached — so any consumed
		// token we'd find is stale and would mislabel a genuine out-of-band closure as
		// redflag_receipt. Always stamp out_of_band.
		meta := models.JSONB{
			"resolution_provenance": "out_of_band",
			"closed_by":             "scan_set_reconciler",
			"ecosystem":             ecosystem,
			"closed_at":             time.Now().UTC().Format(time.RFC3339),
		}

		if err := h.updateQueries.TransitionByID(row.ID, models.StatusInstalled, meta); err != nil {
			// A concurrent state-machine move (operator approved, installed, etc.)
			// may have already advanced the row — that is not an error here.
			log.Printf("[INFO] [server] [reconcile] close_skipped update_id=%s package=%s/%s ecosystem=%s error=%v",
				row.ID, row.PackageType, row.PackageName, ecosystem, err)
			continue
		}

		log.Printf("[INFO] [server] [reconcile] closed_by_absence update_id=%s package=%s/%s ecosystem=%s provenance=out_of_band",
			row.ID, row.PackageType, row.PackageName, ecosystem)

		// Emit a system event per closure for auditability (ETHOS §1, history table).
		// Best-effort: a failure here must not rollback the transition already committed.
		if h.agentQueries != nil {
			ev := &models.SystemEvent{
				ID:           uuid.Must(uuid.NewV4()),
				AgentID:      &agentID,
				EventType:    models.EventTypeAgentUpdate,
				EventSubtype: models.SubtypeSuccess,
				Severity:     models.SeverityInfo,
				Component:    models.ComponentServer,
				Message: fmt.Sprintf(
					"scan-set closure: %s/%s absent from %s scan, transitioned to installed (provenance: out_of_band)",
					row.PackageType, row.PackageName, ecosystem,
				),
				Metadata:  meta,
				CreatedAt: time.Now().UTC(),
			}
			if err := h.agentQueries.CreateSystemEvent(ev); err != nil {
				log.Printf("[WARNING] [server] [reconcile] system_event_failed update_id=%s error=%v",
					row.ID, err)
			}
		}
	}
}

// enqueueOSVChecks checks each unique package+version in the batch against
// OSV.dev with bounded concurrency, persisting every outcome (clean, vuln, or
// recorded failure) to current_package_state.metadata. Packages whose stored
// result is still fresh (per OSVRecheckInterval) are skipped via the persisted
// timestamp — not a process-local cache — so the dedup survives restart and a
// failed check is retried next cycle.
func enqueueOSVChecks(events []models.UpdateEvent, q *queries.UpdateQueries) {
	type dedupKey struct {
		agentID                   uuid.UUID
		pkgType, pkgName, version string
	}
	seen := make(map[dedupKey]bool)

	// freshByAgent caches freshness results per agent per namespace to avoid
	// repeated DB queries within one batch.
	type agentNS struct {
		id        uuid.UUID
		namespace string
	}
	freshByAgent := make(map[agentNS]map[string]bool)

	getFresh := func(agentID uuid.UUID, ns string) map[string]bool {
		key := agentNS{agentID, ns}
		if m, ok := freshByAgent[key]; ok {
			return m
		}
		f, err := q.FreshSupplyChainPackages(agentID, services.OSVRecheckInterval, ns)
		if err != nil {
			log.Printf("[WARNING] [supply_chain] fresh_query_failed agent=%s namespace=%s error=%v", agentID, ns, err)
			f = map[string]bool{}
		}
		freshByAgent[key] = f
		return f
	}

	var reqs []services.OSVCheckRequest

	for _, e := range events {
		if !services.NeedsSupplyChainCheck(e.PackageType) {
			continue
		}

		// Remediation check — available version.
		k := dedupKey{e.AgentID, e.PackageType, e.PackageName, e.VersionTo}
		if !seen[k] {
			seen[k] = true
			if !getFresh(e.AgentID, "")[osvFreshKey(e.PackageType, e.PackageName, e.VersionTo)] {
				reqs = append(reqs, services.OSVCheckRequest{
					AgentID: e.AgentID,
					PkgType: e.PackageType,
					PkgName: e.PackageName,
					Version: e.VersionTo,
				})
			}
		}

		// Threat check — installed version. Skip if blank or same as available
		// (no new information) or already fresh.
		if e.VersionFrom == "" || e.VersionFrom == e.VersionTo {
			continue
		}
		ki := dedupKey{e.AgentID, e.PackageType, e.PackageName, e.VersionFrom}
		if seen[ki] {
			continue
		}
		seen[ki] = true
		if !getFresh(e.AgentID, "installed")[osvFreshKey(e.PackageType, e.PackageName, e.VersionFrom)] {
			reqs = append(reqs, services.OSVCheckRequest{
				AgentID:   e.AgentID,
				PkgType:   e.PackageType,
				PkgName:   e.PackageName,
				Version:   e.VersionFrom,
				Namespace: "installed",
			})
		}
	}

	if len(reqs) == 0 {
		return
	}
	services.RunOSVChecks(reqs, func(agentID uuid.UUID, pkgType, pkgName string, meta map[string]interface{}) error {
		return storeOSVCheckResult(q, agentID, pkgType, pkgName, meta)
	})
}

func osvFreshKey(pkgType, pkgName, version string) string {
	return pkgType + "\x00" + pkgName + "\x00" + version
}

func storeOSVCheckResult(q *queries.UpdateQueries, agentID uuid.UUID, pkgType, pkgName string, meta map[string]interface{}) error {
	if err := q.StoreSupplyChainMetadata(agentID, pkgType, pkgName, models.JSONB(meta)); err != nil {
		return err
	}
	if pv := packageVersionFromOSVMeta(pkgType, pkgName, meta); pv != nil {
		if err := q.UpsertPackageVersion(pv); err != nil {
			log.Printf("[WARNING] [server] [versions] osv_catalog_failed pkg=%s version=%s error=%v", pkgName, pv.Version, err)
		}
	}
	return nil
}

func packageVersionFromOSVMeta(pkgType, pkgName string, meta map[string]interface{}) *models.PackageVersion {
	version, vulnsKey := stringMeta(meta["supply_chain_checked_version"]), "supply_chain_vulns"
	if version == "" {
		version, vulnsKey = stringMeta(meta["installed_checked_version"]), "installed_vulns"
	}
	if version == "" {
		return nil
	}

	status := "clean"
	var rawVulns *string
	if raw := vulnMetaString(meta[vulnsKey]); raw != "" {
		status = "vulnerable"
		rawVulns = &raw
	}
	return &models.PackageVersion{
		PackageType: pkgType,
		PackageName: pkgName,
		Version:     version,
		OSVStatus:   &status,
		OSVVulns:    rawVulns,
	}
}

func stringMeta(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func vulnMetaString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		s := strings.TrimSpace(t)
		if s == "" || s == "[]" || s == "null" {
			return ""
		}
		return s
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		s := strings.TrimSpace(string(raw))
		if s == "" || s == "[]" || s == "null" {
			return ""
		}
		return s
	}
}

// ListUpdates retrieves updates with filtering using the new state table
func (h *UpdateHandler) ListUpdates(c *gin.Context) {
	// Parse and validate optional status filter
	var statusFilter models.PackageStatus
	if s := c.Query("status"); s != "" {
		if parsed, err := models.StatusFromString(s); err == nil {
			statusFilter = parsed
		}
	}
	filters := &models.UpdateFilters{
		Status:      statusFilter,
		Severity:    c.Query("severity"),
		PackageType: c.Query("package_type"),
	}

	// Parse agent_id if provided
	if agentIDStr := c.Query("agent_id"); agentIDStr != "" {
		agentID, err := uuid.FromString(agentIDStr)
		if err == nil {
			filters.AgentID = agentID
		}
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	filters.Page = page
	filters.PageSize = pageSize

	updates, total, err := h.updateQueries.ListUpdatesFromState(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list updates"})
		return
	}

	// Get overall statistics for the summary cards
	stats, err := h.updateQueries.GetAllUpdateStats()
	if err != nil {
		// Don't fail the request if stats fail, just log and continue
		// In production, we'd use proper logging
		stats = &models.UpdateStats{}
	}

	c.JSON(http.StatusOK, gin.H{
		"updates":   updates,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"stats":     stats,
	})
}

// severityLabelForRank maps the SQL severity rank back to a label for the UI.
func severityLabelForRank(rank int) string {
	switch rank {
	case 4:
		return "critical"
	case 3:
		return "high"
	case 2:
		return "medium"
	case 1:
		return "low"
	default:
		return "unknown"
	}
}

// ListPackages returns the fleet rolled up by package (one row per package_type +
// package_name across all agents) for the package-centric Updates view. Each row
// carries agent/version counts, a max-severity label, vuln + hash-pin rollups, and a
// representative update id for drill-in. Shares the overall stat cards with ListUpdates.
func (h *UpdateHandler) ListPackages(c *gin.Context) {
	search := c.Query("search")
	packageType := c.Query("type")
	if packageType == "" {
		packageType = c.Query("package_type")
	}
	status := c.Query("status")
	sortBy := c.Query("sort_by")
	sortOrder := c.Query("sort_order")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))

	rows, total, err := h.updateQueries.ListAggregatedPackages(search, packageType, status, sortBy, sortOrder, page, pageSize)
	if err != nil {
		log.Printf("[ERROR] [server] [updates] list_packages_failed error=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list packages"})
		return
	}

	type packageRow struct {
		queries.AggregatedPackage
		MaxSeverity string `json:"max_severity"`
	}
	out := make([]packageRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, packageRow{AggregatedPackage: r, MaxSeverity: severityLabelForRank(r.MaxSeverityRank)})
	}

	stats, err := h.updateQueries.GetAllUpdateStats()
	if err != nil {
		stats = &models.UpdateStats{}
	}

	c.JSON(http.StatusOK, gin.H{
		"packages":  out,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"stats":     stats,
	})
}

// GetUpdate retrieves a single update by ID
func (h *UpdateHandler) GetUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	update.EnrichFromMetadata()

	c.JSON(http.StatusOK, update)
}

// GetPackageFleet returns every agent affected by a package (same name+type),
// for the package detail pane's "affected agents" view. Identified by an update
// id so the client can pivot from one row to the whole fleet without already
// knowing the package coordinates.
func (h *UpdateHandler) GetPackageFleet(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	fleet, err := h.updateQueries.GetPackageFleet(update.PackageType, update.PackageName)
	if err != nil {
		log.Printf("[ERROR] [server] [updates] package_fleet_query_failed id=%s pkg=%s error=%v",
			id, update.PackageName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load package fleet"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"package_name": update.PackageName,
		"package_type": update.PackageType,
		"agents":       fleet,
	})
}

// GetPackageVersions returns the version timeline catalog for a package, identified by
// an update id, for the detail pane's Version Timeline card.
func (h *UpdateHandler) GetPackageVersions(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	versions, err := h.updateQueries.GetPackageVersions(update.PackageType, update.PackageName)
	if err != nil {
		log.Printf("[ERROR] [server] [versions] timeline_query_failed id=%s pkg=%s error=%v",
			id, update.PackageName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load version timeline"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"package_name": update.PackageName,
		"package_type": update.PackageType,
		"versions":     versions,
	})
}

// GetPackageSummaryByCoords returns the package-centric aggregate view for
// GET /updates/package/:type/:name — metadata, fleet status counts, and a
// deduplicated vulnerability list, without being tied to a single agent row.
func (h *UpdateHandler) GetPackageSummaryByCoords(c *gin.Context) {
	pkgType := c.Param("type")
	pkgName := c.Param("name")
	if pkgType == "" || pkgName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package type and name are required"})
		return
	}

	summary, err := h.updateQueries.GetPackageSummary(pkgType, pkgName)
	if err != nil {
		log.Printf("[ERROR] [server] [updates] package_summary_failed type=%s name=%s error=%v",
			pkgType, pkgName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found"})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// GetPackageAgentsByCoords returns per-agent rows for
// GET /updates/package/:type/:name/agents — each agent's version, status, and
// actionable flags (can_approve, can_install, can_retry).
func (h *UpdateHandler) GetPackageAgentsByCoords(c *gin.Context) {
	pkgType := c.Param("type")
	pkgName := c.Param("name")
	if pkgType == "" || pkgName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package type and name are required"})
		return
	}

	fleet, err := h.updateQueries.GetPackageFleet(pkgType, pkgName)
	if err != nil {
		log.Printf("[ERROR] [server] [updates] package_agents_failed type=%s name=%s error=%v",
			pkgType, pkgName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load package agents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"package_type": pkgType,
		"package_name": pkgName,
		"agents":       fleet,
	})
}

// GetPackageVersionsByCoords returns the version timeline for
// GET /updates/package/:type/:name/versions.
func (h *UpdateHandler) GetPackageVersionsByCoords(c *gin.Context) {
	pkgType := c.Param("type")
	pkgName := c.Param("name")
	if pkgType == "" || pkgName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package type and name are required"})
		return
	}

	versions, err := h.updateQueries.GetPackageVersions(pkgType, pkgName)
	if err != nil {
		log.Printf("[ERROR] [server] [versions] package_versions_failed type=%s name=%s error=%v",
			pkgType, pkgName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load version timeline"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"package_type": pkgType,
		"package_name": pkgName,
		"versions":     versions,
	})
}

// GetPackageVulnerabilitiesByCoords returns the deduplicated vulnerability list for
// GET /updates/package/:type/:name/vulnerabilities — merged from package_versions
// osv_vulns and live agent supply_chain_vulns metadata.
func (h *UpdateHandler) GetPackageVulnerabilitiesByCoords(c *gin.Context) {
	pkgType := c.Param("type")
	pkgName := c.Param("name")
	if pkgType == "" || pkgName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package type and name are required"})
		return
	}

	vulns, err := h.updateQueries.GetPackageVulnerabilitiesByTypeAndName(pkgType, pkgName)
	if err != nil {
		log.Printf("[ERROR] [server] [updates] package_vulns_failed type=%s name=%s error=%v",
			pkgType, pkgName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load vulnerabilities"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"package_type":    pkgType,
		"package_name":    pkgName,
		"vulnerabilities": vulns,
	})
}

// supplyChainHold is the result of the manual-approval supply-chain gate: whether
// approval must stop, whether the cause was an unverifiable closure (vs. a known
// vuln), a human-readable reason, and the top-level vulnerabilities to echo back.
type supplyChainHold struct {
	blocked    bool
	unverified bool // closure OSV could not run — "trust the void", not a known vuln
	reason     string
	vulns      []services.VulnerabilityInfo
}

const (
	selectedVersionSourceKey           = "selected_version_source"
	selectedVersionSourceManual        = "manual"
	selectedVersionSourceInstalledHold = "installed_hold"
)

// resolvePackageAgeGateConfig returns the approval-time package-age gate policy.
// It prefers the live settings service (env > config > DB > default, so a UI/DB
// override actually drives the gate) and falls back to the env-only
// PackageAgeGateConfig when no settings service is wired.
func (h *UpdateHandler) resolvePackageAgeGateConfig() (minAgeHours float64, enforcement string, blockUnknownAge bool) {
	if h.securitySettings != nil {
		return h.securitySettings.GetSupplyChainGateConfig()
	}
	minAgeHours, enforcement = services.PackageAgeGateConfig()
	return
}

// resolveSoakGateConfig returns the install-path version-soak gate policy
// (GATE-005), preferring the live settings service (env > config > DB > default,
// so an admin policy drives the gate) and falling back to env-only SoakGateConfig
// when no settings service is wired.
func (h *UpdateHandler) resolveSoakGateConfig() (requiredDays float64, enforcement string) {
	if h.securitySettings != nil {
		return h.securitySettings.GetSoakGateConfig()
	}
	return services.SoakGateConfig()
}

// resolveGatedTarget returns RedFlag's *automatic* gated install target for an update, or
// "" to fall back to the newest available version. [GATE-006 B]
//
// This is RedFlag pinning a version on the operator's behalf — distinct from a
// manual force-pin, which is recorded as selected_version_source=manual and is
// honored ahead of this. Installed-version holds use the same selected_version
// value column but source=installed_hold, so they do not block future soak-gated
// auto-targeting.
//
// Two guards keep the auto-pin safe:
//   - It runs only when the soak gate is actually enforced ("block"). In "warn"/"off" the
//     install target is left unchanged — we don't silently re-aim installs the admin
//     hasn't committed to gating.
//   - Forward-only where RedFlag has a reliable comparator. DNF/APT versions are
//     package-manager-native strings, so the server only filters exact no-ops for
//     those ecosystems and leaves installability to the agent dry-run/hash gate.
func (h *UpdateHandler) resolveGatedTarget(update *models.UpdateState) string {
	if h.updateQueries == nil {
		return ""
	}
	requiredDays, enforcement := h.resolveSoakGateConfig()
	if requiredDays <= 0 || enforcement != "block" {
		return ""
	}
	pv, err := h.updateQueries.GetGatedVersion(update.PackageType, update.PackageName, requiredDays)
	if err != nil {
		log.Printf("[WARNING] [server] [soak_gate] gated_lookup_failed pkg=%s error=%v", update.PackageName, err)
		return ""
	}
	if pv == nil {
		return "" // nothing has aged past the soak window yet — don't pull anything newer
	}
	if !gatedTargetAhead(update.PackageType, pv.Version, update.CurrentVersion) {
		return "" // the gated version is empty or an exact no-op
	}
	log.Printf("[INFO] [server] [soak_gate] gated_target pkg=%s gated=%s current=%s available=%s soak_days=%.1f",
		update.PackageName, pv.Version, update.CurrentVersion, update.AvailableVersion, requiredDays)
	return pv.Version
}

func gatedTargetAhead(packageType, targetVersion, currentVersion string) bool {
	target := strings.TrimSpace(targetVersion)
	current := strings.TrimSpace(currentVersion)
	if target == "" {
		return false
	}
	if current == "" {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(packageType)) {
	case "apt":
		// dpkg ordering is not semver, and Debian release suffixes such as
		// 1ubuntu1~22.04 are easy to mis-sort without dpkg --compare-versions.
		// Exact equality is the only portable no-op guard here. Dry-run and exact
		// hash binding decide installability.
		return target != current
	case "dnf", "pacman":
		// pacman's vercmp is derived from rpm's, and Arch's epoch:pkgver-pkgrel
		// format maps cleanly to RPM's epoch:version-release. The same comparison
		// logic applies to both.
		return rpmEVRAhead(target, current)
	default:
		return utils.CompareVersions(target, current) > 0
	}
}

func rpmEVRAhead(target, current string) bool {
	if target == current {
		return false
	}
	targetEpoch, targetVersion, targetRelease := splitRPMEVR(target)
	currentEpoch, currentVersion, currentRelease := splitRPMEVR(current)
	if targetEpoch != currentEpoch {
		return targetEpoch > currentEpoch
	}
	if cmp := rpmVersionCompare(targetVersion, currentVersion); cmp != 0 {
		return cmp > 0
	}
	if cmp := rpmVersionCompare(targetRelease, currentRelease); cmp != 0 {
		return cmp > 0
	}
	// All components equal — target is not ahead of current.
	// This catches textually-different but semantically-equal strings such as
	// "0:2.0-1" vs "2.0-1" where splitRPMEVR normalises the missing epoch to 0.
	return false
}

func splitRPMEVR(evr string) (epoch int, version string, release string) {
	version = strings.TrimSpace(evr)
	if before, after, ok := strings.Cut(version, ":"); ok {
		if parsed, err := strconv.Atoi(before); err == nil {
			epoch = parsed
			version = after
		}
	}
	if before, after, ok := strings.Cut(version, "-"); ok {
		version = before
		release = after
	}
	return epoch, version, release
}

func rpmVersionCompare(a, b string) int {
	for a != "" || b != "" {
		a = trimRPMSeparators(a)
		b = trimRPMSeparators(b)
		if a == "" || b == "" {
			return compareRPMEmpty(a, b)
		}

		aDigit := isASCIIDigit(a[0])
		bDigit := isASCIIDigit(b[0])
		if aDigit != bDigit {
			if aDigit {
				return 1
			}
			return -1
		}

		var aSeg, bSeg string
		aSeg, a = nextRPMSegment(a, aDigit)
		bSeg, b = nextRPMSegment(b, bDigit)
		if aDigit {
			aSeg = strings.TrimLeft(aSeg, "0")
			bSeg = strings.TrimLeft(bSeg, "0")
			if len(aSeg) != len(bSeg) {
				if len(aSeg) > len(bSeg) {
					return 1
				}
				return -1
			}
		}
		if aSeg != bSeg {
			if aSeg > bSeg {
				return 1
			}
			return -1
		}
	}
	return 0
}

func trimRPMSeparators(s string) string {
	return strings.TrimLeftFunc(s, func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
}

func compareRPMEmpty(a, b string) int {
	if a == b {
		return 0
	}
	if a != "" {
		return 1
	}
	return -1
}

func nextRPMSegment(s string, digit bool) (segment string, rest string) {
	for i := 0; i < len(s); i++ {
		if isASCIIDigit(s[i]) != digit || !isASCIIAlnum(s[i]) {
			return s[:i], s[i:]
		}
	}
	return s, ""
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isASCIIAlnum(b byte) bool {
	return isASCIIDigit(b) || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func (h *UpdateHandler) resolveInstallTarget(update *models.UpdateState) (version string, explicit bool) {
	if update.SelectedVersion != nil {
		selected := strings.TrimSpace(*update.SelectedVersion)
		if selected != "" {
			source := selectedVersionSource(update)
			if source == selectedVersionSourceInstalledHold || legacySelectedVersionLooksLikeInstalledHold(update, selected) {
				if gated := h.resolveGatedTarget(update); gated != "" {
					return gated, true
				}
				// Fall through to return selected below
			}
			// Not an installed hold (or hold with no gated target) — use selected version.
			return selected, true
		}
	}
	if gated := h.resolveGatedTarget(update); gated != "" {
		return gated, true
	}
	return update.AvailableVersion, false
}

func selectedVersionSource(update *models.UpdateState) string {
	if update.Metadata == nil {
		return ""
	}
	if s, ok := update.Metadata[selectedVersionSourceKey].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func legacySelectedVersionLooksLikeInstalledHold(update *models.UpdateState, selected string) bool {
	if selectedVersionSource(update) != "" {
		return false
	}
	return (update.Status == models.StatusInstalled || update.Status == models.StatusPending) &&
		update.AvailableVersion != "" &&
		selected != update.AvailableVersion
}

// persistedSupplyChainVulns extracts the OSV vulnerabilities recorded on the
// update at *detection* time. OSV scanning runs on the scan-report path
// (enqueueOSVChecks), not at approval — the approval path only reads the verdict
// detection persisted into supply_chain_vulns metadata. Returns nil when none
// were recorded (clean, not-yet-checked, or non-supply-chain ecosystem).
func persistedSupplyChainVulns(update *models.UpdateState, targetVersion string) []services.VulnerabilityInfo {
	if update.Metadata == nil {
		return nil
	}
	checkedVersion, ok := update.Metadata["supply_chain_checked_version"].(string)
	if !ok || checkedVersion == "" || targetVersion == "" || checkedVersion != targetVersion {
		return nil
	}
	raw, ok := update.Metadata["supply_chain_vulns"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	var vulns []services.VulnerabilityInfo
	if err := json.Unmarshal([]byte(raw), &vulns); err != nil {
		log.Printf("[WARNING] [server] [supply_chain] persisted_vulns_unmarshal_failed id=%s error=%v", update.ID, err)
		return nil
	}
	return vulns
}

func metadataHasTargetSupplyChainVulns(update *models.UpdateState, targetVersion string) bool {
	if update.Metadata == nil || targetVersion == "" {
		return false
	}
	checkedVersion, ok := update.Metadata["supply_chain_checked_version"].(string)
	if !ok || checkedVersion == "" || checkedVersion != targetVersion {
		return false
	}
	return models.MetadataHasVulns(*update, "supply_chain_vulns")
}

// recordPackageAgeMetadata stamps the supply-chain age-gate verdict onto the
// update's metadata for auditability. A known publish date records the timestamp
// and age; a fail-closed-on-unknown block records the unknown marker so an
// operator can see why the install was held. A plain fail-open unknown (allowed)
// records nothing — there is no verdict worth persisting.
func recordPackageAgeMetadata(update *models.UpdateState, dec services.PackageAgeGateDecision) {
	if dec.Unknown && !dec.ShouldBlock {
		return
	}
	if update.Metadata == nil {
		update.Metadata = make(models.JSONB)
	}
	check := map[string]interface{}{
		"min_age_hours": dec.MinAgeHours,
		"enforcement":   dec.Enforcement,
		"blocked":       dec.ShouldBlock,
		"unknown":       dec.Unknown,
	}
	if dec.Unknown {
		check["blocked_unknown"] = dec.BlockedUnknown
	} else {
		update.Metadata["package_published_at"] = dec.PublishedAt.UTC().Format(time.RFC3339)
		update.Metadata["package_age_hours"] = dec.AgeHours
		check["source"] = dec.Source
	}
	update.Metadata["supply_chain_age_check"] = check
}

// evaluateSupplyChainHold decides whether a manual approval must full-stop before
// minting. A known vulnerability anywhere we have looked — the top-level package
// (this call or persisted) or any artifact in the resolved closure — blocks. For
// closure-gated ecosystems, a closure that WAS checked but could not be verified
// (OSV unreachable) also blocks: minting over it would trust artifacts nothing
// vetted. Packages whose closure has not been checked at all (no dry-run yet)
// pass through here — the gate is enforced later at dependency-confirmation time
// by the orchestrator's sweepAutoConfirm, which only advances a closure that was
// OSV-checked AND clean. Mirrors the auto-confirm gate (orchestrator.closureCleared)
// via the shared models predicates.
func (h *UpdateHandler) evaluateSupplyChainHold(update *models.UpdateState, targetVersion string, freshVulns []services.VulnerabilityInfo) supplyChainHold {
	// GATE-006 D: check the version row's osv_status for the target version
	// (the version that will actually install), not just the metadata side-channel.
	// This closes the hole where we OSV-check one version but install another.
	if targetVersion != "" {
		if osvStatus, osvVulns, err := h.updateQueries.GetVersionOSVStatus(update.PackageType, update.PackageName, targetVersion); err == nil && osvStatus == "vulnerable" {
			var vulns []services.VulnerabilityInfo
			if len(osvVulns) > 0 {
				if err := json.Unmarshal(osvVulns, &vulns); err != nil {
					log.Printf("[ERROR] [server] [updates] osv_vulns_unmarshal_failed pkg=%s version=%s error=%v", update.PackageName, targetVersion, err)
					// vulns stays nil; block still fires — fail closed on unmarshal error
				}
			}
			return supplyChainHold{blocked: true, reason: fmt.Sprintf("target version %s has known vulnerabilities (version row)", targetVersion), vulns: vulns}
		}
	}

	// Fallback: metadata side-channel (detection-time verdict). Still needed for
	// packages not yet in the version catalog or where the version row has no
	// osv_status yet.
	if len(freshVulns) > 0 || metadataHasTargetSupplyChainVulns(update, targetVersion) {
		return supplyChainHold{blocked: true, reason: "known vulnerabilities in the package", vulns: freshVulns}
	}
	if models.MetadataHasVulns(*update, "closure_vulns") {
		return supplyChainHold{blocked: true, reason: "known vulnerabilities in a dependency in the resolved closure", vulns: freshVulns}
	}
	if services.NeedsCapabilityGate(update.PackageType) && models.ClosureChecked(*update) && !models.ClosureCleared(*update) {
		return supplyChainHold{
			blocked:    true,
			unverified: true,
			reason:     "dependency closure could not be checked against OSV (service unreachable) — installing trusts unverified artifacts",
			vulns:      freshVulns,
		}
	}
	return supplyChainHold{}
}

// recordGateOverride journals an operator override of any supply-chain gate.
// ETHOS #1: the override is on the record; the signed token still binds real hashes.
func (h *UpdateHandler) recordGateOverride(update *models.UpdateState, eventType, subtype, logTag, message string, meta map[string]interface{}) {
	log.Printf("[SECURITY] [server] [%s] gate_overridden id=%s pkg=%s", logTag, update.ID, update.PackageName)
	event := &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      &update.AgentID,
		EventType:    eventType,
		EventSubtype: subtype,
		Severity:     models.SeverityWarning,
		Component:    models.ComponentSecurity,
		Message:      message,
		Metadata:     meta,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.agentQueries.CreateSystemEvent(event); err != nil {
		log.Printf("[WARNING] [server] [%s] override_event_write_failed id=%s error=%v", logTag, update.ID, err)
	}
}

// recordSupplyChainOverride journals an operator's deliberate decision to install
// past the supply-chain gate.
func (h *UpdateHandler) recordSupplyChainOverride(update *models.UpdateState, targetVersion string, hold supplyChainHold, operatorReason string) {
	subtype := "vuln_overridden"
	if hold.unverified {
		subtype = "unverified_overridden"
	}
	h.recordGateOverride(update, "supply_chain_override", subtype, "supply_chain",
		fmt.Sprintf("Operator overrode the supply-chain gate for %s %s and approved install anyway: %s",
			update.PackageName, targetVersion, hold.reason),
		map[string]interface{}{
			"package_name":    update.PackageName,
			"package_type":    update.PackageType,
			"version":         targetVersion,
			"cause":           hold.reason,
			"unverified":      hold.unverified,
			"operator_reason": operatorReason,
			"approved_by":     "admin",
		})
}

// ApproveUpdate marks an update as approved, with OSV.dev supply chain check
func (h *UpdateHandler) ApproveUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// Optional override body. Absent body = no override, the default and safe path.
	var req struct {
		OverrideSupplyChain bool   `json:"override_supply_chain"`
		OverrideReason      string `json:"override_reason"`
	}
	_ = c.ShouldBindJSON(&req)

	// Look up update details for supply chain check
	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get update details"})
		return
	}
	targetVersion, _ := h.resolveInstallTarget(update)

	// OSV vulnerability data is produced at *detection* (enqueueOSVChecks on the
	// scan-report path) and persisted to supply_chain_vulns metadata. Approval
	// reads that persisted verdict — it does not re-scan OSV here.
	vulns := persistedSupplyChainVulns(update, targetVersion)
	var ageDecision services.PackageAgeGateDecision
	if services.NeedsSupplyChainCheck(update.PackageType) {
		ecosystem := services.EcosystemFromPackageType(update.PackageType)

		// Time-delayed update gate (Shai-Hulud defense). Independent of OSV;
		// runs even when there are no known CVEs because freshness itself is
		// the signal — worm waves propagate inside the first-24h window.
		minAgeHours, enforcement, blockUnknownAge := h.resolvePackageAgeGateConfig()
		if enforcement != "off" {
			age := services.GetPackagePublishDate(update.PackageName, ecosystem, targetVersion)
			ageDecision = services.EvaluatePackageAgeGate(age, services.PackageAgeGatePolicy{
				MinAgeHours:     minAgeHours,
				Enforcement:     enforcement,
				BlockUnknownAge: blockUnknownAge,
				EcosystemAged:   services.EcosystemSupportsPackageAge(ecosystem),
			})
			recordPackageAgeMetadata(update, ageDecision)
			if ageDecision.ShouldBlock {
				log.Printf("[WARNING] [supply_chain] approval_blocked_by_age_gate id=%s pkg=%s age_hours=%.2f min=%.2f unknown=%t",
					id, update.PackageName, ageDecision.AgeHours, ageDecision.MinAgeHours, ageDecision.BlockedUnknown)
				resp := gin.H{
					"error":         "approval blocked by supply chain age gate",
					"reason":        ageDecision.WarnMessage,
					"package":       update.PackageName,
					"version":       targetVersion,
					"min_age_hours": ageDecision.MinAgeHours,
					"override_hint": "set REDFLAG_SUPPLY_CHAIN_GATE_ENFORCEMENT=warn (or off) and retry; or wait until threshold passes",
				}
				if !ageDecision.BlockedUnknown {
					resp["published_at"] = ageDecision.PublishedAt.UTC().Format(time.RFC3339)
					resp["age_hours"] = ageDecision.AgeHours
				}
				c.JSON(http.StatusConflict, resp)
				return
			}
		}
	}

	// Supply-chain full stop. A known vulnerability — top-level or anywhere in the
	// resolved dependency closure — or a closure OSV could not verify, must not
	// silently approve and mint a capability token. This is the hole the old
	// "sovereignty principle" left open: it let vulns through with only a warning.
	// Now it is a hard stop; only an explicit admin override ("I need this version
	// anyway") proceeds, and that decision is journaled to system_events. The override
	// waives the VULNERABILITY judgment only — it never touches the token's signature
	// or artifact-hash verification, which remain absolute (ETHOS: no skip-verify path).
	hold := h.evaluateSupplyChainHold(update, targetVersion, vulns)
	if hold.blocked {
		if !req.OverrideSupplyChain {
			log.Printf("[SECURITY] [server] [supply_chain] approval_blocked id=%s pkg=%s reason=%q unverified=%t",
				id, update.PackageName, hold.reason, hold.unverified)
			c.JSON(http.StatusConflict, gin.H{
				"error":           "approval blocked by supply chain gate",
				"reason":          hold.reason,
				"package":         update.PackageName,
				"version":         targetVersion,
				"unverified":      hold.unverified,
				"vulnerabilities": vulns,
				"override_hint":   "resubmit with override_supply_chain=true and a non-empty override_reason to install anyway",
			})
			return
		}
		if strings.TrimSpace(req.OverrideReason) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  "supply chain override requires a reason",
				"reason": hold.reason,
			})
			return
		}
		h.recordSupplyChainOverride(update, targetVersion, hold, req.OverrideReason)
	}

	// Layer 1: Hash Registry — compute and store expected_sha256 BEFORE writing the
	// approval. The hash registry is the tamper-detection gate; an approval whose
	// hash step errored must not stand (fail closed, ETHOS §2).
	// Empty hash with no error is legitimate: agent-sourced ecosystems (dnf/apt)
	// report their closure hashes via ReportDependencies, so computeAndStorePackageHash
	// returns ("", nil) for those — approval proceeds normally.
	artifactHash, err := h.computeAndStorePackageHash(update, targetVersion)
	if err != nil {
		log.Printf("[ERROR] [server] [updates] hash_computation_failed id=%s pkg=%s error=%v",
			id, update.PackageName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("hash computation failed; approval not written: %v", err)})
		return
	}
	if artifactHash != "" {
		log.Printf("[INFO] [server] [updates] hash_stored id=%s pkg=%s sha256=%s",
			id, update.PackageName, artifactHash[:16]+"...")
	}

	// Proceed with approval (sovereignty principle: vulns + warn-mode age findings
	// don't block; only "block" enforcement of the age gate does, handled above).
	if len(vulns) > 0 || (!ageDecision.Unknown && ageDecision.WarnMessage != "") {
		if err := h.updateQueries.ApproveUpdateWithVulns(id, "admin", update.Metadata); err != nil {
			log.Printf("[ERROR] [server] [updates] approve_update_failed id=%s error=%v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to approve update: %v", err)})
			return
		}
	} else {
		if err := h.updateQueries.ApproveUpdate(id, "admin"); err != nil {
			log.Printf("[ERROR] [server] [updates] approve_update_failed id=%s error=%v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to approve update: %v", err)})
			return
		}
	}

	// Enrich the version timeline with what this approval learned — OSV posture,
	// publish date, artifact hash — so the detail pane and as-of-date resolution see
	// it. Best-effort; never blocks an approval that already cleared the gates.
	{
		pv := &models.PackageVersion{
			PackageType: update.PackageType,
			PackageName: update.PackageName,
			Version:     targetVersion,
		}
		if artifactHash != "" {
			pv.SHA256 = &artifactHash
		}
		if services.NeedsSupplyChainCheck(update.PackageType) {
			status := "clean"
			if len(vulns) > 0 {
				status = "vulnerable"
			}
			pv.OSVStatus = &status
			if raw, ok := update.Metadata["supply_chain_vulns"].(string); ok && raw != "" {
				pv.OSVVulns = &raw
			}
		}
		if !ageDecision.Unknown {
			published := ageDecision.PublishedAt.UTC()
			pv.PublishedAt = &published
		}
		if err := h.updateQueries.UpsertPackageVersion(pv); err != nil {
			log.Printf("[WARNING] [server] [versions] approve_enrich_failed id=%s pkg=%s error=%v",
				id, update.PackageName, err)
		}
	}

	// Supply Chain Gate — mint a signed capability token over the approved
	// closure. Single-entry closure today (top-level package + its expected
	// hash); transitive resolution lands here later. Best-effort: a minting
	// failure is logged at SECURITY but does not roll back an approval that has
	// already cleared OSV/age/hash.
	if h.minter.Enabled() {
		if artifactHash == "" {
			// Agent-sourced ecosystems (dnf/apt): the closure was
			// already stored by pinReportedClosure during dry-run.
			if _, err := h.mintResolvedClosure(update); err != nil {
				log.Printf("[SECURITY] [server] [capability] mint_failed id=%s pkg=%s error=%v",
					id, update.PackageName, err)
			}
		} else {
			closure := []capability.ClosureEntry{{
				Name:    update.PackageName,
				Version: targetVersion,
				SHA256:  artifactHash,
				Source:  "registry",
			}}
			if _, err := h.minter.MintForUpdate(update, closure); err != nil {
				log.Printf("[SECURITY] [server] [capability] mint_failed id=%s pkg=%s error=%v",
					id, update.PackageName, err)
			}
		}
	}

	response := gin.H{"message": "update approved"}
	if len(vulns) > 0 {
		response["warnings"] = vulns
	}
	if !ageDecision.Unknown && ageDecision.WarnMessage != "" {
		response["age_warning"] = gin.H{
			"message":       ageDecision.WarnMessage,
			"age_hours":     ageDecision.AgeHours,
			"min_age_hours": ageDecision.MinAgeHours,
			"published_at":  ageDecision.PublishedAt.UTC().Format(time.RFC3339),
		}
	}
	c.JSON(http.StatusOK, response)
}

// computeAndStorePackageHash downloads the package artifact from upstream,
// computes its SHA256, and stores it in the database for install-time verification.
// Returns the hash string (empty on failure).
func (h *UpdateHandler) computeAndStorePackageHash(update *models.UpdateState, targetVersion string) (string, error) {
	// Only compute hashes for ecosystems the server can fetch from public
	// registries (npm/PyPI). Agent-sourced ecosystems (dnf/apt) report
	// their hashes via ReportDependencies → pinReportedClosure instead.
	if !services.CanServerFetchArtifact(update.PackageType) {
		return "", nil
	}
	if targetVersion == "" {
		targetVersion = update.AvailableVersion
	}

	ecosystem := services.EcosystemFromPackageType(update.PackageType)

	// Call the download handler's artifact endpoint using the server's own URL
	// This ensures the hash is computed from the same source agents will use
	baseURL := h.config.Server.PublicURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", h.config.Server.Host, h.config.Server.Port)
	}
	downloadURL := fmt.Sprintf("%s/api/v1/downloads/artifact?ecosystem=%s&package_name=%s&version=%s",
		baseURL, ecosystem, update.PackageName, targetVersion)

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return "", err
	}

	// Set basic auth for the request
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download artifact failed with status %d", resp.StatusCode)
	}

	// Read and compute SHA256 while streaming
	sha := sha256.New()
	_, err = io.Copy(sha, resp.Body)
	if err != nil {
		return "", err
	}

	shaHex := hex.EncodeToString(sha.Sum(nil))

	// Store the expected hash
	if err := h.updateQueries.StoreExpectedSHA256(update.ID, shaHex); err != nil {
		return shaHex, err
	}

	return shaHex, nil
}

// ReportLog handles update execution logs from agents
func (h *UpdateHandler) ReportLog(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	var req models.UpdateLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Idempotency check: reject a resubmission only once the command is
	// FINALIZED (ETHOS #4). This deliberately keys off terminal status, not
	// "result already recorded" — those are different questions and must not be
	// conflated. A command can carry a result while still open: update_agent
	// reports "started" (which records a result) and then reports its terminal
	// success/failed under the same command_id. Rejecting on result-present would
	// 409 that finalizing report and the command would never close. (The pending
	// result-ack, by contrast, correctly clears on result-recorded — it only
	// needs to know the server received the report, not that it finalized.)
	if req.CommandID != "" {
		commandID, err := uuid.FromString(req.CommandID)
		if err == nil {
			command, err := h.commandQueries.GetCommandByID(commandID)
			if err == nil && command != nil {
				if command.Status == models.CommandStatusCompleted || command.Status == models.CommandStatusFailed || command.Status == models.CommandStatusTimedOut {
					log.Printf("[INFO] [server] [updates] duplicate_log_rejected agent_id=%s command_id=%s status=%s",
						agentID, commandID, command.Status)
					c.JSON(http.StatusConflict, gin.H{
						"error":          "duplicate log submission",
						"command_id":     commandID.String(),
						"current_status": command.Status,
					})
					return
				}
			}
		}
	}

	// Validate and map result to comply with database constraint.
	// isValidResult is the authority; only values not in that map reach
	// this fallthrough. "started" and "running" pass straight through now
	// that the DB constraint accepts them (migration 048).
	validResult := req.Result
	if !isValidResult(validResult) {
		switch validResult {
		case "timed_out", "timeout", "cancelled":
			validResult = "failed"
		case "updated":
			validResult = "success"
		case "rollback":
			validResult = "success"
		case "partial_failure":
			validResult = "partial"
		case "failure":
			validResult = "failed"
		default:
			validResult = "failed"
		}
	}

	// Extract subsystem from request if provided, otherwise try to parse from action
	subsystem := req.Subsystem
	if subsystem == "" && strings.HasPrefix(req.Action, "scan_") {
		subsystem = strings.TrimPrefix(req.Action, "scan_")
	}

	logEntry := &models.UpdateLog{
		ID:              uuid.Must(uuid.NewV4()),
		AgentID:         agentID,
		Action:          req.Action,
		Subsystem:       subsystem,
		Result:          validResult,
		Stdout:          req.Stdout,
		Stderr:          req.Stderr,
		ExitCode:        req.ExitCode,
		DurationSeconds: req.DurationSeconds,
		ExecutedAt:      time.Now().UTC(),
	}

	// Add HISTORY logging
	log.Printf("[INFO] [server] [update] log_created agent_id=%s subsystem=%s action=%s result=%s timestamp=%s",
		agentID, subsystem, req.Action, validResult, time.Now().UTC().Format(time.RFC3339))
	log.Printf("[HISTORY] [server] [update] log_created agent_id=%s subsystem=%s action=%s result=%s timestamp=%s",
		agentID, subsystem, req.Action, validResult, time.Now().UTC().Format(time.RFC3339))

	// Store the log entry
	if err := h.updateQueries.CreateUpdateLog(logEntry); err != nil {
		log.Printf("[ERROR] [server] [update] log_save_failed agent_id=%s error=%v", agentID, err)
		log.Printf("[HISTORY] [server] [update] log_save_failed error=\"%v\" timestamp=%s", err, time.Now().UTC().Format(time.RFC3339))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save log"})
		return
	}

	// NEW: Update command status if command_id is provided
	if req.CommandID != "" {
		commandID, err := uuid.FromString(req.CommandID)
		if err != nil {
			// Log warning but don't fail the request
			log.Printf("[WARNING] [server] [updates] invalid_command_id_format command_id=%s", req.CommandID)
		} else {
			// Prepare result data for command update
			result := models.JSONB{
				"stdout":           req.Stdout,
				"stderr":           req.Stderr,
				"exit_code":        req.ExitCode,
				"duration_seconds": req.DurationSeconds,
				"logged_at":        time.Now().UTC(),
			}

			// Update command status based on log result.
			// MarkCommand* errors are ETHOS #1 violations if dropped silently — the
			// agent thinks the result was recorded and won't resend. Surface via
			// structured ERROR log + a should_retry hint in the response.
			var markErr error
			if req.Result == "success" || req.Result == "completed" {
				if markErr = h.commandQueries.MarkCommandCompleted(commandID, result); markErr != nil {
					log.Printf("[ERROR] [server] [updates] mark_completed_failed agent_id=%s command_id=%s error=%q",
						agentID, commandID, markErr)
				}

				// If this was a successful confirm_dependencies command, close the
				// installing package. The source-state guard keeps stale or replayed
				// command results from resolving a pending/approved update.
				command, err := h.commandQueries.GetCommandByID(commandID)
				if err == nil && command.CommandType == models.CommandTypeConfirmDependencies {
					// Extract package info from command params
					if packageName, ok := command.Params["package_name"].(string); ok {
						if packageType, ok := command.Params["package_type"].(string); ok {
							// Extract actual completion timestamp from command result for accurate audit trail
							var completionTime *time.Time
							if loggedAtStr, ok := command.Result["logged_at"].(string); ok {
								if parsed, err := time.Parse(time.RFC3339Nano, loggedAtStr); err == nil {
									completionTime = &parsed
								}
							}

							if err := h.updateQueries.TransitionByPackageFrom(agentID, packageType, packageName, models.StatusInstalling, models.StatusInstalled, nil, completionTime); err != nil {
								log.Printf("[ERROR] [server] [updates] confirm_dependencies_status_update_failed package=%s type=%s to=%s error=%v",
									packageName, packageType, models.StatusInstalled, err)
							} else {
								log.Printf("[INFO] [server] [updates] confirm_dependencies_status_updated package=%s type=%s status=%s",
									packageName, packageType, models.StatusInstalled)
								// Pin the installed version so subsequent scans don't
								// silently advance to a newer unapproved version.
								if upd, lookupErr := h.updateQueries.GetUpdateByPackage(agentID, packageType, packageName); lookupErr == nil {
									installedVersion := upd.AvailableVersion
									if upd.SelectedVersion != nil && *upd.SelectedVersion != "" {
										installedVersion = *upd.SelectedVersion
									}
									if installedVersion != "" {
										_ = h.updateQueries.SetInstalledVersionHold(upd.ID, installedVersion)
									}
								}
							}
						}
					}
				}
			} else if req.Result == "failed" || req.Result == "failure" || req.Result == "dry_run_failed" || req.Result == "partial_failure" {
				if markErr = h.commandQueries.MarkCommandFailed(commandID, result); markErr != nil {
					log.Printf("[ERROR] [server] [updates] mark_failed_failed agent_id=%s command_id=%s error=%q",
						agentID, commandID, markErr)
				}

				// If this was a failed confirm_dependencies command, close the
				// installing package as failed with the same source-state guard.
				command, err := h.commandQueries.GetCommandByID(commandID)
				if err == nil && command.CommandType == models.CommandTypeConfirmDependencies {
					if packageName, ok := command.Params["package_name"].(string); ok {
						if packageType, ok := command.Params["package_type"].(string); ok {
							if err := h.updateQueries.TransitionByPackageFrom(agentID, packageType, packageName, models.StatusInstalling, models.StatusFailed, nil, nil); err != nil {
								log.Printf("[ERROR] [server] [updates] confirm_dependencies_status_update_failed package=%s type=%s to=%s error=%v",
									packageName, packageType, models.StatusFailed, err)
							} else {
								log.Printf("[INFO] [server] [updates] confirm_dependencies_status_updated package=%s type=%s status=%s",
									packageName, packageType, models.StatusFailed)
							}
						}
					}
				}

				// A failed update_agent is definitive: clear is_updating now so the
				// operator can retry immediately instead of waiting out the stuck-update
				// timeout (TimeoutService reconcile). The success path is deliberately
				// NOT cleared here — success is proven by the new binary attesting its
				// version on check-in, not by the old binary's pre-restart log.
				if err == nil && command.CommandType == models.CommandTypeUpdateAgent {
					if clearErr := h.agentQueries.ClearAgentUpdating(agentID); clearErr != nil {
						log.Printf("[ERROR] [server] [updates] clear_updating_on_fail_failed agent_id=%s command_id=%s error=%q",
							agentID, commandID, clearErr)
					} else {
						log.Printf("[INFO] [server] [updates] agent_update_failed_flag_cleared agent_id=%s command_id=%s",
							agentID, commandID)
					}
				}
			} else {
				// For other results, just update the result field
				if markErr = h.commandQueries.UpdateCommandResult(commandID, result); markErr != nil {
					log.Printf("[ERROR] [server] [updates] update_result_failed agent_id=%s command_id=%s error=%q",
						agentID, commandID, markErr)
				}
			}

			if markErr != nil {
				c.JSON(http.StatusOK, gin.H{
					"message":      "log saved but command state update failed",
					"should_retry": true,
					"reason":       markErr.Error(),
				})
				return
			}
		}
	}

	// ETHOS #1: every failed agent action is an auditable error — journal it inward
	// so it surfaces in the events API / history, not only in update_logs. This was
	// previously gated to update_agent/verify_command, which left scan, dry-run, and
	// confirm_dependencies failures invisible to the operator. Best-effort — log and
	// continue on failure, don't block the response.
	if validResult == "failed" {
		failureReason := req.Stderr
		if failureReason == "" && req.ExitCode != 0 {
			failureReason = fmt.Sprintf("exit code %d", req.ExitCode)
		}
		if failureReason == "" {
			failureReason = "agent reported failure"
		}

		// Preserve the existing event type for update flows; everything else is a
		// generic command failure.
		eventType := models.EventTypeCommandFailed
		if req.Action == "update_agent" {
			eventType = "agent_update"
		}

		message := services.RenderUpdateLog(req.Action, validResult, req.Stderr)
		event := &models.SystemEvent{
			ID:           uuid.Must(uuid.NewV4()),
			AgentID:      &agentID,
			EventType:    eventType,
			EventSubtype: "failed",
			Severity:     "error",
			Component:    "agent",
			Message:      message,
			Metadata: map[string]interface{}{
				"action":         req.Action,
				"result":         validResult,
				"stderr":         req.Stderr,
				"failure_reason": failureReason,
			},
			CreatedAt: time.Now().UTC(),
		}
		if err := h.agentQueries.CreateSystemEvent(event); err != nil {
			log.Printf("[WARNING] [server] [updates] system_event_write_failed agent_id=%s action=%s error=%v",
				agentID, req.Action, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "log recorded"})
}

// GetPackageHistory returns version history for a specific package
func (h *UpdateHandler) GetPackageHistory(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	packageType := c.Query("package_type")
	packageName := c.Query("package_name")

	if packageType == "" || packageName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package_type and package_name are required"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	history, err := h.updateQueries.GetPackageHistory(agentID, packageType, packageName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get package history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"history":      history,
		"package_type": packageType,
		"package_name": packageName,
		"count":        len(history),
	})
}

// GetBatchStatus returns recent batch processing status for an agent
func (h *UpdateHandler) GetBatchStatus(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	batches, err := h.updateQueries.GetBatchStatus(agentID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get batch status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"batches": batches,
		"count":   len(batches),
	})
}

// UpdatePackageStatus updates the status of a package (for when updates are installed)
func (h *UpdateHandler) UpdatePackageStatus(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req struct {
		PackageType string                 `json:"package_type" binding:"required"`
		PackageName string                 `json:"package_name" binding:"required"`
		Status      models.PackageStatus   `json:"status" binding:"required"`
		Metadata    map[string]interface{} `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.updateQueries.UpdatePackageStatus(agentID, req.PackageType, req.PackageName, req.Status, req.Metadata, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "package status updated"})
}

// ApproveUpdates handles bulk approval of updates, running the same OSV.dev
// supply chain check as single-approve for npm/PyPI packages. Per-update
// loop instead of BulkApproveUpdates because the OSV-enriched approval
// stores per-update metadata; the warning aggregate is returned to the UI.
func (h *UpdateHandler) ApproveUpdates(c *gin.Context) {
	var req struct {
		UpdateIDs   []string `json:"update_ids" binding:"required"`
		ScheduledAt *string  `json:"scheduled_at"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	type warning struct {
		UpdateID        string                       `json:"update_id"`
		PackageName     string                       `json:"package_name"`
		Vulnerabilities []services.VulnerabilityInfo `json:"vulnerabilities,omitempty"`
		AgeWarning      string                       `json:"age_warning,omitempty"`
		AgeHours        float64                      `json:"age_hours,omitempty"`
	}
	type blocked struct {
		UpdateID    string  `json:"update_id"`
		PackageName string  `json:"package_name"`
		Reason      string  `json:"reason"`
		AgeHours    float64 `json:"age_hours"`
	}
	warnings := make([]warning, 0)
	blockedList := make([]blocked, 0)
	approved := 0

	// Bulk reads the gate config once — applies to every item in the batch.
	gateMin, gateEnforcement, gateBlockUnknown := h.resolvePackageAgeGateConfig()

	for _, idStr := range req.UpdateIDs {
		id, err := uuid.FromString(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID: " + idStr})
			return
		}

		update, err := h.updateQueries.GetUpdateByID(id)
		if err != nil {
			log.Printf("[ERROR] [server] [updates] bulk_approve_lookup_failed id=%s error=%v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load update %s", idStr)})
			return
		}
		targetVersion, _ := h.resolveInstallTarget(update)

		// OSV verdict comes from detection-time metadata (see persistedSupplyChainVulns
		// and the single-approve path); bulk approval never re-scans OSV.
		vulns := persistedSupplyChainVulns(update, targetVersion)
		var ageDecision services.PackageAgeGateDecision
		if services.NeedsSupplyChainCheck(update.PackageType) {
			ecosystem := services.EcosystemFromPackageType(update.PackageType)

			if gateEnforcement != "off" {
				age := services.GetPackagePublishDate(update.PackageName, ecosystem, targetVersion)
				ageDecision = services.EvaluatePackageAgeGate(age, services.PackageAgeGatePolicy{
					MinAgeHours:     gateMin,
					Enforcement:     gateEnforcement,
					BlockUnknownAge: gateBlockUnknown,
					EcosystemAged:   services.EcosystemSupportsPackageAge(ecosystem),
				})
				recordPackageAgeMetadata(update, ageDecision)
				if ageDecision.ShouldBlock {
					log.Printf("[WARNING] [supply_chain] bulk_approval_blocked_by_age_gate id=%s pkg=%s age_hours=%.2f",
						id, update.PackageName, ageDecision.AgeHours)
					blockedList = append(blockedList, blocked{
						UpdateID:    idStr,
						PackageName: update.PackageName,
						Reason:      ageDecision.WarnMessage,
						AgeHours:    ageDecision.AgeHours,
					})
					continue
				}
			}
		}

		// Supply-chain full stop, same gate as single-approve. Bulk approval never
		// carries a blanket override — a known vuln or an unverifiable closure is
		// returned as blocked, and the operator must approve that one individually
		// with an explicit reason. Keeps "override everything at once" impossible.
		if hold := h.evaluateSupplyChainHold(update, targetVersion, vulns); hold.blocked {
			log.Printf("[SECURITY] [server] [supply_chain] bulk_approval_blocked id=%s pkg=%s reason=%q unverified=%t",
				id, update.PackageName, hold.reason, hold.unverified)
			blockedList = append(blockedList, blocked{
				UpdateID:    idStr,
				PackageName: update.PackageName,
				Reason:      hold.reason,
			})
			continue
		}

		// Layer 1: Hash Registry — compute and store expected_sha256 BEFORE writing
		// the approval. Fail closed: hash error blocks this item (added to blockedList)
		// without aborting the rest of the batch. Empty hash with no error is fine
		// (agent-sourced ecosystems report hashes via ReportDependencies).
		artifactHash, err := h.computeAndStorePackageHash(update, targetVersion)
		if err != nil {
			log.Printf("[ERROR] [server] [updates] bulk_hash_computation_failed id=%s pkg=%s error=%v",
				id, update.PackageName, err)
			blockedList = append(blockedList, blocked{
				UpdateID:    idStr,
				PackageName: update.PackageName,
				Reason:      fmt.Sprintf("hash computation failed: %v", err),
			})
			continue
		}
		if artifactHash != "" {
			log.Printf("[INFO] [server] [updates] bulk_hash_stored id=%s pkg=%s sha256=%s",
				id, update.PackageName, artifactHash[:16]+"...")
		}

		needsMeta := len(vulns) > 0 || (!ageDecision.Unknown && ageDecision.WarnMessage != "")
		if needsMeta {
			if err := h.updateQueries.ApproveUpdateWithVulns(id, "admin", update.Metadata); err != nil {
				log.Printf("[ERROR] [server] [updates] bulk_approve_failed id=%s error=%v", id, err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to approve update %s: %v", idStr, err)})
				return
			}
			w := warning{UpdateID: idStr, PackageName: update.PackageName, Vulnerabilities: vulns}
			if !ageDecision.Unknown && ageDecision.WarnMessage != "" {
				w.AgeWarning = ageDecision.WarnMessage
				w.AgeHours = ageDecision.AgeHours
			}
			warnings = append(warnings, w)
		} else {
			if err := h.updateQueries.ApproveUpdate(id, "admin"); err != nil {
				log.Printf("[ERROR] [server] [updates] bulk_approve_failed id=%s error=%v", id, err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to approve update %s: %v", idStr, err)})
				return
			}
		}
		approved++
	}

	response := gin.H{
		"message": "updates approved",
		"count":   approved,
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}
	if len(blockedList) > 0 {
		response["blocked"] = blockedList
		response["blocked_count"] = len(blockedList)
	}
	c.JSON(http.StatusOK, response)
}

// RejectUpdate rejects a single update
func (h *UpdateHandler) RejectUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// For now, use "admin" as rejecter. Will integrate with proper auth later
	if err := h.updateQueries.RejectUpdate(id, "admin"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject update"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "update rejected"})
}

// InstallUpdate marks an update as ready for installation and creates a dry run command for the agent
func (h *UpdateHandler) InstallUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// Get the full update details to extract agent_id, package_name, and package_type
	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get update details"})
		return
	}

	// Check maintenance window before proceeding
	if inside, err := h.maintenanceWindowQueries.IsWithinMaintenanceWindow(time.Now().UTC()); err != nil {
		log.Printf("[ERROR] [server] [maintenance_window] check_failed id=%s error=%v", id, err)
		// Fail-open on DB error: allow the operation but log the failure
	} else if !inside {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Installation blocked: outside configured maintenance window",
		})
		return
	}

	// Policy gate: dry runs may be disabled per-deployment. Strict default is
	// "dry run required" — when policy.allow_dry_runs=false an operator has
	// explicitly chosen to skip the dependency-check step. Refused with 403
	// here so the rejection surfaces in history; structural "queue install
	// directly" semantics may follow in a later scope.
	allowDryRuns := true
	if h.securitySettings != nil {
		allowDryRuns = h.securitySettings.GetPolicyBool("allow_dry_runs", true)
	}
	if !allowDryRuns {
		log.Printf("[INFO] [server] [updates] dry_run_refused update_id=%s reason=policy.allow_dry_runs=false agent_id=%s",
			id, update.AgentID)
		c.JSON(http.StatusForbidden, gin.H{
			"error":          "Dry runs are disabled by policy (policy.allow_dry_runs=false). Operator action required to proceed.",
			"_error_context": "policy_dry_runs_disabled",
		})
		return
	}

	// Create the signed dry-run command and advance to checking_dependencies.
	// Shared with the orchestrator's auto-approve path via EnqueueDryRun.
	if err := h.EnqueueDryRun(update); err != nil {
		log.Printf("[ERROR] [server] [updates] enqueue_dry_run_failed update_id=%s error=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start dry run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "dry run command created for agent",
	})
}

// InstallVersion selects a specific version for installation, evaluating the
// version soak gate before proceeding. POST /updates/:id/install-version
func (h *UpdateHandler) InstallVersion(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	var req models.InstallVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	// Check maintenance window before proceeding.
	if inside, err := h.maintenanceWindowQueries.IsWithinMaintenanceWindow(time.Now().UTC()); err != nil {
		log.Printf("[ERROR] [server] [maintenance_window] check_failed id=%s error=%v", id, err)
	} else if !inside {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Installation blocked: outside configured maintenance window",
		})
		return
	}

	// Validate version exists in the catalog.
	pv, err := h.updateQueries.GetPackageVersionByString(update.PackageType, update.PackageName, req.Version)
	if err != nil {
		log.Printf("[ERROR] [server] [soak_gate] version_lookup_failed id=%s version=%s error=%v", id, req.Version, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found in package catalog"})
		return
	}
	if pv == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found in package catalog"})
		return
	}

	// Evaluate soak gate.
	requiredDays, enforcement := h.resolveSoakGateConfig()
	decision := services.EvaluateSoakGate(pv.FirstScannedAt, requiredDays, enforcement)

	if !decision.Eligible && !decision.Unknown {
		if strings.TrimSpace(req.OverrideReason) == "" {
			log.Printf("[SECURITY] [server] [soak_gate] install_blocked id=%s pkg=%s version=%s soak_days=%.1f required=%.1f remaining=%.1f",
				id, update.PackageName, req.Version, decision.SoakDays, decision.RequiredDays, decision.DaysRemaining)
			c.JSON(http.StatusConflict, gin.H{
				"error":            "install blocked by version soak gate",
				"reason":           services.SoakGateWarnMessage(decision),
				"version":          req.Version,
				"soak_days":        decision.SoakDays,
				"required_days":    decision.RequiredDays,
				"days_remaining":   decision.DaysRemaining,
				"first_scanned_at": decision.FirstScannedAt.UTC().Format(time.RFC3339),
				"override_hint":    "resubmit with a non-empty override_reason to install anyway",
			})
			return
		}
		h.recordSoakOverride(update, decision, req.OverrideReason)
	}

	// Set selected_version on the update row, then update the local struct to match.
	if err := h.updateQueries.SetTargetVersion(id, req.Version); err != nil {
		log.Printf("[ERROR] [server] [soak_gate] set_target_failed id=%s version=%s error=%v", id, req.Version, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set target version"})
		return
	}
	update.SelectedVersion = &req.Version

	// [GATE-006 slice A] Re-run OSV against the version that will actually install.
	// Detection (enqueueOSVChecks) checked available_version — the newest upstream.
	// The operator just pinned a (soak-gated) selected_version, which is what the
	// gate installs (target resolves selected_version ?? available_version). The
	// persisted supply_chain_vulns must describe THAT version, since
	// evaluateSupplyChainHold reads it at approval. Checking the version we never
	// install while installing one we never checked hollows the supply-chain
	// guarantee. Background + best-effort, mirroring the detection path.
	if services.NeedsSupplyChainCheck(update.PackageType) {
		agentID, pkgType, pkgName, target := update.AgentID, update.PackageType, update.PackageName, req.Version
		h.bg("osv_recheck_selected", func() {
			services.RunOSVChecks(
				[]services.OSVCheckRequest{{AgentID: agentID, PkgType: pkgType, PkgName: pkgName, Version: target}},
				func(aID uuid.UUID, pt, pn string, meta map[string]interface{}) error {
					return storeOSVCheckResult(h.updateQueries, aID, pt, pn, meta)
				},
			)
		})
	}

	if err := h.EnqueueDryRun(update); err != nil {
		log.Printf("[ERROR] [server] [soak_gate] enqueue_dry_run_failed id=%s version=%s error=%v", id, req.Version, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start dry run for selected version"})
		return
	}

	log.Printf("[INFO] [server] [soak_gate] install_version_selected id=%s pkg=%s version=%s soak_eligible=%t",
		id, update.PackageName, req.Version, decision.Eligible)

	c.JSON(http.StatusOK, gin.H{
		"message":          "version selected and dry run enqueued",
		"selected_version": req.Version,
		"soak_days":        decision.SoakDays,
		"required_days":    decision.RequiredDays,
	})
}

// recordSoakOverride journals an operator's decision to install before soak completes.
func (h *UpdateHandler) recordSoakOverride(update *models.UpdateState, dec services.SoakGateDecision, operatorReason string) {
	if h.agentQueries == nil {
		return
	}
	h.recordGateOverride(update, "soak_override", "", "soak_gate",
		fmt.Sprintf("Operator overrode version soak gate for %s (%.1f/%.0f days): %s",
			update.PackageName, dec.SoakDays, dec.RequiredDays, operatorReason),
		map[string]interface{}{
			"package_name":    update.PackageName,
			"soak_days":       dec.SoakDays,
			"required_days":   dec.RequiredDays,
			"days_remaining":  dec.DaysRemaining,
			"operator_reason": operatorReason,
		})
}

// GetUpdateLogs retrieves installation logs for a specific update
func (h *UpdateHandler) GetUpdateLogs(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// Parse limit from query params
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	logs, err := h.updateQueries.GetUpdateLogs(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve update logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"count": len(logs),
	})
}

// ReportDependencies handles dependency reporting from agents after dry run
func (h *UpdateHandler) ReportDependencies(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	var req models.DependencyReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash registry (OS package managers): dry-run records the canonical artifact
	// hashes the agent's signed repo metadata anchors. This pins the closure, but
	// does not mint an executable capability token yet; final operator intent is
	// established below for no-dependency updates or in ConfirmDependencies.
	if err := h.pinReportedClosure(agentID, &req); err != nil {
		log.Printf("[SECURITY] [server] [capability] closure_pin_rejected agent_id=%s pkg=%s/%s error=%v",
			agentID, req.PackageType, req.PackageName, err)
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	// If there are NO dependencies, auto-approve and proceed directly to installation
	// This prevents updates with zero dependencies from getting stuck in "pending_dependencies"
	if len(req.Dependencies) == 0 {
		// Get the update by package to retrieve its ID
		update, err := h.updateQueries.GetUpdateByPackage(agentID, req.PackageType, req.PackageName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get update details"})
			return
		}

		if h.usesCapabilityExecution(update.PackageType) {
			closure, err := h.updateQueries.GetResolvedClosure(update.ID)
			if err != nil || len(closure) == 0 {
				log.Printf("[SECURITY] [server] [capability] no_pinned_closure update_id=%s pkg=%s — agent reported zero deps but no hashed closure was stored",
					update.ID, update.PackageName)
				c.JSON(http.StatusConflict, gin.H{"error": "no hashed closure available to sign — agent must include artifact hashes in the closure report"})
				return
			}
			if err := h.updateQueries.SetInstallingWithNoDependencies(update.ID, req.Dependencies); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status to installing"})
				return
			}
			if _, err := h.mintResolvedClosure(update); err != nil {
				log.Printf("[SECURITY] [server] [capability] auto_mint_failed update_id=%s pkg=%s error=%v",
					update.ID, update.PackageName, err)
				if statusErr := h.updateQueries.UpdatePackageStatus(update.AgentID, update.PackageType, update.PackageName, models.StatusFailed, models.JSONB{
					"capability_authorization_error": err.Error(),
				}, nil); statusErr != nil {
					log.Printf("[ERROR] [server] [capability] auto_mint_failed_status_update_failed update_id=%s error=%v",
						update.ID, statusErr)
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize capability token"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message": "no dependencies found - capability token authorized for agent execution",
			})
			return
		}

		// Automatically create installation command since no dependencies need approval
		command := &models.AgentCommand{
			ID:          uuid.Must(uuid.NewV4()),
			AgentID:     agentID,
			CommandType: models.CommandTypeConfirmDependencies,
			Params: map[string]interface{}{
				"update_id":    update.ID.String(),
				"package_name": req.PackageName,
				"package_type": req.PackageType,
				"dependencies": []string{}, // Empty dependencies array
			},
			Status:    models.CommandStatusPending,
			Source:    models.CommandSourceManual,
			CreatedAt: time.Now().UTC(),
		}

		// Check if heartbeat should be enabled (avoid duplicates)
		if shouldEnable, err := h.shouldEnableHeartbeat(agentID, 10); err == nil && shouldEnable {
			heartbeatCmd := &models.AgentCommand{
				ID:          uuid.Must(uuid.NewV4()),
				AgentID:     agentID,
				CommandType: models.CommandTypeEnableHeartbeat,
				Params: models.JSONB{
					"duration_minutes": 10,
				},
				Status:    models.CommandStatusPending,
				Source:    models.CommandSourceSystem,
				CreatedAt: time.Now().UTC(),
			}

			if err := h.agentHandler.signAndCreateCommand(heartbeatCmd); err != nil {
				log.Printf("[Heartbeat] Warning: Failed to create heartbeat command for agent %s: %v", agentID, err)
			} else {
				log.Printf("[Heartbeat] Command created for agent %s before installation", agentID)
			}
		} else {
			log.Printf("[Heartbeat] Skipping heartbeat command for agent %s (already active)", agentID)
		}

		if err := h.agentHandler.signAndCreateCommand(command); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create installation command"})
			return
		}

		// Record that dependencies were checked (empty array) and transition directly to installing
		if err := h.updateQueries.SetInstallingWithNoDependencies(update.ID, req.Dependencies); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status to installing"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "no dependencies found - installation command created automatically",
			"command_id": command.ID.String(),
		})
		return
	}

	// Dependencies EXIST: park the package in pending_dependencies. An operator
	// confirms it from the dashboard — unless auto-approval policy covers it, in
	// which case the orchestrator auto-confirms and mints the token below.
	if err := h.updateQueries.SetPendingDependencies(agentID, req.PackageType, req.PackageName, req.Dependencies); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status"})
		return
	}

	// Dependency-level supply-chain gate: the closure resolved at dry-run is the
	// exact artifact set the capability token will authorize the network-less
	// executor to install. The discovery-time OSV check only covered the
	// top-level package, so check every closure entry now. The result is
	// persisted on the row (closure_vulns, visible to the operator) and, when the
	// closure comes back clean, the orchestrator is triggered to auto-confirm if
	// policy + window allow. Async so the agent's report returns promptly.
	if update, err := h.updateQueries.GetUpdateByPackage(agentID, req.PackageType, req.PackageName); err == nil {
		updateID, pkgType, closure := update.ID, req.PackageType, req.Closure
		h.bg("closure_advance", func() { h.checkClosureAndAdvance(updateID, pkgType, closure) })
	} else {
		log.Printf("[ERROR] [server] [capability] closure_check_lookup_failed agent=%s pkg=%s/%s error=%v",
			agentID, req.PackageType, req.PackageName, err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "dependencies reported and status updated"})
}

// checkClosureAndAdvance runs the dependency-closure OSV check for a package
// that has just entered pending_dependencies, persists the result on the row,
// and — only when the closure is checked AND clean — triggers the orchestrator
// to auto-confirm without waiting for the next timer sweep. Fail-closed: if OSV
// cannot vet the closure, closure_checked_at is left unset and auto-confirm
// holds off until a later report re-attempts the check. A closure with vulns is
// recorded and left for an operator — auto-confirm never fires over it.
func (h *UpdateHandler) checkClosureAndAdvance(updateID uuid.UUID, pkgType string, closure []models.ClosureItem) {
	if !services.NeedsSupplyChainCheck(pkgType) {
		// No OSV mapping for this ecosystem — nothing to vet. Mark the closure
		// checked-and-clean so the gate doesn't stall, then advance.
		if err := h.updateQueries.StoreClosureCheck(updateID, "[]"); err != nil {
			log.Printf("[ERROR] [server] [capability] closure_check_store_failed update_id=%s error=%v", updateID, err)
			return
		}
	} else {
		entries := make([]services.ClosurePkg, 0, len(closure))
		for _, e := range closure {
			if e.Name == "" || e.Version == "" {
				continue
			}
			entries = append(entries, services.ClosurePkg{Name: e.Name, Version: e.Version})
		}

		found, ok := services.CheckClosureOSV(pkgType, entries)
		if !ok {
			// Fail-closed: a closure OSV could not vet is not "clean".
			log.Printf("[SECURITY] [server] [capability] closure_osv_incomplete update_id=%s closure_size=%d — not cleared, auto-confirm held",
				updateID, len(entries))
			if err := h.updateQueries.RecordClosureCheckError(updateID, "osv_closure_check_failed"); err != nil {
				log.Printf("[ERROR] [server] [capability] closure_error_store_failed update_id=%s error=%v", updateID, err)
			}
			return
		}

		closureVulnsJSON := "[]"
		if len(found) > 0 {
			if b, err := json.Marshal(found); err == nil {
				closureVulnsJSON = string(b)
			}
			log.Printf("[SECURITY] [server] [capability] closure_vulns_found update_id=%s artifacts=%d — auto-confirm blocked, operator review required",
				updateID, len(found))
		}
		if err := h.updateQueries.StoreClosureCheck(updateID, closureVulnsJSON); err != nil {
			log.Printf("[ERROR] [server] [capability] closure_check_store_failed update_id=%s error=%v", updateID, err)
			return
		}
		if len(found) > 0 {
			return // vulnerable closure — operator decides, no auto-advance
		}
	}

	if h.orchestrator != nil {
		h.orchestrator.OnDependenciesReported(updateID)
	}
}

// pinReportedClosure stores the top-level expected SHA256 and the reported
// closure. It deliberately does not mint an executable token: dependency
// confirmation is the authorization boundary.
func (h *UpdateHandler) pinReportedClosure(agentID uuid.UUID, req *models.DependencyReportRequest) error {
	updateID, err := uuid.FromString(req.UpdateID)
	if err != nil {
		return fmt.Errorf("invalid update id for closure pin: %w", err)
	}

	update, err := h.updateQueries.GetUpdateByID(updateID)
	if err != nil {
		return fmt.Errorf("load update for closure pin: %w", err)
	}

	// Trust boundary: the report is agent-authenticated; only let an agent pin a
	// hash onto its own update row.
	if update.AgentID != agentID {
		return fmt.Errorf("closure report agent mismatch")
	}

	if len(req.Closure) == 0 {
		if services.NeedsCapabilityGate(update.PackageType) {
			return fmt.Errorf("hashed closure required for capability-gated package %s/%s", req.PackageType, req.PackageName)
		}
		return nil
	}

	targetVersion, _ := h.resolveInstallTarget(update)
	if req.TargetVersion != "" && targetVersion != "" && strings.TrimSpace(req.TargetVersion) != targetVersion {
		return fmt.Errorf("dry-run target mismatch: report=%s expected=%s", req.TargetVersion, targetVersion)
	}

	top, ok := topLevelClosure(req.Closure, req.PackageName)
	if !ok {
		return fmt.Errorf("closure missing top-level artifact for %s", req.PackageName)
	}
	if top.SHA256 == "" {
		return fmt.Errorf("closure top-level artifact for %s has no sha256", req.PackageName)
	}
	if targetVersion != "" && top.Version != targetVersion {
		return fmt.Errorf("closure top-level version mismatch: report=%s expected=%s", top.Version, targetVersion)
	}

	// Pin the top-level artifact hash for Layer-1 install-time verification.
	if err := h.updateQueries.StoreExpectedSHA256(update.ID, top.SHA256); err != nil {
		return fmt.Errorf("store expected sha256: %w", err)
	}
	log.Printf("[INFO] [server] [capability] hash_pinned update_id=%s pkg=%s version=%s sha256=%s closure_size=%d",
		updateID, req.PackageName, top.Version, shortSHA(top.SHA256), len(req.Closure))

	closure := make([]capability.ClosureEntry, 0, len(req.Closure))
	for _, e := range req.Closure {
		if e.SHA256 == "" {
			continue
		}
		closure = append(closure, capability.ClosureEntry{
			Name:    e.Name,
			Version: e.Version,
			SHA256:  e.SHA256,
			Source:  e.Source,
		})

		// Each resolved artifact is a version with a known hash — record it in the
		// timeline catalog (same package_type as the update being installed).
		sha := e.SHA256
		pv := &models.PackageVersion{
			PackageType: update.PackageType,
			PackageName: e.Name,
			Version:     e.Version,
			SHA256:      &sha,
		}
		if e.Source != "" {
			src := e.Source
			pv.Source = &src
		}
		if err := h.updateQueries.UpsertPackageVersion(pv); err != nil {
			log.Printf("[WARNING] [server] [versions] closure_catalog_failed update_id=%s pkg=%s error=%v",
				update.ID, e.Name, err)
		}
	}
	if len(closure) == 0 {
		return fmt.Errorf("closure contains no hashed artifacts")
	}

	if err := h.updateQueries.StoreResolvedClosure(update.ID, closure); err != nil {
		return fmt.Errorf("store resolved closure: %w", err)
	} else {
		log.Printf("[INFO] [server] [capability] closure_stored update_id=%s closure_size=%d", updateID, len(closure))
	}
	return nil
}

func topLevelClosure(closure []models.ClosureItem, packageName string) (models.ClosureItem, bool) {
	for _, e := range closure {
		if e.Name == packageName {
			return e, true
		}
	}
	return models.ClosureItem{}, false
}

func shortSHA(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:16] + "..."
}

func (h *UpdateHandler) usesCapabilityExecution(packageType string) bool {
	return h.minter != nil && h.minter.Enabled() && h.tokenQueries != nil && services.NeedsCapabilityGate(packageType)
}

func (h *UpdateHandler) mintResolvedClosure(update *models.UpdateState) (*capability.Token, error) {
	if !h.usesCapabilityExecution(update.PackageType) {
		return nil, nil
	}
	closure, err := h.updateQueries.GetResolvedClosure(update.ID)
	if err != nil {
		return nil, err
	}
	if len(closure) == 0 {
		return nil, fmt.Errorf("no resolved closure stored for update")
	}
	if active, err := h.tokenQueries.HasActiveForUpdate(update.ID, time.Now().UTC().Unix()); err != nil {
		return nil, err
	} else if active {
		log.Printf("[INFO] [server] [capability] mint_skipped update_id=%s reason=active_token_exists", update.ID)
		return nil, nil
	}
	return h.minter.MintForUpdate(update, closure)
}

// enableHeartbeatBeforeInstall asks the agent to poll rapidly so it picks up the
// minted token / install command without waiting a full poll interval. Best
// effort and de-duplicated: a no-op when a heartbeat is already active.
func (h *UpdateHandler) enableHeartbeatBeforeInstall(agentID uuid.UUID) {
	shouldEnable, err := h.shouldEnableHeartbeat(agentID, 10)
	if err != nil || !shouldEnable {
		if err != nil {
			log.Printf("[Heartbeat] Warning: heartbeat check failed for agent %s: %v", agentID, err)
		}
		return
	}
	heartbeatCmd := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     agentID,
		CommandType: models.CommandTypeEnableHeartbeat,
		Params:      models.JSONB{"duration_minutes": 10},
		Status:      models.CommandStatusPending,
		Source:      models.CommandSourceSystem,
		CreatedAt:   time.Now().UTC(),
	}
	if err := h.agentHandler.signAndCreateCommand(heartbeatCmd); err != nil {
		log.Printf("[Heartbeat] Warning: Failed to create heartbeat command for agent %s: %v", agentID, err)
	}
}

// confirmCapabilityInstall is the capability-path confirm core shared by the
// operator-driven ConfirmDependencies endpoint and the orchestrator's headless
// ConfirmDependenciesAuto. It enables a heartbeat, transitions the package to
// installing, and mints the capability token over the already-stored closure.
// On mint failure it marks the package failed and returns the error. The
// maintenance window and capability-path eligibility are the caller's
// responsibility — this method assumes both have been checked.
func (h *UpdateHandler) confirmCapabilityInstall(update *models.UpdateState) error {
	h.enableHeartbeatBeforeInstall(update.AgentID)

	if err := h.updateQueries.InstallUpdate(update.ID); err != nil {
		return fmt.Errorf("set installing: %w", err)
	}
	if _, err := h.mintResolvedClosure(update); err != nil {
		log.Printf("[SECURITY] [server] [capability] confirm_mint_failed update_id=%s pkg=%s error=%v",
			update.ID, update.PackageName, err)
		if statusErr := h.updateQueries.UpdatePackageStatus(update.AgentID, update.PackageType, update.PackageName, models.StatusFailed, models.JSONB{
			"capability_authorization_error": err.Error(),
		}, nil); statusErr != nil {
			log.Printf("[ERROR] [server] [capability] confirm_mint_failed_status_update_failed update_id=%s error=%v",
				update.ID, statusErr)
		}
		return err
	}
	return nil
}

// ConfirmDependenciesAuto is the orchestrator's headless entry to the
// capability-path confirm core (satisfies orchestrator.DependencyConfirmer). It
// returns handled=false for non-capability (legacy) packages so the orchestrator
// leaves them for the operator. The caller has already checked the maintenance
// window and auto-approval policy.
func (h *UpdateHandler) ConfirmDependenciesAuto(update *models.UpdateState) (bool, error) {
	if !h.usesCapabilityExecution(update.PackageType) {
		return false, nil
	}
	if err := h.confirmCapabilityInstall(update); err != nil {
		return true, err
	}
	return true, nil
}

// ConfirmDependencies handles user confirmation to proceed with dependency installation
func (h *UpdateHandler) ConfirmDependencies(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// Get the update details
	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	// Check maintenance window before proceeding
	if inside, err := h.maintenanceWindowQueries.IsWithinMaintenanceWindow(time.Now().UTC()); err != nil {
		log.Printf("[ERROR] [server] [maintenance_window] check_failed id=%s error=%v", id, err)
	} else if !inside {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Installation blocked: outside configured maintenance window",
		})
		return
	}

	// Capability-gated packages (dnf/apt) go through the token path: transition
	// to installing and mint the signed closure. Shared with the orchestrator's
	// headless auto-confirm via confirmCapabilityInstall.
	if h.usesCapabilityExecution(update.PackageType) {
		if err := h.confirmCapabilityInstall(update); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize capability token"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "dependency installation confirmed and capability token authorized for agent execution",
		})
		return
	}

	// Legacy command path (docker/winget/Windows): send a confirm_dependencies
	// command for the agent to execute directly.
	command := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     update.AgentID,
		CommandType: models.CommandTypeConfirmDependencies,
		Params: map[string]interface{}{
			"update_id":    id.String(),
			"package_name": update.PackageName,
			"package_type": update.PackageType,
			"dependencies": update.Metadata["dependencies"], // Dependencies stored in metadata
		},
		Status:    models.CommandStatusPending,
		Source:    models.CommandSourceManual,
		CreatedAt: time.Now().UTC(),
	}

	h.enableHeartbeatBeforeInstall(update.AgentID)

	// Store the command in database
	if err := h.agentHandler.signAndCreateCommand(command); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create confirmation command"})
		return
	}

	// Update the package status to 'installing'
	if err := h.updateQueries.InstallUpdate(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "dependency installation confirmed and command created",
		"command_id": command.ID.String(),
	})
}

// GetAllLogs retrieves logs across all agents with filtering for universal log view
// Returns the fleet activity feed across all five event sources.
func (h *UpdateHandler) GetAllLogs(c *gin.Context) {
	filters := &models.LogFilters{
		Action:   c.Query("action"),
		Result:   c.Query("result"),
		Type:     c.Query("type"),
		Severity: c.Query("severity"),
	}

	// Parse agent_id if provided
	if agentIDStr := c.Query("agent_id"); agentIDStr != "" {
		agentID, err := uuid.FromString(agentIDStr)
		if err == nil {
			filters.AgentID = agentID
		}
	}

	// Parse since timestamp if provided
	if sinceStr := c.Query("since"); sinceStr != "" {
		sinceTime, err := time.Parse(time.RFC3339, sinceStr)
		if err == nil {
			filters.Since = &sinceTime
		}
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	filters.Page = page
	filters.PageSize = pageSize

	// Get fleet activity across all event sources
	items, total, err := h.updateQueries.GetFleetActivity(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve history"})
		return
	}

	// Populate narrative so the UI never has to compose its own verbiage.
	// A retry carries the same action/result as its original, so the renderer
	// stays focused on the outcome and the retry context is prefixed here.
	for i := range items {
		narrative := services.RenderUpdateLog(items[i].Action, items[i].Result, items[i].Stderr)
		if items[i].IsRetry {
			narrative = "Retry — " + narrative
		}
		items[i].Narrative = narrative
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":      items, // JSON key kept as "logs" for frontend compatibility
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetActiveOperations retrieves currently running operations for live status view
func (h *UpdateHandler) GetActiveOperations(c *gin.Context) {
	operations, err := h.updateQueries.GetActiveOperations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve active operations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"operations": operations,
		"count":      len(operations),
	})
}

// buildRetryCommand constructs a new pending command that retries `original`,
// signs it via the agent handler's keying flow (F-5: retries must re-sign), and
// persists it. Returns the new command on success.
//
// This is the single security-sensitive code path used by RetryCommand (caller
// supplies the command ID directly). Failed updates now reopen/resolve through
// the lifecycle endpoints rather than a command-level retry.
func (h *UpdateHandler) buildRetryCommand(original *models.AgentCommand) (*models.AgentCommand, error) {
	newCommand := &models.AgentCommand{
		ID:            uuid.Must(uuid.NewV4()),
		AgentID:       original.AgentID,
		CommandType:   original.CommandType,
		Params:        original.Params,
		Status:        models.CommandStatusPending,
		Source:        original.Source,
		CreatedAt:     time.Now().UTC(),
		RetriedFromID: &original.ID,
	}
	if err := h.agentHandler.signAndCreateCommand(newCommand); err != nil {
		return nil, err
	}
	return newCommand, nil
}

// RetryCommand retries a failed, timed_out, or cancelled command by command ID.
func (h *UpdateHandler) RetryCommand(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command ID"})
		return
	}

	original, err := h.commandQueries.GetCommandByID(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to get original command: %v", err)})
		return
	}

	if original.Status != "failed" && original.Status != "timed_out" && original.Status != "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command must be failed, timed_out, or cancelled to retry"})
		return
	}

	newCommand, err := h.buildRetryCommand(original)
	if err != nil {
		log.Printf("[ERROR] [server] [updates] RetryCommand: build/sign failed for original=%s agent=%s: %v", id, original.AgentID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to retry command: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "command retry created",
		"command_id": newCommand.ID.String(),
		"new_id":     newCommand.ID.String(),
	})
}

// GetUpdateLifecycleHistory returns the lifecycle transition history for the
// package behind an update, scoped to that update's agent. Backs the Lifecycle
// History panel on the update detail view.
// GET /updates/:id/lifecycle
func (h *UpdateHandler) GetUpdateLifecycleHistory(c *gin.Context) {
	updateID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	update, err := h.updateQueries.GetUpdateByID(updateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	history, err := h.updateQueries.GetPackageHistory(update.AgentID, update.PackageType, update.PackageName, 100)
	if err != nil {
		log.Printf("[ERROR] [server] [updates] lifecycle_history_failed update=%s: %v", updateID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load lifecycle history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"history":      history,
		"package_type": update.PackageType,
		"package_name": update.PackageName,
		"count":        len(history),
	})
}

// ReopenUpdate moves a failed update back to pending for another lifecycle
// attempt, clearing the failure markers. The orchestrator re-evaluates it on
// the next discovery sweep (fired immediately when wired). The transition guard
// rejects anything not currently failed.
// POST /updates/:id/reopen
func (h *UpdateHandler) ReopenUpdate(c *gin.Context) {
	updateID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	if err := h.updateQueries.ReopenFailedUpdate(updateID); err != nil {
		log.Printf("[WARNING] [server] [updates] reopen_failed update=%s: %v", updateID, err)
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot reopen update: %v", err)})
		return
	}

	if h.orchestrator != nil {
		h.orchestrator.OnPackagesDiscovered()
	}

	log.Printf("[INFO] [server] [updates] update_reopened update=%s", updateID)
	c.JSON(http.StatusOK, gin.H{"message": "update reopened"})
}

// ResolveUpdate closes a failed update as installed — used when the update no
// longer applies (already current, resolved out of band) so it leaves the
// actionable list. The transition guard rejects anything not currently failed.
// POST /updates/:id/resolve
func (h *UpdateHandler) ResolveUpdate(c *gin.Context) {
	updateID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	if err := h.updateQueries.ResolveFailedUpdate(updateID); err != nil {
		log.Printf("[WARNING] [server] [updates] resolve_failed update=%s: %v", updateID, err)
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot resolve update: %v", err)})
		return
	}

	log.Printf("[INFO] [server] [updates] update_resolved update=%s", updateID)
	c.JSON(http.StatusOK, gin.H{"message": "update resolved as installed"})
}

// CancelCommand cancels a pending or sent command
func (h *UpdateHandler) CancelCommand(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command ID"})
		return
	}

	// Capture the command before cancelling so the audit event carries agent + type.
	command, _ := h.commandQueries.GetCommandByID(id)

	if err := h.commandQueries.CancelCommand(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to cancel command: %v", err)})
		return
	}

	// ETHOS #1: cancellation is an operator-driven state change against an agent's
	// command — journal it inward so it surfaces in the events API / history.
	if command != nil {
		event := &models.SystemEvent{
			ID:           uuid.Must(uuid.NewV4()),
			AgentID:      &command.AgentID,
			EventType:    models.EventTypeCommandFailed,
			EventSubtype: "cancelled",
			Severity:     models.SeverityWarning,
			Component:    models.ComponentServer,
			Message:      fmt.Sprintf("Command %s (%s) cancelled by operator", command.CommandType, command.ID),
			Metadata: map[string]interface{}{
				"command_id":   command.ID.String(),
				"command_type": command.CommandType,
				"prior_status": command.Status,
			},
			CreatedAt: time.Now().UTC(),
		}
		if err := h.agentQueries.CreateSystemEvent(event); err != nil {
			log.Printf("[WARNING] [server] [updates] system_event_write_failed command_id=%s error=%v", command.ID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "command cancelled"})
}

// GetActiveCommands retrieves currently active commands for live operations view
func (h *UpdateHandler) GetActiveCommands(c *gin.Context) {
	commands, err := h.commandQueries.GetActiveCommands()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve active commands"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"commands": commands,
		"count":    len(commands),
	})
}

// GetCommandByID retrieves a single command by ID. Used by the UI to poll
// for the result of a capture_screenshot command.
func (h *UpdateHandler) GetCommandByID(c *gin.Context) {
	commandID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command ID"})
		return
	}

	command, err := h.commandQueries.GetCommandByID(commandID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "command not found"})
		return
	}

	c.JSON(http.StatusOK, command)
}

// GetRecentCommands retrieves recent commands for retry functionality
func (h *UpdateHandler) GetRecentCommands(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	commands, err := h.commandQueries.GetRecentCommands(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve recent commands"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"commands": commands,
		"count":    len(commands),
		"limit":    limit,
	})
}

// ClearFailedCommands manually removes failed/timed_out commands with cheeky warning
func (h *UpdateHandler) ClearFailedCommands(c *gin.Context) {
	// Get query parameters for filtering
	olderThanDaysStr := c.Query("older_than_days")
	onlyRetriedStr := c.Query("only_retried")
	allFailedStr := c.Query("all_failed")

	var count int64
	var err error

	// Parse parameters
	olderThanDays := 7 // default
	if olderThanDaysStr != "" {
		if days, err := strconv.Atoi(olderThanDaysStr); err == nil && days > 0 {
			olderThanDays = days
		}
	}

	onlyRetried := onlyRetriedStr == "true"
	allFailed := allFailedStr == "true"

	// Build the appropriate cleanup query based on parameters
	if allFailed {
		// Clear ALL failed commands regardless of age (most aggressive)
		count, err = h.commandQueries.ClearAllFailedCommandsRegardlessOfAge()
	} else if onlyRetried {
		// Clear only failed commands that have been retried
		count, err = h.commandQueries.ClearRetriedFailedCommands(olderThanDays)
	} else {
		// Clear failed commands older than specified days (default behavior)
		count, err = h.commandQueries.ClearOldFailedCommands(olderThanDays)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to clear failed commands",
			"details": err.Error(),
		})
		return
	}

	// Return success with cheeky message
	message := fmt.Sprintf("Archived %d failed commands", count)
	if count > 0 {
		message += ". WARNING: This shouldn't be necessary if the retry logic is working properly - you might want to check what's causing commands to fail in the first place!"
		message += " (History preserved - commands moved to archived status)"
	} else {
		message += ". No failed commands found matching your criteria. SUCCESS!"
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        message,
		"count":          count,
		"cheeky_warning": "Consider this a developer experience enhancement - the system should clean up after itself automatically!",
	})
}

// GetExpectedHash returns the expected SHA256 hash for a package (Layer 1: Hash Registry)
// The agent calls this to verify packages before installation
func (h *UpdateHandler) GetExpectedHash(c *gin.Context) {
	packageType := c.Query("package_type")
	packageName := c.Query("package_name")
	agentIDStr := c.Query("agent_id")

	if packageType == "" || packageName == "" || agentIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package_type, package_name, and agent_id are required"})
		return
	}

	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_id"})
		return
	}

	update, err := h.updateQueries.GetUpdateByPackage(agentID, packageType, packageName)
	if err != nil {
		if err.Error() == "sql: no rows" {
			c.JSON(http.StatusNotFound, gin.H{"error": "update not found for this package"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to lookup update"})
		}
		return
	}

	sha, err := h.updateQueries.GetExpectedSHA256(update.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve hash"})
		return
	}

	if sha == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no expected hash stored for this update"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"package_type":    packageType,
		"package_name":    packageName,
		"expected_sha256": sha,
	})
}

// GetCapabilityTokens delivers an agent's minted-but-undelivered capability
// tokens. The agent passes each to the privileged executor, which verifies the
// signature and artifact hashes independently before performing the operation.
// Tokens are marked delivered as they are served; the executor's local replay
// guard — not delivery state — is the authority on single use.
func (h *UpdateHandler) GetCapabilityTokens(c *gin.Context) {
	if h.tokenQueries == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "capability gate not enabled"})
		return
	}

	agentID := c.MustGet("agent_id").(uuid.UUID)

	ids, err := h.tokenQueries.ListUndeliveredForAgent(agentID, time.Now().UTC().Unix())
	if err != nil {
		log.Printf("[ERROR] [server] [capability] list_undelivered_failed agent_id=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list capability tokens"})
		return
	}

	tokens := make([]*capability.Token, 0, len(ids))
	for _, id := range ids {
		token, err := h.tokenQueries.GetByID(id)
		if err != nil {
			log.Printf("[ERROR] [server] [capability] token_load_failed token_id=%s error=%v", id, err)
			continue
		}
		tokens = append(tokens, token)
		if err := h.tokenQueries.MarkDelivered(id); err != nil {
			log.Printf("[WARNING] [server] [capability] mark_delivered_failed token_id=%s error=%v", id, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

// GetCapabilityTokenStatus returns token lifecycle history for an update.
// Authenticated via WebAuthMiddleware (dashboard), not agent auth.
// Returns 404 when no tokens exist for the given (agent_id, update_id) pair.
func (h *UpdateHandler) GetCapabilityTokenStatus(c *gin.Context) {
	if h.tokenQueries == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "capability gate not enabled"})
		return
	}

	agentIDStr := c.Query("agent_id")
	updateIDStr := c.Query("update_id")
	if agentIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id query parameter is required"})
		return
	}
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_id"})
		return
	}

	var updateID uuid.UUID
	if updateIDStr != "" {
		updateID, err = uuid.FromString(updateIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update_id"})
			return
		}
	}

	history, err := h.tokenQueries.GetTokenHistory(agentID, updateID)
	if err != nil {
		log.Printf("[ERROR] [server] [capability] token_history_query_failed agent_id=%s update_id=%s error=%v",
			agentID, updateID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query token history"})
		return
	}

	activeCount, err := h.tokenQueries.GetActiveTokenCount()
	if err != nil {
		log.Printf("[ERROR] [server] [capability] active_token_count_failed error=%v", err)
		// Non-fatal: return what we have
		activeCount = 0
	}

	type tokenRow struct {
		queries.TokenStatusHistory
		Status string `json:"status"`
	}
	out := make([]tokenRow, 0, len(history))
	for _, t := range history {
		s := t.Status()
		out = append(out, tokenRow{TokenStatusHistory: t, Status: s})
	}

	c.JSON(http.StatusOK, gin.H{
		"tokens":       out,
		"count":        len(out),
		"active_count": activeCount,
	})
}

// ReportCapabilityResult records the executor's outcome for a delivered token.
// This is an audit receipt: the executor's local replay guard is authoritative on
// single use, so a receipt only marks the server-side consumed_at and logs the
// decision. Tokens are bound to the calling agent; an agent cannot post a receipt
// for a token that is not its own.
// Idempotency: duplicate receipt is rejected with 409 Conflict (ETHOS #4).
func (h *UpdateHandler) ReportCapabilityResult(c *gin.Context) {
	if h.tokenQueries == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "capability gate not enabled"})
		return
	}

	agentID := c.MustGet("agent_id").(uuid.UUID)
	tokenID, err := uuid.FromString(c.Param("token_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token_id"})
		return
	}

	var body struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
		ExitCode int    `json:"exit_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid receipt body"})
		return
	}

	token, err := h.tokenQueries.GetByID(tokenID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	if token.AgentID != agentID.String() {
		log.Printf("[SECURITY] [server] [capability] receipt_agent_mismatch token_id=%s token_agent=%s caller_agent=%s",
			tokenID, token.AgentID, agentID)
		c.JSON(http.StatusForbidden, gin.H{"error": "token not bound to this agent"})
		return
	}

	// Idempotency guard: reject duplicate receipt (ETHOS #4).
	if consumed, err := h.tokenQueries.IsConsumed(tokenID); err != nil {
		log.Printf("[ERROR] [server] [capability] receipt_consumed_check_failed token_id=%s error=%v", tokenID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check token state"})
		return
	} else if consumed {
		log.Printf("[INFO] [server] [capability] duplicate_receipt_rejected token_id=%s agent_id=%s", tokenID, agentID)
		c.JSON(http.StatusConflict, gin.H{"error": "duplicate receipt — token already consumed"})
		return
	}

	if _, err := h.tokenQueries.MarkConsumed(tokenID); err != nil {
		log.Printf("[ERROR] [server] [capability] mark_consumed_failed token_id=%s error=%v", tokenID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record receipt"})
		return
	}

	if updateID, ok, err := h.tokenQueries.GetUpdateIDForToken(tokenID); err != nil {
		log.Printf("[ERROR] [server] [capability] receipt_update_lookup_failed token_id=%s error=%v", tokenID, err)
	} else if ok {
		if update, err := h.updateQueries.GetUpdateByID(updateID); err != nil {
			log.Printf("[ERROR] [server] [capability] receipt_update_load_failed token_id=%s update_id=%s error=%v",
				tokenID, updateID, err)
		} else {
			status := models.StatusFailed
			if body.Decision == "executed" && body.ExitCode == 0 {
				status = models.StatusInstalled
			}
			metadata := models.JSONB{
				"capability_token_id":  tokenID.String(),
				"capability_decision":  body.Decision,
				"capability_reason":    body.Reason,
				"capability_exit_code": body.ExitCode,
			}
			completedAt := time.Now().UTC()
			if err := h.updateQueries.UpdatePackageStatus(update.AgentID, update.PackageType, update.PackageName, status, metadata, &completedAt); err != nil {
				log.Printf("[ERROR] [server] [capability] receipt_status_update_failed token_id=%s update_id=%s status=%s error=%v",
					tokenID, updateID, status, err)
			} else {
				log.Printf("[INFO] [server] [capability] receipt_status_updated token_id=%s update_id=%s status=%s",
					tokenID, updateID, status)
				// Pin the installed version so subsequent scans don't
				// silently advance to a newer unapproved version.
				if status == models.StatusInstalled {
					installedVersion := update.AvailableVersion
					if update.SelectedVersion != nil && *update.SelectedVersion != "" {
						installedVersion = *update.SelectedVersion
					}
					if installedVersion != "" {
						_ = h.updateQueries.SetInstalledVersionHold(updateID, installedVersion)
					}
				}
			}
		}
	}

	log.Printf("[SECURITY] [server] [capability] receipt token_id=%s agent_id=%s decision=%s reason=%s exit=%d",
		tokenID, agentID, body.Decision, body.Reason, body.ExitCode)

	c.JSON(http.StatusOK, gin.H{"message": "receipt recorded"})
}
