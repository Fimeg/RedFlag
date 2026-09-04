package client

import "time"

// InventoryItem represents a single inventory record -- something present on
// the system. Distinct from UpdateReportItem (which represents an available
// update) and MetricsReportItem (which represents a point-in-time measurement).
//
// Inspired by osquery's per-ecosystem table model (rpm_packages, deb_packages,
// programs, docker_images) where each item has typed, indexed fields rather
// than a freeform Metadata map.
type InventoryItem struct {
	Ecosystem   string                 `json:"inventory_ecosystem"` // "docker", "system", "apt", "dnf", etc.
	ItemName    string                 `json:"item_name"`          // Primary identifier within ecosystem
	ItemVersion string                 `json:"item_version"`       // Installed version / digest
	Description string                 `json:"description,omitempty"`
	Arch        string                 `json:"arch,omitempty"`
	InstallTime string                 `json:"install_time,omitempty"` // RFC3339 or ecosystem-specific
	SizeBytes   int64                  `json:"size_bytes,omitempty"`
	Vendor      string                 `json:"vendor,omitempty"` // Registry, repo, manufacturer
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// InventoryReport is sent by agents when reporting inventory data.
type InventoryReport struct {
	CommandID     string          `json:"command_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Ecosystem     string          `json:"inventory_ecosystem"`
	Items         []InventoryItem `json:"items"`
	ScanSucceeded bool            `json:"scan_succeeded"`
}
