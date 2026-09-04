package queries

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/capability"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/doug-martin/goqu/v9"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

type UpdateQueries struct {
	db *sqlx.DB
}

func NewUpdateQueries(db *sqlx.DB) *UpdateQueries {
	return &UpdateQueries{db: db}
}

// GetUpdateByID retrieves a single update by ID from the new state table
func (q *UpdateQueries) GetUpdateByID(id uuid.UUID) (*models.UpdateState, error) {
	var update models.UpdateState
	query := `SELECT * FROM current_package_state WHERE id = $1`
	err := q.db.Get(&update, query, id)
	if err != nil {
		return nil, err
	}
	return &update, nil
}

// GetExpectedSHA256 retrieves the expected SHA256 hash for an update by ID
func (q *UpdateQueries) GetExpectedSHA256(id uuid.UUID) (string, error) {
	var sha sql.NullString
	query := `SELECT expected_sha256 FROM current_package_state WHERE id = $1`
	err := q.db.Get(&sha, query, id)
	if err != nil {
		return "", err
	}
	if !sha.Valid {
		return "", nil
	}
	return sha.String, nil
}

// StoreExpectedSHA256 stores the expected SHA256 hash for an update
func (q *UpdateQueries) StoreExpectedSHA256(id uuid.UUID, sha string) error {
	query := `UPDATE current_package_state SET expected_sha256 = $1 WHERE id = $2`
	_, err := q.db.Exec(query, sha, id)
	return err
}

// StoreResolvedClosure records the agent-reported artifact closure without
// authorizing execution. Final authorization happens when the operator confirms.
func (q *UpdateQueries) StoreResolvedClosure(id uuid.UUID, closure []capability.ClosureEntry) error {
	closureJSON, err := json.Marshal(closure)
	if err != nil {
		return err
	}
	query := `
		UPDATE current_package_state
		SET metadata = jsonb_set(
				COALESCE(metadata, '{}'::jsonb),
				'{resolved_closure}',
				$2::jsonb,
				true
			),
			last_updated_at = NOW()
		WHERE id = $1`
	_, err = q.db.Exec(query, id, closureJSON)
	return err
}

// GetResolvedClosure returns the closure recorded during dry-run.
func (q *UpdateQueries) GetResolvedClosure(id uuid.UUID) ([]capability.ClosureEntry, error) {
	var raw sql.NullString
	query := `SELECT metadata->'resolved_closure' FROM current_package_state WHERE id = $1`
	if err := q.db.Get(&raw, query, id); err != nil {
		return nil, err
	}
	if !raw.Valid || raw.String == "" || raw.String == "null" {
		return nil, nil
	}
	var closure []capability.ClosureEntry
	if err := json.Unmarshal([]byte(raw.String), &closure); err != nil {
		return nil, err
	}
	return closure, nil
}

// GetUpdateByPackage retrieves a single update by agent_id, package_type, and package_name
func (q *UpdateQueries) GetUpdateByPackage(agentID uuid.UUID, packageType, packageName string) (*models.UpdateState, error) {
	var update models.UpdateState
	query := `SELECT * FROM current_package_state WHERE agent_id = $1 AND package_type = $2 AND package_name = $3`
	err := q.db.Get(&update, query, agentID, packageType, packageName)
	if err != nil {
		return nil, err
	}
	return &update, nil
}

// PackageFleetEntry is one agent's view of a package across the fleet: the same
// package name+type as it sits on each host, with the per-host version, status,
// and pinned hash. It answers "which agents have this package / are affected."
type PackageFleetEntry struct {
	AgentID          uuid.UUID `json:"agent_id" db:"agent_id"`
	Hostname         string    `json:"hostname" db:"hostname"`
	UpdateID         uuid.UUID `json:"update_id" db:"update_id"`
	CurrentVersion   string    `json:"current_version" db:"current_version"`
	AvailableVersion string    `json:"available_version" db:"available_version"`
	Status           string    `json:"status" db:"status"`
	Severity         string    `json:"severity" db:"severity"`
	ExpectedSHA256   *string   `json:"expected_sha256" db:"expected_sha256"`
	LastUpdatedAt    time.Time `json:"last_updated_at" db:"last_updated_at"`
	SelectedVersion  *string   `json:"selected_version" db:"selected_version"`
	// Action flags — derived from status after query, not stored.
	CanApprove bool `json:"can_approve" db:"-"`
	CanInstall bool `json:"can_install" db:"-"`
	CanRetry   bool `json:"can_retry" db:"-"`
}

// UpsertPackageVersion records or enriches one version in the timeline catalog. It is
// idempotent: re-running a scan or approval bumps last_seen_at and fills in any newly
// known provenance (publish date / hash / OSV) via COALESCE, never clobbering known
// data with nulls and never duplicating a (type, name, version). Scan-time callers
// pass only source; approval-time callers add publish/hash/OSV.
func (q *UpdateQueries) UpsertPackageVersion(pv *models.PackageVersion) error {
	if pv.PackageType == "" || pv.PackageName == "" || pv.Version == "" {
		return nil // nothing to anchor a catalog row on; skip silently
	}
	query := `
		INSERT INTO package_versions
			(package_type, package_name, version, published_at, sha256, osv_status, osv_vulns, source, first_scanned_at, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, now(), now())
		ON CONFLICT (package_type, package_name, version) DO UPDATE SET
			last_seen_at = now(),
			published_at = COALESCE(EXCLUDED.published_at, package_versions.published_at),
			sha256       = COALESCE(EXCLUDED.sha256, package_versions.sha256),
			osv_status   = COALESCE(EXCLUDED.osv_status, package_versions.osv_status),
			osv_vulns    = COALESCE(EXCLUDED.osv_vulns, package_versions.osv_vulns),
			source       = COALESCE(EXCLUDED.source, package_versions.source)`
	_, err := q.db.Exec(query,
		pv.PackageType, pv.PackageName, pv.Version,
		pv.PublishedAt, pv.SHA256, pv.OSVStatus, pv.OSVVulns, pv.Source)
	return err
}

// GetPackageVersions returns the version timeline for a package, newest first by the
// best available time signal (publish date when known, else first-scanned). Drives the
// detail pane's Version Timeline and is the read surface for as-of-date resolution.
func (q *UpdateQueries) GetPackageVersions(packageType, packageName string) ([]models.PackageVersion, error) {
	query := `
		SELECT id, package_type, package_name, version, published_at,
		       first_scanned_at, last_seen_at, sha256, osv_status, osv_vulns, source
		FROM package_versions
		WHERE package_type = $1 AND package_name = $2
		ORDER BY COALESCE(published_at, first_scanned_at) DESC`
	var rows []models.PackageVersion
	if err := q.db.Select(&rows, query, packageType, packageName); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetVersionOSVStatus returns the osv_status and osv_vulns for a specific version
// from the package_versions catalog. Returns ("", nil) when no row exists for that
// version — the caller treats this as "not yet checked" (distinct from "clean").
func (q *UpdateQueries) GetVersionOSVStatus(packageType, packageName, version string) (string, []byte, error) {
	query := `SELECT osv_status, osv_vulns FROM package_versions
		WHERE package_type = $1 AND package_name = $2 AND version = $3`
	var row struct {
		OSVStatus string `db:"osv_status"`
		OSVVulns  []byte `db:"osv_vulns"`
	}
	if err := q.db.Get(&row, query, packageType, packageName, version); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return "", nil, nil
		}
		return "", nil, err
	}
	return row.OSVStatus, row.OSVVulns, nil
}

// AggregatedPackage is one package rolled up across the whole fleet — the row the
// package-centric Updates view renders. Versions are scalar samples (exact when the
// fleet agrees, otherwise a count tells the UI to say "N versions"). RepresentativeID
// is any one update row for the package, so the client can drill into the detail/fleet
// view without already knowing the package coordinates.
type AggregatedPackage struct {
	PackageType      string    `json:"package_type" db:"package_type"`
	PackageName      string    `json:"package_name" db:"package_name"`
	AgentCount       int       `json:"agent_count" db:"agent_count"`
	VersionCount     int       `json:"version_count" db:"version_count"`
	SampleCurrent    string    `json:"sample_current_version" db:"sample_current_version"`
	SampleAvailable  string    `json:"sample_available_version" db:"sample_available_version"`
	MaxSeverityRank  int       `json:"max_severity_rank" db:"max_severity_rank"`
	HasVulns         bool      `json:"has_vulns" db:"has_vulns"`
	VulnCount        int       `json:"vuln_count" db:"vuln_count"`
	HashPinnedCount  int       `json:"hash_pinned_count" db:"hash_pinned_count"`
	PendingCount     int       `json:"pending_count" db:"pending_count"`
	ApprovedCount    int       `json:"approved_count" db:"approved_count"`
	InstallingCount  int       `json:"installing_count" db:"installing_count"`
	InstalledCount   int       `json:"installed_count" db:"installed_count"`
	FailedCount      int       `json:"failed_count" db:"failed_count"`
	PendingDepsCount int       `json:"pending_dependencies_count" db:"pending_dependencies_count"`
	LastDiscoveredAt time.Time `json:"last_discovered_at" db:"last_discovered_at"`
	RepresentativeID uuid.UUID `json:"representative_id" db:"representative_id"`
}

