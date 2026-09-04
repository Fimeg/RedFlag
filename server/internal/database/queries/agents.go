package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/doug-martin/goqu/v9"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

type AgentQueries struct {
	db *sqlx.DB
	DB *sqlx.DB // Public field for access by config_builder
}

func NewAgentQueries(db *sqlx.DB) *AgentQueries {
	return &AgentQueries{
		db: db,
		DB: db, // Expose for external use
	}
}

// CreateAgent inserts a new agent into the database
func (q *AgentQueries) CreateAgent(agent *models.Agent) error {
	query := `
		INSERT INTO agents (
			id, hostname, os_type, os_version, os_architecture,
			agent_version, current_version, machine_id, public_key_fingerprint,
			device_type, device_model, os_distro,
			last_seen, status, metadata
		) VALUES (
			:id, :hostname, :os_type, :os_version, :os_architecture,
			:agent_version, :current_version, :machine_id, :public_key_fingerprint,
			:device_type, :device_model, :os_distro,
			:last_seen, :status, :metadata
		)
	`
	_, err := q.db.NamedExec(query, agent)
	if err != nil {
		return fmt.Errorf("failed to create agent %s (version %s): %w", agent.Hostname, agent.CurrentVersion, err)
	}
	return nil
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

// UpdateAgent updates an agent's full record including metadata
func (q *AgentQueries) UpdateAgent(agent *models.Agent) error {
	query := `
		UPDATE agents SET
			hostname = :hostname,
			os_type = :os_type,
			os_version = :os_version,
			os_architecture = :os_architecture,
			agent_version = :agent_version,
			device_type = :device_type,
			device_model = :device_model,
			os_distro = :os_distro,
			last_seen = :last_seen,
			status = :status,
			metadata = :metadata
		WHERE id = :id
	`
	_, err := q.db.NamedExec(query, agent)
	return err
}

// UpdateAgentMetadata updates only the metadata, last_seen, and status fields
// Used for metrics updates to avoid overwriting version tracking
func (q *AgentQueries) UpdateAgentMetadata(id uuid.UUID, metadata models.JSONB, status string, lastSeen time.Time) error {
	query := `
		UPDATE agents SET
			last_seen = $1,
			status = $2,
			metadata = $3
		WHERE id = $4
	`
	_, err := q.db.Exec(query, lastSeen, status, metadata, id)
	return err
}

// ListAgents returns all agents with optional filtering
func (q *AgentQueries) ListAgents(status, osType string) ([]models.Agent, error) {
	var agents []models.Agent
	sd := PG().From("agents")

	if status != "" {
		sd = sd.Where(goqu.Ex{"status": status})
	}
	if osType != "" {
		sd = sd.Where(goqu.Ex{"os_type": osType})
	}

	sql, args, err := sd.Select("*").Order(goqu.C("last_seen").Desc()).ToSQL()
	if err != nil {
		return nil, err
	}
	err = q.db.Select(&agents, sql, args...)
	return agents, err
}

// UpdateMachineID updates the machine_id for an agent (F-D1-2 admin rebind)
func (q *AgentQueries) UpdateMachineID(agentID uuid.UUID, newMachineID string) error {
	query := `UPDATE agents SET machine_id = $1 WHERE id = $2`
	_, err := q.db.Exec(query, newMachineID, agentID)
	return err
}

// UpdateDeviceTypeManual sets or clears the operator's device-type override
// (SERVER-002). nil clears the override, reverting to the agent-detected type.
func (q *AgentQueries) UpdateDeviceTypeManual(agentID uuid.UUID, deviceType *string) error {
	query := `UPDATE agents SET device_type_manual = $1 WHERE id = $2`
	_, err := q.db.Exec(query, deviceType, agentID)
	return err
}

// MarkOfflineAgents marks agents as offline if they haven't checked in recently
func (q *AgentQueries) MarkOfflineAgents(threshold time.Duration) error {
	query := `
		UPDATE agents
		SET status = 'offline'
		WHERE last_seen < $1 AND status = 'online'
	`
	_, err := q.db.Exec(query, time.Now().UTC().Add(-threshold))
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

// UpdateAgentVersion updates the agent's version information and checks for updates
func (q *AgentQueries) UpdateAgentVersion(id uuid.UUID, currentVersion string) error {
	query := `
		UPDATE agents SET
			current_version = $1,
			last_version_check = $2
		WHERE id = $3
	`
	_, err := q.db.Exec(query, currentVersion, time.Now().UTC(), id)
	return err
}

// UpdateAgentUpdateAvailable sets whether an update is available for an agent
func (q *AgentQueries) UpdateAgentUpdateAvailable(id uuid.UUID, updateAvailable bool) error {
	query := `
		UPDATE agents SET
			update_available = $1
		WHERE id = $2
	`
	_, err := q.db.Exec(query, updateAvailable, id)
	return err
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

// GetActiveAgentCount returns the count of active (online) agents
func (q *AgentQueries) GetActiveAgentCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM agents WHERE status = 'online'`
	err := q.db.Get(&count, query)
	return count, err
}

// GetTotalAgentCount returns the total count of registered agents
func (q *AgentQueries) GetTotalAgentCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM agents`
	err := q.db.Get(&count, query)
	return count, err
}

// GetAgentCountByVersion returns the count of agents by version (for version compliance)
func (q *AgentQueries) GetAgentCountByVersion(minVersion string) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM agents WHERE current_version >= $1`
	err := q.db.Get(&count, query, minVersion)
	return count, err
}

// GetAgentsWithMachineBinding returns count of agents that have machine IDs set
func (q *AgentQueries) GetAgentsWithMachineBinding() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM agents WHERE machine_id IS NOT NULL AND machine_id != ''`
	err := q.db.Get(&count, query)
	return count, err
}

// UpdateAgentRebootStatus updates the reboot status for an agent
func (q *AgentQueries) UpdateAgentRebootStatus(id uuid.UUID, required bool, reason string) error {
	query := `
		UPDATE agents 
		SET reboot_required = $1, 
		    reboot_reason = $2,
		    updated_at = $3
		WHERE id = $4
	`
	_, err := q.db.Exec(query, required, reason, time.Now().UTC(), id)
	return err
}

// UpdateAgentLastReboot updates the last reboot timestamp for an agent
func (q *AgentQueries) UpdateAgentLastReboot(id uuid.UUID, rebootTime time.Time) error {
	query := `
		UPDATE agents 
		SET last_reboot_at = $1,
		    reboot_required = FALSE,
		    reboot_reason = '',
		    updated_at = $2
		WHERE id = $3
	`
	_, err := q.db.Exec(query, rebootTime, time.Now().UTC(), id)
	return err
}

// GetAgentByMachineID retrieves an agent by its machine ID
func (q *AgentQueries) GetAgentByMachineID(machineID string) (*models.Agent, error) {
	query := `
		SELECT id, hostname, os_type, os_version, os_architecture, agent_version,
		       current_version, update_available, last_version_check, machine_id,
		       public_key_fingerprint, is_updating, updating_to_version,
		       update_initiated_at, last_seen, status, metadata, reboot_required,
		       last_reboot_at, reboot_reason, created_at, updated_at
		FROM agents
		WHERE machine_id = $1
	`

	var agent models.Agent
	err := q.db.Get(&agent, query, machineID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Return nil if not found (not an error)
		}
		return nil, fmt.Errorf("failed to get agent by machine ID: %w", err)
	}

	return &agent, nil
}

