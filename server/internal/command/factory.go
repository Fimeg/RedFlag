package command

import (
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// Factory creates validated AgentCommand instances
type Factory struct {
	validator      *Validator
	commandQueries *queries.CommandQueries
}

// NewFactory creates a new command factory
func NewFactory(commandQueries *queries.CommandQueries) *Factory {
	return &Factory{
		validator:      NewValidator(),
		commandQueries: commandQueries,
	}
}

// Create generates a new validated AgentCommand with unique ID
func (f *Factory) Create(agentID uuid.UUID, commandType string, params map[string]interface{}) (*models.AgentCommand, error) {
	cmd := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     agentID,
		CommandType: commandType,
		Status:      "pending",
		Source:      determineSource(commandType),
		Params:      params,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := f.validator.Validate(cmd); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	return cmd, nil
}

// CreateWithIdempotency generates a command with idempotency protection
// If a command with the same idempotency key exists, returns it instead of creating a duplicate
func (f *Factory) CreateWithIdempotency(agentID uuid.UUID, commandType string, params map[string]interface{}, idempotencyKey string) (*models.AgentCommand, error) {
	// If no idempotency key provided, create normally
	if idempotencyKey == "" {
		return f.Create(agentID, commandType, params)
	}

	// Check for existing command with same idempotency key
	existing, err := f.commandQueries.GetCommandByIdempotencyKey(agentID, idempotencyKey)
	if err != nil {
		// If no existing command found, proceed with creation
		if err.Error() == "sql: no rows in result set" || err.Error() == "command not found" {
			cmd := &models.AgentCommand{
				ID:             uuid.Must(uuid.NewV4()),
				AgentID:        agentID,
				CommandType:    commandType,
				Status:         "pending",
				Source:         determineSource(commandType),
				IdempotencyKey: &idempotencyKey,
				Params:         params,
				CreatedAt:      time.Now().UTC(),
				UpdatedAt:      time.Now().UTC(),
			}

			if err := f.validator.Validate(cmd); err != nil {
				return nil, fmt.Errorf("command validation failed: %w", err)
			}
			return cmd, nil
		}
		return nil, fmt.Errorf("failed to check idempotency: %w", err)
	}

	// Return existing command
	return existing, nil
}

// determineSource classifies command source based on type
func determineSource(commandType string) string {
	if isSystemCommand(commandType) {
		return "system"
	}
	return "manual"
}

func isSystemCommand(commandType string) bool {
	systemCommands := []string{
		"enable_heartbeat",
		"disable_heartbeat",
		"update_check",
		"cleanup_old_logs",
		"heartbeat_on",
		"heartbeat_off",
	}

	for _, cmd := range systemCommands {
		if commandType == cmd {
			return true
		}
	}
	return false
}