// ListAggregatedPackages rolls up current_package_state by (package_type, name) for
// the package-centric Updates view. Supports a name search and a type filter; returns
// the rows for the page and the total distinct-package count. Severity is ranked in
// SQL (critical=4 … low=1) and mapped back to a label by the caller.
func (q *UpdateQueries) ListAggregatedPackages(search, packageType, status, sortBy, sortOrder string, page, pageSize int) ([]AggregatedPackage, int, error) {
	where := []string{}
	args := []interface{}{}
	i := 1
	if search != "" {
		where = append(where, fmt.Sprintf("package_name ILIKE $%d", i))
		args = append(args, "%"+search+"%")
		i++
	}
	if packageType != "" {
		where = append(where, fmt.Sprintf("package_type = $%d", i))
		args = append(args, packageType)
		i++
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Post-aggregation status filter — filters to packages that have at least one
	// row in the requested status. Keeps the per-status counts accurate (the HAVING
	// only gates which groups are returned, not the aggregate values themselves).
	havingClause := ""
	if status != "" {
		havingClause = fmt.Sprintf("HAVING COUNT(*) FILTER (WHERE status = $%d) > 0", i)
		args = append(args, status)
		i++
	}

	severityRank := `MAX(CASE LOWER(severity)
		WHEN 'critical' THEN 4 WHEN 'high' THEN 3
		WHEN 'medium' THEN 2 WHEN 'moderate' THEN 2
		WHEN 'low' THEN 1 ELSE 0 END)`
	vulnArrayExpr := `CASE
				WHEN jsonb_typeof(NULLIF(metadata->>'supply_chain_vulns', '')::jsonb) = 'array'
				THEN NULLIF(metadata->>'supply_chain_vulns', '')::jsonb
				ELSE '[]'::jsonb
			END`

	// Whitelist sort columns; default to most-recently-discovered.
	orderCol := "last_discovered_at"
	switch sortBy {
	case "package_name":
		orderCol = "package_name"
	case "agent_count":
		orderCol = "agent_count"
	case "severity":
		orderCol = "max_severity_rank"
	case "last_discovered_at", "":
		orderCol = "last_discovered_at"
	}
	dir := "DESC"
	if strings.EqualFold(sortOrder, "asc") {
		dir = "ASC"
	}

	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM (
		SELECT 1 FROM current_package_state %s GROUP BY package_type, package_name %s
	) t`, whereClause, havingClause)
	if err := q.db.Get(&total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	if pageSize <= 0 {
		pageSize = 100
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	query := fmt.Sprintf(`
		SELECT
			package_type,
			package_name,
			COUNT(DISTINCT agent_id) AS agent_count,
			COUNT(DISTINCT available_version) AS version_count,
			MAX(current_version) AS sample_current_version,
			MAX(available_version) AS sample_available_version,
			%s AS max_severity_rank,
			bool_or(jsonb_array_length(%s) > 0) AS has_vulns,
			COALESCE(MAX(jsonb_array_length(%s)), 0) AS vuln_count,
			COUNT(*) FILTER (WHERE expected_sha256 IS NOT NULL) AS hash_pinned_count,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending_count,
			COUNT(*) FILTER (WHERE status = 'approved') AS approved_count,
			COUNT(*) FILTER (WHERE status = 'installing') AS installing_count,
			COUNT(*) FILTER (WHERE status = 'installed') AS installed_count,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
			COUNT(*) FILTER (WHERE status = 'pending_dependencies') AS pending_dependencies_count,
			MAX(last_discovered_at) AS last_discovered_at,
			MIN(id::text)::uuid AS representative_id
		FROM current_package_state
		%s
		GROUP BY package_type, package_name
		%s
		ORDER BY %s %s, package_name ASC
		LIMIT %d OFFSET %d`,
		severityRank, vulnArrayExpr, vulnArrayExpr, whereClause, havingClause, orderCol, dir, pageSize, offset)

	var rows []AggregatedPackage
	if err := q.db.Select(&rows, query, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetPackageFleet returns every agent that tracks the given package, joined to the
// agent hostname, ordered by hostname. Used by the package detail pane to show
// which hosts are affected and how their versions/pins differ.
func (q *UpdateQueries) GetPackageFleet(packageType, packageName string) ([]PackageFleetEntry, error) {
	query := `
		SELECT cps.agent_id, a.hostname, cps.id AS update_id,
		       cps.current_version, cps.available_version, cps.status,
		       cps.severity, cps.expected_sha256, cps.last_updated_at,
		       cps.selected_version
		FROM current_package_state cps
		JOIN agents a ON a.id = cps.agent_id
		WHERE cps.package_type = $1 AND cps.package_name = $2
		ORDER BY a.hostname`
	var rows []PackageFleetEntry
	if err := q.db.Select(&rows, query, packageType, packageName); err != nil {
		return nil, err
	}
	for i := range rows {
		s := models.PackageStatus(rows[i].Status)
		rows[i].CanApprove = s == models.StatusPending
		rows[i].CanInstall = s == models.StatusApproved
		rows[i].CanRetry = s == models.StatusFailed
	}
	return rows, nil
}

// packageSummaryRow is the raw SQL scan target for GetPackageSummary.
type packageSummaryRow struct {
	PackageType     string       `db:"package_type"`
	PackageName     string       `db:"package_name"`
	Severity        string       `db:"severity"`
	Metadata        models.JSONB `db:"metadata"`
	TotalAgents     int          `db:"total_agents"`
	PendingCount    int          `db:"pending_count"`
	ApprovedCount   int          `db:"approved_count"`
	ActiveCount     int          `db:"active_count"`
	InstalledCount  int          `db:"installed_count"`
	FailedCount     int          `db:"failed_count"`
	IgnoredCount    int          `db:"ignored_count"`
	LatestAvailable string       `db:"latest_available"`
	LatestInstalled *string      `db:"latest_installed"`
}

// GetPackageSummary returns the package-centric aggregate view for a given
// (package_type, package_name): fleet status counts, version extremes, and
// enrichment metadata extracted from the most recently updated agent row.
func (q *UpdateQueries) GetPackageSummary(packageType, packageName string) (*models.PackageSummary, error) {
	query := `
		SELECT
		    package_type,
		    package_name,
		    (SELECT severity FROM current_package_state
		     WHERE package_type = $1 AND package_name = $2
		     ORDER BY CASE severity
		       WHEN 'critical' THEN 4 WHEN 'high' THEN 3
		       WHEN 'medium'   THEN 2 WHEN 'low'  THEN 1 ELSE 0
		     END DESC LIMIT 1)                                     AS severity,
		    (SELECT metadata FROM current_package_state
		     WHERE package_type = $1 AND package_name = $2
		     ORDER BY last_updated_at DESC LIMIT 1)                AS metadata,
		    COUNT(*)                                               AS total_agents,
		    COUNT(*) FILTER (WHERE status = 'pending')             AS pending_count,
		    COUNT(*) FILTER (WHERE status = 'approved')            AS approved_count,
		    COUNT(*) FILTER (WHERE status IN (
		        'checking_dependencies','pending_dependencies','installing'))
		                                                           AS active_count,
		    COUNT(*) FILTER (WHERE status = 'installed')           AS installed_count,
		    COUNT(*) FILTER (WHERE status = 'failed')              AS failed_count,
		    COUNT(*) FILTER (WHERE status = 'ignored')             AS ignored_count,
		    COALESCE(MAX(available_version), '')                   AS latest_available,
		    MAX(current_version) FILTER (WHERE status = 'installed') AS latest_installed
		FROM current_package_state
		WHERE package_type = $1 AND package_name = $2
		GROUP BY package_type, package_name`

	var row packageSummaryRow
	if err := q.db.Get(&row, query, packageType, packageName); err != nil {
		return nil, err
	}

	// Enrich via the existing UpdateState machinery.
	tmp := &models.UpdateState{Metadata: row.Metadata}
	tmp.EnrichFromMetadata()

	return &models.PackageSummary{
		PackageType:        row.PackageType,
		PackageName:        row.PackageName,
		Severity:           row.Severity,
		PackageDescription: tmp.PackageDescription,
		HomepageURL:        tmp.HomepageURL,
		SizeBytes:          tmp.SizeBytes,
		CVEList:            tmp.CVEList,
		Vulnerabilities:    tmp.Vulnerabilities,
		TotalAgents:        row.TotalAgents,
		PendingCount:       row.PendingCount,
		ApprovedCount:      row.ApprovedCount,
		ActiveCount:        row.ActiveCount,
		InstalledCount:     row.InstalledCount,
		FailedCount:        row.FailedCount,
		IgnoredCount:       row.IgnoredCount,
		LatestAvailable:    row.LatestAvailable,
		LatestInstalled:    row.LatestInstalled,
	}, nil
}

// GetPackageVulnerabilitiesByTypeAndName returns a deduplicated vulnerability
// list for a package by merging osv_vulns across all package_versions rows and
// supply_chain_vulns from current agent metadata. Source is "osv" for both
// (both originate from OSV batch checks); duplicates are suppressed by CVE ID.
func (q *UpdateQueries) GetPackageVulnerabilitiesByTypeAndName(packageType, packageName string) ([]models.VulnerabilityEntry, error) {
	// Collect from package_versions catalog (most authoritative — written at approval time).
	versionsQuery := `
		SELECT osv_vulns
		FROM package_versions
		WHERE package_type = $1 AND package_name = $2
		  AND osv_vulns IS NOT NULL AND osv_vulns != '[]'`
	var osvRows []string
	if err := q.db.Select(&osvRows, versionsQuery, packageType, packageName); err != nil {
		return nil, err
	}

	// Also collect supply_chain_vulns from live agent rows (catches packages not
	// yet through an approval cycle).
	agentQuery := `
		SELECT metadata->>'supply_chain_vulns'
		FROM current_package_state
		WHERE package_type = $1 AND package_name = $2
		  AND metadata->>'supply_chain_vulns' IS NOT NULL
		  AND metadata->>'supply_chain_vulns' != '[]'`
	var agentRows []string
	if err := q.db.Select(&agentRows, agentQuery, packageType, packageName); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var result []models.VulnerabilityEntry
	for _, raw := range append(osvRows, agentRows...) {
		var entries []models.VulnerabilityEntry
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			continue
		}
		for _, e := range entries {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			e.Source = "osv"
			result = append(result, e)
		}
	}
	return result, nil
}

// ApproveUpdate marks an update as approved in the new event sourcing system
// --- State machine: the single transition path ---------------------------
//
// Every status change on current_package_state routes through transitionStatus.
// It reads the observed status, validates the move against models.PackageStatusTransitions,
// performs a status-guarded UPDATE (so a concurrent caller cannot double-consume the
// transition), and records version history on terminal moves. The raw-SQL WHERE guards
// that used to live in each function were a half-measure: no named error, no idempotent
// self-check, and silently opt-out for any future caller. This is the one path now.

// statusSelector identifies which current_package_state row to transition: either by
// primary key (id) or by the (agent_id, package_type, package_name) natural key.
type statusSelector struct {
	byID        bool
	id          uuid.UUID
	agentID     uuid.UUID
	packageType string
	packageName string
}

func selByID(id uuid.UUID) statusSelector { return statusSelector{byID: true, id: id} }

func selByPackage(agentID uuid.UUID, packageType, packageName string) statusSelector {
	return statusSelector{agentID: agentID, packageType: packageType, packageName: packageName}
}

// where renders the row-matching predicate using positional placeholders starting at
// $start, returning the SQL fragment and its arguments in order.
func (s statusSelector) where(start int) (string, []interface{}) {
	if s.byID {
		return fmt.Sprintf("id = $%d", start), []interface{}{s.id}
	}
	return fmt.Sprintf("agent_id = $%d AND package_type = $%d AND package_name = $%d", start, start+1, start+2),
		[]interface{}{s.agentID, s.packageType, s.packageName}
}

func (s statusSelector) String() string {
	if s.byID {
		return "id=" + s.id.String()
	}
	return s.packageType + "/" + s.packageName
}

// transitionOpts carries the per-call extras the gold path (UpdatePackageStatus) needs:
// a caller-supplied completion timestamp and the metadata to stamp into version history.
// Both are zero-valued for the routine transition functions.
type transitionOpts struct {
	completedAt *time.Time
	historyMeta models.JSONB // nil → fall back to the row's current metadata
	// requireFrom, when non-empty, constrains the transition to rows currently in
	// this exact state. The state machine alone is too permissive for some callers:
	// e.g. installed → pending is a valid edge (scan-set reactivation), but the
	// "reopen a *failed* update" operation must not silently act on installed rows.
	// The check is atomic with the guarded UPDATE — both pin status = cur.Status.
	requireFrom models.PackageStatus
}

// transitionStatus is the validated, race-guarded core. It loads the selected row,
// validates current -> to via the state machine, performs the guarded UPDATE, and
// records terminal history. Returns the pre-transition row so callers can attach
// follow-up writes (metadata, dependency closures) within the same transaction.
func (q *UpdateQueries) transitionStatus(tx *sqlx.Tx, sel statusSelector, to models.PackageStatus, opts transitionOpts) (models.UpdateState, error) {
	var cur models.UpdateState
	selWhere, selArgs := sel.where(1)
	if err := tx.Get(&cur, `SELECT * FROM current_package_state WHERE `+selWhere, selArgs...); err != nil {
		return cur, fmt.Errorf("transition (%s): load current state: %w", sel, err)
	}

	if err := models.ValidateTransition(cur.Status, to); err != nil {
		return cur, fmt.Errorf("transition %s: %w", sel, err)
	}

	if opts.requireFrom != "" && cur.Status != opts.requireFrom {
		return cur, fmt.Errorf("transition %s -> %q: requires current state %q, got %q",
			sel, to, opts.requireFrom, cur.Status)
	}

	// Guarded UPDATE — $1 is the new status, the trailing placeholder re-checks the
	// status we observed so a racing caller that already advanced the row yields rows == 0.
	// On a transition *into* failed we also merge the failure metadata (failure_reason,
	// failed_by, ...) onto the live row so the dashboard can surface *why* it failed
	// without joining version history. jsonb concat (||) merges rather than replaces,
	// preserving prior keys (e.g. OSV results).
	updWhere, updArgs := sel.where(2)
	args := append([]interface{}{to}, updArgs...)
	var metaClause string
	if to == models.StatusFailed && opts.historyMeta != nil {
		args = append(args, opts.historyMeta)
		metaClause = fmt.Sprintf(", metadata = COALESCE(metadata, '{}'::jsonb) || $%d::jsonb", len(args))
	}
	args = append(args, cur.Status)
	updQuery := fmt.Sprintf(
		`UPDATE current_package_state SET status = $1, last_updated_at = NOW()%s WHERE %s AND status = $%d`,
		metaClause, updWhere, len(args))
	res, err := tx.Exec(updQuery, args...)
	if err != nil {
		return cur, fmt.Errorf("transition %s: update: %w", sel, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return cur, fmt.Errorf("transition %s -> %q: row changed concurrently", sel, to)
	}

	// Record terminal history only on an actual change of status (idempotent re-entry
	// at the same terminal state must not double-write history).
	if (to == models.StatusInstalled || to == models.StatusFailed) && cur.Status != to {
		completedAt := time.Now().UTC()
		if opts.completedAt != nil {
			completedAt = *opts.completedAt
		}
		meta := opts.historyMeta
		if meta == nil {
			meta = cur.Metadata
		}
		if err := q.recordTerminalHistory(tx, cur, to, completedAt, meta); err != nil {
			return cur, err
		}

		// Clear vulnerability metadata on installed transition — the threat is
		// remediated. Log each cleared advisory to security_events for later
		// grouping and audit.
		if to == models.StatusInstalled {
			if err := q.clearVulnsOnInstall(tx, cur); err != nil {
				log.Printf("[WARNING] [server] [updates] vuln_clearance_failed pkg=%s/%s error=%v",
					cur.PackageType, cur.PackageName, err)
				// Non-fatal: transition already committed, clearance is best-effort.
			}
			// Clear selected_version so subsequent scans use the latest available,
			// not the stale pinned version. Without this, the package stays pinned
			// until manual intervention.
			if err := clearTargetVersionTx(tx, cur.ID); err != nil {
				log.Printf("[WARNING] [server] [updates] clear_target_version_failed pkg=%s/%s error=%v",
					cur.PackageType, cur.PackageName, err)
				// Non-fatal: transition already committed, clearance is best-effort.
			}
		}
	}
	return cur, nil
}

// recordTerminalHistory appends a row to update_version_history for a terminal transition.
func (q *UpdateQueries) recordTerminalHistory(tx *sqlx.Tx, cur models.UpdateState, to models.PackageStatus, completedAt time.Time, meta models.JSONB) error {
	const historyQuery = `
		INSERT INTO update_version_history (
			agent_id, package_type, package_name, version_from, version_to,
			severity, repository_source, metadata, update_completed_at, update_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := tx.Exec(historyQuery,
		cur.AgentID, cur.PackageType, cur.PackageName, cur.CurrentVersion,
		cur.AvailableVersion, cur.Severity, cur.RepositorySource, meta, completedAt, to)
	if err != nil {
		return fmt.Errorf("record version history: %w", err)
	}
	return nil
}

// clearVulnsOnInstall clears vulnerability metadata when a package transitions
// to installed (threat remediated). Logs each cleared advisory to security_events
// for later grouping and audit.
func (q *UpdateQueries) clearVulnsOnInstall(tx *sqlx.Tx, cur models.UpdateState) error {
	if cur.Metadata == nil {
		return nil
	}

	// Collect advisories before clearing.
	var cleared []string
	for _, key := range []string{"supply_chain_vulns", "installed_vulns"} {
		raw, ok := cur.Metadata[key]
		if !ok || raw == nil {
			continue
		}
		var vulns []map[string]interface{}
		switch v := raw.(type) {
		case string:
			if err := json.Unmarshal([]byte(v), &vulns); err != nil {
				continue
			}
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					vulns = append(vulns, m)
				}
			}
		}
		for _, vuln := range vulns {
			if id, ok := vuln["id"].(string); ok && id != "" {
				cleared = append(cleared, id)
			}
		}
	}

	if len(cleared) == 0 {
		return nil
	}

	// Clear the vuln metadata keys.
	_, err := tx.Exec(`
		UPDATE current_package_state
		SET metadata = metadata - 'supply_chain_vulns' - 'installed_vulns'
		              - 'supply_chain_checked_at' - 'installed_checked_at'
		              - 'supply_chain_checked_version' - 'installed_checked_version'
		WHERE id = $1`, cur.ID)
	if err != nil {
		return fmt.Errorf("clear vuln metadata: %w", err)
	}

	// Log each cleared advisory to security_events.
	for _, advisoryID := range cleared {
		_, err := tx.Exec(`
			INSERT INTO security_events (timestamp, level, event_type, agent_id, message, trace_id, ip_address, details, metadata)
			VALUES (NOW(), 'INFO', 'SECURITY_ADVISORY_CLEARED', $1, $2, '', '', $3, '{}'::jsonb)`,
			cur.AgentID,
			fmt.Sprintf("Advisory %s cleared — %s/%s updated to %s",
				advisoryID, cur.PackageType, cur.PackageName, cur.AvailableVersion),
			fmt.Sprintf(`{"advisory_id":"%s","package_type":"%s","package_name":"%s","version_to":"%s"}`,
				advisoryID, cur.PackageType, cur.PackageName, cur.AvailableVersion))
		if err != nil {
			log.Printf("[WARNING] [server] [updates] advisory_clearance_log_failed advisory=%s error=%v",
				advisoryID, err)
			// Non-fatal: clearance already happened, logging is best-effort.
		}
	}

	log.Printf("[INFO] [server] [updates] vulns_cleared pkg=%s/%s count=%d advisories=%v",
		cur.PackageType, cur.PackageName, len(cleared), cleared)
	return nil
}

