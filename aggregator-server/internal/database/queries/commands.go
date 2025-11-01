package queries

import (
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type CommandQueries struct {
	db *sqlx.DB
}

func NewCommandQueries(db *sqlx.DB) *CommandQueries {
	return &CommandQueries{db: db}
}

// CreateCommand inserts a new command for an agent
func (q *CommandQueries) CreateCommand(cmd *models.AgentCommand) error {
	query := `
		INSERT INTO agent_commands (
			id, agent_id, command_type, params, status, source, retried_from_id
		) VALUES (
			:id, :agent_id, :command_type, :params, :status, :source, :retried_from_id
		)
	`
	_, err := q.db.NamedExec(query, cmd)
	return err
}

// GetPendingCommands retrieves pending commands for an agent
func (q *CommandQueries) GetPendingCommands(agentID uuid.UUID) ([]models.AgentCommand, error) {
	var commands []models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE agent_id = $1 AND status = 'pending'
		ORDER BY created_at ASC
		LIMIT 10
	`
	err := q.db.Select(&commands, query, agentID)
	return commands, err
}

// MarkCommandSent updates a command's status to sent
func (q *CommandQueries) MarkCommandSent(id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE agent_commands
		SET status = 'sent', sent_at = $1
		WHERE id = $2
	`
	_, err := q.db.Exec(query, now, id)
	return err
}

// MarkCommandCompleted updates a command's status to completed
func (q *CommandQueries) MarkCommandCompleted(id uuid.UUID, result models.JSONB) error {
	now := time.Now()
	query := `
		UPDATE agent_commands
		SET status = 'completed', completed_at = $1, result = $2
		WHERE id = $3
	`
	_, err := q.db.Exec(query, now, result, id)
	return err
}

// MarkCommandFailed updates a command's status to failed
func (q *CommandQueries) MarkCommandFailed(id uuid.UUID, result models.JSONB) error {
	now := time.Now()
	query := `
		UPDATE agent_commands
		SET status = 'failed', completed_at = $1, result = $2
		WHERE id = $3
	`
	_, err := q.db.Exec(query, now, result, id)
	return err
}

// GetCommandsByStatus retrieves commands with a specific status
func (q *CommandQueries) GetCommandsByStatus(status string) ([]models.AgentCommand, error) {
	var commands []models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE status = $1
		ORDER BY created_at DESC
	`
	err := q.db.Select(&commands, query, status)
	return commands, err
}

// UpdateCommandStatus updates only the status of a command
func (q *CommandQueries) UpdateCommandStatus(id uuid.UUID, status string) error {
	query := `
		UPDATE agent_commands
		SET status = $1
		WHERE id = $2
	`
	_, err := q.db.Exec(query, status, id)
	return err
}

// UpdateCommandResult updates only the result of a command
func (q *CommandQueries) UpdateCommandResult(id uuid.UUID, result interface{}) error {
	query := `
		UPDATE agent_commands
		SET result = $1
		WHERE id = $2
	`
	_, err := q.db.Exec(query, result, id)
	return err
}

// GetCommandByID retrieves a specific command by ID
func (q *CommandQueries) GetCommandByID(id uuid.UUID) (*models.AgentCommand, error) {
	var command models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE id = $1
	`
	err := q.db.Get(&command, query, id)
	if err != nil {
		return nil, err
	}
	return &command, nil
}

// CancelCommand marks a command as cancelled
func (q *CommandQueries) CancelCommand(id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE agent_commands
		SET status = 'cancelled', completed_at = $1
		WHERE id = $2 AND status IN ('pending', 'sent')
	`
	_, err := q.db.Exec(query, now, id)
	return err
}

// RetryCommand creates a new command based on a failed/timed_out/cancelled command
func (q *CommandQueries) RetryCommand(originalID uuid.UUID) (*models.AgentCommand, error) {
	// Get the original command
	original, err := q.GetCommandByID(originalID)
	if err != nil {
		return nil, err
	}

	// Only allow retry of failed, timed_out, or cancelled commands
	if original.Status != "failed" && original.Status != "timed_out" && original.Status != "cancelled" {
		return nil, fmt.Errorf("command must be failed, timed_out, or cancelled to retry")
	}

	// Create new command with same parameters, linking it to the original
	newCommand := &models.AgentCommand{
		ID:            uuid.New(),
		AgentID:       original.AgentID,
		CommandType:   original.CommandType,
		Params:        original.Params,
		Status:        models.CommandStatusPending,
		CreatedAt:     time.Now(),
		RetriedFromID: &originalID,
	}

	// Store the new command
	if err := q.CreateCommand(newCommand); err != nil {
		return nil, err
	}

	return newCommand, nil
}

// GetActiveCommands retrieves commands that are not in a final/terminal state
// Shows anything that's in progress or can be retried (excludes completed and cancelled)
func (q *CommandQueries) GetActiveCommands() ([]models.ActiveCommandInfo, error) {
	var commands []models.ActiveCommandInfo

	query := `
		SELECT
			c.id,
			c.agent_id,
			c.command_type,
			c.params,
			c.status,
			c.source,
			c.created_at,
			c.sent_at,
			c.result,
			c.retried_from_id,
			a.hostname as agent_hostname,
			COALESCE(ups.package_name, 'N/A') as package_name,
			COALESCE(ups.package_type, 'N/A') as package_type,
			(c.retried_from_id IS NOT NULL) as is_retry,
			EXISTS(SELECT 1 FROM agent_commands WHERE retried_from_id = c.id) as has_been_retried,
			COALESCE((
				WITH RECURSIVE retry_chain AS (
					SELECT id, retried_from_id, 1 as depth
					FROM agent_commands
					WHERE id = c.id
					UNION ALL
					SELECT ac.id, ac.retried_from_id, rc.depth + 1
					FROM agent_commands ac
					JOIN retry_chain rc ON ac.id = rc.retried_from_id
				)
				SELECT MAX(depth) FROM retry_chain
			), 1) - 1 as retry_count
		FROM agent_commands c
		LEFT JOIN agents a ON c.agent_id = a.id
		LEFT JOIN current_package_state ups ON (
			c.params->>'update_id' = ups.id::text OR
			(c.params->>'package_name' = ups.package_name AND c.params->>'package_type' = ups.package_type)
		)
		WHERE c.status NOT IN ('completed', 'cancelled', 'archived_failed')
		AND NOT (
			c.status IN ('failed', 'timed_out')
			AND EXISTS (
				SELECT 1 FROM agent_commands retry
				WHERE retry.retried_from_id = c.id
				AND retry.status = 'completed'
			)
		)
		ORDER BY c.created_at DESC
	`

	err := q.db.Select(&commands, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active commands: %w", err)
	}

	return commands, nil
}

// GetRecentCommands retrieves recent commands (including failed, completed, etc.) for retry functionality
func (q *CommandQueries) GetRecentCommands(limit int) ([]models.ActiveCommandInfo, error) {
	var commands []models.ActiveCommandInfo

	if limit == 0 {
		limit = 50 // Default limit
	}

	query := `
		SELECT
			c.id,
			c.agent_id,
			c.command_type,
			c.status,
			c.source,
			c.created_at,
			c.sent_at,
			c.completed_at,
			c.result,
			c.retried_from_id,
			a.hostname as agent_hostname,
			COALESCE(ups.package_name, 'N/A') as package_name,
			COALESCE(ups.package_type, 'N/A') as package_type,
			(c.retried_from_id IS NOT NULL) as is_retry,
			EXISTS(SELECT 1 FROM agent_commands WHERE retried_from_id = c.id) as has_been_retried,
			COALESCE((
				WITH RECURSIVE retry_chain AS (
					SELECT id, retried_from_id, 1 as depth
					FROM agent_commands
					WHERE id = c.id
					UNION ALL
					SELECT ac.id, ac.retried_from_id, rc.depth + 1
					FROM agent_commands ac
					JOIN retry_chain rc ON ac.id = rc.retried_from_id
				)
				SELECT MAX(depth) FROM retry_chain
			), 1) - 1 as retry_count
		FROM agent_commands c
		LEFT JOIN agents a ON c.agent_id = a.id
		LEFT JOIN current_package_state ups ON (
			c.params->>'update_id' = ups.id::text OR
			(c.params->>'package_name' = ups.package_name AND c.params->>'package_type' = ups.package_type)
		)
		ORDER BY c.created_at DESC
		LIMIT $1
	`

	err := q.db.Select(&commands, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent commands: %w", err)
	}

	return commands, nil
}

// ClearOldFailedCommands archives failed commands older than specified days by changing status to 'archived_failed'
func (q *CommandQueries) ClearOldFailedCommands(days int) (int64, error) {
	query := fmt.Sprintf(`
		UPDATE agent_commands
		SET status = 'archived_failed'
		WHERE status IN ('failed', 'timed_out')
		AND created_at < NOW() - INTERVAL '%d days'
	`, days)

	result, err := q.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to archive old failed commands: %w", err)
	}

	return result.RowsAffected()
}

// ClearRetriedFailedCommands archives failed commands that have been retried and are older than specified days
func (q *CommandQueries) ClearRetriedFailedCommands(days int) (int64, error) {
	query := fmt.Sprintf(`
		UPDATE agent_commands
		SET status = 'archived_failed'
		WHERE status IN ('failed', 'timed_out')
		AND EXISTS (SELECT 1 FROM agent_commands WHERE retried_from_id = agent_commands.id)
		AND created_at < NOW() - INTERVAL '%d days'
	`, days)

	result, err := q.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to archive retried failed commands: %w", err)
	}

	return result.RowsAffected()
}

// ClearAllFailedCommands archives all failed commands older than specified days (most aggressive)
func (q *CommandQueries) ClearAllFailedCommands(days int) (int64, error) {
	query := fmt.Sprintf(`
		UPDATE agent_commands
		SET status = 'archived_failed'
		WHERE status IN ('failed', 'timed_out')
		AND created_at < NOW() - INTERVAL '%d days'
	`, days)

	result, err := q.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to archive all failed commands: %w", err)
	}

	return result.RowsAffected()
}
