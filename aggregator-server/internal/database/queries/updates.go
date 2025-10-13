package queries

import (
	"fmt"
	"strings"

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

// ListUpdates retrieves updates with filtering
func (q *UpdateQueries) ListUpdates(filters *models.UpdateFilters) ([]models.UpdatePackage, int, error) {
	var updates []models.UpdatePackage
	whereClause := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if filters.AgentID != nil {
		whereClause = append(whereClause, fmt.Sprintf("agent_id = $%d", argIdx))
		args = append(args, *filters.AgentID)
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

// GetUpdateByID retrieves a single update by ID
func (q *UpdateQueries) GetUpdateByID(id uuid.UUID) (*models.UpdatePackage, error) {
	var update models.UpdatePackage
	query := `SELECT * FROM update_packages WHERE id = $1`
	err := q.db.Get(&update, query, id)
	if err != nil {
		return nil, err
	}
	return &update, nil
}

// ApproveUpdate marks an update as approved
func (q *UpdateQueries) ApproveUpdate(id uuid.UUID, approvedBy string) error {
	query := `
		UPDATE update_packages
		SET status = 'approved', approved_by = $1, approved_at = NOW()
		WHERE id = $2 AND status = 'pending'
	`
	_, err := q.db.Exec(query, approvedBy, id)
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
