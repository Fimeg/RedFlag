package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
)

// HandleEnableHeartbeat flips the agent into rapid-polling mode for the
// duration the server requested. The polling loop reads RapidPollingEnabled
// and RapidPollingUntil on every iteration (see agent/loop.go), so persisting
// the change to config is enough — the next loop tick picks it up.
func HandleEnableHeartbeat(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, params map[string]interface{}, commandID string) error {
	durationMinutes := 60
	if d, ok := params["duration_minutes"].(float64); ok && d > 0 {
		durationMinutes = int(d)
	}

	cfg.RapidPollingEnabled = true
	cfg.RapidPollingUntil = time.Now().UTC().Add(time.Duration(durationMinutes) * time.Minute)

	if err := cfg.Save(constants.GetAgentConfigPath()); err != nil {
		log.Printf("[WARNING] [agent] [heartbeat] config_save_failed error=%v", err)
	}

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "enable_heartbeat",
		Result:          "success",
		Stdout:          fmt.Sprintf("Heartbeat enabled for %d minutes", durationMinutes),
		ExitCode:        0,
		DurationSeconds: 0,
	}
	if err := ReportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[WARNING] [agent] [heartbeat] report_enable_failed error=%v", err)
	}

	log.Printf("[INFO] [agent] [heartbeat] enabled duration_minutes=%d until=%s",
		durationMinutes, cfg.RapidPollingUntil.Format(time.RFC3339))
	return nil
}

// HandleDisableHeartbeat clears rapid-polling state. Called by the server when
// a multi-step flow finishes (or by the timeout reconciler if the flow stalls).
func HandleDisableHeartbeat(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, commandID string) error {
	cfg.RapidPollingEnabled = false
	cfg.RapidPollingUntil = time.Time{}

	if err := cfg.Save(constants.GetAgentConfigPath()); err != nil {
		log.Printf("[WARNING] [agent] [heartbeat] config_save_failed error=%v", err)
	}

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "disable_heartbeat",
		Result:          "success",
		Stdout:          "Heartbeat disabled",
		ExitCode:        0,
		DurationSeconds: 0,
	}
	if err := ReportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[WARNING] [agent] [heartbeat] report_disable_failed error=%v", err)
	}

	log.Printf("[INFO] [agent] [heartbeat] disabled")
	return nil
}