// runTransition opens a transaction, applies a single transition, runs an optional
// follow-up write in the same transaction, and commits.
func (q *UpdateQueries) runTransition(sel statusSelector, to models.PackageStatus, after func(tx *sqlx.Tx, cur models.UpdateState) error) error {
	return q.runTransitionOpts(sel, to, transitionOpts{}, after)
}

// runTransitionOpts is runTransition with caller-supplied transitionOpts (e.g. a
// requireFrom precondition). The precondition is validated inside the same
// transaction as the transition, so it is atomic with the guarded UPDATE.
func (q *UpdateQueries) runTransitionOpts(sel statusSelector, to models.PackageStatus, opts transitionOpts, after func(tx *sqlx.Tx, cur models.UpdateState) error) error {
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	cur, err := q.transitionStatus(tx, sel, to, opts)
	if err != nil {
		return err
	}
	if after != nil {
		if err := after(tx, cur); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TransitionByID is the exported transition entry point for callers outside this
// package (the lifecycle orchestrator). It applies a validated, race-guarded
// transition on the row identified by id, stamping historyMeta into version
// history on terminal moves. nil historyMeta falls back to the row's metadata.
func (q *UpdateQueries) TransitionByID(id uuid.UUID, to models.PackageStatus, historyMeta models.JSONB) error {
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := q.transitionStatus(tx, selByID(id), to, transitionOpts{historyMeta: historyMeta}); err != nil {
		return err
	}
	return tx.Commit()
}

// TransitionByPackage is the natural-key counterpart to TransitionByID.
func (q *UpdateQueries) TransitionByPackage(agentID uuid.UUID, packageType, packageName string, to models.PackageStatus, historyMeta models.JSONB) error {
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := q.transitionStatus(tx, selByPackage(agentID, packageType, packageName), to, transitionOpts{historyMeta: historyMeta}); err != nil {
		return err
	}
	return tx.Commit()
}

// TransitionByPackageFrom applies a package transition only when the row is
// currently in the expected source state. Use this for command-result paths
// where the state machine has broader edges for reconciler or operator flows.
func (q *UpdateQueries) TransitionByPackageFrom(agentID uuid.UUID, packageType, packageName string, from, to models.PackageStatus, historyMeta models.JSONB, completedAt *time.Time) error {
	return q.runTransitionOpts(
		selByPackage(agentID, packageType, packageName),
		to,
		transitionOpts{
			completedAt: completedAt,
			historyMeta: historyMeta,
			requireFrom: from,
		},
		nil,
	)
}

// GetPackagesInStatus returns every current_package_state row currently in the
// given status, ordered oldest-first by last_updated_at so the orchestrator
// sweeps the longest-waiting packages first.
func (q *UpdateQueries) GetPackagesInStatus(status models.PackageStatus) ([]models.UpdateState, error) {
	var rows []models.UpdateState
	query := `SELECT * FROM current_package_state WHERE status = $1 ORDER BY last_updated_at ASC`
	if err := q.db.Select(&rows, query, status); err != nil {
		return nil, fmt.Errorf("failed to list packages in status %q: %w", status, err)
	}
	return rows, nil
}

// BumpRetryCounter atomically increments an integer counter stored at metadata.{key}
// and returns the new value. Used by the orchestrator to bound dry-run re-enqueue
// attempts on stuck packages without holding state of its own.
func (q *UpdateQueries) BumpRetryCounter(id uuid.UUID, key string) (int, error) {
	var next int
	query := `
		UPDATE current_package_state
		SET metadata = jsonb_set(
				COALESCE(metadata, '{}'::jsonb),
				ARRAY[$2],
				to_jsonb(COALESCE((metadata->>$2)::int, 0) + 1),
				true
			)
		WHERE id = $1
		RETURNING (metadata->>$2)::int`
	if err := q.db.Get(&next, query, id, key); err != nil {
		return 0, fmt.Errorf("failed to bump retry counter %q: %w", key, err)
	}
	return next, nil
}

func (q *UpdateQueries) ApproveUpdate(id uuid.UUID, approvedBy string) error {
	return q.runTransition(selByID(id), models.StatusApproved, nil)
}

// ApproveUpdateWithVulns approves an update and merges supply chain metadata
// into the existing metadata JSONB. Uses || so concurrent writers (e.g.
// checkClosureAndAdvance) do not lose their keys.
func (q *UpdateQueries) ApproveUpdateWithVulns(id uuid.UUID, approvedBy string, metadata models.JSONB) error {
	return q.runTransition(selByID(id), models.StatusApproved, func(tx *sqlx.Tx, cur models.UpdateState) error {
		metaJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal supply chain metadata: %w", err)
		}
		if _, err := tx.Exec(`UPDATE current_package_state SET metadata = COALESCE(metadata, '{}'::jsonb) || $2::jsonb WHERE id = $1`, id, string(metaJSON)); err != nil {
			return fmt.Errorf("store supply chain metadata: %w", err)
		}
		return nil
	})
}

// ApproveUpdateByPackage approves an update by agent_id, package_type, and package_name
func (q *UpdateQueries) ApproveUpdateByPackage(agentID uuid.UUID, packageType, packageName, approvedBy string) error {
	return q.runTransition(selByPackage(agentID, packageType, packageName), models.StatusApproved, nil)
}

// BulkApproveUpdates approves multiple updates by their IDs
func (q *UpdateQueries) BulkApproveUpdates(updateIDs []uuid.UUID, approvedBy string) error {
	if len(updateIDs) == 0 {
		return nil
	}

	// Start transaction
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, id := range updateIDs {
		if _, err := q.transitionStatus(tx, selByID(id), models.StatusApproved, transitionOpts{}); err != nil {
			return fmt.Errorf("failed to approve update %s: %w", id, err)
		}
	}

	return tx.Commit()
}

// RejectUpdate marks an update as rejected/ignored
func (q *UpdateQueries) RejectUpdate(id uuid.UUID, rejectedBy string) error {
	return q.runTransition(selByID(id), models.StatusIgnored, nil)
}

// RejectUpdateByPackage rejects an update by agent_id, package_type, and package_name
func (q *UpdateQueries) RejectUpdateByPackage(agentID uuid.UUID, packageType, packageName, rejectedBy string) error {
	return q.runTransition(selByPackage(agentID, packageType, packageName), models.StatusIgnored, nil)
}

// ReopenFailedUpdate re-opens a failed package for another attempt by moving it
// back to pending and clearing the live failure markers from the row. The
// failure record itself stays in update_version_history — this only resets the
// actionable state so the normal approve -> dry-run -> install lifecycle (which
// re-checks whether the update still applies) can run again.
//
// Scoped to failed-only via requireFrom: the state machine permits installed ->
// pending (scan-set reactivation, RECONCILE-001), so the generic pending edge is
// not enough to keep this operation off installed rows. requireFrom pins it to
// rows that are actually failed.
func (q *UpdateQueries) ReopenFailedUpdate(id uuid.UUID) error {
	return q.runTransitionOpts(selByID(id), models.StatusPending, transitionOpts{requireFrom: models.StatusFailed}, func(tx *sqlx.Tx, _ models.UpdateState) error {
		_, err := tx.Exec(
			`UPDATE current_package_state
			 SET metadata = COALESCE(metadata, '{}'::jsonb) - 'failure_reason' - 'failed_by' - 'dry_run_attempts'
			 WHERE id = $1`, id)
		return err
	})
}

// ResolveFailedUpdate closes out a failed package as installed. Used when the
// update no longer applies (resolved out of band, package already current) so it
// leaves the actionable list. The installed transition records terminal history.
func (q *UpdateQueries) ResolveFailedUpdate(id uuid.UUID) error {
	return q.runTransition(selByID(id), models.StatusInstalled, nil)
}

// InstallUpdate marks an update as ready for installation
func (q *UpdateQueries) InstallUpdate(id uuid.UUID) error {
	return q.runTransition(selByID(id), models.StatusInstalling, nil)
}

// SetCheckingDependencies marks an update as being checked for dependencies
func (q *UpdateQueries) SetCheckingDependencies(id uuid.UUID) error {
	return q.runTransition(selByID(id), models.StatusCheckingDependencies, nil)
}

// SetPendingDependencies stores dependency information and sets status based on whether dependencies exist
// If dependencies array is empty, this function only updates metadata without changing status
// (the handler should auto-approve and proceed to installation in this case)
// If dependencies array has items, status is set to 'pending_dependencies' requiring manual approval
func (q *UpdateQueries) SetPendingDependencies(agentID uuid.UUID, packageType, packageName string, dependencies []string) error {
	// Marshal dependencies to JSON for database storage
	depsJSON, err := json.Marshal(dependencies)
	if err != nil {
		return fmt.Errorf("failed to marshal dependencies: %w", err)
	}

	// Note: When dependencies array is empty, the handler should bypass this status change
	// and proceed directly to installation. This function still records the empty array
	// in metadata for audit purposes before the handler transitions to 'installing'.
	return q.runTransition(selByPackage(agentID, packageType, packageName), models.StatusPendingDependencies,
		func(tx *sqlx.Tx, cur models.UpdateState) error {
			_, err := tx.Exec(`
				UPDATE current_package_state
				SET metadata = jsonb_set(
						jsonb_set(metadata, '{dependencies}', $4::jsonb),
						'{dependencies_reported_at}', to_jsonb(NOW()))
				WHERE agent_id = $1 AND package_type = $2 AND package_name = $3`,
				agentID, packageType, packageName, depsJSON)
			if err != nil {
				return fmt.Errorf("store reported dependencies: %w", err)
			}
			return nil
		})
}

// SetInstallingWithNoDependencies records zero dependencies and transitions directly to installing
// This function is used when a package has NO dependencies and can skip the pending_dependencies state
func (q *UpdateQueries) SetInstallingWithNoDependencies(id uuid.UUID, dependencies []string) error {
	depsJSON, err := json.Marshal(dependencies)
	if err != nil {
		return fmt.Errorf("failed to marshal dependencies: %w", err)
	}

	return q.runTransition(selByID(id), models.StatusInstalling, func(tx *sqlx.Tx, cur models.UpdateState) error {
		_, err := tx.Exec(`
			UPDATE current_package_state
			SET metadata = jsonb_set(
					jsonb_set(metadata, '{dependencies}', $2::jsonb),
					'{dependencies_reported_at}', to_jsonb(NOW()))
			WHERE id = $1`, id, depsJSON)
		if err != nil {
			return fmt.Errorf("store dependency closure: %w", err)
		}
		return nil
	})
}

// CreateUpdateLog inserts an update log entry
func (q *UpdateQueries) CreateUpdateLog(log *models.UpdateLog) error {
	query := `
		INSERT INTO update_logs (
			id, agent_id, update_package_id, action, result,
			stdout, stderr, exit_code, duration_seconds
		) VALUES (
			:id, :agent_id, :update_package_id, :action, :result,
			:stdout, :stderr, :exit_code, :duration_seconds
		)
	`
	_, err := q.db.NamedExec(query, log)
	return err
}

// NEW EVENT SOURCING IMPLEMENTATION

// CreateUpdateEvent stores a single update event
func (q *UpdateQueries) CreateUpdateEvent(event *models.UpdateEvent) error {
	query := `
		INSERT INTO update_events (
			agent_id, package_type, package_name, version_from, version_to,
			severity, repository_source, metadata, event_type
		) VALUES (
			:agent_id, :package_type, :package_name, :version_from, :version_to,
			:severity, :repository_source, :metadata, :event_type
		)
	`
	_, err := q.db.NamedExec(query, event)
	return err
}

// CreateUpdateEventsBatch creates multiple update events in a transaction
func (q *UpdateQueries) CreateUpdateEventsBatch(events []models.UpdateEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Start transaction
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create batch record
	batch := &models.UpdateBatch{
		ID:        uuid.Must(uuid.NewV4()),
		AgentID:   events[0].AgentID,
		BatchSize: len(events),
		Status:    "processing",
	}

	batchQuery := `
		INSERT INTO update_batches (id, agent_id, batch_size, status)
		VALUES (:id, :agent_id, :batch_size, :status)
	`
	if _, err := tx.NamedExec(batchQuery, batch); err != nil {
		return fmt.Errorf("failed to create batch record: %w", err)
	}

	// Insert events in batches to avoid memory issues
	batchSize := 100
	processedCount := 0
	failedCount := 0

	for i := 0; i < len(events); i += batchSize {
		end := i + batchSize
		if end > len(events) {
			end = len(events)
		}

		currentBatch := events[i:end]

		// Prepare query with multiple value sets
		query := `
			INSERT INTO update_events (
				agent_id, package_type, package_name, version_from, version_to,
				severity, repository_source, metadata, event_type
			) VALUES (
				:agent_id, :package_type, :package_name, :version_from, :version_to,
				:severity, :repository_source, :metadata, :event_type
			)
		`

		for _, event := range currentBatch {
			_, err := tx.NamedExec(query, event)
			if err != nil {
				failedCount++
				continue
			}
			processedCount++

			// Update current state
			if err := q.UpdateCurrentStateInTx(tx, &event); err != nil {
				// Log error but don't fail the entire batch
				log.Printf("[WARNING] [server] [database] update_state_failed package=%s error=%v", event.PackageName, err)
			}
		}
	}

	// Update batch record
	batchUpdateQuery := `
		UPDATE update_batches
		SET processed_count = $1, failed_count = $2, status = $3, completed_at = $4
		WHERE id = $5
	`
	batchStatus := "completed"
	if failedCount > 0 {
		batchStatus = "completed_with_errors"
	}

	_, err = tx.Exec(batchUpdateQuery, processedCount, failedCount, batchStatus, time.Now().UTC(), batch.ID)
	if err != nil {
		return fmt.Errorf("failed to update batch record: %w", err)
	}

	// Commit transaction
	return tx.Commit()
}

// updateCurrentStateInTx updates the current_package_state table within a transaction
func (q *UpdateQueries) UpdateCurrentStateInTx(tx *sqlx.Tx, event *models.UpdateEvent) error {
	query := `
		INSERT INTO current_package_state (
			agent_id, package_type, package_name, current_version, available_version,
			severity, repository_source, metadata, last_discovered_at, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
		ON CONFLICT (agent_id, package_type, package_name)
		DO UPDATE SET
			current_version = CASE
			WHEN current_package_state.status = 'installing'
			THEN current_package_state.current_version
			ELSE EXCLUDED.current_version
		END,
			available_version = EXCLUDED.available_version,
			severity = EXCLUDED.severity,
			repository_source = EXCLUDED.repository_source,
			metadata = COALESCE(current_package_state.metadata, '{}'::jsonb) || EXCLUDED.metadata,
			last_discovered_at = EXCLUDED.last_discovered_at,
			-- Re-discovery reconciliation: this CASE is the SQL twin of
			-- models.ReconcileFromScan — keep them in sync (RECONCILE-001).
			--
			-- Rules (Target Model, 2026-06-06):
			--   ignored : preserved — operator decision, scan cannot override
			--   failed  : preserved — recovery is operator-driven (see
			--             ReopenFailedUpdate / ResolveFailedUpdate); auto-reopen
			--             would mask failure context and trigger install loops
			--             under auto-approve
			--   installed: reopen to pending — a package reappearing in check-update
			--             means a *new* version is available; this is deliberate
			--             reactivation (RECONCILE-001 Target Model). The state machine
			--             now allows installed → pending.
			--   all other states: reset to pending
			status = CASE
				WHEN current_package_state.status IN ('ignored', 'failed')
				THEN current_package_state.status
				ELSE 'pending'
			END
	`
	_, err := tx.Exec(query,
		event.AgentID,
		event.PackageType,
		event.PackageName,
		event.VersionFrom,
		event.VersionTo,
		event.Severity,
		event.RepositorySource,
		event.Metadata,
		event.CreatedAt)
	return err
}

// UpsertCurrentState inserts or updates a row in current_package_state from an
// UpdateEvent. Opens its own transaction — use for single-row writes outside a batch.
func (q *UpdateQueries) UpsertCurrentState(event *models.UpdateEvent) error {
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := q.UpdateCurrentStateInTx(tx, event); err != nil {
		return err
	}

	return tx.Commit()
}

// ListUpdatesFromState returns paginated updates from current state with filtering
func (q *UpdateQueries) ListUpdatesFromState(filters *models.UpdateFilters) ([]models.UpdateState, int, error) {
	var updates []models.UpdateState

	sd := PG().From("current_package_state")

	if filters.AgentID != uuid.Nil {
		sd = sd.Where(goqu.Ex{"agent_id": filters.AgentID})
	}
	if filters.PackageType != "" {
		sd = sd.Where(goqu.Ex{"package_type": filters.PackageType})
	}
	if filters.Severity != "" {
		sd = sd.Where(goqu.Ex{"severity": filters.Severity})
	}
	if filters.Status != "" {
		statuses := strings.Split(string(filters.Status), ",")
		if len(statuses) == 1 {
			sd = sd.Where(goqu.Ex{"status": statuses[0]})
		} else {
			sd = sd.Where(goqu.Ex{"status": statuses})
		}
	} else {
		sd = sd.Where(goqu.C("status").NotIn("installed", "ignored"))
	}

	total, err := Paginated(q.db, sd, uint(filters.Page), uint(filters.PageSize),
		goqu.C("last_discovered_at").Desc(),
		[]string{"id", "agent_id", "package_type", "package_name", "current_version",
			"available_version", "severity", "repository_source", "metadata",
			"last_discovered_at", "last_updated_at", "status"},
		&updates)
	if err != nil {
		return nil, 0, err
	}

	return updates, total, nil
}

// GetPackageHistory returns version history for a specific package
func (q *UpdateQueries) GetPackageHistory(agentID uuid.UUID, packageType, packageName string, limit int) ([]models.UpdateHistory, error) {
	var history []models.UpdateHistory

	query := `
		SELECT
			id, agent_id, package_type, package_name, version_from, version_to,
			severity, repository_source, metadata, update_initiated_at,
			update_completed_at, update_status, failure_reason
		FROM update_version_history
		WHERE agent_id = $1 AND package_type = $2 AND package_name = $3
		ORDER BY update_completed_at DESC
		LIMIT $4
	`

	err := q.db.Select(&history, query, agentID, packageType, packageName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get package history: %w", err)
	}

	return history, nil
}

// UpdatePackageStatus updates the status of a package and records history.
// Transition validation is performed via models.ValidateTransition; the SQL
// UPDATE is guarded on the current status to prevent races.
// completedAt is optional - if nil, uses time.Now().UTC(). Pass actual completion time for accurate audit trails.
func (q *UpdateQueries) UpdatePackageStatus(agentID uuid.UUID, packageType, packageName string, status models.PackageStatus, metadata models.JSONB, completedAt *time.Time) error {
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := q.transitionStatus(tx, selByPackage(agentID, packageType, packageName), status,
		transitionOpts{completedAt: completedAt, historyMeta: metadata}); err != nil {
		return err
	}

	return tx.Commit()
}

// CleanupOldEvents removes old events to prevent table bloat
func (q *UpdateQueries) CleanupOldEvents(olderThan time.Duration) error {
	query := `DELETE FROM update_events WHERE created_at < $1`
	result, err := q.db.Exec(query, time.Now().UTC().Add(-olderThan))
	if err != nil {
		return fmt.Errorf("failed to cleanup old events: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[INFO] [server] [database] update_events_cleanup removed=%d", rowsAffected)
	return nil
}

// GetBatchStatus returns the status of recent batches
func (q *UpdateQueries) GetBatchStatus(agentID uuid.UUID, limit int) ([]models.UpdateBatch, error) {
	var batches []models.UpdateBatch

	query := `
		SELECT id, agent_id, batch_size, processed_count, failed_count,
			   status, error_details, created_at, completed_at
		FROM update_batches
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	err := q.db.Select(&batches, query, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch status: %w", err)
	}

	return batches, nil
}

// GetUpdateStatsFromState returns statistics about updates from current state
func (q *UpdateQueries) GetUpdateStatsFromState(agentID uuid.UUID) (*models.UpdateStats, error) {
	stats := &models.UpdateStats{}

	query := `
		SELECT
			COUNT(*) as total_updates,
			COUNT(*) FILTER (WHERE status = 'pending') as pending_updates,
			COUNT(*) FILTER (WHERE status = 'installed') as installed_updates,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_updates,
			COUNT(*) FILTER (WHERE severity = 'critical'  AND status NOT IN ('installed', 'failed', 'ignored')) as critical_updates,
			COUNT(*) FILTER (WHERE severity = 'important' AND status NOT IN ('installed', 'failed', 'ignored')) as important_updates,
			COUNT(*) FILTER (WHERE severity = 'moderate'  AND status NOT IN ('installed', 'failed', 'ignored')) as moderate_updates,
			COUNT(*) FILTER (WHERE severity = 'low'       AND status NOT IN ('installed', 'failed', 'ignored')) as low_updates
		FROM current_package_state
		WHERE agent_id = $1
	`

	err := q.db.Get(stats, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get update stats: %w", err)
	}

	return stats, nil
}

// GetAllUpdateStats returns overall statistics about updates across all agents
func (q *UpdateQueries) GetAllUpdateStats() (*models.UpdateStats, error) {
	stats := &models.UpdateStats{}

	// Severity counts are scoped to non-terminal statuses: a package that is
	// already installed, failed, or ignored is not "attention-worthy" severity
	// on the dashboard. Counting them inflated the bars past 100% because the
	// denominator (total_updates / pending_updates) is also scoped smaller.
	// UI-DASHBOARD-AUDIT finding #2.
	query := `
		SELECT
			COUNT(*) as total_updates,
			COUNT(*) FILTER (WHERE status = 'pending') as pending_updates,
			COUNT(*) FILTER (WHERE status = 'approved') as approved_updates,
			COUNT(*) FILTER (WHERE status = 'installed') as installed_updates,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_updates,
			COUNT(*) FILTER (WHERE severity = 'critical'  AND status NOT IN ('installed', 'failed', 'ignored')) as critical_updates,
			COUNT(*) FILTER (WHERE severity = 'important' AND status NOT IN ('installed', 'failed', 'ignored')) as high_updates,
			COUNT(*) FILTER (WHERE severity = 'moderate'  AND status NOT IN ('installed', 'failed', 'ignored')) as moderate_updates,
			COUNT(*) FILTER (WHERE severity = 'low'       AND status NOT IN ('installed', 'failed', 'ignored')) as low_updates
		FROM current_package_state
	`

	err := q.db.Get(stats, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all update stats: %w", err)
	}

	return stats, nil
}

// typeCount is one row of GetUpdatesByType.
type typeCount struct {
	PackageType string `db:"package_type"`
	Count       int    `db:"count"`
}

// GetUpdatesByType returns a count of actionable updates grouped by package type.
// Scoped to non-terminal statuses to match the severity breakdown: installed,
// failed, and ignored packages are not actionable workload. The Dashboard's
// "Updates by Type" card reads this — without it the card always renders empty
// because the handler never populated the map. UI-DASHBOARD-AUDIT.
func (q *UpdateQueries) GetUpdatesByType() (map[string]int, error) {
	var rows []typeCount
	query := `
		SELECT package_type, COUNT(*) as count
		FROM current_package_state
		WHERE status NOT IN ('installed', 'failed', 'ignored')
		GROUP BY package_type
	`
	if err := q.db.Select(&rows, query); err != nil {
		return nil, fmt.Errorf("failed to get updates by type: %w", err)
	}

	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.PackageType] = r.Count
	}
	return out, nil
}

// GetAvailableFixCount returns the number of distinct OSV advisory IDs present
// in supply_chain_vulns across all non-terminal package rows. Deduplicates by
// advisory ID so sub-packages sharing one advisory count as one, not many.
func (q *UpdateQueries) GetAvailableFixCount() (int, error) {
	query := `
		SELECT COUNT(DISTINCT vuln->>'id')
		FROM current_package_state,
		     jsonb_array_elements(
		       CASE WHEN jsonb_typeof((metadata->>'supply_chain_vulns')::jsonb) = 'array'
		            THEN (metadata->>'supply_chain_vulns')::jsonb
		            ELSE '[]'::jsonb
		       END
		     ) AS vuln
		WHERE metadata IS NOT NULL
		  AND metadata ? 'supply_chain_vulns'
		  AND metadata->>'supply_chain_vulns' NOT IN ('[]', '')
		  AND status NOT IN ('installed', 'ignored', 'failed')
	`
	var count int
	if err := q.db.Get(&count, query); err != nil {
		return 0, fmt.Errorf("get security update count: %w", err)
	}
	return count, nil
}

// GetOpenThreatCount returns the number of distinct OSV advisory IDs found
// in installed_vulns — the threat check against the currently-installed version.
// Only counts rows in active states (not installed/ignored/failed).
func (q *UpdateQueries) GetOpenThreatCount() (int, error) {
	query := `
		SELECT COUNT(DISTINCT vuln->>'id')
		FROM current_package_state,
		     jsonb_array_elements(
		       CASE WHEN jsonb_typeof((metadata->>'installed_vulns')::jsonb) = 'array'
		            THEN (metadata->>'installed_vulns')::jsonb
		            ELSE '[]'::jsonb
		       END
		     ) AS vuln
		WHERE metadata IS NOT NULL
		  AND metadata ? 'installed_vulns'
		  AND metadata->>'installed_vulns' NOT IN ('[]', '')
		  AND status NOT IN ('installed', 'ignored', 'failed')
	`
	var count int
	if err := q.db.Get(&count, query); err != nil {
		return 0, fmt.Errorf("get installed CVE count: %w", err)
	}
	return count, nil
}

// TopThreat is one row of the dashboard's worst-open-threats summary — a
// distinct advisory affecting installed versions, with the packages it hits.
type TopThreat struct {
	ID             string  `db:"id" json:"id"`
	Severity       string  `db:"severity" json:"severity,omitempty"`
	CVSSScore      float64 `db:"cvss_score" json:"cvss_score,omitempty"`
	KnownExploited bool    `db:"known_exploited" json:"known_exploited,omitempty"`
	Packages       string  `db:"packages" json:"packages"`
}

// GetTopOpenThreats returns the worst distinct advisories on installed
// versions — KEV entries first, then by CVSS score. Same active-row predicate
// as GetOpenThreatCount.
func (q *UpdateQueries) GetTopOpenThreats(limit int) ([]TopThreat, error) {
	query := `
		SELECT vuln->>'id' AS id,
		       COALESCE(MAX(NULLIF(vuln->>'severity','')), '') AS severity,
		       COALESCE(MAX((NULLIF(vuln->>'cvss_score',''))::float8), 0) AS cvss_score,
		       BOOL_OR(COALESCE((vuln->>'known_exploited')::boolean, false)) AS known_exploited,
		       STRING_AGG(DISTINCT package_name, ', ') AS packages
		FROM current_package_state,
		     jsonb_array_elements(
		       CASE WHEN jsonb_typeof((metadata->>'installed_vulns')::jsonb) = 'array'
		            THEN (metadata->>'installed_vulns')::jsonb
		            ELSE '[]'::jsonb
		       END
		     ) AS vuln
		WHERE metadata IS NOT NULL
		  AND metadata ? 'installed_vulns'
		  AND metadata->>'installed_vulns' NOT IN ('[]', '')
		  AND status NOT IN ('installed', 'ignored', 'failed')
		GROUP BY vuln->>'id'
		ORDER BY known_exploited DESC, cvss_score DESC, id DESC
		LIMIT $1
	`
	var threats []TopThreat
	if err := q.db.Select(&threats, query, limit); err != nil {
		return nil, fmt.Errorf("get top open threats: %w", err)
	}
	return threats, nil
}

// GetUpdateLogs retrieves installation logs for a specific update
func (q *UpdateQueries) GetUpdateLogs(updateID uuid.UUID, limit int) ([]models.UpdateLog, error) {
	var logs []models.UpdateLog

	query := `
		SELECT
			id, agent_id, update_package_id, action, result,
			stdout, stderr, exit_code, duration_seconds, executed_at
		FROM update_logs
		WHERE update_package_id = $1
		ORDER BY executed_at DESC
		LIMIT $2
	`

	if limit == 0 {
		limit = 50 // Default limit
	}

	err := q.db.Select(&logs, query, updateID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get update logs: %w", err)
	}

	return logs, nil
}

// GetAllLogs retrieves logs across all agents with filtering
func (q *UpdateQueries) GetAllLogs(filters *models.LogFilters) ([]models.UpdateLog, int, error) {
	var logs []models.UpdateLog
	whereClause := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	// Add filters
	if filters.AgentID != uuid.Nil {
		whereClause = append(whereClause, fmt.Sprintf("agent_id = $%d", argIdx))
		args = append(args, filters.AgentID)
		argIdx++
	}

	if filters.Action != "" {
		whereClause = append(whereClause, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, filters.Action)
		argIdx++
	}

	if filters.Result != "" {
		whereClause = append(whereClause, fmt.Sprintf("result = $%d", argIdx))
		args = append(args, filters.Result)
		argIdx++
	}

	if filters.Since != nil {
		whereClause = append(whereClause, fmt.Sprintf("executed_at >= $%d", argIdx))
		args = append(args, filters.Since)
		argIdx++
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM update_logs WHERE " + strings.Join(whereClause, " AND ")
	var total int
	err := q.db.Get(&total, countQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get logs count: %w", err)
	}

	// Get paginated results
	query := fmt.Sprintf(`
		SELECT
			id, agent_id, update_package_id, action, result,
			stdout, stderr, exit_code, duration_seconds, executed_at
		FROM update_logs
		WHERE %s
		ORDER BY executed_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(whereClause, " AND "), argIdx, argIdx+1)

	limit := filters.PageSize
	if limit == 0 {
		limit = 100 // Default limit
	}
	offset := (filters.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	err = q.db.Select(&logs, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get all logs: %w", err)
	}

	return logs, total, nil
}

// ActivityItem represents a single item in fleet activity (a command, log,
// package event, install transition, or system event).
type ActivityItem struct {
	ID              uuid.UUID `json:"id" db:"id"`
	AgentID         uuid.UUID `json:"agent_id" db:"agent_id"`
	Type            string    `json:"type" db:"type"` // "command" or "log"
	Action          string    `json:"action" db:"action"`
	Status          string    `json:"status" db:"status"`
	Result          string    `json:"result" db:"result"`
	PackageName     string    `json:"package_name" db:"package_name"`
	PackageType     string    `json:"package_type" db:"package_type"`
	Stdout          string    `json:"stdout" db:"stdout"`
	Stderr          string    `json:"stderr" db:"stderr"`
	ExitCode        int       `json:"exit_code" db:"exit_code"`
	DurationSeconds int       `json:"duration_seconds" db:"duration_seconds"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	Hostname        string    `json:"hostname" db:"hostname"`
	// IsRetry marks a command that re-attempts an earlier one (retried_from_id
	// set). Logs are never retries. RetriedFromID links to the original command
	// so the UI can thread the chain. Both come straight from the row.
	IsRetry       bool       `json:"is_retry" db:"is_retry"`
	RetriedFromID *uuid.UUID `json:"retried_from_id,omitempty" db:"retried_from_id"`
	// Narrative is a server-rendered, operator-facing summary. Populated by
	// the handler before responding (services.RenderUpdateLog). Not stored.
	Narrative string `json:"narrative,omitempty" db:"-"`
}

// GetFleetActivity retrieves the fleet activity feed across all five event sources.
func (q *UpdateQueries) GetFleetActivity(filters *models.LogFilters) ([]ActivityItem, int, error) {
	whereClause := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	// Add filters
	if filters.AgentID != uuid.Nil {
		whereClause = append(whereClause, fmt.Sprintf("agent_id = $%d", argIdx))
		args = append(args, filters.AgentID)
		argIdx++
	}

	if filters.Action != "" {
		whereClause = append(whereClause, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, filters.Action)
		argIdx++
	}

	if filters.Result != "" {
		whereClause = append(whereClause, fmt.Sprintf("result = $%d", argIdx))
		args = append(args, filters.Result)
		argIdx++
	}

	if filters.Since != nil {
		whereClause = append(whereClause, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, filters.Since)
		argIdx++
	}

	// Type filter — restricts to one source arm ("command", "log", "package_event",
	// "install_transition", "system_event").
	if filters.Type != "" {
		whereClause = append(whereClause, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, filters.Type)
		argIdx++
	}

	// Severity filter — for system_events this is the severity column; for other
	// types the "result" column carries status-like values so we match both.
	if filters.Severity != "" {
		whereClause = append(whereClause, fmt.Sprintf("result = $%d", argIdx))
		args = append(args, filters.Severity)
		argIdx++
	}

	// Build the activity query using UNION ALL
	whereStr := strings.Join(whereClause, " AND ")

	// Each source arm projects the same 16-column shape. Filters are applied once
	// on the outer aliased result below — never injected per-arm: the logs arm
	// joins update_packages, which also has an agent_id column, so a bare
	// "agent_id = $1" inside that arm is ambiguous and fails the whole UNION.
	commandsQuery := `
		SELECT
			ac.id,
			ac.agent_id,
			'command' as type,
			ac.command_type as action,
			ac.status,
			COALESCE(ac.result::text, '') as result,
			COALESCE(ac.params->>'package_name', 'System Operation') as package_name,
			COALESCE(ac.params->>'package_type', 'system') as package_type,
			COALESCE(ac.result->>'stdout', '') as stdout,
			COALESCE(ac.result->>'stderr', '') as stderr,
			COALESCE((ac.result->>'exit_code')::int, 0) as exit_code,
			COALESCE((ac.result->>'duration_seconds')::int, 0) as duration_seconds,
			ac.created_at,
			COALESCE(a.hostname, '') as hostname,
			(ac.retried_from_id IS NOT NULL) as is_retry,
			ac.retried_from_id
		FROM agent_commands ac
		LEFT JOIN agents a ON ac.agent_id = a.id
	`

	logsQuery := `
		SELECT
			ul.id,
			ul.agent_id,
			'log' as type,
			ul.action,
			'' as status,
			ul.result,
			COALESCE(up.package_name, '') as package_name,
			COALESCE(up.package_type, '') as package_type,
			ul.stdout,
			ul.stderr,
			ul.exit_code,
			ul.duration_seconds,
			ul.executed_at as created_at,
			COALESCE(a.hostname, '') as hostname,
			false as is_retry,
			NULL::uuid as retried_from_id
		FROM update_logs ul
		LEFT JOIN agents a ON ul.agent_id = a.id
		LEFT JOIN update_packages up ON up.id = ul.update_package_id
	`

	// update_events: package discovered/updated/failed/ignored
	updateEventsQuery := `
		SELECT
			ue.id,
			ue.agent_id,
			'package_event' as type,
			ue.event_type as action,
			'' as status,
			ue.event_type as result,
			ue.package_name,
			ue.package_type,
			concat(ue.package_name, ': ', COALESCE(ue.version_from, '?'), ' -> ', ue.version_to) as stdout,
			'' as stderr,
			0 as exit_code,
			0 as duration_seconds,
			ue.created_at,
			COALESCE(a.hostname, '') as hostname,
			false as is_retry,
			NULL::uuid as retried_from_id
		FROM update_events ue
		LEFT JOIN agents a ON ue.agent_id = a.id
	`

	// update_version_history: install transitions (success/failed/rollback)
	installQuery := `
		SELECT
			uvh.id,
			uvh.agent_id,
			'install_transition' as type,
			'install' as action,
			uvh.update_status as status,
			uvh.update_status as result,
			uvh.package_name,
			uvh.package_type,
			concat(uvh.package_name, ': ', uvh.version_from, ' -> ', uvh.version_to) as stdout,
			COALESCE(uvh.failure_reason, '') as stderr,
			CASE WHEN uvh.update_status = 'failed' THEN 1 ELSE 0 END as exit_code,
			COALESCE(EXTRACT(EPOCH FROM (uvh.update_completed_at - uvh.update_initiated_at))::int, 0) as duration_seconds,
			COALESCE(uvh.update_completed_at, uvh.update_initiated_at, NOW()) as created_at,
			COALESCE(a.hostname, '') as hostname,
			false as is_retry,
			NULL::uuid as retried_from_id
		FROM update_version_history uvh
		LEFT JOIN agents a ON uvh.agent_id = a.id
	`

	// system_events: agent/server lifecycle
	systemQuery := `
		SELECT
			se.id,
			se.agent_id,
			'system_event' as type,
			se.event_type as action,
			se.event_subtype as status,
			se.severity as result,
			COALESCE(se.metadata->>'package_name', '') as package_name,
			'' as package_type,
			COALESCE(se.message, '') as stdout,
			'' as stderr,
			0 as exit_code,
			0 as duration_seconds,
			se.created_at,
			COALESCE(a.hostname, '') as hostname,
			false as is_retry,
			NULL::uuid as retried_from_id
		FROM system_events se
		LEFT JOIN agents a ON se.agent_id = a.id
	`

	// Combined query — all five sources, filtered once on the outer aliased result.
	activityQuery := fmt.Sprintf(`
		SELECT * FROM (
			%s
			UNION ALL
			%s
			UNION ALL
			%s
			UNION ALL
			%s
			UNION ALL
			%s
		) AS history
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, commandsQuery, logsQuery, updateEventsQuery, installQuery, systemQuery, whereStr, argIdx, argIdx+1)

	// Get total count
	countCommandsQuery := fmt.Sprintf("SELECT COUNT(*) FROM agent_commands WHERE %s", whereStr)
	countLogsQuery := fmt.Sprintf("SELECT COUNT(*) FROM update_logs WHERE %s", whereStr)
	countUpdateEventsQuery := fmt.Sprintf("SELECT COUNT(*) FROM update_events WHERE %s", whereStr)
	countInstallQuery := fmt.Sprintf("SELECT COUNT(*) FROM update_version_history WHERE %s", whereStr)
	countSystemQuery := fmt.Sprintf("SELECT COUNT(*) FROM system_events WHERE %s", whereStr)

	var totalCommands, totalLogs, totalUpdateEvents, totalInstall, totalSystem int
	q.db.Get(&totalCommands, countCommandsQuery, args...)
	q.db.Get(&totalLogs, countLogsQuery, args...)
	q.db.Get(&totalUpdateEvents, countUpdateEventsQuery, args...)
	q.db.Get(&totalInstall, countInstallQuery, args...)
	q.db.Get(&totalSystem, countSystemQuery, args...)
	total := totalCommands + totalLogs + totalUpdateEvents + totalInstall + totalSystem

	// Add pagination parameters
	limit := filters.PageSize
	if limit == 0 {
		limit = 100 // Default limit
	}
	offset := (filters.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)

	// Execute query
	var items []ActivityItem
	err := q.db.Select(&items, activityQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get fleet activity: %w", err)
	}

	return items, total, nil
}

// GetActiveOperations returns currently running operations with capability token status
func (q *UpdateQueries) GetActiveOperations() ([]models.ActiveOperation, error) {
	var operations []models.ActiveOperation

	query := `
		SELECT DISTINCT ON (cps.agent_id, cps.package_type, cps.package_name)
			cps.id,
			cps.agent_id,
			cps.package_type,
			cps.package_name,
			cps.current_version,
			cps.available_version,
			cps.severity,
			cps.status,
			cps.last_updated_at,
			cps.metadata,
			ct.token_id::text AS active_token_id,
			CASE
				WHEN ct.consumed_at IS NOT NULL THEN 'consumed'
				WHEN ct.delivered_at IS NOT NULL THEN 'delivered'
				ELSE 'pending'
			END AS active_token_status
		FROM current_package_state cps
		LEFT JOIN LATERAL (
			SELECT token_id, delivered_at, consumed_at
			FROM capability_tokens
			WHERE capability_tokens.update_id = cps.id
			  AND capability_tokens.expires_at > EXTRACT(EPOCH FROM NOW())::bigint
			ORDER BY created_at DESC
			LIMIT 1
		) ct ON true
		WHERE cps.status IN ('checking_dependencies', 'installing', 'pending_dependencies')
		ORDER BY cps.agent_id, cps.package_type, cps.package_name, cps.last_updated_at DESC
	`

	err := q.db.Select(&operations, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active operations: %w", err)
	}

	return operations, nil
}

// GetLogsByAgentAndSubsystem retrieves logs for a specific agent filtered by subsystem
func (q *UpdateQueries) GetLogsByAgentAndSubsystem(agentID uuid.UUID, subsystem string) ([]models.UpdateLog, error) {
	var logs []models.UpdateLog
	query := `
		SELECT id, agent_id, update_package_id, action, subsystem, result,
		       stdout, stderr, exit_code, duration_seconds, executed_at
		FROM update_logs
		WHERE agent_id = $1 AND subsystem = $2
		ORDER BY executed_at DESC
	`
	err := q.db.Select(&logs, query, agentID, subsystem)
	return logs, err
}

// GetSubsystemStats returns scan counts by subsystem for an agent
func (q *UpdateQueries) GetSubsystemStats(agentID uuid.UUID) (map[string]int64, error) {
	query := `
		SELECT subsystem, COUNT(*) as count
		FROM update_logs
		WHERE agent_id = $1 AND action LIKE 'scan_%'
		GROUP BY subsystem
	`
	stats := make(map[string]int64)
	rows, err := q.db.Queryx(query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var subsystem string
		var count int64
		if err := rows.Scan(&subsystem, &count); err != nil {
			return nil, err
		}
		stats[subsystem] = count
	}

	return stats, nil
}

// StoreSupplyChainMetadata merges OSV.dev vulnerability results into the
// current_package_state metadata for a given agent + package. Uses jsonb
// concatenation so existing metadata keys are preserved.
func (q *UpdateQueries) StoreSupplyChainMetadata(agentID uuid.UUID, pkgType, pkgName string, meta models.JSONB) error {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal supply chain metadata: %w", err)
	}
	query := `
		UPDATE current_package_state
		SET metadata = COALESCE(metadata, '{}'::jsonb) || $4::jsonb,
		    last_updated_at = NOW()
		WHERE agent_id = $1 AND package_type = $2 AND package_name = $3`
	_, err = q.db.Exec(query, agentID, pkgType, pkgName, string(metaJSON))
	return err
}

