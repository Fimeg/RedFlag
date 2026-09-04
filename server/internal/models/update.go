package models

import (
	"encoding/json"
	"time"

	"github.com/gofrs/uuid/v5"
)

// UpdateReportRequest is sent by agents when reporting discovered updates.
// RECONCILE-001 contract extension (2026-06-06):
//   - Ecosystem: the package manager that produced this scan (e.g. "dnf", "apt").
//     Required when ScanSucceeded is true. The server uses this to scope the
//     set-diff closure to a single (agent, ecosystem) pair.
//   - ScanSucceeded: true only when the scanner returned exit 0 and a complete
//     result. MUST be false (or absent) for partial, failed, or error scans.
//     The server NEVER closes rows by absence unless this is true.
type UpdateReportRequest struct {
	CommandID     string             `json:"command_id"`
	Timestamp     time.Time          `json:"timestamp"`
	Updates       []UpdateReportItem `json:"updates"`
	Ecosystem     string             `json:"ecosystem,omitempty"`      // RECONCILE-001: package manager ("dnf", "apt", …)
	ScanSucceeded bool               `json:"scan_succeeded,omitempty"` // RECONCILE-001: exit 0, complete scan
}

// UpdateReportItem represents a single update discovered by an agent
type UpdateReportItem struct {
	PackageType        string   `json:"package_type" binding:"required"`
	PackageName        string   `json:"package_name" binding:"required"`
	PackageDescription string   `json:"package_description"`
	CurrentVersion     string   `json:"current_version"`
	AvailableVersion   string   `json:"available_version" binding:"required"`
	Severity           string   `json:"severity"`
	CVEList            []string `json:"cve_list"`
	KBID               string   `json:"kb_id"`
	RepositorySource   string   `json:"repository_source"`
	SizeBytes          int64    `json:"size_bytes"`
	Metadata           JSONB    `json:"metadata"`
}

// UpdateLog represents an execution log entry
type UpdateLog struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	AgentID         uuid.UUID  `json:"agent_id" db:"agent_id"`
	UpdatePackageID *uuid.UUID `json:"update_package_id,omitempty" db:"update_package_id"`
	Action          string     `json:"action" db:"action"`
	Subsystem       string     `json:"subsystem,omitempty" db:"subsystem"`
	Result          string     `json:"result" db:"result"`
	Stdout          string     `json:"stdout" db:"stdout"`
	Stderr          string     `json:"stderr" db:"stderr"`
	ExitCode        int        `json:"exit_code" db:"exit_code"`
	DurationSeconds int        `json:"duration_seconds" db:"duration_seconds"`
	ExecutedAt      time.Time  `json:"executed_at" db:"executed_at"`
	// Narrative is a server-rendered, operator-facing summary of the log row.
	// Populated by handlers before returning to the UI; never persisted.
	// See services/event_renderer.go for the renderer.
	Narrative string `json:"narrative,omitempty" db:"-"`
}

