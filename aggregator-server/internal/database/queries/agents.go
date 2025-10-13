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
	_, err := q.db.Exec(query, time.Now(), id)
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
