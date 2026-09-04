package queries

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// SubsystemQueriesInterface defines the interface for subsystem operations (enables transaction support)
type SubsystemQueriesInterface interface {
	GetSubsystems(agentID uuid.UUID) ([]models.AgentSubsystem, error)
	GetSubsystem(agentID uuid.UUID, subsystem string) (*models.AgentSubsystem, error)
	CreateSubsystem(sub *models.AgentSubsystem) error
	CreateSubsystemWithTx(tx *sqlx.Tx, sub *models.AgentSubsystem) error
	CreateDefaultSubsystems(agentID uuid.UUID, availableScanners []string) error
	CreateDefaultSubsystemsWithTx(tx *sqlx.Tx, agentID uuid.UUID, availableScanners []string) error
}

type SubsystemQueries struct {
	db *sqlx.DB
}

func NewSubsystemQueries(db *sqlx.DB) *SubsystemQueries {
	return &SubsystemQueries{db: db}
}

// GetSubsystems retrieves all subsystems for an agent
func (q *SubsystemQueries) GetSubsystems(agentID uuid.UUID) ([]models.AgentSubsystem, error) {
	query := `
		SELECT id, agent_id, subsystem, enabled, interval_minutes, auto_run,
		       last_run_at, next_run_at, created_at, updated_at
		FROM agent_subsystems
		WHERE agent_id = $1
		ORDER BY subsystem
	`

	var subsystems []models.AgentSubsystem
	err := q.db.Select(&subsystems, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subsystems: %w", err)
	}

	return subsystems, nil
}

// GetSubsystemsForAgents retrieves all subsystems for the given set of agent IDs
// in a single query. Use this instead of calling GetSubsystems once per agent to
// avoid an N+1 at startup (SCALE-001 S10).
func (q *SubsystemQueries) GetSubsystemsForAgents(agentIDs []uuid.UUID) ([]models.AgentSubsystem, error) {
	if len(agentIDs) == 0 {
		return nil, nil
	}

	query := `
		SELECT id, agent_id, subsystem, enabled, interval_minutes, auto_run,
		       last_run_at, next_run_at, created_at, updated_at
		FROM agent_subsystems
		WHERE agent_id = ANY($1)
		ORDER BY agent_id, subsystem
	`

	var subsystems []models.AgentSubsystem
	err := q.db.Select(&subsystems, query, pq.Array(agentIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to get subsystems for agents: %w", err)
	}

	return subsystems, nil
}

// GetSubsystem retrieves a specific subsystem for an agent
func (q *SubsystemQueries) GetSubsystem(agentID uuid.UUID, subsystem string) (*models.AgentSubsystem, error) {
	query := `
		SELECT id, agent_id, subsystem, enabled, interval_minutes, auto_run,
		       last_run_at, next_run_at, created_at, updated_at
		FROM agent_subsystems
		WHERE agent_id = $1 AND subsystem = $2
	`

	var sub models.AgentSubsystem
	err := q.db.Get(&sub, query, agentID, subsystem)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subsystem: %w", err)
	}

	return &sub, nil
}

// UpdateSubsystem updates a subsystem configuration
func (q *SubsystemQueries) UpdateSubsystem(agentID uuid.UUID, subsystem string, config models.SubsystemConfig) error {
	// Build dynamic update query based on provided fields
	updates := []string{}
	args := []interface{}{agentID, subsystem}
	argIdx := 3

	if config.Enabled != nil {
		updates = append(updates, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *config.Enabled)
		argIdx++
	}

	if config.IntervalMinutes != nil {
		updates = append(updates, fmt.Sprintf("interval_minutes = $%d", argIdx))
		args = append(args, *config.IntervalMinutes)
		argIdx++
	}

	if config.AutoRun != nil {
		updates = append(updates, fmt.Sprintf("auto_run = $%d", argIdx))
		args = append(args, *config.AutoRun)
		argIdx++

		// If enabling auto_run, calculate next_run_at
		if *config.AutoRun {
			updates = append(updates, fmt.Sprintf("next_run_at = NOW() + INTERVAL '%d minutes'", argIdx))
		}
	}

	if len(updates) == 0 {
		return fmt.Errorf("no fields to update")
	}

	updates = append(updates, "updated_at = NOW()")

	query := fmt.Sprintf(`
		UPDATE agent_subsystems
		SET %s
		WHERE agent_id = $1 AND subsystem = $2
	`, joinUpdates(updates))

	result, err := q.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update subsystem: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("subsystem not found")
	}

	return nil
}