// UpdateLogRequest is sent by agents when reporting execution results
type UpdateLogRequest struct {
	CommandID       string `json:"command_id"`
	Action          string `json:"action" binding:"required"`
	Subsystem       string `json:"subsystem,omitempty"`
	Result          string `json:"result" binding:"required"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        int    `json:"exit_code"`
	DurationSeconds int    `json:"duration_seconds"`
}

// DependencyReportRequest is used by agents to report dependencies after dry run
type DependencyReportRequest struct {
	PackageName   string         `json:"package_name" binding:"required"`
	PackageType   string         `json:"package_type" binding:"required"`
	TargetVersion string         `json:"target_version,omitempty"`
	Dependencies  []string       `json:"dependencies" binding:"required"`
	UpdateID      string         `json:"update_id" binding:"required"`
	DryRunResult  *InstallResult `json:"dry_run_result,omitempty"`
	// Closure carries the resolved artifacts (top-level + dependencies) with the
	// SHA256 the agent's signed repo metadata anchors. Present for OS package
	// managers (dnf/apt) whose hash is sourced agent-side; the server pins the
	// top-level hash and mints the capability token over this set.
	Closure []ClosureItem `json:"closure,omitempty"`
}

// ClosureItem is one resolved artifact reported by an agent. JSON tags match the
// capability.ClosureEntry so the reported set maps directly onto a minted token.
type ClosureItem struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Source  string `json:"source"`
}

// InstallResult represents the result of a package installation attempt (from agent)
type InstallResult struct {
	Success           bool     `json:"success"`
	ErrorMessage      string   `json:"error_message,omitempty"`
	Stdout            string   `json:"stdout,omitempty"`
	Stderr            string   `json:"stderr,omitempty"`
	ExitCode          int      `json:"exit_code"`
	DurationSeconds   int      `json:"duration_seconds"`
	Action            string   `json:"action,omitempty"` // "install", "upgrade", "dry_run", etc.
	PackagesInstalled []string `json:"packages_installed,omitempty"`
	ContainersUpdated []string `json:"containers_updated,omitempty"`
	Dependencies      []string `json:"dependencies,omitempty"` // List of dependency packages found during dry run
	IsDryRun          bool     `json:"is_dry_run"`             // Whether this is a dry run result
}

// UpdateFilters for querying updates
type UpdateFilters struct {
	AgentID     uuid.UUID
	Status      PackageStatus
	Severity    string
	PackageType string
	Page        int
	PageSize    int
}

// EVENT SOURCING MODELS

// UpdateEvent represents a single update event in the event sourcing system
type UpdateEvent struct {
	ID               uuid.UUID `json:"id" db:"id"`
	AgentID          uuid.UUID `json:"agent_id" db:"agent_id"`
	PackageType      string    `json:"package_type" db:"package_type"`
	PackageName      string    `json:"package_name" db:"package_name"`
	VersionFrom      string    `json:"version_from" db:"version_from"`
	VersionTo        string    `json:"version_to" db:"version_to"`
	Severity         string    `json:"severity" db:"severity"`
	RepositorySource string    `json:"repository_source" db:"repository_source"`
	Metadata         JSONB     `json:"metadata" db:"metadata"`
	EventType        string    `json:"event_type" db:"event_type"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// UpdateState represents the current state of a package (denormalized for queries)
type UpdateState struct {
	ID               uuid.UUID     `json:"id" db:"id"`
	AgentID          uuid.UUID     `json:"agent_id" db:"agent_id"`
	PackageType      string        `json:"package_type" db:"package_type"`
	PackageName      string        `json:"package_name" db:"package_name"`
	CurrentVersion   string        `json:"current_version" db:"current_version"`
	AvailableVersion string        `json:"available_version" db:"available_version"`
	Severity         string        `json:"severity" db:"severity"`
	RepositorySource string        `json:"repository_source" db:"repository_source"`
	Metadata         JSONB         `json:"metadata" db:"metadata"`
	LastDiscoveredAt time.Time     `json:"last_discovered_at" db:"last_discovered_at"`
	LastUpdatedAt    time.Time     `json:"last_updated_at" db:"last_updated_at"`
	Status           PackageStatus `json:"status" db:"status"`
	ExpectedSHA256   *string       `json:"expected_sha256" db:"expected_sha256"`             // Layer 1: Hash Registry
	SelectedVersion  *string       `json:"selected_version,omitempty" db:"selected_version"` // GATE-005: soak-gate target

	// Enrichment fields — populated from Metadata by EnrichFromMetadata().
	// Not persisted; db:"-" excludes them from SQL scans.
	PackageDescription string                 `json:"package_description" db:"-"`
	HomepageURL        string                 `json:"homepage_url" db:"-"`
	SizeBytes          int64                  `json:"size_bytes" db:"-"`
	CVEList            []string               `json:"cve_list" db:"-"`
	KBID               string                 `json:"kb_id" db:"-"`
	Vulnerabilities    []VulnerabilityEntry   `json:"vulnerabilities" db:"-"`
	DisplayMetadata    map[string]interface{} `json:"display_metadata,omitempty" db:"-"`
}

// PackageVersion is one row of the version timeline catalog: a single
// (package_type, package_name, version) RedFlag has seen across the fleet, with the
// provenance it accumulated. OSVVulns is raw JSON text (parsed client-side, same as
// the supply_chain_vulns metadata field). Nullable fields are filled in over time as
// scans and approvals enrich the row.
type PackageVersion struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	PackageType    string     `json:"package_type" db:"package_type"`
	PackageName    string     `json:"package_name" db:"package_name"`
	Version        string     `json:"version" db:"version"`
	PublishedAt    *time.Time `json:"published_at" db:"published_at"`
	FirstScannedAt time.Time  `json:"first_scanned_at" db:"first_scanned_at"`
	LastSeenAt     time.Time  `json:"last_seen_at" db:"last_seen_at"`
	SHA256         *string    `json:"sha256" db:"sha256"`
	OSVStatus      *string    `json:"osv_status" db:"osv_status"`
	OSVVulns       *string    `json:"osv_vulns" db:"osv_vulns"`
	Source         *string    `json:"source" db:"source"`
}

