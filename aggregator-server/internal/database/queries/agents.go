package queries

import (
	"time"

	"github.com/aggregator-project/aggregator-server/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type AgentQueries struct {
	db *sqlx.DB
}

func NewAgentQueries(db *sqlx.DB) *AgentQueries {
	return &AgentQueries{db: db}
}

// CreateAgent inserts a new agent into the database
func (q *AgentQueries) CreateAgent(agent *models.Agent) error {
	query := `
		INSERT INTO agents (
			id, hostname, os_type, os_version, os_architecture,
			agent_version, last_seen, status, metadata
		) VALUES (
			:id, :hostname, :os_type, :os_version, :os_architecture,
			:agent_version, :last_seen, :status, :metadata
		)
	`
	_, err := q.db.NamedExec(query, agent)
	return err
}

// GetAgentByID retrieves an agent by ID
func (q *AgentQueries) GetAgentByID(id uuid.UUID) (*models.Agent, error) {
	var agent models.Agent
	query := `SELECT * FROM agents WHERE id = $1`
	err := q.db.Get(&agent, query, id)
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

// UpdateAgentLastSeen updates the agent's last_seen timestamp
func (q *AgentQueries) UpdateAgentLastSeen(id uuid.UUID) error {
	query := `UPDATE agents SET last_seen = $1, status = 'online' WHERE id = $2`
	_, err := q.db.Exec(query, time.Now().UTC(), id)
	return err
}

// ListAgents returns all agents with optional filtering
func (q *AgentQueries) ListAgents(status, osType string) ([]models.Agent, error) {
	var agents []models.Agent
	query := `SELECT * FROM agents WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		query += ` AND status = $` + string(rune(argIdx+'0'))
		args = append(args, status)
		argIdx++
	}
	if osType != "" {
		query += ` AND os_type = $` + string(rune(argIdx+'0'))
		args = append(args, osType)
	}

	query += ` ORDER BY last_seen DESC`
	err := q.db.Select(&agents, query, args...)
	return agents, err
}

// MarkOfflineAgents marks agents as offline if they haven't checked in recently
func (q *AgentQueries) MarkOfflineAgents(threshold time.Duration) error {
	query := `
		UPDATE agents
		SET status = 'offline'
		WHERE last_seen < $1 AND status = 'online'
	`
	_, err := q.db.Exec(query, time.Now().Add(-threshold))
	return err
}

// GetAgentLastScan gets the last scan time from update events
func (q *AgentQueries) GetAgentLastScan(id uuid.UUID) (*time.Time, error) {
	var lastScan time.Time
	query := `SELECT MAX(created_at) FROM update_events WHERE agent_id = $1`
	err := q.db.Get(&lastScan, query, id)
	if err != nil {
		return nil, err
	}
	return &lastScan, nil
}

// GetAgentWithLastScan gets agent information including last scan time
func (q *AgentQueries) GetAgentWithLastScan(id uuid.UUID) (*models.AgentWithLastScan, error) {
	var agent models.AgentWithLastScan
	query := `
		SELECT
			a.*,
			(SELECT MAX(created_at) FROM update_events WHERE agent_id = a.id) as last_scan
		FROM agents a
		WHERE a.id = $1`
	err := q.db.Get(&agent, query, id)
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

// ListAgentsWithLastScan returns all agents with their last scan times
func (q *AgentQueries) ListAgentsWithLastScan(status, osType string) ([]models.AgentWithLastScan, error) {
	var agents []models.AgentWithLastScan
	query := `
		SELECT
			a.*,
			(SELECT MAX(created_at) FROM update_events WHERE agent_id = a.id) as last_scan
		FROM agents a
		WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		query += ` AND a.status = $` + string(rune(argIdx+'0'))
		args = append(args, status)
		argIdx++
	}
	if osType != "" {
		query += ` AND a.os_type = $` + string(rune(argIdx+'0'))
		args = append(args, osType)
		argIdx++
	}

	query += ` ORDER BY a.last_seen DESC`
	err := q.db.Select(&agents, query, args...)
	return agents, err
}

// DeleteAgent removes an agent and all associated data
func (q *AgentQueries) DeleteAgent(id uuid.UUID) error {
	// Start a transaction for atomic deletion
	tx, err := q.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete the agent (CASCADE will handle related records)
	_, err = tx.Exec("DELETE FROM agents WHERE id = $1", id)
	if err != nil {
		return err
	}

	// Commit the transaction
	return tx.Commit()
}
