package queries

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// StorageMetricsQueries handles storage metrics database operations
type StorageMetricsQueries struct {
	db *sql.DB
}

// NewStorageMetricsQueries creates a new storage metrics queries instance
func NewStorageMetricsQueries(db *sql.DB) *StorageMetricsQueries {
	return &StorageMetricsQueries{db: db}
}

// InsertStorageMetric inserts a new storage metric
func (q *StorageMetricsQueries) InsertStorageMetric(ctx context.Context, metric models.StorageMetric) error {
	query := `
		INSERT INTO storage_metrics (
			id, agent_id, mountpoint, device, disk_type, filesystem,
			total_bytes, used_bytes, available_bytes, used_percent,
			severity, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := q.db.ExecContext(ctx, query,
		metric.ID, metric.AgentID, metric.Mountpoint, metric.Device,
		metric.DiskType, metric.Filesystem, metric.TotalBytes,
		metric.UsedBytes, metric.AvailableBytes, metric.UsedPercent,
		metric.Severity, metric.Metadata, metric.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert storage metric: %w", err)
	}

	return nil
}

// GetStorageMetricsByAgentID retrieves storage metrics for an agent
func (q *StorageMetricsQueries) GetStorageMetricsByAgentID(ctx context.Context, agentID uuid.UUID, limit, offset int) ([]models.StorageMetric, error) {
	query := `
		SELECT id, agent_id, mountpoint, device, disk_type, filesystem,
		       total_bytes, used_bytes, available_bytes, used_percent,
		       severity, metadata, created_at
		FROM storage_metrics
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := q.db.QueryContext(ctx, query, agentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query storage metrics: %w", err)
	}
	defer rows.Close()

	var metrics []models.StorageMetric
	for rows.Next() {
		var metric models.StorageMetric

		err := rows.Scan(
			&metric.ID, &metric.AgentID, &metric.Mountpoint, &metric.Device,
			&metric.DiskType, &metric.Filesystem, &metric.TotalBytes,
			&metric.UsedBytes, &metric.AvailableBytes, &metric.UsedPercent,
			&metric.Severity, &metric.Metadata, &metric.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan storage metric: %w", err)
		}

		metrics = append(metrics, metric)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating storage metrics: %w", err)
	}

	return metrics, nil
}

// GetLatestStorageMetrics retrieves the most recent storage metrics per mountpoint
func (q *StorageMetricsQueries) GetLatestStorageMetrics(ctx context.Context, agentID uuid.UUID) ([]models.StorageMetric, error) {
	query := `
		SELECT DISTINCT ON (mountpoint) 
			id, agent_id, mountpoint, device, disk_type, filesystem,
			total_bytes, used_bytes, available_bytes, used_percent,
			severity, metadata, created_at
		FROM storage_metrics
		WHERE agent_id = $1
		ORDER BY mountpoint, created_at DESC
	`

	rows, err := q.db.QueryContext(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest storage metrics: %w", err)
	}
	defer rows.Close()

	var metrics []models.StorageMetric
	for rows.Next() {
		var metric models.StorageMetric

		err := rows.Scan(
			&metric.ID, &metric.AgentID, &metric.Mountpoint, &metric.Device,
			&metric.DiskType, &metric.Filesystem, &metric.TotalBytes,
			&metric.UsedBytes, &metric.AvailableBytes, &metric.UsedPercent,
			&metric.Severity, &metric.Metadata, &metric.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan storage metric: %w", err)
		}

		metrics = append(metrics, metric)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating latest storage metrics: %w", err)
	}

	return metrics, nil
}

// GetStorageMetricsSummary returns summary statistics for an agent
func (q *StorageMetricsQueries) GetStorageMetricsSummary(ctx context.Context, agentID uuid.UUID) (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) as total_disks,
			COUNT(CASE WHEN severity = 'critical' THEN 1 END) as critical_disks,
			COUNT(CASE WHEN severity = 'important' THEN 1 END) as important_disks,
			AVG(used_percent) as avg_used_percent,
			MAX(used_percent) as max_used_percent,
			MIN(created_at) as first_collected_at,
			MAX(created_at) as last_collected_at
		FROM storage_metrics
		WHERE agent_id = $1
		  AND created_at >= NOW() - INTERVAL '24 hours'
	`

	var (
		totalDisks       int
		criticalDisks    int
		importantDisks   int
		avgUsedPercent   sql.NullFloat64
		maxUsedPercent   sql.NullFloat64
		firstCollectedAt sql.NullTime
		lastCollectedAt  sql.NullTime
	)

	err := q.db.QueryRowContext(ctx, query, agentID).Scan(
		&totalDisks,
		&criticalDisks,
		&importantDisks,
		&avgUsedPercent,
		&maxUsedPercent,
		&firstCollectedAt,
		&lastCollectedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage metrics summary: %w", err)
	}

	summary := map[string]interface{}{
		"total_disks":        totalDisks,
		"critical_disks":     criticalDisks,
		"important_disks":    importantDisks,
		"avg_used_percent":   avgUsedPercent.Float64,
		"max_used_percent":   maxUsedPercent.Float64,
		"first_collected_at": firstCollectedAt.Time,
		"last_collected_at":  lastCollectedAt.Time,
	}

	return summary, nil
}
