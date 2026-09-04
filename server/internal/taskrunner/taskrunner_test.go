package taskrunner

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGo_RunsAllSubmittedTasks(t *testing.T) {
	r := New(4, 64)
	defer r.Stop()

	const n = 100
	var got int64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		r.Go("unit", func() {
			atomic.AddInt64(&got, 1)
			wg.Done()
		})
	}
	wg.Wait()

	if got != n {
		t.Fatalf("ran %d tasks, want %d", got, n)
	}
	s := r.Snapshot()
	if s.Submitted != n {
		t.Errorf("submitted=%d, want %d", s.Submitted, n)
	}
	if s.Completed != n {
		t.Errorf("completed=%d, want %d", s.Completed, n)
	}
}

func TestRun_PanicIsIsolatedAndCounted(t *testing.T) {
	r := New(2, 8)
	defer r.Stop()

	var done sync.WaitGroup
	done.Add(2)
	r.Go("boom", func() { defer done.Done(); panic("kaboom") })
	r.Go("ok", func() { defer done.Done() })
	done.Wait()

	// Give the panicking task's deferred counters a moment to settle.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if r.Snapshot().Panicked >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if p := r.Snapshot().Panicked; p != 1 {
		t.Errorf("panicked=%d, want 1", p)
	}
}

func TestGo_OverflowRunsDetachedAndIsCounted(t *testing.T) {
	// One worker, queue of one, blocked work forces overflow on later submits.
	r := New(1, 1)
	defer r.Stop()

	release := make(chan struct{})
	var ran int64
	var wg sync.WaitGroup

	// Occupy the single worker.
	wg.Add(1)
	r.Go("blocker", func() {
		defer wg.Done()
		atomic.AddInt64(&ran, 1)
		<-release
	})
	// Let the worker pick up the blocker so it's no longer in the queue.
	time.Sleep(20 * time.Millisecond)

	// Submit more than the queue can hold; the excess must overflow (detached),
	// never be dropped.
	const extra = 20
	wg.Add(extra)
	for i := 0; i < extra; i++ {
		r.Go("extra", func() {
			defer wg.Done()
			atomic.AddInt64(&ran, 1)
		})
	}

	close(release)
	wg.Wait()

	if ran != extra+1 {
		t.Fatalf("ran %d tasks, want %d (no work lost)", ran, extra+1)
	}
	if r.Snapshot().Overflow == 0 {
		t.Error("expected overflow > 0 under saturation")
	}
}

func TestEvery_RunsAndIsReported(t *testing.T) {
	r := New(2, 8)
	defer r.Stop()

	var hits int64
	r.Every("tick", 10*time.Millisecond, 0, func() { atomic.AddInt64(&hits, 1) })

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&hits) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if atomic.LoadInt64(&hits) < 2 {
		t.Fatalf("periodic task ran %d times, want >= 2", hits)
	}
	s := r.Snapshot()
	if len(s.Periodic) != 1 || s.Periodic[0].Name != "tick" {
		t.Fatalf("periodic snapshot = %+v, want one task named tick", s.Periodic)
	}
	if s.Periodic[0].Runs < 2 {
		t.Errorf("periodic runs=%d, want >= 2", s.Periodic[0].Runs)
	}
}

func TestStop_IsIdempotent(t *testing.T) {
	r := New(2, 4)
	r.Stop()
	r.Stop() // must not panic on double close
}