// PackageSummary is the package-centric aggregate view returned by
// GET /updates/package/:type/:name. It carries metadata from one representative
// agent row, fleet-wide status counts, and version extremes.
type PackageSummary struct {
	PackageType        string               `json:"package_type"`
	PackageName        string               `json:"package_name"`
	Severity           string               `json:"severity"`
	PackageDescription string               `json:"package_description"`
	HomepageURL        string               `json:"homepage_url"`
	SizeBytes          int64                `json:"size_bytes"`
	CVEList            []string             `json:"cve_list"`
	Vulnerabilities    []VulnerabilityEntry `json:"vulnerabilities"`
	TotalAgents        int                  `json:"total_agents"`
	PendingCount       int                  `json:"pending_count"`
	ApprovedCount      int                  `json:"approved_count"`
	ActiveCount        int                  `json:"active_count"`
	InstalledCount     int                  `json:"installed_count"`
	FailedCount        int                  `json:"failed_count"`
	IgnoredCount       int                  `json:"ignored_count"`
	LatestAvailable    string               `json:"latest_available"`
	LatestInstalled    *string              `json:"latest_installed,omitempty"`
}

// UpdateHistory represents the version history of a package
type UpdateHistory struct {
	ID                uuid.UUID     `json:"id" db:"id"`
	AgentID           uuid.UUID     `json:"agent_id" db:"agent_id"`
	PackageType       string        `json:"package_type" db:"package_type"`
	PackageName       string        `json:"package_name" db:"package_name"`
	VersionFrom       string        `json:"version_from" db:"version_from"`
	VersionTo         string        `json:"version_to" db:"version_to"`
	Severity          string        `json:"severity" db:"severity"`
	RepositorySource  string        `json:"repository_source" db:"repository_source"`
	Metadata          JSONB         `json:"metadata" db:"metadata"`
	UpdateInitiatedAt *time.Time    `json:"update_initiated_at" db:"update_initiated_at"`
	UpdateCompletedAt time.Time     `json:"update_completed_at" db:"update_completed_at"`
	UpdateStatus      HistoryStatus `json:"update_status" db:"update_status"`
	FailureReason     *string       `json:"failure_reason" db:"failure_reason"`
}

