package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// StorageMetricReport represents storage metrics from an agent
type StorageMetricReport struct {
	AgentID   uuid.UUID          `json:"agent_id"`
	CommandID string             `json:"command_id"`
	Timestamp time.Time          `json:"timestamp"`
	Metrics   []StorageMetric    `json:"metrics"`
}

// StorageMetric represents a single disk/storage metric
type StorageMetric struct {
	Mountpoint     string                 `json:"mountpoint"`
	Device         string                 `json:"device"`
	DiskType       string                 `json:"disk_type"`
	Filesystem     string                 `json:"filesystem"`
	TotalBytes     int64                  `json:"total_bytes"`
	UsedBytes      int64                  `json:"used_bytes"`
	AvailableBytes int64                  `json:"available_bytes"`
	UsedPercent    float64                `json:"used_percent"`
	IsRoot         bool                   `json:"is_root"`
	IsLargest      bool                   `json:"is_largest"`
	Severity       string                 `json:"severity"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}