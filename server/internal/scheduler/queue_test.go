package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
)

func TestPriorityQueue_BasicOperations(t *testing.T) {
	pq := NewPriorityQueue()

	// Test empty queue
	if pq.Len() != 0 {
		t.Fatalf("expected empty queue, got len=%d", pq.Len())
	}

	if pq.Peek() != nil {
		t.Fatal("Peek on empty queue should return nil")
	}

	if pq.Pop() != nil {
		t.Fatal("Pop on empty queue should return nil")
	}

	// Push a job
	agent1 := uuid.Must(uuid.NewV4())
	job1 := &SubsystemJob{
		AgentID:         agent1,
		AgentHostname:   "agent-01",
		Subsystem:       "updates",
		IntervalMinutes: 15,
		NextRunAt:       time.Now().Add(10 * time.Minute),
	}
	pq.Push(job1)

	if pq.Len() != 1 {
		t.Fatalf("expected len=1 after push, got %d", pq.Len())
	}

	// Peek should return the job without removing it
	peeked := pq.Peek()
	if peeked == nil {
		t.Fatal("Peek should return job")
	}
	if peeked.AgentID != agent1 {
		t.Fatal("Peek returned wrong job")
	}
	if pq.Len() != 1 {
		t.Fatal("Peek should not remove job")
	}

	// Pop should return and remove the job
	popped := pq.Pop()
	if popped == nil {
		t.Fatal("Pop should return job")
	}
	if popped.AgentID != agent1 {
		t.Fatal("Pop returned wrong job")
	}
	if pq.Len() != 0 {
		t.Fatal("Pop should remove job")
	}
}

func TestPriorityQueue_Ordering(t *testing.T) {
	pq := NewPriorityQueue()
	now := time.Now()

	// Push jobs in random order
	jobs := []*SubsystemJob{
		{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "updates",
			NextRunAt: now.Add(30 * time.Minute), // Third
		},
		{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "storage",
			NextRunAt: now.Add(5 * time.Minute), // First
		},
		{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "docker",
			NextRunAt: now.Add(15 * time.Minute), // Second
		},
	}

	for _, job := range jobs {
		pq.Push(job)
	}

	// Pop should return jobs in NextRunAt order
	first := pq.Pop()
	if first.Subsystem != "storage" {
		t.Fatalf("expected 'storage' first, got '%s'", first.Subsystem)
	}

	second := pq.Pop()
	if second.Subsystem != "docker" {
		t.Fatalf("expected 'docker' second, got '%s'", second.Subsystem)
	}

	third := pq.Pop()
	if third.Subsystem != "updates" {
		t.Fatalf("expected 'updates' third, got '%s'", third.Subsystem)
	}
}

func TestPriorityQueue_UpdateExisting(t *testing.T) {
	pq := NewPriorityQueue()
	agentID := uuid.Must(uuid.NewV4())
	now := time.Now()

	// Push initial job
	job1 := &SubsystemJob{
		AgentID:         agentID,
		Subsystem:       "updates",
		IntervalMinutes: 15,
		NextRunAt:       now.Add(15 * time.Minute),
	}
	pq.Push(job1)

	if pq.Len() != 1 {
		t.Fatalf("expected len=1, got %d", pq.Len())
	}

	// Push same agent+subsystem with different NextRunAt
	job2 := &SubsystemJob{
		AgentID:         agentID,
		Subsystem:       "updates",
		IntervalMinutes: 30,
		NextRunAt:       now.Add(30 * time.Minute),
	}
	pq.Push(job2)

	// Should still be 1 job (updated, not added)
	if pq.Len() != 1 {
		t.Fatalf("expected len=1 after update, got %d", pq.Len())
	}

	// Verify the job was updated
	job := pq.Pop()
	if job.IntervalMinutes != 30 {
		t.Fatalf("expected interval=30, got %d", job.IntervalMinutes)
	}
	if !job.NextRunAt.Equal(now.Add(30 * time.Minute)) {
		t.Fatal("NextRunAt was not updated")
	}
}

func TestPriorityQueue_Remove(t *testing.T) {
	pq := NewPriorityQueue()

	agent1 := uuid.Must(uuid.NewV4())
	agent2 := uuid.Must(uuid.NewV4())

	pq.Push(&SubsystemJob{
		AgentID:   agent1,
		Subsystem: "updates",
		NextRunAt: time.Now(),
	})
	pq.Push(&SubsystemJob{
		AgentID:   agent2,
		Subsystem: "storage",
		NextRunAt: time.Now(),
	})

	if pq.Len() != 2 {
		t.Fatalf("expected len=2, got %d", pq.Len())
	}

	// Remove existing job
	removed := pq.Remove(agent1, "updates")
	if !removed {
		t.Fatal("Remove should return true for existing job")
	}
	if pq.Len() != 1 {
		t.Fatalf("expected len=1 after remove, got %d", pq.Len())
	}

	// Remove non-existent job
	removed = pq.Remove(agent1, "updates")
	if removed {
		t.Fatal("Remove should return false for non-existent job")
	}
	if pq.Len() != 1 {
		t.Fatal("Remove of non-existent job should not affect queue")
	}
}