// CountSupplyChainDeferred returns how many package rows carry an unresolved OSV
// check failure — i.e. the advisory check could not run (feed unreachable or the
// breaker was open) so the package is awaiting a recheck. Self-healing: a later
// successful check sets supply_chain_check_error to null, dropping the row out of
// this count. Surfaced to the operator (SCALE-001 S8 fail-open is loud, not
// silent): a deferred package is one the auto-confirm gate will not clear.
func (q *UpdateQueries) CountSupplyChainDeferred() (int, error) {
	var n int
	err := q.db.Get(&n, `SELECT COUNT(*) FROM current_package_state WHERE metadata->>'supply_chain_check_error' = 'osv_batch_failed'`)
	return n, err
}

// StoreClosureCheck records the result of the dependency-closure OSV check on
// the top-level update row: closure_checked_at gates auto-confirmation (only a
// checked closure may auto-mint) and closure_vulns (a JSON array, empty when
// clean) records which resolved artifacts carry known advisories. Clears any
// prior closure_check_error. Merged into metadata so other keys are preserved.
func (q *UpdateQueries) StoreClosureCheck(id uuid.UUID, closureVulnsJSON string) error {
	meta := models.JSONB{
		"closure_checked_at":  time.Now().UTC().Format(time.RFC3339),
		"closure_vulns":       closureVulnsJSON,
		"closure_check_error": nil,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal closure check: %w", err)
	}
	_, err = q.db.Exec(`
		UPDATE current_package_state
		SET metadata = COALESCE(metadata, '{}'::jsonb) || $2::jsonb,
		    last_updated_at = NOW()
		WHERE id = $1`, id, string(metaJSON))
	return err
}

