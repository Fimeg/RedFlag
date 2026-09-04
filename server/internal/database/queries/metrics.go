package queries

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/doug-martin/goqu/v9"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// MetricsQueries handles database operations for metrics
type MetricsQueries struct {
	db *sqlx.DB
}

func NewMetricsQueries(db *sqlx.DB) *MetricsQueries {
	return &MetricsQueries{db: db}
}

// CreateMetricsEventsBatch creates multiple metric events in a single transaction
func (q *MetricsQueries) CreateMetricsEventsBatch(events []models.StoredMetric) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := q.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare the insert statement
	stmt, err := tx.Prepare(`
		INSERT INTO metrics (
			id, agent_id, package_type, package_name, current_version, available_version,
			severity, repository_source, metadata, event_type, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (agent_id, package_name, package_type, created_at) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert each event with error isolation
	for _, event := range events {
		_, err := stmt.Exec(
			event.ID,
			event.AgentID,
			event.PackageType,
			event.PackageName,
			event.CurrentVersion,
			event.AvailableVersion,
			event.Severity,
			event.RepositorySource,
			event.Metadata,
			event.EventType,
			event.CreatedAt,
		)
		if err != nil {
			// Log error but continue with other events
			log.Printf("[WARNING] [server] [database] metric_event_insert_failed event_id=%s error=%v", event.ID, err)
			continue
		}
	}

	return tx.Commit()
}

// GetMetrics retrieves metrics based on filter criteria
func (q *MetricsQueries) GetMetrics(filter *models.MetricFilter) (*models.MetricResult, error) {
	var metrics []models.StoredMetric

	sd := PG().From("metrics")

	if filter.AgentID != nil {
		sd = sd.Where(goqu.Ex{"agent_id": *filter.AgentID})
	}
	if filter.PackageType != nil {
		sd = sd.Where(goqu.Ex{"package_type": *filter.PackageType})
	}
	if filter.Severity != nil {
		sd = sd.Where(goqu.Ex{"severity": *filter.Severity})
	}

	page := uint(1)
	pageSize := uint(50)
	if filter.Limit != nil {
		pageSize = uint(*filter.Limit)
		if filter.Offset != nil {
			page = uint(*filter.Offset / *filter.Limit) + 1
		}
	}

	cols := []string{"id", "agent_id", "package_type", "package_name", "current_version",
		"available_version", "severity", "repository_source", "metadata", "event_type", "created_at"}

	total, err := Paginated(q.db, sd, page, pageSize, goqu.C("created_at").Desc(), cols, &metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}

	return &models.MetricResult{
		Metrics: metrics,
		Total:   total,
		Page:    int(page),
		PerPage: int(pageSize),
	}, nil
}

// GetMetricsByAgentID retrieves metrics for a specific agent
func (q *MetricsQueries) GetMetricsByAgentID(agentID uuid.UUID, limit int) ([]models.StoredMetric, error) {
	query := `
		SELECT id, agent_id, package_type, package_name, current_version, available_version,
		       severity, repository_source, metadata, event_type, created_at
		FROM metrics
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := q.db.Query(query, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics by agent: %w", err)
	}
	defer rows.Close()

	var metrics []models.StoredMetric
	for rows.Next() {
		var metric models.StoredMetric
		err := rows.Scan(
			&metric.ID,
			&metric.AgentID,
			&metric.PackageType,
			&metric.PackageName,
			&metric.CurrentVersion,
			&metric.AvailableVersion,
			&metric.Severity,
			&metric.RepositorySource,
			&metric.Metadata,
			&metric.EventType,
			&metric.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric: %w", err)
		}
		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// GetLatestMetricsByType retrieves the latest metrics for a specific type
func (q *MetricsQueries) GetLatestMetricsByType(agentID uuid.UUID, packageType string) (*models.StoredMetric, error) {
	query := `
		SELECT id, agent_id, package_type, package_name, current_version, available_version,
		       severity, repository_source, metadata, event_type, created_at
		FROM metrics
		WHERE agent_id = $1 AND package_type = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	var metric models.StoredMetric
	err := q.db.QueryRow(query, agentID, packageType).Scan(
		&metric.ID,
		&metric.AgentID,
		&metric.PackageType,
		&metric.PackageName,
		&metric.CurrentVersion,
		&metric.AvailableVersion,
		&metric.Severity,
		&metric.RepositorySource,
		&metric.Metadata,
		&metric.EventType,
		&metric.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest metric: %w", err)
	}

	return &metric, nil
}

// DeleteOldMetrics deletes metrics older than the specified number of days
func (q *MetricsQueries) DeleteOldMetrics(days int) error {
	query := `DELETE FROM metrics WHERE created_at < NOW() - INTERVAL '1 day' * $1`

	result, err := q.db.Exec(query, days)
	if err != nil {
		return fmt.Errorf("failed to delete old metrics: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("[INFO] [server] [database] metrics_cleanup removed=%d", rowsAffected)
	}

	return nil
}