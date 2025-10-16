package models

import (
	"time"

	"github.com/google/uuid"
)

// UpdatePackage represents a single update available for installation
type UpdatePackage struct {
	ID                 uuid.UUID    `json:"id" db:"id"`
	AgentID            uuid.UUID    `json:"agent_id" db:"agent_id"`
	PackageType        string       `json:"package_type" db:"package_type"`
	PackageName        string       `json:"package_name" db:"package_name"`
	PackageDescription string       `json:"package_description" db:"package_description"`
	CurrentVersion     string       `json:"current_version" db:"current_version"`
	AvailableVersion   string       `json:"available_version" db:"available_version"`
	Severity           string       `json:"severity" db:"severity"`
	CVEList            StringArray  `json:"cve_list" db:"cve_list"`
	KBID               string       `json:"kb_id" db:"kb_id"`
	RepositorySource   string       `json:"repository_source" db:"repository_source"`
	SizeBytes          int64        `json:"size_bytes" db:"size_bytes"`
	Status             string       `json:"status" db:"status"`
	DiscoveredAt       time.Time    `json:"discovered_at" db:"discovered_at"`
	ApprovedBy         string       `json:"approved_by,omitempty" db:"approved_by"`
	ApprovedAt         *time.Time   `json:"approved_at,omitempty" db:"approved_at"`
	ScheduledFor       *time.Time   `json:"scheduled_for,omitempty" db:"scheduled_for"`
	InstalledAt        *time.Time   `json:"installed_at,omitempty" db:"installed_at"`
	ErrorMessage       string       `json:"error_message,omitempty" db:"error_message"`
	Metadata           JSONB        `json:"metadata" db:"metadata"`
}

// UpdateReportRequest is sent by agents when reporting discovered updates
type UpdateReportRequest struct {
	CommandID string                    `json:"command_id"`
	Timestamp time.Time                 `json:"timestamp"`
	Updates   []UpdateReportItem        `json:"updates"`
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
	Result          string     `json:"result" db:"result"`
	Stdout          string     `json:"stdout" db:"stdout"`
	Stderr          string     `json:"stderr" db:"stderr"`
	ExitCode        int        `json:"exit_code" db:"exit_code"`
	DurationSeconds int        `json:"duration_seconds" db:"duration_seconds"`
	ExecutedAt      time.Time  `json:"executed_at" db:"executed_at"`
}

// UpdateLogRequest is sent by agents when reporting execution results
type UpdateLogRequest struct {
	CommandID       string    `json:"command_id"`
	Action          string    `json:"action" binding:"required"`
	Result          string    `json:"result" binding:"required"`
	Stdout          string    `json:"stdout"`
	Stderr          string    `json:"stderr"`
	ExitCode        int       `json:"exit_code"`
	DurationSeconds int       `json:"duration_seconds"`
}

// UpdateFilters for querying updates
type UpdateFilters struct {
	AgentID     uuid.UUID
	Status      string
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
	ID                uuid.UUID `json:"id" db:"id"`
	AgentID           uuid.UUID `json:"agent_id" db:"agent_id"`
	PackageType       string    `json:"package_type" db:"package_type"`
	PackageName       string    `json:"package_name" db:"package_name"`
	CurrentVersion    string    `json:"current_version" db:"current_version"`
	AvailableVersion  string    `json:"available_version" db:"available_version"`
	Severity          string    `json:"severity" db:"severity"`
	RepositorySource  string    `json:"repository_source" db:"repository_source"`
	Metadata          JSONB     `json:"metadata" db:"metadata"`
	LastDiscoveredAt  time.Time `json:"last_discovered_at" db:"last_discovered_at"`
	LastUpdatedAt     time.Time `json:"last_updated_at" db:"last_updated_at"`
	Status            string    `json:"status" db:"status"`
}

// UpdateHistory represents the version history of a package
type UpdateHistory struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	AgentID           uuid.UUID  `json:"agent_id" db:"agent_id"`
	PackageType       string     `json:"package_type" db:"package_type"`
	PackageName       string     `json:"package_name" db:"package_name"`
	VersionFrom       string     `json:"version_from" db:"version_from"`
	VersionTo         string     `json:"version_to" db:"version_to"`
	Severity          string     `json:"severity" db:"severity"`
	RepositorySource  string     `json:"repository_source" db:"repository_source"`
	Metadata          JSONB      `json:"metadata" db:"metadata"`
	UpdateInitiatedAt *time.Time `json:"update_initiated_at" db:"update_initiated_at"`
	UpdateCompletedAt time.Time  `json:"update_completed_at" db:"update_completed_at"`
	UpdateStatus      string     `json:"update_status" db:"update_status"`
	FailureReason     string     `json:"failure_reason" db:"failure_reason"`
}

// UpdateBatch represents a batch of update events
type UpdateBatch struct {
	ID            uuid.UUID `json:"id" db:"id"`
	AgentID       uuid.UUID `json:"agent_id" db:"agent_id"`
	BatchSize     int       `json:"batch_size" db:"batch_size"`
	ProcessedCount int      `json:"processed_count" db:"processed_count"`
	FailedCount   int       `json:"failed_count" db:"failed_count"`
	Status        string    `json:"status" db:"status"`
	ErrorDetails  JSONB     `json:"error_details" db:"error_details"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	CompletedAt   *time.Time `json:"completed_at" db:"completed_at"`
}

// UpdateStats represents statistics about updates
type UpdateStats struct {
	TotalUpdates     int `json:"total_updates" db:"total_updates"`
	PendingUpdates   int `json:"pending_updates" db:"pending_updates"`
	ApprovedUpdates  int `json:"approved_updates" db:"approved_updates"`
	UpdatedUpdates   int `json:"updated_updates" db:"updated_updates"`
	FailedUpdates    int `json:"failed_updates" db:"failed_updates"`
	CriticalUpdates  int `json:"critical_updates" db:"critical_updates"`
	HighUpdates      int `json:"high_updates" db:"high_updates"`
	ImportantUpdates int `json:"important_updates" db:"important_updates"`
	ModerateUpdates  int `json:"moderate_updates" db:"moderate_updates"`
	LowUpdates       int `json:"low_updates" db:"low_updates"`
}
