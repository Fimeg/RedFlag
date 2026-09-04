package queries

import (
	"fmt"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

type CommandQueries struct {
	db *sqlx.DB
}

func NewCommandQueries(db *sqlx.DB) *CommandQueries {
	return &CommandQueries{db: db}
}

// DB returns the underlying database connection for transaction management
func (q *CommandQueries) DB() *sqlx.DB {
	return q.db
}

// commandDefaultTTL is the default time-to-live for new commands
const commandDefaultTTL = 4 * time.Hour

// CreateCommand inserts a new command for an agent
func (q *CommandQueries) CreateCommand(cmd *models.AgentCommand) error {
	// Set expires_at if not already set (default: commandDefaultTTL from now)
	if cmd.ExpiresAt == nil {
		exp := time.Now().UTC().Add(commandDefaultTTL)
		cmd.ExpiresAt = &exp
	}

	if cmd.IdempotencyKey != nil {
		query := `
			INSERT INTO agent_commands (
				id, agent_id, command_type, params, status, source, signature, key_id, signed_at, expires_at, idempotency_key, retried_from_id
			) VALUES (
				:id, :agent_id, :command_type, :params, :status, :source, :signature, :key_id, :signed_at, :expires_at, :idempotency_key, :retried_from_id
			)
			ON CONFLICT (idempotency_key) DO NOTHING
		`
		_, err := q.db.NamedExec(query, cmd)
		return err
	}

	// Without idempotency_key
	query := `
		INSERT INTO agent_commands (
			id, agent_id, command_type, params, status, source, signature, key_id, signed_at, expires_at, retried_from_id
		) VALUES (
			:id, :agent_id, :command_type, :params, :status, :source, :signature, :key_id, :signed_at, :expires_at, :retried_from_id
		)
	`
	_, err := q.db.NamedExec(query, cmd)
	return err
}

// GetPendingCommands retrieves pending commands for an agent
// Only returns 'pending' status - 'sent' commands are handled by timeout service
// Filters out expired commands via expires_at (F-6 fix)
func (q *CommandQueries) GetPendingCommands(agentID uuid.UUID) ([]models.AgentCommand, error) {
	var commands []models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE agent_id = $1 AND status = 'pending'
		AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at ASC
		LIMIT 100
	`
	err := q.db.Select(&commands, query, agentID)
	return commands, err
}

// GetPendingCommandsTx retrieves pending commands with FOR UPDATE SKIP LOCKED (F-B2-2 fix)
// Must be called inside a transaction. Locks rows to prevent duplicate delivery.
func (q *CommandQueries) GetPendingCommandsTx(tx *sqlx.Tx, agentID uuid.UUID) ([]models.AgentCommand, error) {
	var commands []models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE agent_id = $1 AND status = 'pending'
		AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at ASC
		LIMIT 100
		FOR UPDATE SKIP LOCKED
	`
	err := tx.Select(&commands, query, agentID)
	return commands, err
}


// GetCommandsByAgentID retrieves all commands for a specific agent
func (q *CommandQueries) GetCommandsByAgentID(agentID uuid.UUID) ([]models.AgentCommand, error) {
	var commands []models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT 100
	`
	err := q.db.Select(&commands, query, agentID)
	return commands, err
}

// GetCommandByIdempotencyKey retrieves a command by agent ID and idempotency key
func (q *CommandQueries) GetCommandByIdempotencyKey(agentID uuid.UUID, idempotencyKey string) (*models.AgentCommand, error) {
	var cmd models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE agent_id = $1 AND idempotency_key = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	err := q.db.Get(&cmd, query, agentID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// MarkCommandSent updates a command's status to sent
func (q *CommandQueries) MarkCommandSent(id uuid.UUID) error {
	now := time.Now().UTC()
	query := `
		UPDATE agent_commands
		SET status = 'sent', sent_at = $1
		WHERE id = $2
	`
	_, err := q.db.Exec(query, now, id)
	return err
}

// MarkCommandSentTx updates a command's status to sent within a transaction (F-B2-2 fix)
func (q *CommandQueries) MarkCommandSentTx(tx *sqlx.Tx, id uuid.UUID) error {
	now := time.Now().UTC()
	query := `
		UPDATE agent_commands
		SET status = 'sent', sent_at = $1
		WHERE id = $2
	`
	_, err := tx.Exec(query, now, id)
	return err
}

// RedeliverStuckCommandTx marks a stuck command for re-delivery, incrementing retry_count (DEV-029 fix).
// Only used for stuck command re-delivery, NOT for initial first delivery.
func (q *CommandQueries) RedeliverStuckCommandTx(tx *sqlx.Tx, id uuid.UUID) error {
	now := time.Now().UTC()
	query := `
		UPDATE agent_commands
		SET status = 'sent', sent_at = $1, retry_count = retry_count + 1
		WHERE id = $2
	`
	_, err := tx.Exec(query, now, id)
	return err
}

// GetStuckCommandsTx retrieves stuck commands with FOR UPDATE SKIP LOCKED (F-B2-2 fix).
// Excludes commands that have exceeded max retries (F-B2-10 fix).
// Migration 033: 'received' commands are NOT re-issued here — the agent has them and
// is working, just not yet done. Use GetStuckReceivedCommandsTx with a longer threshold
// for that case (handled by TimeoutService, not the per-poll re-issuer).
func (q *CommandQueries) GetStuckCommandsTx(tx *sqlx.Tx, agentID uuid.UUID, olderThan time.Duration, maxRetries int) ([]models.AgentCommand, error) {
	var commands []models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE agent_id = $1
		AND status IN ('pending', 'sent')
		AND (expires_at IS NULL OR expires_at > NOW())
		AND retry_count < $3
		AND (
			(sent_at < $2 AND sent_at IS NOT NULL)
			OR (created_at < $2 AND sent_at IS NULL)
		)
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
	`
	err := tx.Select(&commands, query, agentID, time.Now().UTC().Add(-olderThan), maxRetries)
	return commands, err
}

// MarkCommandsReceivedTx batch-transitions a set of command IDs from 'sent' to 'received'.
// Only commands belonging to the named agent and currently in status 'sent' are flipped —
// this prevents an agent from claiming receipt of another agent's commands, and prevents
// re-transitioning out of terminal states. Returns the IDs that were actually flipped.
//
// Doctrine: TODO-full-command-lifecycle.md §2. Receipt confirmation closes the gap where
// GetStuckCommandsTx would re-issue blindly.
func (q *CommandQueries) MarkCommandsReceivedTx(tx *sqlx.Tx, agentID uuid.UUID, commandIDs []string) ([]string, error) {
	if len(commandIDs) == 0 {
		return []string{}, nil
	}

	parsed := make([]uuid.UUID, 0, len(commandIDs))
	for _, idStr := range commandIDs {
		id, err := uuid.FromString(idStr)
		if err != nil {
			continue
		}
		parsed = append(parsed, id)
	}
	if len(parsed) == 0 {
		return []string{}, nil
	}

	placeholders := make([]string, len(parsed))
	args := make([]interface{}, 0, len(parsed)+1)
	args = append(args, agentID)
	for i, id := range parsed {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		UPDATE agent_commands
		SET status = 'received', received_at = NOW()
		WHERE agent_id = $1
		AND status = 'sent'
		AND id IN (%s)
		RETURNING id::text
	`, strings.Join(placeholders, ","))

	var confirmed []string
	if err := tx.Select(&confirmed, query, args...); err != nil {
		return nil, fmt.Errorf("mark_received batch failed: %w", err)
	}
	return confirmed, nil
}

// GetStuckReceivedCommandsTx returns commands that have been 'received' by an agent
// but haven't transitioned to completed/failed within the longer threshold. These are
// timed out by the TimeoutService rather than re-issued — the agent had it, the agent
// is presumed broken or the command is presumed lost on the agent side.
func (q *CommandQueries) GetStuckReceivedCommandsTx(tx *sqlx.Tx, olderThan time.Duration) ([]models.AgentCommand, error) {
	var commands []models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE status = 'received'
		AND received_at IS NOT NULL
		AND received_at < $1
		ORDER BY received_at ASC
		FOR UPDATE SKIP LOCKED
	`
	err := tx.Select(&commands, query, time.Now().UTC().Add(-olderThan))
	return commands, err
}

// MarkCommandCompleted updates a command's status to completed
func (q *CommandQueries) MarkCommandCompleted(id uuid.UUID, result models.JSONB) error {
	now := time.Now().UTC()
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
	now := time.Now().UTC()
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

// CancelCommand marks a command as cancelled. It also records a result: a
// cancelled command is resolved server-side, so the agent's pending result-ack
// (which clears on result-recorded) should stop retrying rather than stranding
// until its 24h max-age. COALESCE preserves any result the agent already reported.
func (q *CommandQueries) CancelCommand(id uuid.UUID) error {
	now := time.Now().UTC()
	query := `
		UPDATE agent_commands
		SET status = 'cancelled', completed_at = $1,
		    result = COALESCE(result, '{"cancelled": true, "reason": "operator cancelled command"}'::jsonb)
		WHERE id = $2 AND status IN ('pending', 'sent')
	`
	_, err := q.db.Exec(query, now, id)
	return err
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
			c.signature,
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
			c.signature,
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

// ClearAllFailedCommandsRegardlessOfAge archives ALL failed/timed_out commands regardless of age
// This is used when all_failed=true is passed to truly clear all failed commands
func (q *CommandQueries) ClearAllFailedCommandsRegardlessOfAge() (int64, error) {
	query := `
		UPDATE agent_commands
		SET status = 'archived_failed'
		WHERE status IN ('failed', 'timed_out')
	`

	result, err := q.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to archive all failed commands regardless of age: %w", err)
	}

	return result.RowsAffected()
}

// CountPendingCommandsForAgent returns the number of pending commands for a specific agent
// Used by scheduler for backpressure detection
func (q *CommandQueries) CountPendingCommandsForAgent(agentID uuid.UUID) (int, error) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM agent_commands
		WHERE agent_id = $1 AND status = 'pending'
	`
	err := q.db.Get(&count, query, agentID)
	return count, err
}

// GetPendingCommandByType checks if a pending command of specific type exists for agent
// Used to prevent duplicate command creation (scheduler fix)
func (q *CommandQueries) GetPendingCommandByType(agentID uuid.UUID, commandType string) (*models.AgentCommand, error) {
	var cmd models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE agent_id = $1 
		AND command_type = $2 
		AND status = 'pending'
		AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at ASC
		LIMIT 1
	`
	err := q.db.Get(&cmd, query, agentID, commandType)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// GetTotalPendingCommands returns total pending commands across all agents
func (q *CommandQueries) GetTotalPendingCommands() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM agent_commands WHERE status = 'pending'`
	err := q.db.Get(&count, query)
	return count, err
}

// GetAgentsWithPendingCommands returns count of agents with pending commands
func (q *CommandQueries) GetAgentsWithPendingCommands() (int, error) {
	var count int
	query := `
		SELECT COUNT(DISTINCT agent_id)
		FROM agent_commands
		WHERE status = 'pending'
	`
	err := q.db.Get(&count, query)
	return count, err
}

// GetCommandsInTimeRange returns count of commands processed in a time range
func (q *CommandQueries) GetCommandsInTimeRange(hours int) (int, error) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM agent_commands
		WHERE created_at >= $1 AND status IN ('completed', 'failed', 'timed_out')
	`
	err := q.db.Get(&count, query, time.Now().UTC().Add(-time.Duration(hours)*time.Hour))
	return count, err
}

// GetStuckCommands retrieves commands stuck in 'pending' or 'sent' status.
// Excludes expired commands and commands that have exceeded max retries (F-B2-10 fix).
// 'received' is excluded — see GetStuckCommandsTx for rationale.
func (q *CommandQueries) GetStuckCommands(agentID uuid.UUID, olderThan time.Duration, maxRetries int) ([]models.AgentCommand, error) {
	var commands []models.AgentCommand
	query := `
		SELECT * FROM agent_commands
		WHERE agent_id = $1
		AND status IN ('pending', 'sent')
		AND (expires_at IS NULL OR expires_at > NOW())
		AND retry_count < $3
		AND (
			(sent_at < $2 AND sent_at IS NOT NULL)
			OR (created_at < $2 AND sent_at IS NULL)
		)
		ORDER BY created_at ASC
	`
	err := q.db.Select(&commands, query, agentID, time.Now().UTC().Add(-olderThan), maxRetries)
	return commands, err
}

// VerifyResultsRecorded returns the subset of the given command IDs whose result
// the server has durably stored. The pending ack exists for one thing: at-least-once
// delivery of the agent's result. Every ReportLog path writes that result onto the
// command row, so a non-null result is the receipt — once it's set, the agent can
// stop resending. A command with no result yet stays unacked and the agent keeps
// retrying, which is exactly what at-least-once requires.
//
// This deliberately does not key off command lifecycle status. Doing so stranded
// acks whenever a command settled into a status the agent's result didn't drive
// (progress reports, cancelled/archived rows) — they recycled every poll until the
// agent's 24h max-age finally dropped them.
func (q *CommandQueries) VerifyResultsRecorded(commandIDs []string) ([]string, error) {
	if len(commandIDs) == 0 {
		return []string{}, nil
	}

	// Convert string IDs to UUIDs
	uuidIDs := make([]uuid.UUID, 0, len(commandIDs))
	for _, idStr := range commandIDs {
		id, err := uuid.FromString(idStr)
		if err != nil {
			// Skip invalid UUIDs
			continue
		}
		uuidIDs = append(uuidIDs, id)
	}

	if len(uuidIDs) == 0 {
		return []string{}, nil
	}

	// Convert UUIDs back to strings for SQL query
	uuidStrs := make([]string, len(uuidIDs))
	for i, id := range uuidIDs {
		uuidStrs[i] = id.String()
	}

	// Query for commands that are completed or failed
	// Use ANY with proper array literal for PostgreSQL
	placeholders := make([]string, len(uuidStrs))
	args := make([]interface{}, len(uuidStrs))
	for i, id := range uuidStrs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id
		FROM agent_commands
		WHERE id::text = ANY(%s)
		AND result IS NOT NULL
	`, fmt.Sprintf("ARRAY[%s]", strings.Join(placeholders, ",")))

	var completedUUIDs []uuid.UUID
	err := q.db.Select(&completedUUIDs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to verify command completion: %w", err)
	}

	// Convert back to strings
	completedIDs := make([]string, len(completedUUIDs))
	for i, id := range completedUUIDs {
		completedIDs[i] = id.String()
	}

	return completedIDs, nil
}

// HasPendingUpdateCommand checks if an agent has a pending update_agent command
// This is used to allow old agents to check in and receive updates even if they're below minimum version
func (q *CommandQueries) HasPendingUpdateCommand(agentID string) (bool, error) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM agent_commands
		WHERE agent_id = $1
		AND command_type = 'update_agent'
		AND status = 'pending'
	`

	agentUUID, err := uuid.FromString(agentID)
	if err != nil {
		return false, fmt.Errorf("invalid agent ID: %w", err)
	}

	err = q.db.Get(&count, query, agentUUID)
	if err != nil {
		return false, fmt.Errorf("failed to check for pending update commands: %w", err)
	}

	return count > 0, nil
}
