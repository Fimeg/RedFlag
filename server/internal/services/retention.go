package services

import (
	"log"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
)

// RetentionService (RETAIN-001) prunes aged rows from append-only history
// tables on a schedule. Without it, metrics/system_events/agent_commands
// grow unbounded — at 1,000 agents on 60-second heartbeats that is real
// churn. Three operator-tunable horizons, resolved through the layered
// settings (env > config > DB > default); a value of 0 means keep forever
// (sovereignty: deletion is opt-out-able, never forced).
//
// Deliberately NOT covered: current-state tables, and the process snapshot
// tables (those carry their own 10-snapshot cap). Pruned rows are gone —
// this runs against history the operator has aged out, and every sweep
// logs what it removed.
type RetentionService struct {
	queries  *queries.RetentionQueries
	settings *SecuritySettingsService
}

// retentionCategory groups tables that age out on one shared horizon.
type retentionCategory struct {
	settingKey  string // operational.<key>, int days; 0 = keep forever
	defaultDays int
	tables      []string
}

var retentionCategories = []retentionCategory{
	// High-churn telemetry, low audit value once superseded.
	{settingKey: "retention_metrics_days", defaultDays: 30,
		tables: []string{"metrics", "storage_metrics"}},
	// Operational event stream — feeds History/notification bell.
	{settingKey: "retention_events_days", defaultDays: 90,
		tables: []string{"system_events"}},
	// Audit trail — command history and update lifecycle. Long default;
	// this is the "what happened on that machine" record.
	{settingKey: "retention_history_days", defaultDays: 365,
		tables: []string{"agent_commands", "update_logs", "update_version_history"}},
}

const (
	pruneBatchSize        = 5000
	maxPruneBatchesPerRun = 20 // caps one sweep at 100k rows/table; backlog drains over successive sweeps
)

func NewRetentionService(q *queries.RetentionQueries, settings *SecuritySettingsService) *RetentionService {
	return &RetentionService{queries: q, settings: settings}
}

// Sweep prunes every category once. Errors on one table are logged and do
// not stop the rest of the sweep (assume failure; the next sweep retries).
// Intended to run on the taskrunner periodic registry.
func (s *RetentionService) Sweep() {
	for _, cat := range retentionCategories {
		days := cat.defaultDays
		if s.settings != nil {
			days = s.settings.GetOperationalInt(cat.settingKey, cat.defaultDays)
		}
		if days <= 0 {
			// 0 (or negative) = retention disabled for this category.
			continue
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -days)

		for _, table := range cat.tables {
			var total int64
			for i := 0; i < maxPruneBatchesPerRun; i++ {
				n, err := s.queries.PruneBatch(table, cutoff, pruneBatchSize)
				if err != nil {
					log.Printf("[WARNING] [server] [retention] prune_failed table=%s error=%q", table, err)
					break
				}
				total += n
				if n < pruneBatchSize {
					break
				}
			}
			if total > 0 {
				log.Printf("[INFO] [server] [retention] pruned table=%s rows=%d retention_days=%d", table, total, days)
			}
		}
	}
}
