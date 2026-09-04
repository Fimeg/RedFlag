package services

import (
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// TimeoutService handles timeout management for long-running operations.
// Periodic execution is managed by the taskrunner; call CheckTimeouts on each tick.
type TimeoutService struct {
	commandQueries   *queries.CommandQueries
	updateQueries    *queries.UpdateQueries
	agentQueries     *queries.AgentQueries
	securitySettings *SecuritySettingsService // optional; overrides updateTimeout per-tick via operational.update_stuck_minutes
	sentTimeout      time.Duration            // For commands already sent to agents
	pendingTimeout   time.Duration            // For commands stuck in queue
	receivedTimeout  time.Duration            // For commands received by agent but not completed (Migration 033 §4)
	updateTimeout    time.Duration            // Default for agents stuck in is_updating=true; overridden by settings if wired
	checkInterval    time.Duration            // Stored for callers; scheduling is handled by bgRunner
}

// SetSecuritySettings injects the settings service. When wired, the
// reconcileAgentUpdates threshold is read from operational.update_stuck_minutes
// on every tick — changes take effect on the next sweep without restart.
func (ts *TimeoutService) SetSecuritySettings(s *SecuritySettingsService) {
	ts.securitySettings = s
}

// effectiveUpdateTimeout returns the live update-stuck threshold, preferring
// the settings value over the construction-time default.
func (ts *TimeoutService) effectiveUpdateTimeout() time.Duration {
	if ts.securitySettings == nil {
		return ts.updateTimeout
	}
	minutes := ts.securitySettings.GetOperationalInt("update_stuck_minutes", int(ts.updateTimeout.Minutes()))
	if minutes <= 0 {
		return ts.updateTimeout
	}
	return time.Duration(minutes) * time.Minute
}

// NewTimeoutService creates a new timeout service with configurable durations.
// Pass zero values to use defaults (2h sent, 30m pending, 30m received, 15m update,
// 5m check interval).
func NewTimeoutService(cq *queries.CommandQueries, uq *queries.UpdateQueries, aq *queries.AgentQueries, sentTimeout, pendingTimeout, receivedTimeout, updateTimeout, checkInterval time.Duration) *TimeoutService {
	if sentTimeout <= 0 {
		sentTimeout = 2 * time.Hour
	}
	if pendingTimeout <= 0 {
		pendingTimeout = 30 * time.Minute
	}
	if receivedTimeout <= 0 {
		receivedTimeout = 30 * time.Minute
	}
	if updateTimeout <= 0 {
		updateTimeout = 15 * time.Minute
	}
	if checkInterval <= 0 {
		checkInterval = 5 * time.Minute
	}
	return &TimeoutService{
		commandQueries:  cq,
		updateQueries:   uq,
		agentQueries:    aq,
		sentTimeout:     sentTimeout,
		pendingTimeout:  pendingTimeout,
		receivedTimeout: receivedTimeout,
		updateTimeout:   updateTimeout,
		checkInterval:   checkInterval,
	}
}

// CheckTimeouts runs one full sweep: sent/pending command timeouts, received
// command timeouts, and stuck-updating agent reconciliation. Register this with
// bgRunner.Every so the runner owns the ticker, shutdown, and panic isolation.
func (ts *TimeoutService) CheckTimeouts() {
	log.Printf("[INFO] [server] [timeout] check_start sent_timeout=%v pending_timeout=%v received_timeout=%v update_timeout=%v",
		ts.sentTimeout, ts.pendingTimeout, ts.receivedTimeout, ts.effectiveUpdateTimeout())
	ts.checkForTimeouts()
	ts.checkForReceivedTimeouts()
	ts.reconcileAgentUpdates()
}

// checkForTimeouts checks for commands that have been running too long
func (ts *TimeoutService) checkForTimeouts() {
	log.Println("Checking for timed out operations...")

	sentTimeoutThreshold := time.Now().Add(-ts.sentTimeout)
	pendingTimeoutThreshold := time.Now().Add(-ts.pendingTimeout)
	timedOutCommands := make([]models.AgentCommand, 0)

	// Check 'sent' commands (configurable, default 2 hours)
	sentCommands, err := ts.commandQueries.GetCommandsByStatus(models.CommandStatusSent)
	if err != nil {
		log.Printf("Error getting sent commands: %v", err)
	} else {
		for _, command := range sentCommands {
			// Check if command has been sent and is older than sent timeout threshold
			if command.SentAt != nil && command.SentAt.Before(sentTimeoutThreshold) {
				timedOutCommands = append(timedOutCommands, command)
			}
		}
	}

	// Check 'pending' commands (configurable, default 30 minutes)
	pendingCommands, err := ts.commandQueries.GetCommandsByStatus(models.CommandStatusPending)
	if err != nil {
		log.Printf("Error getting pending commands: %v", err)
	} else {
		for _, command := range pendingCommands {
			// Check if command has been pending longer than pending timeout threshold
			if command.CreatedAt.Before(pendingTimeoutThreshold) {
				timedOutCommands = append(timedOutCommands, command)
				log.Printf("Found stuck pending command %s (type: %s, created: %s, age: %v)",
					command.ID, command.CommandType, command.CreatedAt.Format(time.RFC3339), time.Since(command.CreatedAt))
			}
		}
	}

	if len(timedOutCommands) > 0 {
		log.Printf("[INFO] [server] [timeout] timed_out_commands=%d sent_checked=%d pending_checked=%d sent_timeout=%v pending_timeout=%v",
			len(timedOutCommands), len(sentCommands), len(pendingCommands), ts.sentTimeout, ts.pendingTimeout)

		for _, command := range timedOutCommands {
			if err := ts.timeoutCommand(&command); err != nil {
				log.Printf("Error timing out command %s: %v", command.ID, err)
			}
		}
	} else {
		log.Println("No timed out operations found")
	}
}

// timeoutCommand marks a specific command as timed out and updates related entities
func (ts *TimeoutService) timeoutCommand(command *models.AgentCommand) error {
	// Determine which timeout duration was applied
	var appliedTimeout time.Duration
	if command.Status == models.CommandStatusSent {
		appliedTimeout = ts.sentTimeout
	} else {
		appliedTimeout = ts.pendingTimeout
	}

	log.Printf("Timing out command %s (type: %s, agent: %s)",
		command.ID, command.CommandType, command.AgentID)

	// Update command status to timed_out
	if err := ts.commandQueries.UpdateCommandStatus(command.ID, models.CommandStatusTimedOut); err != nil {
		return fmt.Errorf("failed to update command status: %w", err)
	}

	// Update result with timeout information
	result := models.JSONB{
		"error":       "operation timed out",
		"timeout_at":  time.Now().UTC(),
		"duration":    appliedTimeout.String(),
		"command_id":  command.ID.String(),
	}

	if err := ts.commandQueries.UpdateCommandResult(command.ID, result); err != nil {
		return fmt.Errorf("failed to update command result: %w", err)
	}

	// Update related update package status if applicable
	if err := ts.updateRelatedPackageStatus(command, appliedTimeout); err != nil {
		log.Printf("Warning: failed to update related package status: %v", err)
		// Don't return error here as the main timeout operation succeeded
	}

	// Create a log entry for the timeout
	logEntry := &models.UpdateLog{
		ID:              uuid.Must(uuid.NewV4()),
		AgentID:         command.AgentID,
		UpdatePackageID: ts.extractUpdatePackageID(command),
		Action:          command.CommandType,
		Result:          "failed", // Use 'failed' to comply with database constraint
		Stdout:          "",
		Stderr:          fmt.Sprintf("Command %s timed out after %v (timeout_id: %s)", command.CommandType, appliedTimeout, command.ID),
		ExitCode:        124, // Standard timeout exit code
		DurationSeconds: int(appliedTimeout.Seconds()),
		ExecutedAt:      time.Now().UTC(),
	}

	if err := ts.updateQueries.CreateUpdateLog(logEntry); err != nil {
		log.Printf("Warning: failed to create timeout log entry: %v", err)
		// Don't return error here as the main timeout operation succeeded
	}

	// ETHOS #1: a timed-out command is an auditable delivery failure. update_logs
	// alone does not surface in the events API / history pane (reconcileAgentUpdates
	// in this same service writes system_events; this path must too). Best-effort.
	if ts.agentQueries != nil {
		event := &models.SystemEvent{
			ID:           uuid.Must(uuid.NewV4()),
			AgentID:      &command.AgentID,
			EventType:    models.EventTypeCommandFailed,
			EventSubtype: "timed_out",
			Severity:     models.SeverityWarning,
			Component:    models.ComponentServer,
			Message:      fmt.Sprintf("Command %s (%s) timed out after %v with no agent result", command.CommandType, command.ID, appliedTimeout),
			Metadata: map[string]interface{}{
				"command_id":   command.ID.String(),
				"command_type": command.CommandType,
				"prior_status": command.Status,
				"timeout":      appliedTimeout.String(),
			},
			CreatedAt: time.Now().UTC(),
		}
		if err := ts.agentQueries.CreateSystemEvent(event); err != nil {
			log.Printf("[WARNING] [server] [timeout] system_event_write_failed command_id=%s error=%v", command.ID, err)
		}
	}

	log.Printf("Successfully timed out command %s", command.ID)
	return nil
}

// updateRelatedPackageStatus updates the status of related update packages when a command times out
func (ts *TimeoutService) updateRelatedPackageStatus(command *models.AgentCommand, appliedTimeout time.Duration) error {
	// Extract update_id from command params if it exists
	_, ok := command.Params["update_id"].(string)
	if !ok {
		// This command doesn't have an associated update_id, so no package status to update
		return nil
	}

	// Update the package status to 'failed' with timeout reason
	metadata := models.JSONB{
		"timeout":       true,
		"timeout_at":    time.Now().UTC(),
		"timeout_duration": appliedTimeout.String(),
		"command_id":    command.ID.String(),
		"failure_reason": "operation timed out",
	}

	packageType, typeOK := command.Params["package_type"].(string)
	packageName, nameOK := command.Params["package_name"].(string)
	if !typeOK || !nameOK {
		return fmt.Errorf("command %s has update_id but missing package_type/package_name params", command.ID)
	}

	return ts.updateQueries.UpdatePackageStatus(command.AgentID,
		packageType,
		packageName,
		models.StatusFailed,
		metadata,
		nil) // nil = use time.Now().UTC() for timeout operations
}

// extractUpdatePackageID extracts the update package ID from command params
func (ts *TimeoutService) extractUpdatePackageID(command *models.AgentCommand) *uuid.UUID {
	updateIDStr, ok := command.Params["update_id"].(string)
	if !ok {
		return nil
	}

	updateID, err := uuid.FromString(updateIDStr)
	if err != nil {
		return nil
	}

	return &updateID
}

// GetTimeoutStatus returns statistics about timed out operations
func (ts *TimeoutService) GetTimeoutStatus() (map[string]interface{}, error) {
	// Get all timed out commands
	timedOutCommands, err := ts.commandQueries.GetCommandsByStatus(models.CommandStatusTimedOut)
	if err != nil {
		return nil, fmt.Errorf("failed to get timed out commands: %w", err)
	}

	// Get all active commands
	activeCommands, err := ts.commandQueries.GetCommandsByStatus(models.CommandStatusSent)
	if err != nil {
		return nil, fmt.Errorf("failed to get active commands: %w", err)
	}

	// Count commands approaching timeout (within 5 minutes of timeout)
	timeoutThreshold := time.Now().Add(-ts.sentTimeout + 5*time.Minute)
	approachingTimeout := 0
	for _, command := range activeCommands {
		if command.SentAt != nil && command.SentAt.Before(timeoutThreshold) {
			approachingTimeout++
		}
	}

	return map[string]interface{}{
		"total_timed_out":          len(timedOutCommands),
		"total_active":             len(activeCommands),
		"approaching_timeout":      approachingTimeout,
		"sent_timeout_duration":    ts.sentTimeout.String(),
		"pending_timeout_duration": ts.pendingTimeout.String(),
		"last_check":               time.Now().UTC(),
	}, nil
}

// SetTimeoutDuration allows changing the timeout duration for sent commands
// TODO: This should be deprecated in favor of SetSentTimeout and SetPendingTimeout
func (ts *TimeoutService) SetTimeoutDuration(duration time.Duration) {
	ts.sentTimeout = duration
	log.Printf("Sent timeout duration updated to %v", duration)
}

// SetSentTimeout allows changing the timeout duration for sent commands
func (ts *TimeoutService) SetSentTimeout(duration time.Duration) {
	ts.sentTimeout = duration
	log.Printf("Sent timeout duration updated to %v", duration)
}

// SetPendingTimeout allows changing the timeout duration for pending commands
func (ts *TimeoutService) SetPendingTimeout(duration time.Duration) {
	ts.pendingTimeout = duration
	log.Printf("Pending timeout duration updated to %v", duration)
}

// checkForReceivedTimeouts handles commands the agent received but never completed.
// Distinct from sent-timeouts because we know the agent had it — re-issuance won't
// help; the right action is to mark timed_out so the operator (or scheduler) can
// decide whether to retry or escalate.
//
// Doctrine: TODO-full-command-lifecycle.md §4. Migration 033 added the 'received' state.
func (ts *TimeoutService) checkForReceivedTimeouts() {
	tx, err := ts.commandQueries.DB().Beginx()
	if err != nil {
		log.Printf("[ERROR] [server] [timeout] received_tx_begin_failed error=%v", err)
		return
	}
	defer tx.Rollback()

	stuck, err := ts.commandQueries.GetStuckReceivedCommandsTx(tx, ts.receivedTimeout)
	if err != nil {
		log.Printf("[ERROR] [server] [timeout] get_stuck_received_failed error=%v", err)
		return
	}
	if len(stuck) == 0 {
		return
	}

	for _, command := range stuck {
		cmd := command
		if err := ts.timeoutCommand(&cmd); err != nil {
			log.Printf("[ERROR] [server] [timeout] timeout_received_failed command_id=%s error=%v", cmd.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] [server] [timeout] received_tx_commit_failed error=%v", err)
		return
	}

	log.Printf("[INFO] [server] [timeout] received_commands_timed_out count=%d threshold=%v", len(stuck), ts.receivedTimeout)
}

// reconcileAgentUpdates handles agents whose is_updating flag was set but never cleared.
// Decision per agent:
//   - if current_version == updating_to_version: the agent completed the update and reported
//     its new version, but the per-request side-effect to clear is_updating was deliberately
//     omitted. Clear it now (success path).
//   - otherwise: the update never reported in (binary failed to start, network dead, etc).
//     Clear is_updating and log a timeout system_event so the operator sees what happened.
//
// Doctrine: TODO-full-command-lifecycle.md §4 + audit finding "completion loop has no firing path".
func (ts *TimeoutService) reconcileAgentUpdates() {
	if ts.agentQueries == nil {
		return // not wired (test path)
	}
	updateTimeout := ts.effectiveUpdateTimeout()
	threshold := time.Now().Add(-updateTimeout)
	stuck, err := ts.agentQueries.GetAgentsStuckUpdating(threshold)
	if err != nil {
		log.Printf("[ERROR] [server] [timeout] get_stuck_updating_agents_failed error=%v", err)
		return
	}
	if len(stuck) == 0 {
		return
	}

	successes, timeouts := 0, 0
	for _, agent := range stuck {
		target := ""
		if agent.UpdatingToVersion != nil {
			target = *agent.UpdatingToVersion
		}

		if target != "" && agent.CurrentVersion == target {
			// Success — agent reported the new version; just close the flag.
			if err := ts.agentQueries.CompleteAgentUpdate(agent.ID.String(), agent.CurrentVersion); err != nil {
				log.Printf("[ERROR] [server] [timeout] complete_update_failed agent_id=%s error=%v", agent.ID, err)
				continue
			}
			ts.recordUpdateEvent(agent.ID, "succeeded", "info",
				fmt.Sprintf("Agent update succeeded: %s reported", agent.CurrentVersion),
				map[string]interface{}{"new_version": agent.CurrentVersion, "reconciled_by": "timeout_service"})
			successes++
			continue
		}

		// Timeout — version never matched. The new binary either never started
		// or never reached the server. Clear the flag so the operator can retry;
		// surface remediation context because automatic rollback from .bak is
		// not available across the systemd restart boundary (the deferred
		// rollback in agent_update.go cannot survive SIGTERM).
		if err := ts.agentQueries.ClearAgentUpdating(agent.ID); err != nil {
			log.Printf("[ERROR] [server] [timeout] clear_updating_failed agent_id=%s error=%v", agent.ID, err)
			continue
		}
		backupHint := expectedBackupPath(agent.OSType)
		silentSince := ""
		if agent.UpdateInitiatedAt != nil && agent.LastSeen.Before(*agent.UpdateInitiatedAt) {
			silentSince = fmt.Sprintf(" agent has not checked in since update was initiated at %s.", agent.UpdateInitiatedAt.UTC().Format(time.RFC3339))
		}
		ageMessage := fmt.Sprintf(
			"Agent update timed out after %v without version attestation (current=%s, target=%s).%s Manual rollback may be required: restore %s on the agent host and restart the service.",
			updateTimeout, agent.CurrentVersion, target, silentSince, backupHint,
		)
		ts.recordUpdateEvent(agent.ID, "timed_out", "error", ageMessage,
			map[string]interface{}{
				"current_version":      agent.CurrentVersion,
				"target_version":       target,
				"threshold":            updateTimeout.String(),
				"reconciled_by":        "timeout_service",
				"backup_path_hint":     backupHint,
				"agent_silent":         agent.UpdateInitiatedAt != nil && agent.LastSeen.Before(*agent.UpdateInitiatedAt),
				"last_seen":            agent.LastSeen.UTC().Format(time.RFC3339),
				"update_initiated_at":  formatTimePtr(agent.UpdateInitiatedAt),
			})
		timeouts++
	}

	log.Printf("[INFO] [server] [timeout] agent_updates_reconciled stuck=%d succeeded=%d timed_out=%d threshold=%v",
		len(stuck), successes, timeouts, updateTimeout)
}

// expectedBackupPath returns the canonical .bak path written by the agent
// installer for a given OS. Used in operator-facing remediation messages when
// reconcileAgentUpdates can't confirm the new binary is alive.
func expectedBackupPath(osType string) string {
	switch osType {
	case "windows":
		return `C:\Program Files\RedFlag\redflag-agent.exe.bak`
	default:
		return "/usr/local/bin/redflag-agent.bak"
	}
}

func formatTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// recordUpdateEvent writes a system_event row for an update lifecycle transition. Best
// effort — a failure to log shouldn't block the reconciler from continuing on the next agent.
func (ts *TimeoutService) recordUpdateEvent(agentID uuid.UUID, subtype, severity, message string, metadata map[string]interface{}) {
	event := &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      &agentID,
		EventType:    "agent_update",
		EventSubtype: subtype,
		Severity:     severity,
		Component:    "agent",
		Message:      message,
		Metadata:     metadata,
		CreatedAt:    time.Now().UTC(),
	}
	if err := ts.agentQueries.CreateSystemEvent(event); err != nil {
		log.Printf("[WARNING] [server] [timeout] system_event_write_failed agent_id=%s subtype=%s error=%v",
			agentID, subtype, err)
	}
}