package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/system"
)

// HandleScanProcesses performs a full on-demand process scan and reports
// the results to the server. Triggered by the "scan_processes" command
// (issued when a user opens the Processes tab in the dashboard).
func HandleScanProcesses(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, commandID string) error {
	log.Printf("[INFO] [agent] [processes] scan_started command_id=%s", commandID)

	startTime := time.Now()
	snapshot, err := system.GetFullProcessSnapshot()
	duration := time.Since(startTime)

	if err != nil {
		log.Printf("[ERROR] [agent] [processes] scan_failed command_id=%s duration=%s error=%v", commandID, duration, err)
		return fmt.Errorf("process scan failed: %w", err)
	}

	log.Printf("[INFO] [agent] [processes] scan_complete command_id=%s process_count=%d duration=%s", commandID, snapshot.ProcessCount, duration)

	// Report to server
	report := client.ProcessScanReport{
		AgentID:   cfg.AgentID,
		CommandID: commandID,
		Timestamp: time.Now().UTC(),
		Snapshot:  *snapshot,
	}

	if err := apiClient.ReportProcessScan(cfg.AgentID, report); err != nil {
		log.Printf("[ERROR] [agent] [processes] report_failed command_id=%s error=%v", commandID, err)
		return fmt.Errorf("failed to report process scan: %w", err)
	}

	// Audit log
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_processes",
		Result:          "success",
		ExitCode:        0,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "Processes",
			"subsystem":       "processes",
			"process_count":   fmt.Sprintf("%d", snapshot.ProcessCount),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [processes] report_log_failed: %v", err)
	}

	return nil
}