// RecordClosureCheckError records that the closure OSV check could not complete,
// WITHOUT setting closure_checked_at — so the closure stays "not cleared" and
// auto-confirm holds off (fail-closed). The next scan/dry-run cycle re-reports
// the closure and re-attempts the check.
func (q *UpdateQueries) RecordClosureCheckError(id uuid.UUID, reason string) error {
	meta := models.JSONB{
		"closure_check_error": reason,
		"closure_error_at":    time.Now().UTC().Format(time.RFC3339),
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal closure check error: %w", err)
	}
	_, err = q.db.Exec(`
		UPDATE current_package_state
		SET metadata = COALESCE(metadata, '{}'::jsonb) || $2::jsonb,
		    last_updated_at = NOW()
		WHERE id = $1`, id, string(metaJSON))
	return err
}

// FreshSupplyChainPackages returns the set of (package_type, package_name,
// version) for an agent whose stored supply-chain result is newer than within.
// namespace selects which timestamp/version pair to consult: "" or "remediation"
// → supply_chain_checked_at/supply_chain_checked_version, "installed" →
// installed_checked_at/installed_checked_version. Keyed "type\x00name\x00version".
func (q *UpdateQueries) FreshSupplyChainPackages(agentID uuid.UUID, within time.Duration, namespace string) (map[string]bool, error) {
	checkedKey := "supply_chain_checked_at"
	versionKey := "supply_chain_checked_version"
	if namespace == "installed" {
		checkedKey = "installed_checked_at"
		versionKey = "installed_checked_version"
	}
	query := `
		SELECT package_type, package_name, metadata->>$4 AS checked_version
		FROM current_package_state
		WHERE agent_id = $1
		  AND metadata ? $3
		  AND metadata ? $4
		  AND (metadata->>$3)::timestamptz > NOW() - make_interval(secs => $2)`
	rows, err := q.db.Query(query, agentID, within.Seconds(), checkedKey, versionKey)
	if err != nil {
		return nil, fmt.Errorf("query fresh supply-chain packages: %w", err)
	}
	defer rows.Close()

	fresh := make(map[string]bool)
	for rows.Next() {
		var pkgType, pkgName, version string
		if err := rows.Scan(&pkgType, &pkgName, &version); err != nil {
			return nil, fmt.Errorf("scan fresh supply-chain row: %w", err)
		}
		fresh[pkgType+"\x00"+pkgName+"\x00"+version] = true
	}
	return fresh, rows.Err()
}

