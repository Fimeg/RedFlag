package models

import (
	"time"
	"github.com/gofrs/uuid/v5"
)

// MetricsReportRequest is sent by agents when reporting system/storage metrics
type MetricsReportRequest struct {
	CommandID string    `json:"command_id"`
	Timestamp time.Time `json:"timestamp"`
	Metrics   []Metric  `json:"metrics"`
}

// Metric represents a system or storage metric
type Metric struct {
	PackageType      string            `json:"package_type"`      // "storage", "system", "cpu", "memory"
	PackageName      string            `json:"package_name"`      // mount point, metric name
	CurrentVersion   string            `json:"current_version"`   // current usage, value
	AvailableVersion string            `json:"available_version"` // available space, threshold
	Severity         string            `json:"severity"`          // "low", "moderate", "high"
	RepositorySource string            `json:"repository_source"`
	Metadata         map[string]string `json:"metadata"`
}

// Metric represents a stored metric in the database
type StoredMetric struct {
	ID               uuid.UUID         `json:"id" db:"id"`
	AgentID          uuid.UUID         `json:"agent_id" db:"agent_id"`
	PackageType      string            `json:"package_type" db:"package_type"`
	PackageName      string            `json:"package_name" db:"package_name"`
	CurrentVersion   string            `json:"current_version" db:"current_version"`
	AvailableVersion string            `json:"available_version" db:"available_version"`
	Severity         string            `json:"severity" db:"severity"`
	RepositorySource string            `json:"repository_source" db:"repository_source"`
	Metadata         JSONB             `json:"metadata" db:"metadata"`
	EventType        string            `json:"event_type" db:"event_type"`
	CreatedAt        time.Time         `json:"created_at" db:"created_at"`
}

// MetricFilter represents filtering options for metrics queries
type MetricFilter struct {
	AgentID     *uuid.UUID `json:"agent_id,omitempty"`
	PackageType *string    `json:"package_type,omitempty"`
	Severity    *string    `json:"severity,omitempty"`
	Limit       *int       `json:"limit,omitempty"`
	Offset      *int       `json:"offset,omitempty"`
}

// MetricResult represents the result of a metrics query
type MetricResult struct {
	Metrics []StoredMetric `json:"metrics"`
	Total   int             `json:"total"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
}

// StorageMetrics represents storage-specific metrics for easier consumption
type StorageMetrics struct {
	MountPoint    string  `json:"mount_point"`
	TotalBytes    int64   `json:"total_bytes"`
	UsedBytes     int64   `json:"used_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
	UsedPercent   float64 `json:"used_percent"`
	Status        string  `json:"status"` // "low", "moderate", "high", "critical"
	LastUpdated   time.Time `json:"last_updated"`
}

// SystemMetrics represents system-specific metrics for easier consumption
type SystemMetrics struct {
	CPUModel      string    `json:"cpu_model"`
	CPUCores      int       `json:"cpu_cores"`
	CPUThreads    int       `json:"cpu_threads"`
	MemoryTotal   int64     `json:"memory_total"`
	MemoryUsed    int64     `json:"memory_used"`
	MemoryPercent float64   `json:"memory_percent"`
	Processes     int       `json:"processes"`
	Uptime        string    `json:"uptime"`
	LoadAverage   []float64 `json:"load_average"`
	LastUpdated   time.Time `json:"last_updated"`
}