package queries

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// AgentUpdateQueries handles database operations for agent update packages
type AgentUpdateQueries struct {
	db *sqlx.DB
}

// NewAgentUpdateQueries creates a new AgentUpdateQueries instance
func NewAgentUpdateQueries(db *sqlx.DB) *AgentUpdateQueries {
	return &AgentUpdateQueries{db: db}
}

// CreateUpdatePackage stores a signed update package. Upserts on
// (version, platform, architecture): a fresh build of the same version
// (different checksum / signature) replaces the row in place. Combined with
// the unique constraint added in migration 037, this guarantees one row per
// real artifact even across repeated dev builds.
func (q *AgentUpdateQueries) CreateUpdatePackage(pkg *models.AgentUpdatePackage) error {
	query := `
		INSERT INTO agent_update_packages (
			id, version, platform, architecture, binary_path, signature,
			checksum, file_size, created_by, is_active
		) VALUES (
			:id, :version, :platform, :architecture, :binary_path, :signature,
			:checksum, :file_size, :created_by, :is_active
		)
		ON CONFLICT (version, platform, architecture) DO UPDATE SET
			binary_path = EXCLUDED.binary_path,
			signature   = EXCLUDED.signature,
			checksum    = EXCLUDED.checksum,
			file_size   = EXCLUDED.file_size,
			created_by  = EXCLUDED.created_by,
			is_active   = EXCLUDED.is_active,
			created_at  = NOW()
		RETURNING id, created_at
	`

	rows, err := q.db.NamedQuery(query, pkg)
	if err != nil {
		return fmt.Errorf("failed to create update package: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&pkg.ID, &pkg.CreatedAt); err != nil {
			return fmt.Errorf("failed to scan created package: %w", err)
		}
	}

	return nil
}

// GetUpdatePackage retrieves an update package by ID
func (q *AgentUpdateQueries) GetUpdatePackage(id uuid.UUID) (*models.AgentUpdatePackage, error) {
	query := `
		SELECT id, version, platform, architecture, binary_path, signature,
		       checksum, file_size, created_at, created_by, is_active
		FROM agent_update_packages
		WHERE id = $1 AND is_active = true
	`

	var pkg models.AgentUpdatePackage
	err := q.db.Get(&pkg, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("update package not found")
		}
		return nil, fmt.Errorf("failed to get update package: %w", err)
	}

	return &pkg, nil
}

// GetUpdatePackageByVersion retrieves the latest update package for a version and platform
func (q *AgentUpdateQueries) GetUpdatePackageByVersion(version, platform, architecture string) (*models.AgentUpdatePackage, error) {
	query := `
		SELECT id, version, platform, architecture, binary_path, signature,
		       checksum, file_size, created_at, created_by, is_active
		FROM agent_update_packages
		WHERE version = $1 AND platform = $2 AND architecture = $3 AND is_active = true
		ORDER BY created_at DESC
		LIMIT 1
	`

	var pkg models.AgentUpdatePackage
	err := q.db.Get(&pkg, query, version, platform, architecture)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no update package found for version %s on %s/%s", version, platform, architecture)
		}
		return nil, fmt.Errorf("failed to get update package: %w", err)
	}

	return &pkg, nil
}

// ListUpdatePackages retrieves all update packages with optional filtering
func (q *AgentUpdateQueries) ListUpdatePackages(version, platform string, limit, offset int) ([]models.AgentUpdatePackage, error) {
	query := `
		SELECT id, version, platform, architecture, binary_path, signature,
		       checksum, file_size, created_at, created_by, is_active
		FROM agent_update_packages
		WHERE is_active = true
	`

	args := []interface{}{}
	argIndex := 1

	if version != "" {
		query += fmt.Sprintf(" AND version = $%d", argIndex)
		args = append(args, version)
		argIndex++
	}

	if platform != "" {
		query += fmt.Sprintf(" AND platform = $%d", argIndex)
		args = append(args, platform)
		argIndex++
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, limit)
		argIndex++

		if offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argIndex)
			args = append(args, offset)
		}
	}

	var packages []models.AgentUpdatePackage
	err := q.db.Select(&packages, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list update packages: %w", err)
	}

	return packages, nil
}