// GetUncheckedPackages returns distinct (package_type, package_name, version_to,
// agent_id) tuples from current_package_state whose available version has never
// had a supply-chain check. Used by the startup backfill.
func (q *UpdateQueries) GetUncheckedPackages() ([]OSVBackfillRow, error) {
	query := `
		SELECT DISTINCT ON (package_type, package_name, available_version, agent_id)
			package_type, package_name, available_version, agent_id
		FROM current_package_state
		WHERE (
			metadata IS NULL
			OR NOT metadata ? 'supply_chain_checked_at'
			OR NOT metadata ? 'supply_chain_checked_version'
			OR metadata->>'supply_chain_checked_version' IS DISTINCT FROM available_version
		)
		  AND available_version IS NOT NULL AND available_version != ''
		ORDER BY package_type, package_name, available_version, agent_id`
	var rows []OSVBackfillRow
	if err := q.db.Select(&rows, query); err != nil {
		return nil, err
	}
	return rows, nil
}

// OSVBackfillRow is a minimal row for the OSV backfill query.
type OSVBackfillRow struct {
	PackageType string    `db:"package_type"`
	PackageName string    `db:"package_name"`
	Version     string    `db:"available_version"`
	AgentID     uuid.UUID `db:"agent_id"`
}

// GetPackageVersionByString looks up a single version from the package_versions catalog.
func (q *UpdateQueries) GetPackageVersionByString(packageType, packageName, version string) (*models.PackageVersion, error) {
	var pv models.PackageVersion
	query := `
		SELECT id, package_type, package_name, version, published_at,
		       first_scanned_at, last_seen_at, sha256, osv_status, osv_vulns, source
		FROM package_versions
		WHERE package_type = $1 AND package_name = $2 AND version = $3`
	err := q.db.Get(&pv, query, packageType, packageName, version)
	if err != nil {
		return nil, err
	}
	return &pv, nil
}

