package scheduler

import (
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

func TestScheduler_NewScheduler(t *testing.T) {
	config := DefaultConfig()
	s := NewScheduler(config, nil, nil, nil, nil)

	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}

	if s.config.NumWorkers != 10 {
		t.Fatalf("expected 10 workers, got %d", s.config.NumWorkers)
	}

	if s.queue == nil {
		t.Fatal("queue not initialized")
	}

	if len(s.workers) != config.NumWorkers {
		t.Fatalf("expected %d workers, got %d", config.NumWorkers, len(s.workers))
	}
}

func TestScheduler_DefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.CheckInterval != 10*time.Second {
		t.Fatalf("expected check interval 10s, got %v", config.CheckInterval)
	}

	if config.LookaheadWindow != 60*time.Second {
		t.Fatalf("expected lookahead 60s, got %v", config.LookaheadWindow)
	}

	if config.MaxJitter != 30*time.Second {
		t.Fatalf("expected max jitter 30s, got %v", config.MaxJitter)
	}

	if config.NumWorkers != 10 {
		t.Fatalf("expected 10 workers, got %d", config.NumWorkers)
	}

	if config.BackpressureThreshold != 5 {
		t.Fatalf("expected backpressure threshold 5, got %d", config.BackpressureThreshold)
	}

	if config.RateLimitPerSecond != 100 {
		t.Fatalf("expected rate limit 100/s, got %d", config.RateLimitPerSecond)
	}
}

func TestScheduler_QueueIntegration(t *testing.T) {
	config := DefaultConfig()
	s := NewScheduler(config, nil, nil, nil, nil)

	// Add jobs to queue
	agent1 := uuid.Must(uuid.NewV4())
	agent2 := uuid.Must(uuid.NewV4())

	job1 := &SubsystemJob{
		AgentID:         agent1,
		AgentHostname:   "agent-01",
		Subsystem:       "updates",
		IntervalMinutes: 15,
		NextRunAt:       time.Now().Add(5 * time.Minute),
	}

	job2 := &SubsystemJob{
		AgentID:         agent2,
		AgentHostname:   "agent-02",
		Subsystem:       "storage",
		IntervalMinutes: 15,
		NextRunAt:       time.Now().Add(10 * time.Minute),
	}

	s.queue.Push(job1)
	s.queue.Push(job2)

	if s.queue.Len() != 2 {
		t.Fatalf("expected queue len 2, got %d", s.queue.Len())
	}

	// Get stats
	stats := s.GetQueueStats()
	if stats.Size != 2 {
		t.Fatalf("expected stats size 2, got %d", stats.Size)
	}
}

func TestScheduler_GetStats(t *testing.T) {
	config := DefaultConfig()
	s := NewScheduler(config, nil, nil, nil, nil)

	// Initial stats should be zero
	stats := s.GetStats()

	if stats.JobsProcessed != 0 {
		t.Fatalf("expected 0 jobs processed, got %d", stats.JobsProcessed)
	}

	if stats.CommandsCreated != 0 {
		t.Fatalf("expected 0 commands created, got %d", stats.CommandsCreated)
	}

	if stats.BackpressureSkips != 0 {
		t.Fatalf("expected 0 backpressure skips, got %d", stats.BackpressureSkips)
	}

	// Manually update stats (simulating processing)
	s.mu.Lock()
	s.stats.JobsProcessed = 100
	s.stats.CommandsCreated = 95
	s.stats.BackpressureSkips = 5
	s.mu.Unlock()

	stats = s.GetStats()

	if stats.JobsProcessed != 100 {
		t.Fatalf("expected 100 jobs processed, got %d", stats.JobsProcessed)
	}

	if stats.CommandsCreated != 95 {
		t.Fatalf("expected 95 commands created, got %d", stats.CommandsCreated)
	}

	if stats.BackpressureSkips != 5 {
		t.Fatalf("expected 5 backpressure skips, got %d", stats.BackpressureSkips)
	}
}

func TestScheduler_StartStop(t *testing.T) {
	config := Config{
		CheckInterval:         100 * time.Millisecond, // Fast for testing
		LookaheadWindow:       60 * time.Second,
		MaxJitter:             1 * time.Second,
		NumWorkers:            2,
		BackpressureThreshold: 5,
		RateLimitPerSecond:    0, // Disable rate limiting for test
	}

	s := NewScheduler(config, nil, nil, nil, nil)

	// Start scheduler
	err := s.Start()
	if err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}

	// Let it run for a bit
	time.Sleep(500 * time.Millisecond)

	// Stop scheduler
	err = s.Stop()
	if err != nil {
		t.Fatalf("failed to stop scheduler: %v", err)
	}

	// Should stop cleanly
}

func TestScheduler_ProcessQueueEmpty(t *testing.T) {
	config := DefaultConfig()
	s := NewScheduler(config, nil, nil, nil, nil)

	// Process empty queue should not panic
	s.processQueue()

	stats := s.GetStats()
	if stats.JobsProcessed != 0 {
		t.Fatalf("expected 0 jobs processed on empty queue, got %d", stats.JobsProcessed)
	}
}

