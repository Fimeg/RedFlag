package orchestrator

import (
	"fmt"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/system"
)

// SystemScanner scans system metrics (CPU, memory, processes, uptime)
type SystemScanner struct {
	agentVersion string
}

// NewSystemScanner creates a new system scanner
func NewSystemScanner(agentVersion string) *SystemScanner {
	return &SystemScanner{
		agentVersion: agentVersion,
	}
}

// IsAvailable always returns true since system scanning is always available
func (s *SystemScanner) IsAvailable() bool {
	return true
}

// Scan collects system information and returns UpdateReportItems with full data in Metadata
func (s *SystemScanner) Scan() ([]client.UpdateReportItem, error) {
	sysInfo, err := system.GetSystemInfo(s.agentVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	var items []client.UpdateReportItem

	// CPU
	items = append(items, client.UpdateReportItem{
		PackageType:      "system",
		PackageName:      "system-cpu",
		PackageDescription: "System CPU information",
		CurrentVersion:   fmt.Sprintf("%d cores, %d threads", sysInfo.CPUInfo.Cores, sysInfo.CPUInfo.Threads),
		AvailableVersion: sysInfo.CPUInfo.ModelName,
		Severity:         "low",
		RepositorySource: "cpu",
		Metadata: map[string]interface{}{
			"metric_name": "system-cpu",
			"metric_type": "cpu",
			"cpu_model":   sysInfo.CPUInfo.ModelName,
			"cpu_cores":   fmt.Sprintf("%d", sysInfo.CPUInfo.Cores),
			"cpu_threads": fmt.Sprintf("%d", sysInfo.CPUInfo.Threads),
		},
	})

	// Memory
	memSeverity := "low"
	switch {
	case sysInfo.MemoryInfo.UsedPercent >= 95:
		memSeverity = "critical"
	case sysInfo.MemoryInfo.UsedPercent >= 90:
		memSeverity = "important"
	case sysInfo.MemoryInfo.UsedPercent >= 80:
		memSeverity = "moderate"
	}
	items = append(items, client.UpdateReportItem{
		PackageType:      "system",
		PackageName:      "system-memory",
		PackageDescription: "System memory information",
		CurrentVersion:   fmt.Sprintf("%.1f%% used", sysInfo.MemoryInfo.UsedPercent),
		AvailableVersion: fmt.Sprintf("%d GB total", sysInfo.MemoryInfo.Total/(1024*1024*1024)),
		Severity:         memSeverity,
		RepositorySource: "memory",
		Metadata: map[string]interface{}{
			"metric_name":          "system-memory",
			"metric_type":          "memory",
			"memory_total":         fmt.Sprintf("%d", sysInfo.MemoryInfo.Total),
			"memory_used":          fmt.Sprintf("%d", sysInfo.MemoryInfo.Used),
			"memory_available":     fmt.Sprintf("%d", sysInfo.MemoryInfo.Available),
			"memory_used_percent":  fmt.Sprintf("%.1f", sysInfo.MemoryInfo.UsedPercent),
		},
	})

	// Processes
	items = append(items, client.UpdateReportItem{
		PackageType:      "system",
		PackageName:      "system-processes",
		PackageDescription: "Running processes",
		CurrentVersion:   fmt.Sprintf("%d processes", sysInfo.RunningProcesses),
		AvailableVersion: "n/a",
		Severity:         "low",
		RepositorySource: "processes",
		Metadata: map[string]interface{}{
			"metric_name":   "system-processes",
			"metric_type":   "processes",
			"process_count": fmt.Sprintf("%d", sysInfo.RunningProcesses),
		},
	})

	// Uptime
	items = append(items, client.UpdateReportItem{
		PackageType:      "system",
		PackageName:      "system-uptime",
		PackageDescription: "System uptime",
		CurrentVersion:   sysInfo.Uptime,
		AvailableVersion: "n/a",
		Severity:         "low",
		RepositorySource: "uptime",
		Metadata: map[string]interface{}{
			"metric_name": "system-uptime",
			"metric_type": "uptime",
			"uptime":      sysInfo.Uptime,
		},
	})

	// Reboot required
	if sysInfo.RebootRequired {
		items = append(items, client.UpdateReportItem{
			PackageType:      "system",
			PackageName:      "system-reboot",
			PackageDescription: "System reboot status",
			CurrentVersion:   "required",
			AvailableVersion: "n/a",
			Severity:         "important",
			RepositorySource: "reboot",
			Metadata: map[string]interface{}{
				"metric_name":    "system-reboot",
				"metric_type":    "reboot",
				"reboot_required": "true",
				"reboot_reason":  sysInfo.RebootReason,
			},
		})
	}

	return items, nil
}

// Name returns the scanner name
func (s *SystemScanner) Name() string {
	return "System Metrics Reporter"
}
