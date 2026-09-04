package observability

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/circuitbreaker"
	"github.com/Fimeg/RedFlag/server/internal/scheduler"
	"github.com/Fimeg/RedFlag/server/internal/taskrunner"
)

// SchedulerStatsProvider is implemented by *scheduler.Scheduler.
type SchedulerStatsProvider interface {
	GetStats() scheduler.Stats
	GetQueueStats() scheduler.QueueStats
}

// Exporter emits existing RedFlag point-in-time health counters using the
// Prometheus text exposition format. It does not own metric state; values are
// read from live internals on each scrape.
type Exporter struct {
	dbStats                  func() sql.DBStats
	taskSnapshot             func() taskrunner.Snapshot
	scheduler                SchedulerStatsProvider
	breakerStats             map[string]func() circuitbreaker.Stats
	countSupplyChainDeferred func() (int, error)
}

// NewExporter creates the OBS-001A metrics exporter.
func NewExporter(
	dbStats func() sql.DBStats,
	taskSnapshot func() taskrunner.Snapshot,
	scheduler SchedulerStatsProvider,
	breakerStats map[string]func() circuitbreaker.Stats,
	countSupplyChainDeferred func() (int, error),
) *Exporter {
	return &Exporter{
		dbStats:                  dbStats,
		taskSnapshot:             taskSnapshot,
		scheduler:                scheduler,
		breakerStats:             breakerStats,
		countSupplyChainDeferred: countSupplyChainDeferred,
	}
}

// ServeHTTP writes a Prometheus-compatible scrape response.
func (e *Exporter) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	var buf bytes.Buffer
	e.render(&buf)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("[WARNING] [server] [metrics] scrape_write_failed error=%v", err)
	}
}

func (e *Exporter) render(buf *bytes.Buffer) {
	e.renderDB(buf)
	e.renderTaskrunner(buf)
	e.renderScheduler(buf)
	e.renderBreakers(buf)
	e.renderAdvisory(buf)
}

func (e *Exporter) renderDB(buf *bytes.Buffer) {
	if e.dbStats == nil {
		return
	}
	stats := e.dbStats()
	metricHeader(buf, "redflag_db_connections", "Current database connection pool counts by state.", "gauge")
	metric(buf, "redflag_db_connections", float64(stats.OpenConnections), "state", "open")
	metric(buf, "redflag_db_connections", float64(stats.InUse), "state", "in_use")
	metric(buf, "redflag_db_connections", float64(stats.Idle), "state", "idle")

	metricHeader(buf, "redflag_db_max_open_connections", "Configured database maximum open connections.", "gauge")
	metric(buf, "redflag_db_max_open_connections", float64(stats.MaxOpenConnections))

	metricHeader(buf, "redflag_db_wait_total", "Total number of waits for a database connection.", "counter")
	metric(buf, "redflag_db_wait_total", float64(stats.WaitCount))

	metricHeader(buf, "redflag_db_wait_duration_seconds_total", "Total time spent waiting for a database connection.", "counter")
	metric(buf, "redflag_db_wait_duration_seconds_total", stats.WaitDuration.Seconds())

	metricHeader(buf, "redflag_db_connections_closed_total", "Total database connections closed by pool reason.", "counter")
	metric(buf, "redflag_db_connections_closed_total", float64(stats.MaxIdleClosed), "reason", "max_idle")
	metric(buf, "redflag_db_connections_closed_total", float64(stats.MaxIdleTimeClosed), "reason", "max_idle_time")
	metric(buf, "redflag_db_connections_closed_total", float64(stats.MaxLifetimeClosed), "reason", "max_lifetime")
}

func (e *Exporter) renderTaskrunner(buf *bytes.Buffer) {
	if e.taskSnapshot == nil {
		return
	}
	snap := e.taskSnapshot()
	metricHeader(buf, "redflag_taskrunner", "Current taskrunner values by field.", "gauge")
	metric(buf, "redflag_taskrunner", float64(snap.Workers), "field", "workers")
	metric(buf, "redflag_taskrunner", float64(snap.QueueLen), "field", "queue_len")
	metric(buf, "redflag_taskrunner", float64(snap.QueueCap), "field", "queue_cap")
	metric(buf, "redflag_taskrunner", float64(snap.Inflight), "field", "inflight")

	metricHeader(buf, "redflag_taskrunner_total", "Cumulative taskrunner counts by field.", "counter")
	metric(buf, "redflag_taskrunner_total", float64(snap.Submitted), "field", "submitted")
	metric(buf, "redflag_taskrunner_total", float64(snap.Completed), "field", "completed")
	metric(buf, "redflag_taskrunner_total", float64(snap.Overflow), "field", "overflow")
	metric(buf, "redflag_taskrunner_total", float64(snap.Panicked), "field", "panicked")

	metricHeader(buf, "redflag_taskrunner_periodic_runs_total", "Total runs for a registered periodic task.", "counter")
	metricHeader(buf, "redflag_taskrunner_periodic_last_run_timestamp_seconds", "Unix timestamp of the last periodic task start.", "gauge")
	for _, task := range snap.Periodic {
		metric(buf, "redflag_taskrunner_periodic_runs_total", float64(task.Runs), "task", task.Name)
		if task.LastRun == "" {
			continue
		}
		lastRun, err := time.Parse(time.RFC3339, task.LastRun)
		if err != nil {
			log.Printf("[WARNING] [server] [metrics] taskrunner_last_run_parse_failed task=%s value=%q error=%v", task.Name, task.LastRun, err)
			continue
		}
		metric(buf, "redflag_taskrunner_periodic_last_run_timestamp_seconds", float64(lastRun.Unix()), "task", task.Name)
	}
}

