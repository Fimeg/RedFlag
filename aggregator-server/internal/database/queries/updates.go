package queries

import (
	"fmt"
	"strings"
	"time"

	"github.com/aggregator-project/aggregator-server/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type UpdateQueries struct {
	db *sqlx.DB
}

func NewUpdateQueries(db *sqlx.DB) *UpdateQueries {
	return &UpdateQueries{db: db}
}

// UpsertUpdate inserts or updates an update package
func (q *UpdateQueries) UpsertUpdate(update *models.UpdatePackage) error {
	query := `
		INSERT INTO update_packages (
			id, agent_id, package_type, package_name, package_description,
			current_version, available_version, severity, cve_list, kb_id,
			repository_source, size_bytes, status, metadata
		) VALUES (
			:id, :agent_id, :package_type, :package_name, :package_description,
			:current_version, :available_version, :severity, :cve_list, :kb_id,
			:repository_source, :size_bytes, :status, :metadata
		)
		ON CONFLICT (agent_id, package_type, package_name, available_version)
		DO UPDATE SET
			package_description = EXCLUDED.package_description,
			current_version = EXCLUDED.current_version,
			severity = EXCLUDED.severity,
			cve_list = EXCLUDED.cve_list,
			kb_id = EXCLUDED.kb_id,
			repository_source = EXCLUDED.repository_source,
			size_bytes = EXCLUDED.size_bytes,
			metadata = EXCLUDED.metadata,
			discovered_at = NOW()
	`
	_, err := q.db.NamedExec(query, update)
	return err
}

