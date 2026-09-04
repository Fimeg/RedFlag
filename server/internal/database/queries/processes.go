package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// ProcessQueries handles process snapshot database operations.
type ProcessQueries struct {
	db *sql.DB
}

// NewProcessQueries creates a new process queries instance.
func NewProcessQueries(db *sql.DB) *ProcessQueries {
	return &ProcessQueries{db: db}
}

// InsertSnapshot inserts a process snapshot header and returns its ID.
func (q *ProcessQueries) InsertSnapshot(ctx context.Context, snap models.ProcessSnapshot) error {
	query := `
		INSERT INTO agent_process_snapshots (id, agent_id, command_id, process_count, scanned_at, scan_duration_ms, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := q.db.ExecContext(ctx, query,
		snap.ID, snap.AgentID, snap.CommandID, snap.ProcessCount,
		snap.ScannedAt, snap.ScanDurationMs, snap.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	return nil
}

// InsertProcesses bulk-inserts process rows within a single transaction.
func (q *ProcessQueries) InsertProcesses(ctx context.Context, procs []models.Process) error {
	if len(procs) == 0 {
		return nil
	}

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO agent_processes (
			id, snapshot_id, agent_id, pid, name, path, cmdline, cwd, state,
			uid, gid, euid, egid, "user", "group", tty, tty_name,
			cpu_seconds_user, cpu_seconds_system, cpu_percent,
			rss_bytes, vms_bytes, mem_percent, threads, nice,
			start_time_seconds, parent_pid, process_group_id,
			elevation_status, on_disk, disk_bytes_read, disk_bytes_written, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20,
			$21, $22, $23, $24, $25,
			$26, $27, $28,
			$29, $30, $31, $32, $33
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, p := range procs {
		_, err := stmt.ExecContext(ctx,
			p.ID, p.SnapshotID, p.AgentID, p.PID, p.Name, p.Path, p.Cmdline, p.Cwd, p.State,
			p.UID, p.GID, p.EUID, p.EGID, p.User, p.Group, p.TTY, p.TTYName,
			p.CPUSecondsUser, p.CPUSecondsSystem, p.CPUPercent,
			p.RSSBytes, p.VMSBytes, p.MemPercent, p.Threads, p.Nice,
			p.StartTimeSeconds, p.ParentPID, p.ProcessGroupID,
			p.ElevationStatus, p.OnDisk, p.DiskBytesRead, p.DiskBytesWritten, p.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert process pid=%d: %w", p.PID, err)
		}
	}

	return tx.Commit()
}

// InsertRelatedData inserts related data rows for a process.
func (q *ProcessQueries) InsertRelatedData(ctx context.Context, entries []models.ProcessRelated) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO agent_process_related (id, process_id, relation_type, data, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		_, err := stmt.ExecContext(ctx, e.ID, e.ProcessID, e.RelationType, e.Data, e.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert related type=%s: %w", e.RelationType, err)
		}
	}

	return tx.Commit()
}

// GetLatestSnapshot returns the most recent snapshot for an agent, or nil if none.
func (q *ProcessQueries) GetLatestSnapshot(ctx context.Context, agentID uuid.UUID) (*models.ProcessSnapshot, error) {
	query := `
		SELECT id, agent_id, command_id, process_count, scanned_at, scan_duration_ms, created_at
		FROM agent_process_snapshots
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var snap models.ProcessSnapshot
	err := q.db.QueryRowContext(ctx, query, agentID).Scan(
		&snap.ID, &snap.AgentID, &snap.CommandID, &snap.ProcessCount,
		&snap.ScannedAt, &snap.ScanDurationMs, &snap.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest snapshot: %w", err)
	}
	return &snap, nil
}

// GetProcessesBySnapshot returns processes for a snapshot, with optional filtering and sorting.
func (q *ProcessQueries) GetProcessesBySnapshot(ctx context.Context, snapshotID uuid.UUID, filter ProcessFilter) ([]models.Process, int, error) {
	// Count total
	countQuery := `SELECT COUNT(*) FROM agent_processes WHERE snapshot_id = $1`
	var total int
	if err := q.db.QueryRowContext(ctx, countQuery, snapshotID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count processes: %w", err)
	}

	// Build query with optional filters
	query := `
		SELECT id, snapshot_id, agent_id, pid, name, path, cmdline, cwd, state,
			uid, gid, euid, egid, "user", "group", tty, tty_name,
			cpu_seconds_user, cpu_seconds_system, cpu_percent,
			rss_bytes, vms_bytes, mem_percent, threads, nice,
			start_time_seconds, parent_pid, process_group_id,
			elevation_status, on_disk, disk_bytes_read, disk_bytes_written, created_at
		FROM agent_processes
		WHERE snapshot_id = $1
	`
	args := []interface{}{snapshotID}
	argIdx := 2

	if filter.Name != "" {
		query += fmt.Sprintf(" AND name ILIKE $%d", argIdx)
		args = append(args, "%"+filter.Name+"%")
		argIdx++
	}
	if filter.User != "" {
		query += fmt.Sprintf(` AND "user" = $%d`, argIdx)
		args = append(args, filter.User)
		argIdx++
	}
	if filter.State != "" {
		query += fmt.Sprintf(" AND state = $%d", argIdx)
		args = append(args, filter.State)
		argIdx++
	}

	// Sort
	sortCol := "cpu_percent"
	switch filter.SortBy {
	case "mem", "mem_percent":
		sortCol = "mem_percent"
	case "pid":
		sortCol = "pid"
	case "name":
		sortCol = "name"
	case "threads":
		sortCol = "threads"
	case "rss":
		sortCol = "rss_bytes"
	}
	sortDir := "DESC"
	if filter.SortDir == "asc" {
		sortDir = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortCol, sortDir)

	// Pagination
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filter.Limit)
		argIdx++
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filter.Offset)
		argIdx++
	}

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query processes: %w", err)
	}
	defer rows.Close()

	var procs []models.Process
	for rows.Next() {
		var p models.Process
		err := rows.Scan(
			&p.ID, &p.SnapshotID, &p.AgentID, &p.PID, &p.Name, &p.Path, &p.Cmdline, &p.Cwd, &p.State,
			&p.UID, &p.GID, &p.EUID, &p.EGID, &p.User, &p.Group, &p.TTY, &p.TTYName,
			&p.CPUSecondsUser, &p.CPUSecondsSystem, &p.CPUPercent,
			&p.RSSBytes, &p.VMSBytes, &p.MemPercent, &p.Threads, &p.Nice,
			&p.StartTimeSeconds, &p.ParentPID, &p.ProcessGroupID,
			&p.ElevationStatus, &p.OnDisk, &p.DiskBytesRead, &p.DiskBytesWritten, &p.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan process: %w", err)
		}
		procs = append(procs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate processes: %w", err)
	}

	return procs, total, nil
}

