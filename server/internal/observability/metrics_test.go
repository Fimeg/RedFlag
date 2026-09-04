package observability

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/circuitbreaker"
	"github.com/Fimeg/RedFlag/server/internal/scheduler"
	"github.com/Fimeg/RedFlag/server/internal/taskrunner"
)

type fakeScheduler struct {
	stats scheduler.Stats
	queue scheduler.QueueStats
}

func (f fakeScheduler) GetStats() scheduler.Stats {
	return f.stats
}

func (f fakeScheduler) GetQueueStats() scheduler.QueueStats {
	return f.queue
}

func TestExporterExposesLivePointInTimeSources(t *testing.T) {
	nextRun := time.Unix(2000, 0).UTC()
	exporter := NewExporter(
		func() sql.DBStats {
			return sql.DBStats{
				MaxOpenConnections: 10,
				OpenConnections:    3,
				InUse:              2,
				Idle:               1,
				WaitCount:          4,
				WaitDuration:       5 * time.Second,
			}
		},
		func() taskrunner.Snapshot {
			return taskrunner.Snapshot{
				Workers:   8,
				QueueLen:  2,
				QueueCap:  256,
				Inflight:  1,
				Submitted: 12,
				Completed: 10,
				Overflow:  1,
				Panicked:  0,
				Periodic: []taskrunner.PeriodicSnapshot{
					{Name: "rate_limit_cleanup", Runs: 3, LastRun: time.Unix(1000, 0).UTC().Format(time.RFC3339)},
				},
			}
		},
		fakeScheduler{
			stats: scheduler.Stats{
				JobsProcessed:       5,
				JobsSkipped:         1,
				CommandsCreated:     4,
				CommandsFailed:      2,
				BackpressureSkips:   1,
				QueueSize:           7,
				WorkerPoolUtilized:  3,
				AverageProcessingMS: 20,
				LastProcessedAt:     time.Unix(1500, 0).UTC(),
			},
			queue: scheduler.QueueStats{
				Size:            9,
				NextRunAt:       &nextRun,
				JobsBySubsystem: map[string]int{"updates": 6},
			},
		},
		map[string]func() circuitbreaker.Stats{
			"osv": func() circuitbreaker.Stats {
				return circuitbreaker.Stats{Name: "osv", State: "open", RecentFailures: 5}
			},
		},
		func() (int, error) { return 2, nil },
	)

	rec := httptest.NewRecorder()
	exporter.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	assertContains(t, body, "# TYPE redflag_db_connections gauge")
	assertContains(t, body, `redflag_db_connections{state="in_use"} 2`)
	assertContains(t, body, `redflag_taskrunner_total{field="overflow"} 1`)
	assertContains(t, body, `redflag_taskrunner_periodic_runs_total{task="rate_limit_cleanup"} 3`)
	assertContains(t, body, `redflag_scheduler_total{field="jobs_processed"} 5`)
	assertContains(t, body, `redflag_scheduler_queue_jobs{subsystem="updates"} 6`)
	assertContains(t, body, `redflag_circuit_breaker_state{breaker="osv",state="open"} 1`)
	assertContains(t, body, "redflag_advisory_deferred_packages 2")
}

func TestEscapeLabel(t *testing.T) {
	got := escapeLabel("line\nquote\"slash\\")
	want := `line\nquote\"slash\\`
	if got != want {
		t.Fatalf("escapeLabel()=%q, want %q", got, want)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected metrics output to contain %q\n%s", needle, haystack)
	}
}
