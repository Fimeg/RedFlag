// Package taskrunner provides a bounded, observable runner for the server's
// background work (SCALE-001 S6). It serves two roles:
//
//  1. A bounded ad-hoc pool (Go) for the fire-and-forget work that HTTP
//     handlers previously launched as raw `go func()` calls with no
//     backpressure (SCALE-001 S2). Steady-state concurrency is capped at the
//     worker count. Bursts past the queue are run detached but counted as
//     overflow, so saturation shows up in the health snapshot instead of being
//     either silently dropped or blocking the request handler.
//
//  2. A registry of named periodic tasks (Every) with per-task jitter and a
//     unified lifecycle, so background tickers become observable as a group and
//     stop phase-aligning into DB thundering herds (SCALE-001 S7 groundwork).
//
// Every task run is panic-isolated: a panicking background job is recovered and
// counted, never crashing the server.
package taskrunner

import (
	"log"
	"math/rand"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Runner is a bounded background-work executor. The zero value is not usable;
// construct one with New.
type Runner struct {
	workers int
	queue   chan task
	stop    chan struct{}
	wg      sync.WaitGroup

	// counters (atomic)
	submitted uint64
	completed uint64
	overflow  uint64
	panicked  uint64
	inflight  int64

	mu       sync.Mutex
	periodic []*periodicTask
	stopped  bool
}

type task struct {
	name string
	fn   func()
}

type periodicTask struct {
	name     string
	interval time.Duration
	jitter   time.Duration
	runs     uint64 // atomic
	lastRun  int64  // atomic, unix nanos
}

// New starts a Runner with the given worker count and ad-hoc queue depth.
// Non-positive values fall back to safe defaults.
func New(workers, queueSize int) *Runner {
	if workers <= 0 {
		workers = 8
	}
	if queueSize <= 0 {
		queueSize = 256
	}
	r := &Runner{
		workers: workers,
		queue:   make(chan task, queueSize),
		stop:    make(chan struct{}),
	}
	for i := 0; i < workers; i++ {
		r.wg.Add(1)
		go r.worker()
	}
	log.Printf("[INFO] [server] [taskrunner] started workers=%d queue=%d", workers, queueSize)
	return r
}

func (r *Runner) worker() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		case t := <-r.queue:
			r.run(t)
		}
	}
}

// run executes a task with panic isolation and in-flight accounting.
func (r *Runner) run(t task) {
	atomic.AddInt64(&r.inflight, 1)
	defer atomic.AddInt64(&r.inflight, -1)
	defer func() {
		if rec := recover(); rec != nil {
			atomic.AddUint64(&r.panicked, 1)
			log.Printf("[ERROR] [server] [taskrunner] task_panic name=%s recovered=%v\n%s", t.name, rec, debug.Stack())
		}
	}()
	t.fn()
	atomic.AddUint64(&r.completed, 1)
}

// Go schedules fn on the bounded pool. Under steady load concurrency is capped
// at the worker count. If the queue is full (sustained burst) fn is run in a
// detached goroutine and counted as overflow so the pressure is visible via
// Snapshot rather than silently dropped or blocking the caller.
func (r *Runner) Go(name string, fn func()) {
	atomic.AddUint64(&r.submitted, 1)
	t := task{name: name, fn: fn}
	select {
	case r.queue <- t:
	default:
		atomic.AddUint64(&r.overflow, 1)
		log.Printf("[WARN] [server] [taskrunner] queue_saturated name=%s running_detached qlen=%d", name, len(r.queue))
		go r.run(t)
	}
}

// Every registers a named periodic task that runs fn on the given interval with
// up to jitter random delay added per tick. Jitter decorrelates tickers so they
// do not phase-align into DB thundering herds. The first run happens after one
// interval (plus jitter). Registered tasks are reported by Snapshot and stopped
// by Stop.
func (r *Runner) Every(name string, interval, jitter time.Duration, fn func()) {
	pt := &periodicTask{name: name, interval: interval, jitter: jitter}
	r.mu.Lock()
	r.periodic = append(r.periodic, pt)
	r.mu.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-ticker.C:
				if jitter > 0 {
					select {
					case <-time.After(time.Duration(rand.Int63n(int64(jitter)))):
					case <-r.stop:
						return
					}
				}
				atomic.AddUint64(&pt.runs, 1)
				atomic.StoreInt64(&pt.lastRun, time.Now().UnixNano())
				r.run(task{name: name, fn: fn})
			}
		}
	}()
}

// Stop signals all workers and periodic tasks to exit and waits for them.
// Idempotent.
func (r *Runner) Stop() {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.mu.Unlock()
	close(r.stop)
	r.wg.Wait()
}

// Snapshot is a point-in-time view of the runner for health reporting.
type Snapshot struct {
	Workers   int                `json:"workers"`
	QueueLen  int                `json:"queue_len"`
	QueueCap  int                `json:"queue_cap"`
	Inflight  int64              `json:"inflight"`
	Submitted uint64             `json:"submitted"`
	Completed uint64             `json:"completed"`
	Overflow  uint64             `json:"overflow"`
	Panicked  uint64             `json:"panicked"`
	Periodic  []PeriodicSnapshot `json:"periodic"`
}

// PeriodicSnapshot reports one registered periodic task.
type PeriodicSnapshot struct {
	Name     string `json:"name"`
	Interval string `json:"interval"`
	Jitter   string `json:"jitter"`
	Runs     uint64 `json:"runs"`
	LastRun  string `json:"last_run,omitempty"`
}

// Snapshot returns the current counters and registered periodic tasks.
func (r *Runner) Snapshot() Snapshot {
	r.mu.Lock()
	periodic := make([]PeriodicSnapshot, 0, len(r.periodic))
	for _, pt := range r.periodic {
		ps := PeriodicSnapshot{
			Name:     pt.name,
			Interval: pt.interval.String(),
			Jitter:   pt.jitter.String(),
			Runs:     atomic.LoadUint64(&pt.runs),
		}
		if ln := atomic.LoadInt64(&pt.lastRun); ln > 0 {
			ps.LastRun = time.Unix(0, ln).UTC().Format(time.RFC3339)
		}
		periodic = append(periodic, ps)
	}
	r.mu.Unlock()
	return Snapshot{
		Workers:   r.workers,
		QueueLen:  len(r.queue),
		QueueCap:  cap(r.queue),
		Inflight:  atomic.LoadInt64(&r.inflight),
		Submitted: atomic.LoadUint64(&r.submitted),
		Completed: atomic.LoadUint64(&r.completed),
		Overflow:  atomic.LoadUint64(&r.overflow),
		Panicked:  atomic.LoadUint64(&r.panicked),
		Periodic:  periodic,
	}
}