func (e *Exporter) renderScheduler(buf *bytes.Buffer) {
	if e.scheduler == nil {
		return
	}
	stats := e.scheduler.GetStats()
	queue := e.scheduler.GetQueueStats()

	metricHeader(buf, "redflag_scheduler_total", "Cumulative scheduler counts by field.", "counter")
	metric(buf, "redflag_scheduler_total", float64(stats.JobsProcessed), "field", "jobs_processed")
	metric(buf, "redflag_scheduler_total", float64(stats.JobsSkipped), "field", "jobs_skipped")
	metric(buf, "redflag_scheduler_total", float64(stats.CommandsCreated), "field", "commands_created")
	metric(buf, "redflag_scheduler_total", float64(stats.CommandsFailed), "field", "commands_failed")
	metric(buf, "redflag_scheduler_total", float64(stats.BackpressureSkips), "field", "backpressure_skips")

	metricHeader(buf, "redflag_scheduler", "Current scheduler values by field.", "gauge")
	metric(buf, "redflag_scheduler", float64(stats.QueueSize), "field", "dispatch_queue_size")
	metric(buf, "redflag_scheduler", float64(stats.WorkerPoolUtilized), "field", "worker_pool_utilized")
	metric(buf, "redflag_scheduler", float64(stats.AverageProcessingMS), "field", "average_processing_ms")
	metric(buf, "redflag_scheduler", float64(queue.Size), "field", "priority_queue_size")
	if !stats.LastProcessedAt.IsZero() {
		metric(buf, "redflag_scheduler", float64(stats.LastProcessedAt.Unix()), "field", "last_processed_timestamp_seconds")
	}
	if queue.NextRunAt != nil {
		metric(buf, "redflag_scheduler", float64(queue.NextRunAt.Unix()), "field", "next_run_timestamp_seconds")
	}

	metricHeader(buf, "redflag_scheduler_queue_jobs", "Current scheduler queue jobs by subsystem.", "gauge")
	for subsystem, count := range queue.JobsBySubsystem {
		metric(buf, "redflag_scheduler_queue_jobs", float64(count), "subsystem", subsystem)
	}
}

func (e *Exporter) renderBreakers(buf *bytes.Buffer) {
	if len(e.breakerStats) == 0 {
		return
	}
	metricHeader(buf, "redflag_circuit_breaker_state", "Current circuit breaker state; one series is 1 for the active state.", "gauge")
	metricHeader(buf, "redflag_circuit_breaker", "Current circuit breaker values by field.", "gauge")

	names := make([]string, 0, len(e.breakerStats))
	for name := range e.breakerStats {
		names = append(names, name)
	}
	sort.Strings(names)

	states := []string{"closed", "open", "half-open"}
	for _, name := range names {
		fn := e.breakerStats[name]
		if fn == nil {
			continue
		}
		stats := fn()
		for _, state := range states {
			value := 0.0
			if stats.State == state {
				value = 1
			}
			metric(buf, "redflag_circuit_breaker_state", value, "breaker", name, "state", state)
		}
		metric(buf, "redflag_circuit_breaker", float64(stats.RecentFailures), "breaker", name, "field", "recent_failures")
		metric(buf, "redflag_circuit_breaker", float64(stats.ConsecutiveSuccess), "breaker", name, "field", "consecutive_success")
		if stats.NextAttempt != nil {
			metric(buf, "redflag_circuit_breaker", float64(stats.NextAttempt.Unix()), "breaker", name, "field", "next_attempt_timestamp_seconds")
		}
	}
}

func (e *Exporter) renderAdvisory(buf *bytes.Buffer) {
	if e.countSupplyChainDeferred == nil {
		return
	}
	deferred, err := e.countSupplyChainDeferred()
	if err != nil {
		log.Printf("[WARNING] [server] [metrics] advisory_deferred_count_failed error=%v", err)
		deferred = -1
	}
	metricHeader(buf, "redflag_advisory_deferred_packages", "Package rows whose OSV advisory check is deferred. -1 means the count failed.", "gauge")
	metric(buf, "redflag_advisory_deferred_packages", float64(deferred))
}

func metricHeader(buf *bytes.Buffer, name, help, metricType string) {
	fmt.Fprintf(buf, "# HELP %s %s\n", name, escapeHelp(help))
	fmt.Fprintf(buf, "# TYPE %s %s\n", name, metricType)
}

func metric(buf *bytes.Buffer, name string, value float64, labelPairs ...string) {
	fmt.Fprint(buf, name)
	if len(labelPairs) > 0 {
		fmt.Fprint(buf, "{")
		for i := 0; i < len(labelPairs); i += 2 {
			if i > 0 {
				fmt.Fprint(buf, ",")
			}
			key := labelPairs[i]
			val := ""
			if i+1 < len(labelPairs) {
				val = labelPairs[i+1]
			}
			fmt.Fprintf(buf, `%s="%s"`, key, escapeLabel(val))
		}
		fmt.Fprint(buf, "}")
	}
	fmt.Fprintf(buf, " %.17g\n", value)
}

func escapeHelp(value string) string {
	return strings.ReplaceAll(value, "\n", `\n`)
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}