// GetGatedVersion returns the newest catalog version for this package whose age is at
// least soakDays old and whose OSV posture is not known-vulnerable — the
// "newest clean version >= N days old" query package_versions (migration 043) was built
// for. [GATE-006 B]
//
// Age basis is COALESCE(published_at, first_scanned_at): upstream publish date where the
// syncer has it, else RedFlag's own first-sight witness clock (NOT NULL on every row, the
// doctrinally correct "I have watched this N days" basis). OSV: rows with status NULL or
// 'unknown' are eligible (a never-checked version is re-OSV'd when pinned, slice A); only
// 'vulnerable' is excluded. Returns nil when nothing qualifies — the caller falls back to
// the newest available version (fail-open on availability, never on a known vuln).
//
// This does NOT apply a forward-only / no-downgrade guard. That is the CALLER's job,
// against the specific agent's installed version, because the catalog is fleet-wide and
// this query knows nothing about any one host.
func (q *UpdateQueries) GetGatedVersion(packageType, packageName string, soakDays float64) (*models.PackageVersion, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(soakDays * 24 * float64(time.Hour)))
	var pv models.PackageVersion
	query := `
		SELECT id, package_type, package_name, version, published_at,
		       first_scanned_at, last_seen_at, sha256, osv_status, osv_vulns, source
		FROM package_versions
		WHERE package_type = $1 AND package_name = $2
		  AND COALESCE(published_at, first_scanned_at) <= $3
		  AND (osv_status IS NULL OR osv_status NOT IN ('vulnerable'))
		ORDER BY COALESCE(published_at, first_scanned_at) DESC
		LIMIT 1`
	err := q.db.Get(&pv, query, packageType, packageName, cutoff)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get gated version for %s/%s: %w", packageType, packageName, err)
	}
	return &pv, nil
}