func TestPriorityQueue_Get(t *testing.T) {
	pq := NewPriorityQueue()

	agentID := uuid.Must(uuid.NewV4())
	job := &SubsystemJob{
		AgentID:   agentID,
		Subsystem: "updates",
		NextRunAt: time.Now(),
	}
	pq.Push(job)

	// Get existing job
	retrieved := pq.Get(agentID, "updates")
	if retrieved == nil {
		t.Fatal("Get should return job")
	}
	if retrieved.AgentID != agentID {
		t.Fatal("Get returned wrong job")
	}
	if pq.Len() != 1 {
		t.Fatal("Get should not remove job")
	}

	// Get non-existent job
	retrieved = pq.Get(uuid.Must(uuid.NewV4()), "storage")
	if retrieved != nil {
		t.Fatal("Get should return nil for non-existent job")
	}
}

func TestPriorityQueue_PopBefore(t *testing.T) {
	pq := NewPriorityQueue()
	now := time.Now()

	// Add jobs with different NextRunAt times
	for i := 0; i < 5; i++ {
		pq.Push(&SubsystemJob{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "updates",
			NextRunAt: now.Add(time.Duration(i*10) * time.Minute),
		})
	}

	if pq.Len() != 5 {
		t.Fatalf("expected len=5, got %d", pq.Len())
	}

	// Pop jobs before now+25min (should get 3 jobs: 0, 10, 20 minutes)
	cutoff := now.Add(25 * time.Minute)
	jobs := pq.PopBefore(cutoff, 0) // no limit

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	// Verify all returned jobs are before cutoff
	for _, job := range jobs {
		if job.NextRunAt.After(cutoff) {
			t.Fatalf("job NextRunAt %v is after cutoff %v", job.NextRunAt, cutoff)
		}
	}

	// Queue should have 2 jobs left
	if pq.Len() != 2 {
		t.Fatalf("expected len=2 after PopBefore, got %d", pq.Len())
	}
}

func TestPriorityQueue_PopBeforeWithLimit(t *testing.T) {
	pq := NewPriorityQueue()
	now := time.Now()

	// Add 5 jobs all due now
	for i := 0; i < 5; i++ {
		pq.Push(&SubsystemJob{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "updates",
			NextRunAt: now,
		})
	}

	// Pop with limit of 3
	jobs := pq.PopBefore(now.Add(1*time.Hour), 3)

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs (limit), got %d", len(jobs))
	}

	if pq.Len() != 2 {
		t.Fatalf("expected 2 jobs remaining, got %d", pq.Len())
	}
}

func TestPriorityQueue_PeekBefore(t *testing.T) {
	pq := NewPriorityQueue()
	now := time.Now()

	pq.Push(&SubsystemJob{
		AgentID:   uuid.Must(uuid.NewV4()),
		Subsystem: "updates",
		NextRunAt: now.Add(5 * time.Minute),
	})
	pq.Push(&SubsystemJob{
		AgentID:   uuid.Must(uuid.NewV4()),
		Subsystem: "storage",
		NextRunAt: now.Add(15 * time.Minute),
	})

	// Peek before 10 minutes (should see 1 job)
	jobs := pq.PeekBefore(now.Add(10*time.Minute), 0)

	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	// Queue should still have both jobs
	if pq.Len() != 2 {
		t.Fatalf("expected len=2 after PeekBefore, got %d", pq.Len())
	}
}

func TestPriorityQueue_Clear(t *testing.T) {
	pq := NewPriorityQueue()

	// Add some jobs
	for i := 0; i < 10; i++ {
		pq.Push(&SubsystemJob{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "updates",
			NextRunAt: time.Now(),
		})
	}

	if pq.Len() != 10 {
		t.Fatalf("expected len=10, got %d", pq.Len())
	}

	// Clear the queue
	pq.Clear()

	if pq.Len() != 0 {
		t.Fatalf("expected len=0 after clear, got %d", pq.Len())
	}

	if pq.Peek() != nil {
		t.Fatal("Peek should return nil after clear")
	}
}

