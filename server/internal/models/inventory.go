package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// AgentInventoryItem represents a current-state inventory record for an agent.
// Distinct from current_package_state (which tracks available updates) and
// metrics (which track point-in-time measurements).
type AgentInventoryItem struct {
	ID                 uuid.UUID  `db:"id" json:"id"`
	AgentID            uuid.UUID  `db:"agent_id" json:"agent_id"`
	InventoryEcosystem string     `db:"inventory_ecosystem" json:"inventory_ecosystem"`
	ItemName           string     `db:"item_name" json:"item_name"`
	ItemVersion        string     `db:"item_version" json:"item_version"`
	Description        string     `db:"description" json:"description,omitempty"`
	Arch               string     `db:"arch" json:"arch,omitempty"`
	InstallTime        *time.Time `db:"install_time" json:"install_time,omitempty"`
	SizeBytes          int64      `db:"size_bytes" json:"size_bytes,omitempty"`
	Vendor             string     `db:"vendor" json:"vendor,omitempty"`
	Metadata           JSONB      `db:"metadata" json:"metadata,omitempty"`
	LastSeenAt         time.Time  `db:"last_seen_at" json:"last_seen_at"`
	FirstSeenAt        time.Time  `db:"first_seen_at" json:"first_seen_at"`
	CreatedAt          time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time  `db:"updated_at" json:"updated_at"`
}

// InventoryReportRequest is the JSON body for agent inventory reports.
type InventoryReportRequest struct {
	CommandID     string                       `json:"command_id"`
	Timestamp     time.Time                    `json:"timestamp"`
	Ecosystem     string                       `json:"inventory_ecosystem"`
	Items         []AgentInventoryItemRequest   `json:"items"`
	ScanSucceeded bool                         `json:"scan_succeeded"`
}

// AgentInventoryItemRequest is a single inventory item from the agent wire format.
type AgentInventoryItemRequest struct {
	Ecosystem   string                 `json:"inventory_ecosystem"`
	ItemName    string                 `json:"item_name"`
	ItemVersion string                 `json:"item_version"`
	Description string                 `json:"description,omitempty"`
	Arch        string                 `json:"arch,omitempty"`
	InstallTime string                 `json:"install_time,omitempty"`
	SizeBytes   int64                  `json:"size_bytes,omitempty"`
	Vendor      string                 `json:"vendor,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// InventoryFilter supports paginated, filtered queries for inventory data.
type InventoryFilter struct {
	AgentID            *uuid.UUID
	InventoryEcosystem *string
	Limit              *int
	Offset             *int
}
