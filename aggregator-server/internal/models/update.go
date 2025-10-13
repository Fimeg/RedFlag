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
	AgentID     *uuid.UUID
	Status      string
	Severity    string
	PackageType string
	Page        int
	PageSize    int
}