// SetTargetVersion pins a manual operator-selected version for install on the
// given update row.
func (q *UpdateQueries) SetTargetVersion(id uuid.UUID, targetVersion string) error {
	return q.setTargetVersionSource(id, targetVersion, "manual")
}

// SetInstalledVersionHold records the just-installed version as a hold so scans
// do not silently advance the row without a fresh gate decision. The orchestrator
// ignores this hold when choosing the next soak-gated target.
func (q *UpdateQueries) SetInstalledVersionHold(id uuid.UUID, installedVersion string) error {
	return q.setTargetVersionSource(id, installedVersion, "installed_hold")
}

func (q *UpdateQueries) setTargetVersionSource(id uuid.UUID, version, source string) error {
	query := `
		UPDATE current_package_state
		SET selected_version = $2,
		    metadata = jsonb_set(
		        COALESCE(metadata, '{}'::jsonb),
		        '{selected_version_source}',
		        to_jsonb($3::text),
		        true
		    ),
		    last_updated_at = NOW()
		WHERE id = $1`
	_, err := q.db.Exec(query, id, version, source)
	return err
}

// ClearTargetVersion removes the selected_version override.
func (q *UpdateQueries) ClearTargetVersion(id uuid.UUID) error {
	query := `
		UPDATE current_package_state
		SET selected_version = NULL,
		    metadata = COALESCE(metadata, '{}'::jsonb) - 'selected_version_source',
		    last_updated_at = NOW()
		WHERE id = $1`
	_, err := q.db.Exec(query, id)
	return err
}

// clearTargetVersionTx is the transactional variant used during status transitions.
func clearTargetVersionTx(tx *sqlx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(`
		UPDATE current_package_state
		SET selected_version = NULL,
		    metadata = COALESCE(metadata, '{}'::jsonb) - 'selected_version_source',
		    last_updated_at = NOW()
		WHERE id = $1`, id)
	return err
}

// NonRestingRow is a minimal projection of current_package_state used by the
// scan-set reconciler (RECONCILE-001). Only the fields needed for set-diff and
// state-machine closure are selected; the full UpdateState columns are not needed.
type NonRestingRow struct {
	ID          uuid.UUID            `db:"id"`
	AgentID     uuid.UUID            `db:"agent_id"`
	PackageType string               `db:"package_type"`
	PackageName string               `db:"package_name"`
	Status      models.PackageStatus `db:"status"`
}

// GetTrackedNonResting returns all current_package_state rows for the given
// (agentID, packageType/ecosystem) pair whose status is NOT a resting/terminal
// state (i.e., not installed, failed, or ignored). These are the rows that the
// scan-set reconciler must consider for closure by absence:
//
//	tracked_non_resting(agent, ecosystem) − reported_set = to_close
//
// Only call this after a confirmed-successful (exit 0) full scan of the ecosystem.
// RECONCILE-001, §11.8 Render the Divergence.
func (q *UpdateQueries) GetTrackedNonResting(agentID uuid.UUID, ecosystem string) ([]NonRestingRow, error) {
	var rows []NonRestingRow
	query := `
		SELECT id, agent_id, package_type, package_name, status
		FROM current_package_state
		WHERE agent_id = $1
		  AND package_type = $2
		  -- RECONCILE-001 closure scope: only the *waiting* states. In-flight states
		  -- (checking_dependencies, pending_dependencies, installing) are owned by the
		  -- orchestrator + capability-receipt path; the scan-set reconciler must not race
		  -- them or mislabel a RedFlag-driven install (installing -> installed via receipt)
		  -- as out-of-band. Closing a waiting row is out-of-band resolution by construction.
		  AND status IN ('pending', 'approved')`
	if err := q.db.Select(&rows, query, agentID, ecosystem); err != nil {
		return nil, fmt.Errorf("get_tracked_non_resting agent=%s ecosystem=%s: %w", agentID, ecosystem, err)
	}
	return rows, nil
}