// GetProcessByID returns a single process by its ID.
func (q *ProcessQueries) GetProcessByID(ctx context.Context, processID uuid.UUID) (*models.Process, error) {
	query := `
		SELECT id, snapshot_id, agent_id, pid, name, path, cmdline, cwd, state,
			uid, gid, euid, egid, "user", "group", tty, tty_name,
			cpu_seconds_user, cpu_seconds_system, cpu_percent,
			rss_bytes, vms_bytes, mem_percent, threads, nice,
			start_time_seconds, parent_pid, process_group_id,
			elevation_status, on_disk, disk_bytes_read, disk_bytes_written, created_at
		FROM agent_processes
		WHERE id = $1
	`
	var p models.Process
	err := q.db.QueryRowContext(ctx, query, processID).Scan(
		&p.ID, &p.SnapshotID, &p.AgentID, &p.PID, &p.Name, &p.Path, &p.Cmdline, &p.Cwd, &p.State,
		&p.UID, &p.GID, &p.EUID, &p.EGID, &p.User, &p.Group, &p.TTY, &p.TTYName,
		&p.CPUSecondsUser, &p.CPUSecondsSystem, &p.CPUPercent,
		&p.RSSBytes, &p.VMSBytes, &p.MemPercent, &p.Threads, &p.Nice,
		&p.StartTimeSeconds, &p.ParentPID, &p.ProcessGroupID,
		&p.ElevationStatus, &p.OnDisk, &p.DiskBytesRead, &p.DiskBytesWritten, &p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get process: %w", err)
	}
	return &p, nil
}

// GetProcessRelated returns related data for a process, optionally filtered by type.
func (q *ProcessQueries) GetProcessRelated(ctx context.Context, processID uuid.UUID, relationType string) ([]models.ProcessRelated, error) {
	query := `SELECT id, process_id, relation_type, data, created_at FROM agent_process_related WHERE process_id = $1`
	args := []interface{}{processID}

	if relationType != "" {
		query += " AND relation_type = $2"
		args = append(args, relationType)
	}
	query += " ORDER BY relation_type, created_at"

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query related: %w", err)
	}
	defer rows.Close()

	var entries []models.ProcessRelated
	for rows.Next() {
		var e models.ProcessRelated
		if err := rows.Scan(&e.ID, &e.ProcessID, &e.RelationType, &e.Data, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan related: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate related: %w", err)
	}
	return entries, nil
}

// DeleteSnapshot deletes a snapshot by ID (cascades to processes and related data).
func (q *ProcessQueries) DeleteSnapshot(ctx context.Context, snapID uuid.UUID) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM agent_process_snapshots WHERE id = $1", snapID)
	if err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	return nil
}

// CleanupOldSnapshots keeps only the most recent N snapshots per agent.
func (q *ProcessQueries) CleanupOldSnapshots(ctx context.Context, agentID uuid.UUID, keepCount int) (int, error) {
	query := `
		DELETE FROM agent_process_snapshots
		WHERE agent_id = $1
		AND id NOT IN (
			SELECT id FROM agent_process_snapshots
			WHERE agent_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		)
	`
	result, err := q.db.ExecContext(ctx, query, agentID, keepCount)
	if err != nil {
		return 0, fmt.Errorf("cleanup snapshots: %w", err)
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// ProcessFilter defines filtering/sorting options for process queries.
type ProcessFilter struct {
	Name     string
	User     string
	State    string
	SortBy   string // cpu, mem, pid, name, threads, rss
	SortDir  string // asc, desc
	Limit    int
	Offset   int
}

// helper to marshal related data to JSONB
func marshalJSONB(v interface{}) (models.JSONB, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var jb models.JSONB
	if err := json.Unmarshal(data, &jb); err != nil {
		return nil, err
	}
	return jb, nil
}
