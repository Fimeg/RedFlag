package queries

import (
	"time"

	"github.com/aggregator-project/aggregator-server/internal/models"
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
			id, agent_id, command_type, params, status
		) VALUES (
			:id, :agent_id, :command_type, :params, :status
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
