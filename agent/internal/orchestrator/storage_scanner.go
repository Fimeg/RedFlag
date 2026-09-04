package orchestrator

import (
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/system"
)

// StorageScanner scans disk usage metrics
type StorageScanner struct {
	agentVersion string
}

// NewStorageScanner creates a new storage scanner
func NewStorageScanner(agentVersion string) *StorageScanner {
	return &StorageScanner{
		agentVersion: agentVersion,
	}
}

// IsAvailable always returns true since storage scanning is always available
func (s *StorageScanner) IsAvailable() bool {
	return true
}

// Scan collects disk usage and returns UpdateReportItems with full data in Metadata
func (s *StorageScanner) Scan() ([]client.UpdateReportItem, error) {
	sysInfo, err := system.GetSystemInfo(s.agentVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	if len(sysInfo.DiskInfo) == 0 {
		return nil, fmt.Errorf("no disk information available")
	}

	var items []client.UpdateReportItem

	for _, disk := range sysInfo.DiskInfo {
		severity := "low"
		switch {
		case disk.UsedPercent >= 95:
			severity = "critical"
		case disk.UsedPercent >= 90:
			severity = "important"
		case disk.UsedPercent >= 80:
			severity = "moderate"
		}

		item := client.UpdateReportItem{
			PackageType:        "storage",
			PackageName:        disk.Mountpoint,
			PackageDescription: fmt.Sprintf("Storage metrics for %s (%s)", disk.Mountpoint, disk.Filesystem),
			CurrentVersion:     fmt.Sprintf("%.1f%% used", disk.UsedPercent),
			AvailableVersion:   fmt.Sprintf("%.1f GB free", float64(disk.Available)/1024/1024/1024),
			Severity:           severity,
			RepositorySource:   disk.Device,
			SizeBytes:          int64(disk.Total),
			Metadata: map[string]interface{}{
				"mountpoint":      disk.Mountpoint,
				"device":          disk.Device,
				"disk_type":       disk.DiskType,
				"filesystem":      disk.Filesystem,
				"total_bytes":     int64(disk.Total),
				"used_bytes":      int64(disk.Used),
				"available_bytes": int64(disk.Available),
				"used_percent":    disk.UsedPercent,
				"is_root":         disk.IsRoot,
				"is_largest":      disk.IsLargest,
				"severity":        severity,
				"agent_version":   s.agentVersion,
				"collected_at":    time.Now().UTC().Format(time.RFC3339),
			},
		}
		items = append(items, item)
	}

	return items, nil
}

// Name returns the scanner name
func (s *StorageScanner) Name() string {
	return "Disk Usage Reporter"
}
