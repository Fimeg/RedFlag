package services

import (
	"fmt"
	"log"
	"time"

	"github.com/aggregator-project/aggregator-server/internal/database/queries"
	"github.com/aggregator-project/aggregator-server/internal/models"
	"github.com/google/uuid"
)

// TimeoutService handles timeout management for long-running operations
type TimeoutService struct {
	commandQueries   *queries.CommandQueries
	updateQueries    *queries.UpdateQueries
	ticker          *time.Ticker
	stopChan        chan bool
	timeoutDuration time.Duration
}

// NewTimeoutService creates a new timeout service
func NewTimeoutService(cq *queries.CommandQueries, uq *queries.UpdateQueries) *TimeoutService {
	return &TimeoutService{
		commandQueries:   cq,
		updateQueries:    uq,
		timeoutDuration: 2 * time.Hour, // 2 hours timeout - allows for system upgrades and large operations
		stopChan:        make(chan bool),
	}
}

// Start begins the timeout monitoring service
func (ts *TimeoutService) Start() {
	log.Printf("Starting timeout service with %v timeout duration", ts.timeoutDuration)

	// Create a ticker that runs every 5 minutes
	ts.ticker = time.NewTicker(5 * time.Minute)

	go func() {
		for {
			select {
			case <-ts.ticker.C:
				ts.checkForTimeouts()
			case <-ts.stopChan:
				ts.ticker.Stop()
				log.Println("Timeout service stopped")
				return
			}
		}
	}()
}

// Stop stops the timeout monitoring service
func (ts *TimeoutService) Stop() {
	close(ts.stopChan)
}

// checkForTimeouts checks for commands that have been running too long
func (ts *TimeoutService) checkForTimeouts() {
	log.Println("Checking for timed out operations...")

	// Get all commands that are in 'sent' status
	commands, err := ts.commandQueries.GetCommandsByStatus(models.CommandStatusSent)
	if err != nil {
		log.Printf("Error getting sent commands: %v", err)
		return
	}

	timeoutThreshold := time.Now().Add(-ts.timeoutDuration)
	timedOutCommands := make([]models.AgentCommand, 0)

	for _, command := range commands {
		// Check if command has been sent and is older than timeout threshold
		if command.SentAt != nil && command.SentAt.Before(timeoutThreshold) {
			timedOutCommands = append(timedOutCommands, command)
		}
	}

	if len(timedOutCommands) > 0 {
		log.Printf("Found %d timed out commands", len(timedOutCommands))

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
	log.Printf("Timing out command %s (type: %s, agent: %s)",
		command.ID, command.CommandType, command.AgentID)

	// Update command status to timed_out
	if err := ts.commandQueries.UpdateCommandStatus(command.ID, models.CommandStatusTimedOut); err != nil {
		return fmt.Errorf("failed to update command status: %w", err)
	}

	// Update result with timeout information
	result := models.JSONB{
		"error":       "operation timed out",
		"timeout_at":  time.Now(),
		"duration":    ts.timeoutDuration.String(),
		"command_id":  command.ID.String(),
	}

	if err := ts.commandQueries.UpdateCommandResult(command.ID, result); err != nil {
		return fmt.Errorf("failed to update command result: %w", err)
	}

	// Update related update package status if applicable
	if err := ts.updateRelatedPackageStatus(command); err != nil {
		log.Printf("Warning: failed to update related package status: %v", err)
		// Don't return error here as the main timeout operation succeeded
	}

	// Create a log entry for the timeout
	logEntry := &models.UpdateLog{
		ID:              uuid.New(),
		AgentID:         command.AgentID,
		UpdatePackageID: ts.extractUpdatePackageID(command),
		Action:          command.CommandType,
		Result:          "timed_out",
		Stdout:          "",
		Stderr:          fmt.Sprintf("Command %s timed out after %v", command.CommandType, ts.timeoutDuration),
		ExitCode:        124, // Standard timeout exit code
		DurationSeconds: int(ts.timeoutDuration.Seconds()),
		ExecutedAt:      time.Now(),
	}

	if err := ts.updateQueries.CreateUpdateLog(logEntry); err != nil {
		log.Printf("Warning: failed to create timeout log entry: %v", err)
		// Don't return error here as the main timeout operation succeeded
	}

	log.Printf("Successfully timed out command %s", command.ID)
	return nil
}

// updateRelatedPackageStatus updates the status of related update packages when a command times out
func (ts *TimeoutService) updateRelatedPackageStatus(command *models.AgentCommand) error {
	// Extract update_id from command params if it exists
	_, ok := command.Params["update_id"].(string)
	if !ok {
		// This command doesn't have an associated update_id, so no package status to update
		return nil
	}

	// Update the package status to 'failed' with timeout reason
	metadata := models.JSONB{
		"timeout":       true,
		"timeout_at":    time.Now(),
		"timeout_duration": ts.timeoutDuration.String(),
		"command_id":    command.ID.String(),
		"failure_reason": "operation timed out",
	}

	return ts.updateQueries.UpdatePackageStatus(command.AgentID,
		command.Params["package_type"].(string),
		command.Params["package_name"].(string),
		"failed",
		metadata,
		nil) // nil = use time.Now() for timeout operations
}

// extractUpdatePackageID extracts the update package ID from command params
func (ts *TimeoutService) extractUpdatePackageID(command *models.AgentCommand) *uuid.UUID {
	updateIDStr, ok := command.Params["update_id"].(string)
	if !ok {
		return nil
	}

	updateID, err := uuid.Parse(updateIDStr)
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
	timeoutThreshold := time.Now().Add(-ts.timeoutDuration + 5*time.Minute)
	approachingTimeout := 0
	for _, command := range activeCommands {
		if command.SentAt != nil && command.SentAt.Before(timeoutThreshold) {
			approachingTimeout++
		}
	}

	return map[string]interface{}{
		"total_timed_out":     len(timedOutCommands),
		"total_active":        len(activeCommands),
		"approaching_timeout": approachingTimeout,
		"timeout_duration":    ts.timeoutDuration.String(),
		"last_check":         time.Now(),
	}, nil
}

// SetTimeoutDuration allows changing the timeout duration
func (ts *TimeoutService) SetTimeoutDuration(duration time.Duration) {
	ts.timeoutDuration = duration
	log.Printf("Timeout duration updated to %v", duration)
}