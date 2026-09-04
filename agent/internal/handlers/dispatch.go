package handlers

import (
	"fmt"
	"log"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/event"
	"github.com/Fimeg/RedFlag/agent/internal/orchestrator"
)

// reportFailure sends a command failure back to the server so it doesn't
// get stuck in "received" indefinitely. Called when a handler that already
// reported "started" hits an error before completing.
func reportFailure(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, cmdType, cmdID, errMsg string) {
	logReport := client.LogReport{
		CommandID:       cmdID,
		Action:          cmdType,
		Result:          "failed",
		Stderr:          errMsg,
		ExitCode:        1,
		DurationSeconds: 0,
	}
	// Best-effort: don't block the caller if the server is unreachable.
	if rErr := ReportLogWithAck(apiClient, cfg, ackTracker, logReport); rErr != nil {
		log.Printf("[ERROR] [agent] [dispatch] failure_report_failed cmd_type=%s cmd_id=%s error=%q", cmdType, cmdID, rErr)
	}
}

// DispatchCrossPlatformCommand handles command types that work on every
// platform (scans, agent self-update). Returns true if cmd.Type matched a
// known cross-platform command; the caller is then responsible only for
// platform-specific command types. Errors from the underlying handler are
// logged here so both the regular agent loop and the Windows service path
// produce a uniform error trail.
func DispatchCrossPlatformCommand(
	apiClient *client.Client,
	cfg *config.Config,
	ackTracker *acknowledgment.Tracker,
	orch *orchestrator.Orchestrator,
	eventBuffer *event.Buffer,
	cmd client.Command,
) bool {
	var err error
	switch cmd.Type {
	case "scan_storage":
		err = HandleScanStorage(apiClient, cfg, ackTracker, orch, cmd.ID)
	case "scan_system":
		err = HandleScanSystem(apiClient, cfg, ackTracker, orch, cmd.ID)
	case "scan_docker":
		err = HandleScanDocker(apiClient, cfg, ackTracker, orch, cmd.ID)
	case "scan_apt":
		err = HandleScanAPT(apiClient, cfg, ackTracker, orch, cmd.ID)
	case "scan_dnf":
		err = HandleScanDNF(apiClient, cfg, ackTracker, orch, cmd.ID)
	case "scan_windows":
		err = HandleScanWindows(apiClient, cfg, ackTracker, orch, cmd.ID)
	case "scan_winget":
		err = HandleScanWinget(apiClient, cfg, ackTracker, orch, cmd.ID)
	case "scan_updates":
		err = HandleScanUpdates(apiClient, cfg, ackTracker, orch, cmd.ID)
	case "install_updates":
		err = HandleInstallUpdates(apiClient, cfg, ackTracker, eventBuffer, cmd.Params, cmd.ID)
	case "dry_run_update":
		err = HandleDryRunUpdate(apiClient, cfg, ackTracker, cmd.Params, cmd.ID)
	case "confirm_dependencies":
		err = HandleConfirmDependencies(apiClient, cfg, ackTracker, eventBuffer, cmd.Params, cmd.ID)
	case "enable_heartbeat":
		err = HandleEnableHeartbeat(apiClient, cfg, ackTracker, cmd.Params, cmd.ID)
	case "disable_heartbeat":
		err = HandleDisableHeartbeat(apiClient, cfg, ackTracker, cmd.ID)
	case "update_agent":
		err = HandleUpdateAgent(apiClient, cfg, ackTracker, cmd.Params, cmd.ID)
	case "reboot":
		err = HandleReboot(apiClient, cfg, ackTracker, cmd.ID, cmd.Params)
	case "capture_screenshot":
		err = HandleCaptureScreenshot(apiClient, cfg, ackTracker, cmd.ID)
	case "scan_processes":
		err = HandleScanProcesses(apiClient, cfg, ackTracker, cmd.ID)
	default:
		return false
	}
	if err != nil {
		log.Printf("[ERROR] [agent] [%s] command_failed error=%q", cmd.Type, err)
		reportFailure(apiClient, cfg, ackTracker, cmd.Type, cmd.ID, fmt.Sprintf("command failed: %s", err))
	}
	return true
}