// UpdateLastRun updates the last_run_at timestamp for a subsystem
func (q *SubsystemQueries) UpdateLastRun(agentID uuid.UUID, subsystem string) error {
	query := `
		UPDATE agent_subsystems
		SET last_run_at = NOW(),
		    next_run_at = CASE
		        WHEN auto_run THEN NOW() + (interval_minutes || ' minutes')::INTERVAL
		        ELSE next_run_at
		    END,
		    updated_at = NOW()
		WHERE agent_id = $1 AND subsystem = $2
	`

	result, err := q.db.Exec(query, agentID, subsystem)
	if err != nil {
		return fmt.Errorf("failed to update last run: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("subsystem not found")
	}

	return nil
}

// GetDueSubsystems retrieves all subsystems that are due to run
func (q *SubsystemQueries) GetDueSubsystems() ([]models.AgentSubsystem, error) {
	query := `
		SELECT id, agent_id, subsystem, enabled, interval_minutes, auto_run,
		       last_run_at, next_run_at, created_at, updated_at
		FROM agent_subsystems
		WHERE enabled = true
		  AND auto_run = true
		  AND (next_run_at IS NULL OR next_run_at <= NOW())
		ORDER BY next_run_at ASC NULLS FIRST
		LIMIT 1000
	`

	var subsystems []models.AgentSubsystem
	err := q.db.Select(&subsystems, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get due subsystems: %w", err)
	}

	return subsystems, nil
}

// GetSubsystemStats retrieves statistics for a subsystem
func (q *SubsystemQueries) GetSubsystemStats(agentID uuid.UUID, subsystem string) (*models.SubsystemStats, error) {
	query := `
		SELECT
		    s.subsystem,
		    s.enabled,
		    s.last_run_at,
		    s.next_run_at,
		    s.interval_minutes,
		    s.auto_run,
		    COUNT(c.id) FILTER (WHERE c.command_type = 'scan_' || s.subsystem) as run_count,
		    MAX(c.status) FILTER (WHERE c.command_type = 'scan_' || s.subsystem) as last_status,
		    MAX(al.duration_seconds) FILTER (WHERE al.action = 'scan_' || s.subsystem) as last_duration
		FROM agent_subsystems s
		LEFT JOIN agent_commands c ON c.agent_id = s.agent_id
		LEFT JOIN agent_logs al ON al.command_id = c.id
		WHERE s.agent_id = $1 AND s.subsystem = $2
		GROUP BY s.subsystem, s.enabled, s.last_run_at, s.next_run_at, s.interval_minutes, s.auto_run
	`

	var stats models.SubsystemStats
	err := q.db.Get(&stats, query, agentID, subsystem)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subsystem stats: %w", err)
	}

	return &stats, nil
}

// EnableSubsystem enables a subsystem
func (q *SubsystemQueries) EnableSubsystem(agentID uuid.UUID, subsystem string) error {
	enabled := true
	return q.UpdateSubsystem(agentID, subsystem, models.SubsystemConfig{
		Enabled: &enabled,
	})
}

// DisableSubsystem disables a subsystem
func (q *SubsystemQueries) DisableSubsystem(agentID uuid.UUID, subsystem string) error {
	enabled := false
	return q.UpdateSubsystem(agentID, subsystem, models.SubsystemConfig{
		Enabled: &enabled,
	})
}

// SetAutoRun enables or disables auto-run for a subsystem
func (q *SubsystemQueries) SetAutoRun(agentID uuid.UUID, subsystem string, autoRun bool) error {
	return q.UpdateSubsystem(agentID, subsystem, models.SubsystemConfig{
		AutoRun: &autoRun,
	})
}

// SetInterval sets the interval for a subsystem
func (q *SubsystemQueries) SetInterval(agentID uuid.UUID, subsystem string, intervalMinutes int) error {
	return q.UpdateSubsystem(agentID, subsystem, models.SubsystemConfig{
		IntervalMinutes: &intervalMinutes,
	})
}

// CreateSubsystem creates a new subsystem configuration (used for custom subsystems)
func (q *SubsystemQueries) CreateSubsystem(sub *models.AgentSubsystem) error {
	return q.createSubsystemInternal(q.db, sub)
}

// CreateSubsystemWithTx creates a subsystem within a transaction
func (q *SubsystemQueries) CreateSubsystemWithTx(tx *sqlx.Tx, sub *models.AgentSubsystem) error {
	return q.createSubsystemInternal(tx, sub)
}