// UpdateAgentUpdatingStatus updates the agent's update status
func (q *AgentQueries) UpdateAgentUpdatingStatus(id uuid.UUID, isUpdating bool, updatingToVersion *string) error {
	query := `
		UPDATE agents
		SET
			is_updating = $1,
			updating_to_version = $2,
			update_initiated_at = CASE
				WHEN $1 = true THEN NOW()
				ELSE NULL
			END,
			updated_at = NOW()
		WHERE id = $3
	`

	var versionPtr *string
	if updatingToVersion != nil {
		versionPtr = updatingToVersion
	}

	_, err := q.db.Exec(query, isUpdating, versionPtr, id)
	return err
}

// GetAgentsStuckUpdating returns agents where is_updating has been true longer than the
// threshold. Used by TimeoutService to reconcile updates whose completion attestation
// never arrived. Returns the candidates; the caller decides success vs timeout based on
// whether current_version matches updating_to_version.
//
// Doctrine: TODO-full-command-lifecycle.md §4 (Timeout State Machine for update lifecycle).
func (q *AgentQueries) GetAgentsStuckUpdating(threshold time.Time) ([]models.Agent, error) {
	var agents []models.Agent
	query := `
		SELECT id, hostname, os_type, os_version, os_architecture, agent_version,
		       current_version, update_available, last_version_check, machine_id,
		       public_key_fingerprint, is_updating, updating_to_version,
		       update_initiated_at, last_seen, status, metadata, reboot_required,
		       last_reboot_at, reboot_reason, created_at, updated_at
		FROM agents
		WHERE is_updating = true
		AND update_initiated_at IS NOT NULL
		AND update_initiated_at < $1
	`
	err := q.db.Select(&agents, query, threshold)
	return agents, err
}

// ClearAgentUpdating flips is_updating=false WITHOUT changing current_version. Used
// by TimeoutService when an update times out without a version-attestation match;
// the operator can re-trigger from the dashboard.
func (q *AgentQueries) ClearAgentUpdating(agentID uuid.UUID) error {
	query := `
		UPDATE agents
		SET is_updating = false,
		    updating_to_version = NULL,
		    update_initiated_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`
	_, err := q.db.Exec(query, agentID)
	return err
}

