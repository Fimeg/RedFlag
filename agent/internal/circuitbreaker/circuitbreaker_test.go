package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_NormalOperation(t *testing.T) {
	cb := New("test", Config{
		FailureThreshold: 3,
		FailureWindow:    1 * time.Minute,
		OpenDuration:     1 * time.Minute,
		HalfOpenAttempts: 2,
	})

	// Should allow calls in closed state
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cb.State() != StateClosed {
		t.Fatalf("expected state closed, got %v", cb.State())
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := New("test", Config{
		FailureThreshold: 3,
		FailureWindow:    1 * time.Minute,
		OpenDuration:     100 * time.Millisecond,
		HalfOpenAttempts: 2,
	})

	testErr := errors.New("test error")

	// Record 3 failures
	for i := 0; i < 3; i++ {
		cb.Call(func() error { return testErr })
	}

	// Should now be open
	if cb.State() != StateOpen {
		t.Fatalf("expected state open after %d failures, got %v", 3, cb.State())
	}

	// Next call should fail fast
	err := cb.Call(func() error { return nil })
	if err == nil {
		t.Fatal("expected circuit breaker to reject call, but it succeeded")
	}
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	cb := New("test", Config{
		FailureThreshold: 2,
		FailureWindow:    1 * time.Minute,
		OpenDuration:     50 * time.Millisecond,
		HalfOpenAttempts: 2,
	})

	testErr := errors.New("test error")

	// Open the circuit
	cb.Call(func() error { return testErr })
	cb.Call(func() error { return testErr })

	if cb.State() != StateOpen {
		t.Fatal("circuit should be open")
	}

	// Wait for open duration
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow call
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Fatalf("expected call to succeed in half-open state, got %v", err)
	}

	if cb.State() != StateHalfOpen {
		t.Fatalf("expected half-open state, got %v", cb.State())
	}

	// One more success should close it
	cb.Call(func() error { return nil })

	if cb.State() != StateClosed {
		t.Fatalf("expected closed state after %d successes, got %v", 2, cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := New("test", Config{
		FailureThreshold: 2,
		FailureWindow:    1 * time.Minute,
		OpenDuration:     50 * time.Millisecond,
		HalfOpenAttempts: 2,
	})

	testErr := errors.New("test error")

	// Open the circuit
	cb.Call(func() error { return testErr })
	cb.Call(func() error { return testErr })

	// Wait and attempt in half-open
	time.Sleep(60 * time.Millisecond)
	cb.Call(func() error { return nil }) // Half-open

	// Fail in half-open - should go back to open
	cb.Call(func() error { return testErr })

	if cb.State() != StateOpen {
		t.Fatalf("expected open state after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := New("test-subsystem", Config{
		FailureThreshold: 3,
		FailureWindow:    1 * time.Minute,
		OpenDuration:     1 * time.Minute,
		HalfOpenAttempts: 2,
	})

	stats := cb.GetStats()
	if stats.Name != "test-subsystem" {
		t.Fatalf("expected name 'test-subsystem', got %s", stats.Name)
	}
	if stats.State != "closed" {
		t.Fatalf("expected state 'closed', got %s", stats.State)
	}
	if stats.RecentFailures != 0 {
		t.Fatalf("expected 0 failures, got %d", stats.RecentFailures)
	}
}