// createSubsystemInternal is the internal implementation that works with both DB and transactions
func (q *SubsystemQueries) createSubsystemInternal(exec interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}, sub *models.AgentSubsystem) error {
	query := `
		INSERT INTO agent_subsystems (agent_id, subsystem, enabled, interval_minutes, auto_run, last_run_at, next_run_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (agent_id, subsystem) DO NOTHING
		RETURNING id, created_at, updated_at
	`

	err := exec.QueryRow(
		query,
		sub.AgentID,
		sub.Subsystem,
		sub.Enabled,
		sub.IntervalMinutes,
		sub.AutoRun,
		sub.LastRunAt,
		sub.NextRunAt,
	).Scan(&sub.ID, &sub.CreatedAt, &sub.UpdatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("failed to create subsystem: %w", err)
	}

	return nil
}

// DeleteSubsystem deletes a subsystem configuration
func (q *SubsystemQueries) DeleteSubsystem(agentID uuid.UUID, subsystem string) error {
	query := `
		DELETE FROM agent_subsystems
		WHERE agent_id = $1 AND subsystem = $2
	`

	result, err := q.db.Exec(query, agentID, subsystem)
	if err != nil {
		return fmt.Errorf("failed to delete subsystem: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("subsystem not found")
	}

	return nil
}

// CreateDefaultSubsystems creates default subsystems for a new agent
// Now creates platform-specific subsystems (apt, dnf, winget, windows) based on available scanners
func (q *SubsystemQueries) CreateDefaultSubsystems(agentID uuid.UUID, availableScanners []string) error {
	return q.createDefaultSubsystemsInternal(q.db, agentID, availableScanners)
}

// CreateDefaultSubsystemsWithTx creates default subsystems within a transaction
func (q *SubsystemQueries) CreateDefaultSubsystemsWithTx(tx *sqlx.Tx, agentID uuid.UUID, availableScanners []string) error {
	return q.createDefaultSubsystemsInternal(tx, agentID, availableScanners)
}

// createDefaultSubsystemsInternal is the internal implementation
func (q *SubsystemQueries) createDefaultSubsystemsInternal(exec interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}, agentID uuid.UUID, availableScanners []string) error {
	// ARC-005: Always create storage and system. Docker is no longer in the
	// defaults — it's a scanner (only meaningful when the agent reports it),
	// not a universal generic. Docker subsystem rows are now created only
	// from the explicit AvailableScanners loop below or from the per-poll
	// capability advertisement (ARC-001 syncAvailableScanners).
	defaults := []models.AgentSubsystem{
		{AgentID: agentID, Subsystem: "storage", Enabled: true, AutoRun: true, IntervalMinutes: 5},
		{AgentID: agentID, Subsystem: "system", Enabled: true, AutoRun: true, IntervalMinutes: 5},
	}

	// Create platform-specific scanner subsystems based on what the agent reports
	for _, scanner := range availableScanners {
		switch scanner {
		case "apt":
			defaults = append(defaults, models.AgentSubsystem{AgentID: agentID, Subsystem: "apt", Enabled: true, AutoRun: true, IntervalMinutes: 15})
		case "dnf":
			defaults = append(defaults, models.AgentSubsystem{AgentID: agentID, Subsystem: "dnf", Enabled: true, AutoRun: true, IntervalMinutes: 15})
		case "winget":
			defaults = append(defaults, models.AgentSubsystem{AgentID: agentID, Subsystem: "winget", Enabled: true, AutoRun: true, IntervalMinutes: 60})
		case "windows":
			defaults = append(defaults, models.AgentSubsystem{AgentID: agentID, Subsystem: "windows", Enabled: true, AutoRun: true, IntervalMinutes: 60})
		case "docker":
			defaults = append(defaults, models.AgentSubsystem{AgentID: agentID, Subsystem: "docker", Enabled: true, AutoRun: true, IntervalMinutes: 15})
		case "pacman":
			defaults = append(defaults, models.AgentSubsystem{AgentID: agentID, Subsystem: "pacman", Enabled: true, AutoRun: true, IntervalMinutes: 15})
		}
	}

	for _, sub := range defaults {
		if err := q.createSubsystemInternal(exec, &sub); err != nil {
			return fmt.Errorf("failed to create subsystem %s: %w", sub.Subsystem, err)
		}
	}
	return nil
}

// Helper function to join update statements
func joinUpdates(updates []string) string {
	result := ""
	for i, update := range updates {
		if i > 0 {
			result += ", "
		}
		result += update
	}
	return result
}
