package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// TrackedSoftware is a single piece of software the operator wants the
// server to keep an eye on. The (source, source_ref) pair uniquely
// identifies what upstream registry to ask and what to ask it for.
type TrackedSoftware struct {
	ID                    uuid.UUID  `db:"id" json:"id"`
	Name                  string     `db:"name" json:"name"`
	Ecosystem             string     `db:"ecosystem" json:"ecosystem"`
	Source                string     `db:"source" json:"source"`
	SourceRef             string     `db:"source_ref" json:"source_ref"`
	CurrentVersion        *string    `db:"current_version" json:"current_version,omitempty"`
	LatestVersion         *string    `db:"latest_version" json:"latest_version,omitempty"`
	LatestAt              *time.Time `db:"latest_at" json:"latest_at,omitempty"`
	EOLAt                 *time.Time `db:"eol_at" json:"eol_at,omitempty"`
	LastCheckedAt         *time.Time `db:"last_checked_at" json:"last_checked_at,omitempty"`
	LastSyncedAt          *time.Time `db:"last_synced_at" json:"last_synced_at,omitempty"`
	LastError             *string    `db:"last_error" json:"last_error,omitempty"`
	Enabled               bool       `db:"enabled" json:"enabled"`
	TrackPrereleases      bool       `db:"track_prereleases" json:"track_prereleases"`
	RepologySlug          *string    `db:"repology_slug" json:"repology_slug,omitempty"`
	ContainerImagePattern *string    `db:"container_image_pattern" json:"container_image_pattern,omitempty"`
	BinaryProbe           *string    `db:"binary_probe" json:"binary_probe,omitempty"`
	CreatedAt             time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time  `db:"updated_at" json:"updated_at"`
}

// TrackedSoftwareInput is the create/update payload from the admin API.
type TrackedSoftwareInput struct {
	Name                  string  `json:"name" binding:"required"`
	Ecosystem             string  `json:"ecosystem" binding:"required"`
	Source                string  `json:"source" binding:"required"`
	SourceRef             string  `json:"source_ref" binding:"required"`
	CurrentVersion        *string `json:"current_version,omitempty"`
	Enabled               *bool   `json:"enabled,omitempty"`
	TrackPrereleases      *bool   `json:"track_prereleases,omitempty"`
	RepologySlug          *string `json:"repology_slug,omitempty"`
	ContainerImagePattern *string `json:"container_image_pattern,omitempty"`
	BinaryProbe           *string `json:"binary_probe,omitempty"`
}

// TrackedSoftwareSettingsInput is the narrow admin patch payload for
// operator-owned toggles that should not require recreating the tracked row.
type TrackedSoftwareSettingsInput struct {
	Enabled          *bool `json:"enabled,omitempty"`
	TrackPrereleases *bool `json:"track_prereleases,omitempty"`
}

// UpstreamDriftEvent records a point-in-time observation that latest_version
// moved (or that an EOL date passed). Append-only.
type UpstreamDriftEvent struct {
	ID                uuid.UUID `db:"id" json:"id"`
	TrackedSoftwareID uuid.UUID `db:"tracked_software_id" json:"tracked_software_id"`
	ObservedAt        time.Time `db:"observed_at" json:"observed_at"`
	DriftSeverity     string    `db:"drift_severity" json:"drift_severity"`
	FromVersion       *string   `db:"from_version" json:"from_version,omitempty"`
	ToVersion         *string   `db:"to_version" json:"to_version,omitempty"`
	Note              *string   `db:"note" json:"note,omitempty"`
}

// AgentTrackedSoftware is the binding row that says "agent X has the
// software described by tracked_software_id installed at this version
// at this path." Unique on (agent_id, tracked_software_id) — re-binding
// the same pair upserts. See migration 039.
type AgentTrackedSoftware struct {
	ID                uuid.UUID `db:"id" json:"id"`
	AgentID           uuid.UUID `db:"agent_id" json:"agent_id"`
	TrackedSoftwareID uuid.UUID `db:"tracked_software_id" json:"tracked_software_id"`
	InstalledVersion  string    `db:"installed_version" json:"installed_version"`
	InstallPath       *string   `db:"install_path" json:"install_path,omitempty"`
	Notes             *string   `db:"notes" json:"notes,omitempty"`
	MatchMethod       *string   `db:"match_method" json:"match_method,omitempty"`
	PackageName       *string   `db:"package_name" json:"package_name,omitempty"`
	LastObservedAt    time.Time `db:"last_observed_at" json:"last_observed_at"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at" json:"updated_at"`
}