// CompleteAgentUpdate marks an agent update as successful and updates version
func (q *AgentQueries) CompleteAgentUpdate(agentID string, newVersion string) error {
	query := `
		UPDATE agents
		SET
			current_version = $2,
			is_updating = false,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := q.db.ExecContext(ctx, query, agentID, newVersion)
	if err != nil {
		return fmt.Errorf("failed to complete agent update: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return fmt.Errorf("agent not found or version not updated")
	}

	return nil
}

// CreateSystemEvent creates a new system event entry in the system_events table
func (q *AgentQueries) CreateSystemEvent(event *models.SystemEvent) error {
	query := `
		INSERT INTO system_events (
			id, agent_id, event_type, event_subtype, severity, component, message, metadata, created_at
		) VALUES (
			:id, :agent_id, :event_type, :event_subtype, :severity, :component, :message, :metadata, :created_at
		)
	`
	_, err := q.db.NamedExec(query, event)
	if err != nil {
		return fmt.Errorf("failed to create system event: %w", err)
	}
	return nil
}

// CreateSecurityEvent inserts a security event into the security_events table.
func (q *AgentQueries) CreateSecurityEvent(event *models.SecurityEvent) error {
	detailsJSON, _ := json.Marshal(event.Details)
	metadataJSON, _ := json.Marshal(event.Metadata)

	query := `
		INSERT INTO security_events (timestamp, level, event_type, agent_id, message, trace_id, ip_address, details, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := q.db.Exec(query,
		event.Timestamp,
		event.Level,
		event.EventType,
		event.AgentID,
		event.Message,
		event.TraceID,
		event.IPAddress,
		detailsJSON,
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to create security event: %w", err)
	}
	return nil
}

// GetAgentEvents retrieves system events for an agent with optional severity filtering
func (q *AgentQueries) GetAgentEvents(agentID uuid.UUID, severity string, limit int) ([]models.SystemEvent, error) {
	query := `
		SELECT id, agent_id, event_type, event_subtype, severity, component, 
		       message, metadata, created_at
		FROM system_events
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	args := []interface{}{agentID, limit}

	if severity != "" {
		query = `
			SELECT id, agent_id, event_type, event_subtype, severity, component, 
			       message, metadata, created_at
			FROM system_events
			WHERE agent_id = $1 AND severity = ANY(string_to_array($2, ','))
			ORDER BY created_at DESC
			LIMIT $3
		`
		args = []interface{}{agentID, severity, limit}
	}

	var events []models.SystemEvent
	err := q.db.Select(&events, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent events: %w", err)
	}

	return events, nil
}

// GetGlobalEvents retrieves recent system events across all agents.
// Used by the notification bell to surface fleet-wide activity.
// Excludes client_error events — those are frontend diagnostics, not operator alerts.
func (q *AgentQueries) GetGlobalEvents(severity string, limit int) ([]models.SystemEvent, error) {
	query := `
		SELECT id, agent_id, event_type, event_subtype, severity, component,
		       message, metadata, created_at
		FROM system_events
		WHERE event_type != 'client_error'
		ORDER BY created_at DESC
		LIMIT $1
	`
	args := []interface{}{limit}

	if severity != "" {
		query = `
			SELECT id, agent_id, event_type, event_subtype, severity, component,
			       message, metadata, created_at
			FROM system_events
			WHERE event_type != 'client_error'
			  AND severity = ANY(string_to_array($1, ','))
			ORDER BY created_at DESC
			LIMIT $2
		`
		args = []interface{}{severity, limit}
	}

	var events []models.SystemEvent
	err := q.db.Select(&events, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch global events: %w", err)
	}

	return events, nil
}

// SetAgentUpdating marks an agent as updating with nonce
func (q *AgentQueries) SetAgentUpdating(agentID string, isUpdating bool, targetVersion string) error {
	query := `
		UPDATE agents
		SET is_updating = $2, updating_to_version = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := q.db.Exec(query, agentID, isUpdating, targetVersion)
	if err != nil {
		return fmt.Errorf("failed to set agent updating state: %w", err)
	}

	return nil
}

// HasPendingUpdateCommand checks if an agent has a pending update_agent command
// This is used to allow old agents to check in and receive updates even if they're below minimum version
func (q *AgentQueries) HasPendingUpdateCommand(agentID string) (bool, error) {
	// Check if agent_id is a valid UUID
	agentUUID, err := uuid.FromString(agentID)
	if err != nil {
		return false, fmt.Errorf("invalid agent ID: %w", err)
	}

	var count int
	query := `
		SELECT COUNT(*)
		FROM agent_commands
		WHERE agent_id = $1
		AND command_type = 'update_agent'
		AND status = 'pending'
	`

	err = q.db.Get(&count, query, agentUUID)
	if err != nil {
		return false, fmt.Errorf("failed to check for pending update commands: %w", err)
	}

	return count > 0, nil
}