func TestPriorityQueue_GetStats(t *testing.T) {
	pq := NewPriorityQueue()
	now := time.Now()

	// Empty queue stats
	stats := pq.GetStats()
	if stats.Size != 0 {
		t.Fatalf("expected size=0, got %d", stats.Size)
	}
	if stats.NextRunAt != nil {
		t.Fatal("empty queue should have nil NextRunAt")
	}

	// Add jobs
	pq.Push(&SubsystemJob{
		AgentID:         uuid.Must(uuid.NewV4()),
		AgentHostname:   "agent-01",
		Subsystem:       "updates",
		NextRunAt:       now.Add(5 * time.Minute),
		IntervalMinutes: 15,
	})
	pq.Push(&SubsystemJob{
		AgentID:   uuid.Must(uuid.NewV4()),
		Subsystem: "storage",
		NextRunAt: now.Add(10 * time.Minute),
	})
	pq.Push(&SubsystemJob{
		AgentID:   uuid.Must(uuid.NewV4()),
		Subsystem: "updates",
		NextRunAt: now.Add(15 * time.Minute),
	})

	stats = pq.GetStats()

	if stats.Size != 3 {
		t.Fatalf("expected size=3, got %d", stats.Size)
	}

	if stats.NextRunAt == nil {
		t.Fatal("NextRunAt should not be nil")
	}

	// Should be the earliest job (5 minutes)
	expectedNext := now.Add(5 * time.Minute)
	if !stats.NextRunAt.Equal(expectedNext) {
		t.Fatalf("expected NextRunAt=%v, got %v", expectedNext, stats.NextRunAt)
	}

	// Check subsystem counts
	if stats.JobsBySubsystem["updates"] != 2 {
		t.Fatalf("expected 2 updates jobs, got %d", stats.JobsBySubsystem["updates"])
	}
	if stats.JobsBySubsystem["storage"] != 1 {
		t.Fatalf("expected 1 storage job, got %d", stats.JobsBySubsystem["storage"])
	}
}

func TestPriorityQueue_Concurrency(t *testing.T) {
	pq := NewPriorityQueue()
	var wg sync.WaitGroup

	// Concurrent pushes
	numGoroutines := 100
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			pq.Push(&SubsystemJob{
				AgentID:   uuid.Must(uuid.NewV4()),
				Subsystem: "updates",
				NextRunAt: time.Now().Add(time.Duration(idx) * time.Second),
			})
		}(i)
	}

	wg.Wait()

	if pq.Len() != numGoroutines {
		t.Fatalf("expected len=%d after concurrent pushes, got %d", numGoroutines, pq.Len())
	}

	// Concurrent pops
	wg.Add(numGoroutines)
	popped := make(chan *SubsystemJob, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			if job := pq.Pop(); job != nil {
				popped <- job
			}
		}()
	}

	wg.Wait()
	close(popped)

	// Count popped jobs
	count := 0
	for range popped {
		count++
	}

	if count != numGoroutines {
		t.Fatalf("expected %d popped jobs, got %d", numGoroutines, count)
	}

	if pq.Len() != 0 {
		t.Fatalf("expected empty queue after concurrent pops, got len=%d", pq.Len())
	}
}

func TestPriorityQueue_ConcurrentReadWrite(t *testing.T) {
	pq := NewPriorityQueue()
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			pq.Push(&SubsystemJob{
				AgentID:   uuid.Must(uuid.NewV4()),
				Subsystem: "updates",
				NextRunAt: time.Now(),
			})
			time.Sleep(1 * time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			pq.Peek()
			pq.GetStats()
			time.Sleep(1 * time.Microsecond)
		}
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// Should not panic and queue should be consistent
	if pq.Len() < 0 {
		t.Fatal("queue length became negative (race condition)")
	}
}

func BenchmarkPriorityQueue_Push(b *testing.B) {
	pq := NewPriorityQueue()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pq.Push(&SubsystemJob{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "updates",
			NextRunAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}
}

func BenchmarkPriorityQueue_Pop(b *testing.B) {
	pq := NewPriorityQueue()

	// Pre-fill the queue
	for i := 0; i < b.N; i++ {
		pq.Push(&SubsystemJob{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "updates",
			NextRunAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.Pop()
	}
}

func BenchmarkPriorityQueue_Peek(b *testing.B) {
	pq := NewPriorityQueue()

	// Pre-fill with 10000 jobs
	for i := 0; i < 10000; i++ {
		pq.Push(&SubsystemJob{
			AgentID:   uuid.Must(uuid.NewV4()),
			Subsystem: "updates",
			NextRunAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.Peek()
	}
}
