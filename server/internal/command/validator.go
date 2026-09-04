package command

import (
	"errors"
	"fmt"

	"github.com/gofrs/uuid/v5"
	"github.com/Fimeg/RedFlag/server/internal/models"
)

// Validator validates command parameters
type Validator struct {
	minCheckInSeconds int
	maxCheckInSeconds int
	minScannerMinutes int
	maxScannerMinutes int
}

// NewValidator creates a new command validator
func NewValidator() *Validator {
	return &Validator{
		minCheckInSeconds: 60,   // 1 minute minimum
		maxCheckInSeconds: 3600, // 1 hour maximum
		minScannerMinutes: 1,    // 1 minute minimum
		maxScannerMinutes: 1440, // 24 hours maximum
	}
}

// Validate performs comprehensive command validation
func (v *Validator) Validate(cmd *models.AgentCommand) error {
	if cmd == nil {
		return errors.New("command cannot be nil")
	}

	if cmd.ID == uuid.Nil {
		return errors.New("command ID cannot be zero UUID")
	}

	if cmd.AgentID == uuid.Nil {
		return errors.New("agent ID is required")
	}

	if cmd.CommandType == "" {
		return errors.New("command type is required")
	}

	if cmd.Status == "" {
		return errors.New("status is required")
	}

	validStatuses := []string{"pending", "running", "completed", "failed", "cancelled"}
	if !contains(validStatuses, cmd.Status) {
		return fmt.Errorf("invalid status: %s", cmd.Status)
	}

	if cmd.Source != "manual" && cmd.Source != "system" {
		return fmt.Errorf("source must be 'manual' or 'system', got: %s", cmd.Source)
	}

	// Validate command type format
	if err := v.validateCommandType(cmd.CommandType); err != nil {
		return err
	}

	return nil
}

// ValidateSubsystemAction validates subsystem-specific actions
func (v *Validator) ValidateSubsystemAction(subsystem string, action string) error {
	validActions := map[string][]string{
		"storage": {"trigger", "enable", "disable", "set_interval"},
		"system":  {"trigger", "enable", "disable", "set_interval"},
		"docker":  {"trigger", "enable", "disable", "set_interval"},
		"updates": {"trigger", "enable", "disable", "set_interval"},
	}

	actions, ok := validActions[subsystem]
	if !ok {
		return fmt.Errorf("unknown subsystem: %s", subsystem)
	}

	if !contains(actions, action) {
		return fmt.Errorf("invalid action '%s' for subsystem '%s'", action, subsystem)
	}

	return nil
}

// ValidateInterval ensures scanner intervals are within bounds
func (v *Validator) ValidateInterval(subsystem string, minutes int) error {
	if minutes < v.minScannerMinutes {
		return fmt.Errorf("interval %d minutes below minimum %d for subsystem %s",
			minutes, v.minScannerMinutes, subsystem)
	}

	if minutes > v.maxScannerMinutes {
		return fmt.Errorf("interval %d minutes above maximum %d for subsystem %s",
			minutes, v.maxScannerMinutes, subsystem)
	}

	return nil
}

func (v *Validator) validateCommandType(commandType string) error {
	validPrefixes := []string{"scan_", "install_", "update_", "enable_", "disable_", "reboot"}

	for _, prefix := range validPrefixes {
		if len(commandType) >= len(prefix) && commandType[:len(prefix)] == prefix {
			return nil
		}
	}

	return fmt.Errorf("invalid command type format: %s", commandType)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
