package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// StorageMetric represents a storage metric from an agent
type StorageMetric struct {
	ID             uuid.UUID              `json:"id" db:"id"`
	AgentID        uuid.UUID              `json:"agent_id" db:"agent_id"`
	Mountpoint     string                 `json:"mountpoint" db:"mountpoint"`
	Device         string                 `json:"device" db:"device"`
	DiskType       string                 `json:"disk_type" db:"disk_type"`
	Filesystem     string                 `json:"filesystem" db:"filesystem"`
	TotalBytes     int64                  `json:"total_bytes" db:"total_bytes"`
	UsedBytes      int64                  `json:"used_bytes" db:"used_bytes"`
	AvailableBytes int64                  `json:"available_bytes" db:"available_bytes"`
	UsedPercent    float64                `json:"used_percent" db:"used_percent"`
	Severity       string                 `json:"severity" db:"severity"`
	Metadata       JSONB                  `json:"metadata,omitempty" db:"metadata"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
}

// StorageMetricRequest represents the request payload for storage metrics
type StorageMetricRequest struct {
	AgentID   uuid.UUID       `json:"agent_id"`
	CommandID string          `json:"command_id"`
	Timestamp time.Time       `json:"timestamp"`
	Metrics   []StorageMetric `json:"metrics"`
}

// StorageMetricsList represents a list of storage metrics with pagination
type StorageMetricsList struct {
	Metrics []StorageMetric `json:"metrics"`
	Total   int             `json:"total"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
}