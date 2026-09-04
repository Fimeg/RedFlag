package queries

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// RetentionQueries deletes aged rows from append-only history tables
// (RETAIN-001). The table -> timestamp-column map below is a closed
// whitelist: table and column names never reach SQL from caller-supplied
// strings, so the dynamic DELETE is injection-free by construction.
type RetentionQueries struct {
	db *sqlx.DB
}

// prunableTables maps each table the retention sweep may touch to the
// timestamp column that defines row age. Adding a table here is a
// reviewable act — current-state tables (agents, update_packages,
// current_package_state, docker_*) must never appear; only append-only
// history belongs.
var prunableTables = map[string]string{
	"metrics":                "created_at",
	"storage_metrics":        "created_at",
	"system_events":          "created_at",
	"agent_commands":         "created_at",
	"update_logs":            "executed_at",
	"update_version_history": "update_completed_at",
}

func NewRetentionQueries(db *sqlx.DB) *RetentionQueries {
	return &RetentionQueries{db: db}
}

// PruneBatch deletes up to limit rows older than cutoff from table.
// Returns the number of rows deleted. ctid-based so it works regardless
// of the table's primary key shape; bounded so a first sweep over months
// of backlog cannot hold a long transaction.
func (q *RetentionQueries) PruneBatch(table string, cutoff time.Time, limit int) (int64, error) {
	column, ok := prunableTables[table]
	if !ok {
		return 0, fmt.Errorf("retention: table %q is not on the prunable whitelist", table)
	}

	query := fmt.Sprintf(
		`DELETE FROM %s WHERE ctid IN (SELECT ctid FROM %s WHERE %s < $1 LIMIT $2)`,
		table, table, column,
	)
	res, err := q.db.Exec(query, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("retention: prune %s: %w", table, err)
	}
	return res.RowsAffected()
}