// ListUpdates retrieves updates with filtering (legacy method for update_packages table)
func (q *UpdateQueries) ListUpdates(filters *models.UpdateFilters) ([]models.UpdatePackage, int, error) {
	var updates []models.UpdatePackage
	whereClause := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if filters.AgentID != uuid.Nil {
		whereClause = append(whereClause, fmt.Sprintf("agent_id = $%d", argIdx))
		args = append(args, filters.AgentID)
		argIdx++
	}
	if filters.Status != "" {
		whereClause = append(whereClause, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.Severity != "" {
		whereClause = append(whereClause, fmt.Sprintf("severity = $%d", argIdx))
		args = append(args, filters.Severity)
		argIdx++
	}
	if filters.PackageType != "" {
		whereClause = append(whereClause, fmt.Sprintf("package_type = $%d", argIdx))
		args = append(args, filters.PackageType)
		argIdx++
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM update_packages WHERE " + strings.Join(whereClause, " AND ")
	var total int
	err := q.db.Get(&total, countQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	query := fmt.Sprintf(`
		SELECT * FROM update_packages
		WHERE %s
		ORDER BY discovered_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(whereClause, " AND "), argIdx, argIdx+1)

	limit := filters.PageSize
	if limit == 0 {
		limit = 50
	}
	offset := (filters.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	err = q.db.Select(&updates, query, args...)
	return updates, total, err
}

// GetUpdateByID retrieves a single update by ID from the new state table
func (q *UpdateQueries) GetUpdateByID(id uuid.UUID) (*models.UpdateState, error) {
	var update models.UpdateState
	query := `SELECT * FROM current_package_state WHERE id = $1`
	err := q.db.Get(&update, query, id)
	if err != nil {
		return nil, err
	}
	return &update, nil
}

// GetUpdateByPackage retrieves a single update by agent_id, package_type, and package_name
func (q *UpdateQueries) GetUpdateByPackage(agentID uuid.UUID, packageType, packageName string) (*models.UpdateState, error) {
	var update models.UpdateState
	query := `SELECT * FROM current_package_state WHERE agent_id = $1 AND package_type = $2 AND package_name = $3`
	err := q.db.Get(&update, query, agentID, packageType, packageName)
	if err != nil {
		return nil, err
	}
	return &update, nil
}

// ApproveUpdate marks an update as approved in the new event sourcing system
func (q *UpdateQueries) ApproveUpdate(id uuid.UUID, approvedBy string) error {
	query := `
		UPDATE current_package_state
		SET status = 'approved', last_updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`
	_, err := q.db.Exec(query, id)
	return err
}

// ApproveUpdateByPackage approves an update by agent_id, package_type, and package_name
func (q *UpdateQueries) ApproveUpdateByPackage(agentID uuid.UUID, packageType, packageName, approvedBy string) error {
	query := `
		UPDATE current_package_state
		SET status = 'approved', last_updated_at = NOW()
		WHERE agent_id = $1 AND package_type = $2 AND package_name = $3 AND status = 'pending'
	`
	_, err := q.db.Exec(query, agentID, packageType, packageName)
	return err
}

// BulkApproveUpdates approves multiple updates by their IDs
func (q *UpdateQueries) BulkApproveUpdates(updateIDs []uuid.UUID, approvedBy string) error {
	if len(updateIDs) == 0 {
		return nil
	}

	// Start transaction
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update each update
	for _, id := range updateIDs {
		query := `
			UPDATE current_package_state
			SET status = 'approved', last_updated_at = NOW()
			WHERE id = $1 AND status = 'pending'
		`
		_, err := tx.Exec(query, id)
		if err != nil {
			return fmt.Errorf("failed to approve update %s: %w", id, err)
		}
	}

	return tx.Commit()
}

// RejectUpdate marks an update as rejected/ignored
func (q *UpdateQueries) RejectUpdate(id uuid.UUID, rejectedBy string) error {
	query := `
		UPDATE current_package_state
		SET status = 'ignored', last_updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'approved')
	`
	_, err := q.db.Exec(query, id)
	return err
}

// RejectUpdateByPackage rejects an update by agent_id, package_type, and package_name
func (q *UpdateQueries) RejectUpdateByPackage(agentID uuid.UUID, packageType, packageName, rejectedBy string) error {
	query := `
		UPDATE current_package_state
		SET status = 'ignored', last_updated_at = NOW()
		WHERE agent_id = $1 AND package_type = $2 AND package_name = $3 AND status IN ('pending', 'approved')
	`
	_, err := q.db.Exec(query, agentID, packageType, packageName)
	return err
}

// InstallUpdate marks an update as ready for installation
func (q *UpdateQueries) InstallUpdate(id uuid.UUID) error {
	query := `
		UPDATE current_package_state
		SET status = 'installing', last_updated_at = NOW()
		WHERE id = $1 AND status = 'approved'
	`
	_, err := q.db.Exec(query, id)
	return err
}

// CreateUpdateLog inserts an update log entry
func (q *UpdateQueries) CreateUpdateLog(log *models.UpdateLog) error {
	query := `
		INSERT INTO update_logs (
			id, agent_id, update_package_id, action, result,
			stdout, stderr, exit_code, duration_seconds
		) VALUES (
			:id, :agent_id, :update_package_id, :action, :result,
			:stdout, :stderr, :exit_code, :duration_seconds
		)
	`
	_, err := q.db.NamedExec(query, log)
	return err
}

// NEW EVENT SOURCING IMPLEMENTATION

// CreateUpdateEvent stores a single update event
func (q *UpdateQueries) CreateUpdateEvent(event *models.UpdateEvent) error {
	query := `
		INSERT INTO update_events (
			agent_id, package_type, package_name, version_from, version_to,
			severity, repository_source, metadata, event_type
		) VALUES (
			:agent_id, :package_type, :package_name, :version_from, :version_to,
			:severity, :repository_source, :metadata, :event_type
		)
	`
	_, err := q.db.NamedExec(query, event)
	return err
}

// CreateUpdateEventsBatch creates multiple update events in a transaction
func (q *UpdateQueries) CreateUpdateEventsBatch(events []models.UpdateEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Start transaction
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create batch record
	batch := &models.UpdateBatch{
		ID:       uuid.New(),
		AgentID:  events[0].AgentID,
		BatchSize: len(events),
		Status:   "processing",
	}

	batchQuery := `
		INSERT INTO update_batches (id, agent_id, batch_size, status)
		VALUES (:id, :agent_id, :batch_size, :status)
	`
	if _, err := tx.NamedExec(batchQuery, batch); err != nil {
		return fmt.Errorf("failed to create batch record: %w", err)
	}

	// Insert events in batches to avoid memory issues
	batchSize := 100
	processedCount := 0
	failedCount := 0

	for i := 0; i < len(events); i += batchSize {
		end := i + batchSize
		if end > len(events) {
			end = len(events)
		}

		currentBatch := events[i:end]

		// Prepare query with multiple value sets
		query := `
			INSERT INTO update_events (
				agent_id, package_type, package_name, version_from, version_to,
				severity, repository_source, metadata, event_type
			) VALUES (
				:agent_id, :package_type, :package_name, :version_from, :version_to,
				:severity, :repository_source, :metadata, :event_type
			)
		`

		for _, event := range currentBatch {
			_, err := tx.NamedExec(query, event)
			if err != nil {
				failedCount++
				continue
			}
			processedCount++

			// Update current state
			if err := q.updateCurrentStateInTx(tx, &event); err != nil {
				// Log error but don't fail the entire batch
				fmt.Printf("Warning: failed to update current state for %s: %v\n", event.PackageName, err)
			}
		}
	}

	// Update batch record
	batchUpdateQuery := `
		UPDATE update_batches
		SET processed_count = $1, failed_count = $2, status = $3, completed_at = $4
		WHERE id = $5
	`
	batchStatus := "completed"
	if failedCount > 0 {
		batchStatus = "completed_with_errors"
	}

	_, err = tx.Exec(batchUpdateQuery, processedCount, failedCount, batchStatus, time.Now(), batch.ID)
	if err != nil {
		return fmt.Errorf("failed to update batch record: %w", err)
	}

	// Commit transaction
	return tx.Commit()
}

// updateCurrentStateInTx updates the current_package_state table within a transaction
func (q *UpdateQueries) updateCurrentStateInTx(tx *sqlx.Tx, event *models.UpdateEvent) error {
	query := `
		INSERT INTO current_package_state (
			agent_id, package_type, package_name, current_version, available_version,
			severity, repository_source, metadata, last_discovered_at, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
		ON CONFLICT (agent_id, package_type, package_name)
		DO UPDATE SET
			available_version = EXCLUDED.available_version,
			severity = EXCLUDED.severity,
			repository_source = EXCLUDED.repository_source,
			metadata = EXCLUDED.metadata,
			last_discovered_at = EXCLUDED.last_discovered_at,
			status = CASE
				WHEN current_package_state.status IN ('updated', 'ignored')
				THEN current_package_state.status
				ELSE 'pending'
			END
	`
	_, err := tx.Exec(query,
		event.AgentID,
		event.PackageType,
		event.PackageName,
		event.VersionFrom,
		event.VersionTo,
		event.Severity,
		event.RepositorySource,
		event.Metadata,
		event.CreatedAt)
	return err
}

// ListUpdatesFromState returns paginated updates from current state with filtering
func (q *UpdateQueries) ListUpdatesFromState(filters *models.UpdateFilters) ([]models.UpdateState, int, error) {
	var updates []models.UpdateState
	var count int

	// Build base query
	baseQuery := `
		SELECT
			id, agent_id, package_type, package_name, current_version,
			available_version, severity, repository_source, metadata,
			last_discovered_at, last_updated_at, status
		FROM current_package_state
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM current_package_state WHERE 1=1`

	args := []interface{}{}
	argIdx := 1

	// Add filters
	if filters.AgentID != uuid.Nil {
		baseQuery += fmt.Sprintf(" AND agent_id = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND agent_id = $%d", argIdx)
		args = append(args, filters.AgentID)
		argIdx++
	}

	if filters.PackageType != "" {
		baseQuery += fmt.Sprintf(" AND package_type = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND package_type = $%d", argIdx)
		args = append(args, filters.PackageType)
		argIdx++
	}

	if filters.Severity != "" {
		baseQuery += fmt.Sprintf(" AND severity = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, filters.Severity)
		argIdx++
	}

	if filters.Status != "" {
		baseQuery += fmt.Sprintf(" AND status = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, filters.Status)
		argIdx++
	}

	// Get total count
	err := q.db.Get(&count, countQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get updates count: %w", err)
	}

	// Add ordering and pagination
	baseQuery += " ORDER BY last_discovered_at DESC"
	baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, filters.PageSize, (filters.Page-1)*filters.PageSize)

	// Execute query
	err = q.db.Select(&updates, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list updates: %w", err)
	}

	return updates, count, nil
}

// GetPackageHistory returns version history for a specific package
func (q *UpdateQueries) GetPackageHistory(agentID uuid.UUID, packageType, packageName string, limit int) ([]models.UpdateHistory, error) {
	var history []models.UpdateHistory

	query := `
		SELECT
			id, agent_id, package_type, package_name, version_from, version_to,
			severity, repository_source, metadata, update_initiated_at,
			update_completed_at, update_status, failure_reason
		FROM update_version_history
		WHERE agent_id = $1 AND package_type = $2 AND package_name = $3
		ORDER BY update_completed_at DESC
		LIMIT $4
	`

	err := q.db.Select(&history, query, agentID, packageType, packageName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get package history: %w", err)
	}

	return history, nil
}

// UpdatePackageStatus updates the status of a package and records history
func (q *UpdateQueries) UpdatePackageStatus(agentID uuid.UUID, packageType, packageName, status string, metadata map[string]interface{}) error {
	tx, err := q.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current state
	var currentState models.UpdateState
	query := `SELECT * FROM current_package_state WHERE agent_id = $1 AND package_type = $2 AND package_name = $3`
	err = tx.Get(&currentState, query, agentID, packageType, packageName)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	// Update status
	updateQuery := `
		UPDATE current_package_state
		SET status = $1, last_updated_at = $2
		WHERE agent_id = $3 AND package_type = $4 AND package_name = $5
	`
	_, err = tx.Exec(updateQuery, status, time.Now(), agentID, packageType, packageName)
	if err != nil {
		return fmt.Errorf("failed to update package status: %w", err)
	}

	// Record in history if this is an update completion
	if status == "updated" || status == "failed" {
		historyQuery := `
			INSERT INTO update_version_history (
				agent_id, package_type, package_name, version_from, version_to,
				severity, repository_source, metadata, update_completed_at, update_status
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`
		_, err = tx.Exec(historyQuery,
			agentID, packageType, packageName, currentState.CurrentVersion,
			currentState.AvailableVersion, currentState.Severity,
			currentState.RepositorySource, metadata, time.Now(), status)
		if err != nil {
			return fmt.Errorf("failed to record version history: %w", err)
		}
	}

	return tx.Commit()
}

// CleanupOldEvents removes old events to prevent table bloat
func (q *UpdateQueries) CleanupOldEvents(olderThan time.Duration) error {
	query := `DELETE FROM update_events WHERE created_at < $1`
	result, err := q.db.Exec(query, time.Now().Add(-olderThan))
	if err != nil {
		return fmt.Errorf("failed to cleanup old events: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("Cleaned up %d old update events\n", rowsAffected)
	return nil
}

// GetBatchStatus returns the status of recent batches
func (q *UpdateQueries) GetBatchStatus(agentID uuid.UUID, limit int) ([]models.UpdateBatch, error) {
	var batches []models.UpdateBatch

	query := `
		SELECT id, agent_id, batch_size, processed_count, failed_count,
			   status, error_details, created_at, completed_at
		FROM update_batches
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	err := q.db.Select(&batches, query, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch status: %w", err)
	}

	return batches, nil
}

// GetUpdateStatsFromState returns statistics about updates from current state
func (q *UpdateQueries) GetUpdateStatsFromState(agentID uuid.UUID) (*models.UpdateStats, error) {
	stats := &models.UpdateStats{}

	query := `
		SELECT
			COUNT(*) as total_updates,
			COUNT(*) FILTER (WHERE status = 'pending') as pending_updates,
			COUNT(*) FILTER (WHERE status = 'updated') as updated_updates,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_updates,
			COUNT(*) FILTER (WHERE severity = 'critical') as critical_updates,
			COUNT(*) FILTER (WHERE severity = 'important') as important_updates,
			COUNT(*) FILTER (WHERE severity = 'moderate') as moderate_updates,
			COUNT(*) FILTER (WHERE severity = 'low') as low_updates
		FROM current_package_state
		WHERE agent_id = $1
	`

	err := q.db.Get(stats, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get update stats: %w", err)
	}

	return stats, nil
}

// GetAllUpdateStats returns overall statistics about updates across all agents
func (q *UpdateQueries) GetAllUpdateStats() (*models.UpdateStats, error) {
	stats := &models.UpdateStats{}

	query := `
		SELECT
			COUNT(*) as total_updates,
			COUNT(*) FILTER (WHERE status = 'pending') as pending_updates,
			COUNT(*) FILTER (WHERE status = 'approved') as approved_updates,
			COUNT(*) FILTER (WHERE status = 'updated') as updated_updates,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_updates,
			COUNT(*) FILTER (WHERE severity = 'critical') as critical_updates,
			COUNT(*) FILTER (WHERE severity = 'important') as high_updates,
			COUNT(*) FILTER (WHERE severity = 'moderate') as moderate_updates,
			COUNT(*) FILTER (WHERE severity = 'low') as low_updates
		FROM current_package_state
	`

	err := q.db.Get(stats, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all update stats: %w", err)
	}

	return stats, nil
}