// UpdateBatch represents a batch of update events
type UpdateBatch struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	AgentID        uuid.UUID  `json:"agent_id" db:"agent_id"`
	BatchSize      int        `json:"batch_size" db:"batch_size"`
	ProcessedCount int        `json:"processed_count" db:"processed_count"`
	FailedCount    int        `json:"failed_count" db:"failed_count"`
	Status         string     `json:"status" db:"status"`
	ErrorDetails   JSONB      `json:"error_details" db:"error_details"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	CompletedAt    *time.Time `json:"completed_at" db:"completed_at"`
}

// UpdateStats represents statistics about updates
type UpdateStats struct {
	TotalUpdates     int `json:"total_updates" db:"total_updates"`
	PendingUpdates   int `json:"pending_updates" db:"pending_updates"`
	ApprovedUpdates  int `json:"approved_updates" db:"approved_updates"`
	InstalledUpdates int `json:"installed_updates" db:"installed_updates"`
	FailedUpdates    int `json:"failed_updates" db:"failed_updates"`
	CriticalUpdates  int `json:"critical_updates" db:"critical_updates"`
	HighUpdates      int `json:"high_updates" db:"high_updates"`
	ImportantUpdates int `json:"important_updates" db:"important_updates"`
	ModerateUpdates  int `json:"moderate_updates" db:"moderate_updates"`
	LowUpdates       int `json:"low_updates" db:"low_updates"`
}

// VulnerabilityEntry represents a single CVE/vulnerability from agent-reported or
// OSV-sourced data.
type VulnerabilityEntry struct {
	ID             string   `json:"id"`
	Summary        string   `json:"summary,omitempty"`
	Severity       string   `json:"severity,omitempty"`
	Description    string   `json:"description,omitempty"`
	Source         string   `json:"source,omitempty"` // "osv" or "agent"
	FixedVersion   string   `json:"fixed_version,omitempty"`
	Aliases        []string `json:"aliases,omitempty"`
	CVSSVector     string   `json:"cvss_vector,omitempty"`
	CVSSScore      float64  `json:"cvss_score,omitempty"`
	Published      string   `json:"published,omitempty"`
	AdvisoryType   string   `json:"advisory_type,omitempty"`
	AffectedRanges []string `json:"affected_ranges,omitempty"`
	KnownExploited bool     `json:"known_exploited,omitempty"`
}

// reservedMetadataKeys are internal keys excluded from DisplayMetadata.
var reservedMetadataKeys = map[string]bool{
	"description": true, "homepage_url": true, "kb_id": true,
	"size_bytes": true, "cve_list": true,
	"supply_chain_vulns": true, "supply_chain_checked_at": true,
	"supply_chain_checked_version": true, "supply_chain_check_error": true,
	"supply_chain_error_at": true, "installed_vulns": true,
	"installed_checked_at": true, "installed_checked_version": true,
	"installed_check_error":   true,
	"selected_version_source": true,
	"resolved_closure":        true, "dependencies": true,
	"failure_reason": true, "capability_token_id": true,
	"capability_decision": true, "capability_reason": true,
	"package_published_at": true, "package_age_hours": true,
	"supply_chain_age_check": true,
}

// EnrichFromMetadata populates the enrichment fields from Metadata.
func (u *UpdateState) EnrichFromMetadata() {
	if u.Metadata == nil {
		return
	}
	if v, ok := u.Metadata["description"].(string); ok {
		u.PackageDescription = v
	}
	if v, ok := u.Metadata["homepage_url"].(string); ok {
		u.HomepageURL = v
	}
	if v, ok := u.Metadata["kb_id"].(string); ok {
		u.KBID = v
	}
	switch v := u.Metadata["size_bytes"].(type) {
	case float64:
		u.SizeBytes = int64(v)
	case int64:
		u.SizeBytes = v
	}
	switch v := u.Metadata["cve_list"].(type) {
	case []interface{}:
		u.CVEList = make([]string, 0, len(v))
		for _, entry := range v {
			if s, ok := entry.(string); ok {
				u.CVEList = append(u.CVEList, s)
			}
		}
	}
	u.Vulnerabilities = u.mergedVulnerabilities()

	// Build display metadata from non-reserved keys.
	dm := make(map[string]interface{})
	for k, v := range u.Metadata {
		if !reservedMetadataKeys[k] {
			dm[k] = v
		}
	}
	if len(dm) > 0 {
		u.DisplayMetadata = dm
	}
}

// mergedVulnerabilities combines OSV-sourced and agent-reported vulnerabilities.
func (u *UpdateState) mergedVulnerabilities() []VulnerabilityEntry {
	seen := make(map[string]bool)
	var result []VulnerabilityEntry

	for _, id := range u.CVEList {
		if seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, VulnerabilityEntry{ID: id, Source: "agent"})
	}

	if raw, ok := u.Metadata["supply_chain_vulns"].(string); ok && raw != "" && raw != "[]" {
		var osvVulns []VulnerabilityEntry
		if err := json.Unmarshal([]byte(raw), &osvVulns); err == nil {
			for _, v := range osvVulns {
				if seen[v.ID] {
					continue
				}
				seen[v.ID] = true
				v.Source = "osv"
				result = append(result, v)
			}
		}
	}
	return result
}

// InstallVersionRequest is the body for POST /updates/:id/install-version.
type InstallVersionRequest struct {
	Version        string `json:"version" binding:"required"`
	OverrideReason string `json:"override_reason"`
}

// LogFilters for querying logs across all agents
type LogFilters struct {
	AgentID  uuid.UUID
	Action   string
	Result   string
	Type     string // "command", "log", "package_event", "install_transition", "system_event"
	Severity string // "info", "warning", "error", "critical"
	Since    *time.Time
	Page     int
	PageSize int
}

// ActiveOperation represents a currently running operation
type ActiveOperation struct {
	ID                uuid.UUID     `json:"id" db:"id"`
	AgentID           uuid.UUID     `json:"agent_id" db:"agent_id"`
	PackageType       string        `json:"package_type" db:"package_type"`
	PackageName       string        `json:"package_name" db:"package_name"`
	CurrentVersion    string        `json:"current_version" db:"current_version"`
	AvailableVersion  string        `json:"available_version" db:"available_version"`
	Severity          string        `json:"severity" db:"severity"`
	Status            PackageStatus `json:"status" db:"status"`
	LastUpdatedAt     time.Time     `json:"last_updated_at" db:"last_updated_at"`
	Metadata          JSONB         `json:"metadata" db:"metadata"`
	ActiveTokenID     *string       `json:"active_token_id,omitempty" db:"active_token_id"`
	ActiveTokenStatus *string       `json:"active_token_status,omitempty" db:"active_token_status"`
}