// AgentTrackedSoftwareInput is the create/upsert payload.
type AgentTrackedSoftwareInput struct {
	TrackedSoftwareID uuid.UUID `json:"tracked_software_id" binding:"required"`
	InstalledVersion  string    `json:"installed_version" binding:"required"`
	InstallPath       *string   `json:"install_path,omitempty"`
	Notes             *string   `json:"notes,omitempty"`
}

// AgentTrackedSoftwareView joins a binding with its parent tracked_software
// row so the API can return one shape per binding without round-trips on
// the client. Drift booleans are derived server-side so the UI can render
// indicators without re-doing the comparison.
type AgentTrackedSoftwareView struct {
	BindingID         uuid.UUID  `db:"binding_id" json:"binding_id"`
	AgentID           uuid.UUID  `db:"agent_id" json:"agent_id"`
	TrackedSoftwareID uuid.UUID  `db:"tracked_software_id" json:"tracked_software_id"`
	Name              string     `db:"name" json:"name"`
	Ecosystem         string     `db:"ecosystem" json:"ecosystem"`
	Source            string     `db:"source" json:"source"`
	SourceRef         string     `db:"source_ref" json:"source_ref"`
	InstalledVersion  string     `db:"installed_version" json:"installed_version"`
	LatestVersion     *string    `db:"latest_version" json:"latest_version,omitempty"`
	LatestAt          *time.Time `db:"latest_at" json:"latest_at,omitempty"`
	EOLAt             *time.Time `db:"eol_at" json:"eol_at,omitempty"`
	InstallPath       *string    `db:"install_path" json:"install_path,omitempty"`
	Notes             *string    `db:"notes" json:"notes,omitempty"`
	LastObservedAt    time.Time  `db:"last_observed_at" json:"last_observed_at"`
	LastSyncedAt      *time.Time `db:"last_synced_at" json:"last_synced_at,omitempty"`
	LastError         *string    `db:"last_error" json:"last_error,omitempty"`
	Drifted           bool       `db:"drifted" json:"drifted"`
	PastEOL           bool       `db:"past_eol" json:"past_eol"`
}

// RepologyAlias is a cached row from the Repology /packages endpoint mapping
// a Repology project slug to an ecosystem-specific package name.
type RepologyAlias struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Slug      string    `db:"slug" json:"slug"`
	Ecosystem string    `db:"ecosystem" json:"ecosystem"`
	PkgNames  []string  `db:"pkg_names" json:"pkg_names"`
	RepoRaw   *string   `db:"repo_raw" json:"repo_raw,omitempty"`
	FetchedAt time.Time `db:"fetched_at" json:"fetched_at"`
}

// MatchResult represents a reconciliation hit: an agent's reported package
// matched a tracked_software entry via one of the discovery methods.
type MatchResult struct {
	AgentID     uuid.UUID `db:"agent_id" json:"agent_id"`
	TrackedID   uuid.UUID `db:"tracked_software_id" json:"tracked_software_id"`
	PackageName string    `db:"package_name" json:"package_name"`
	Version     string    `db:"version" json:"version"`
	PackageType string    `db:"package_type" json:"package_type"`
}

// AgentInstallation is the symmetric view: "who runs this tracked software?"
// Used by the upstream-tracking settings page so an operator can see which
// hosts have each entry installed.
type AgentInstallation struct {
	BindingID        uuid.UUID `db:"binding_id" json:"binding_id"`
	AgentID          uuid.UUID `db:"agent_id" json:"agent_id"`
	Hostname         string    `db:"hostname" json:"hostname"`
	InstalledVersion string    `db:"installed_version" json:"installed_version"`
	InstallPath      *string   `db:"install_path" json:"install_path,omitempty"`
	LastObservedAt   time.Time `db:"last_observed_at" json:"last_observed_at"`
}