func TestScheduler_ProcessQueueWithJobs(t *testing.T) {
	config := Config{
		CheckInterval:         1 * time.Second,
		LookaheadWindow:       60 * time.Second,
		MaxJitter:             5 * time.Second,
		NumWorkers:            2,
		BackpressureThreshold: 5,
		RateLimitPerSecond:    0, // Disable for test
	}

	s := NewScheduler(config, nil, nil, nil, nil)

	// Add jobs that are due now
	for i := 0; i < 5; i++ {
		job := &SubsystemJob{
			AgentID:         uuid.Must(uuid.NewV4()),
			AgentHostname:   "test-agent",
			Subsystem:       "updates",
			IntervalMinutes: 15,
			NextRunAt:       time.Now(), // Due now
		}
		s.queue.Push(job)
	}

	if s.queue.Len() != 5 {
		t.Fatalf("expected 5 jobs in queue, got %d", s.queue.Len())
	}

	// Process the queue
	s.processQueue()

	// Jobs should be dispatched to job channel
	// Note: Without database, workers can't actually process them
	// But we can verify they were dispatched

	stats := s.GetStats()
	if stats.JobsProcessed == 0 {
		t.Fatal("expected some jobs to be processed")
	}
}

func TestScheduler_RateLimiterRefill(t *testing.T) {
	config := Config{
		CheckInterval:         1 * time.Second,
		LookaheadWindow:       60 * time.Second,
		MaxJitter:             1 * time.Second,
		NumWorkers:            2,
		BackpressureThreshold: 5,
		RateLimitPerSecond:    10, // 10 tokens per second
	}

	s := NewScheduler(config, nil, nil, nil, nil)

	if s.rateLimiter == nil {
		t.Fatal("rate limiter not initialized")
	}

	// Start refill goroutine
	go s.refillRateLimiter()

	// Wait for some tokens to be added
	time.Sleep(200 * time.Millisecond)

	// Should have some tokens available
	tokensAvailable := 0
	for i := 0; i < 15; i++ {
		select {
		case <-s.rateLimiter:
			tokensAvailable++
		default:
			break
		}
	}

	if tokensAvailable == 0 {
		t.Fatal("expected some tokens to be available after refill")
	}

	// Should not exceed buffer size (10)
	if tokensAvailable > 10 {
		t.Fatalf("token bucket overflowed: got %d tokens, max is 10", tokensAvailable)
	}
}

func TestScheduler_ConcurrentQueueAccess(t *testing.T) {
	config := DefaultConfig()
	s := NewScheduler(config, nil, nil, nil, nil)

	done := make(chan bool)

	// Concurrent pushes
	go func() {
		for i := 0; i < 100; i++ {
			job := &SubsystemJob{
				AgentID:         uuid.Must(uuid.NewV4()),
				Subsystem:       "updates",
				IntervalMinutes: 15,
				NextRunAt:       time.Now(),
			}
			s.queue.Push(job)
		}
		done <- true
	}()

	// Concurrent stats reads
	go func() {
		for i := 0; i < 100; i++ {
			s.GetStats()
			s.GetQueueStats()
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done

	// Should not panic and should have queued jobs
	if s.queue.Len() <= 0 {
		t.Fatal("expected jobs in queue after concurrent pushes")
	}
}

// TestGroupSubsystemsByAgent exercises the pure grouping helper that
// LoadSubsystems uses after the batch query (SCALE-001 S10). No DB needed.
func TestGroupSubsystemsByAgent(t *testing.T) {
	agentA := uuid.Must(uuid.NewV4())
	agentB := uuid.Must(uuid.NewV4())
	agentC := uuid.Must(uuid.NewV4()) // agent with no rows

	rows := []models.AgentSubsystem{
		{AgentID: agentA, Subsystem: "apt"},
		{AgentID: agentA, Subsystem: "storage"},
		{AgentID: agentB, Subsystem: "dnf"},
	}

	got := groupSubsystemsByAgent(rows)

	// agentA should have exactly 2 rows
	if len(got[agentA]) != 2 {
		t.Fatalf("agentA: expected 2 subsystems, got %d", len(got[agentA]))
	}

	// agentB should have exactly 1 row
	if len(got[agentB]) != 1 {
		t.Fatalf("agentB: expected 1 subsystem, got %d", len(got[agentB]))
	}
	if got[agentB][0].Subsystem != "dnf" {
		t.Fatalf("agentB: expected subsystem=dnf, got %s", got[agentB][0].Subsystem)
	}

	// agentC has no rows — map lookup should return nil slice (zero jobs)
	if len(got[agentC]) != 0 {
		t.Fatalf("agentC: expected 0 subsystems, got %d", len(got[agentC]))
	}

	// empty input should return empty map
	empty := groupSubsystemsByAgent(nil)
	if len(empty) != 0 {
		t.Fatalf("empty input: expected empty map, got len=%d", len(empty))
	}
}

// TestGroupSubsystemsByAgent_EmptyInput confirms nil/empty slice input is safe.
func TestGroupSubsystemsByAgent_EmptyInput(t *testing.T) {
	got := groupSubsystemsByAgent([]models.AgentSubsystem{})
	if got == nil {
		t.Fatal("expected non-nil map for empty input")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got len=%d", len(got))
	}
}

func BenchmarkScheduler_ProcessQueue(b *testing.B) {
	config := DefaultConfig()
	s := NewScheduler(config, nil, nil, nil, nil)

	// Pre-fill queue with jobs
	for i := 0; i < 1000; i++ {
		job := &SubsystemJob{
			AgentID:         uuid.Must(uuid.NewV4()),
			Subsystem:       "updates",
			IntervalMinutes: 15,
			NextRunAt:       time.Now(),
		}
		s.queue.Push(job)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.processQueue()
	}
}