// DeactivateUpdatePackage marks a package as inactive
func (q *AgentUpdateQueries) DeactivateUpdatePackage(id uuid.UUID) error {
	query := `UPDATE agent_update_packages SET is_active = false WHERE id = $1`

	result, err := q.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to deactivate update package: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no update package found to deactivate")
	}

	return nil
}

// UpdateAgentMachineInfo updates the machine ID and public key fingerprint for an agent
func (q *AgentUpdateQueries) UpdateAgentMachineInfo(agentID uuid.UUID, machineID, publicKeyFingerprint string) error {
	query := `
		UPDATE agents
		SET machine_id = $1, public_key_fingerprint = $2, updated_at = $3
		WHERE id = $4
	`

	_, err := q.db.Exec(query, machineID, publicKeyFingerprint, time.Now().UTC(), agentID)
	if err != nil {
		return fmt.Errorf("failed to update agent machine info: %w", err)
	}

	return nil
}

// UpdateAgentUpdatingStatus sets the update status for an agent
func (q *AgentUpdateQueries) UpdateAgentUpdatingStatus(agentID uuid.UUID, isUpdating bool, targetVersion *string) error {
	query := `
		UPDATE agents
		SET is_updating = $1,
		    updating_to_version = $2,
		    update_initiated_at = CASE WHEN $1 = true THEN NOW() ELSE update_initiated_at END,
		    updated_at = NOW()
		WHERE id = $3
	`

	_, err := q.db.Exec(query, isUpdating, targetVersion, agentID)
	if err != nil {
		return fmt.Errorf("failed to update agent updating status: %w", err)
	}

	return nil
}

// GetAgentByMachineID retrieves an agent by its machine ID
func (q *AgentUpdateQueries) GetAgentByMachineID(machineID string) (*models.Agent, error) {
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
			return nil, fmt.Errorf("agent not found for machine ID")
		}
		return nil, fmt.Errorf("failed to get agent by machine ID: %w", err)
	}

	return &agent, nil
}

// GetLatestVersion retrieves the latest available version for a platform
func (q *AgentUpdateQueries) GetLatestVersion(platform string) (string, error) {
	query := `
		SELECT version FROM agent_update_packages
		WHERE platform = $1 AND is_active = true
		ORDER BY version DESC LIMIT 1
	`

	var latestVersion string
	err := q.db.Get(&latestVersion, query, platform)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no update packages available for platform %s", platform)
		}
		return "", fmt.Errorf("failed to get latest version: %w", err)
	}

	return latestVersion, nil
}

// GetLatestVersionByTypeAndArch retrieves the latest available version for a specific os_type and architecture
func (q *AgentUpdateQueries) GetLatestVersionByTypeAndArch(osType, osArch string) (string, error) {
	// Use combined platform format to match agent_update_packages storage
	platformStr := osType + "-" + osArch

	query := `
		SELECT version FROM agent_update_packages
		WHERE (platform || '-' || architecture) = $1 AND is_active = true
		ORDER BY version DESC LIMIT 1
	`

	var latestVersion string
	err := q.db.Get(&latestVersion, query, platformStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no update packages available for platform %s", platformStr)
		}
		return "", fmt.Errorf("failed to get latest version: %w", err)
	}

	return latestVersion, nil
}

// GetPendingUpdateCommand retrieves the most recent pending update command for an agent
func (q *AgentUpdateQueries) GetPendingUpdateCommand(agentID string) (*models.AgentCommand, error) {
	query := `
		SELECT id, agent_id, command_type, params, status, source, created_at, sent_at, completed_at, result, retried_from_id
		FROM agent_commands
		WHERE agent_id = $1 AND command_type = 'install_update' AND status = 'pending'
		ORDER BY created_at DESC
		LIMIT 1
	`

	var command models.AgentCommand
	err := q.db.Get(&command, query, agentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No pending update command found
		}
		return nil, fmt.Errorf("failed to get pending update command: %w", err)
	}

	return &command, nil
}