package handlers

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
)

// HandleReboot schedules a system reboot in response to a server-issued
// `reboot` command. Ported from the pre-TD-001 main.go (commit 9da5134e^)
// where it was lost in the god-function split. The server-side endpoint
// (POST /api/v1/agents/:id/reboot) creates the command; without this
// handler the agent silently drops it in dispatch's default case.
func HandleReboot(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, commandID string, params map[string]interface{}) error {
	delayMinutes := 1
	message := "System reboot requested by RedFlag"

	if delay, ok := params["delay_minutes"]; ok {
		if delayFloat, ok := delay.(float64); ok {
			delayMinutes = int(delayFloat)
		}
	}
	if msg, ok := params["message"].(string); ok && msg != "" {
		message = msg
	}

	log.Printf("[INFO] [agent] [reboot] scheduled delay_minutes=%d message=%q", delayMinutes, message)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("shutdown", "-r", fmt.Sprintf("+%d", delayMinutes), message)
	case "windows":
		delaySeconds := delayMinutes * 60
		cmd = exec.Command("shutdown", "/r", "/t", fmt.Sprintf("%d", delaySeconds), "/c", message)
	default:
		err := fmt.Errorf("reboot not supported on platform: %s", runtime.GOOS)
		log.Printf("[ERROR] [agent] [reboot] unsupported_platform os=%s", runtime.GOOS)
		ReportLogWithAck(apiClient, cfg, ackTracker, client.LogReport{
			CommandID: commandID,
			Action:    "reboot",
			Result:    "failed",
			Stderr:    err.Error(),
			ExitCode:  1,
		})
		return err
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERROR] [agent] [reboot] schedule_failed error=%v output=%q", err, string(output))
		ReportLogWithAck(apiClient, cfg, ackTracker, client.LogReport{
			CommandID: commandID,
			Action:    "reboot",
			Result:    "failed",
			Stdout:    string(output),
			Stderr:    err.Error(),
			ExitCode:  1,
		})
		return err
	}

	log.Printf("[INFO] [agent] [reboot] scheduled_ok delay_minutes=%d", delayMinutes)

	if reportErr := ReportLogWithAck(apiClient, cfg, ackTracker, client.LogReport{
		CommandID: commandID,
		Action:    "reboot",
		Result:    "success",
		Stdout:    fmt.Sprintf("System reboot scheduled for %d minute(s) from now. Message: %s", delayMinutes, message),
		ExitCode:  0,
	}); reportErr != nil {
		log.Printf("[ERROR] [agent] [reboot] report_failed error=%v", reportErr)
	}

	return nil
}
