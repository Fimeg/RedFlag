package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		FailureThreshold: 3,
		FailureWindow:    time.Second,
		OpenDuration:     50 * time.Millisecond,
		HalfOpenAttempts: 2,
	}
}

func TestClosedStaysClosedOnSuccess(t *testing.T) {
	cb := New("t", testConfig())
	for i := 0; i < 10; i++ {
		if err := cb.Call(func() error { return nil }); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if cb.State() != StateClosed {
		t.Fatalf("state=%s, want closed", cb.State())
	}
}

func TestOpensAfterThresholdAndFailsFast(t *testing.T) {
	cb := New("t", testConfig())
	boom := errors.New("boom")
	for i := 0; i < 3; i++ {
		_ = cb.Call(func() error { return boom })
	}
	if cb.State() != StateOpen {
		t.Fatalf("state=%s, want open after %d failures", cb.State(), 3)
	}

	// While open, fn must not run and Call returns the breaker error.
	ran := false
	err := cb.Call(func() error { ran = true; return nil })
	if ran {
		t.Error("fn ran while breaker open — should fail fast")
	}
	if err == nil {
		t.Error("expected breaker error while open")
	}
}

func TestHalfOpenRecoversToClosed(t *testing.T) {
	cb := New("t", testConfig())
	boom := errors.New("boom")
	for i := 0; i < 3; i++ {
		_ = cb.Call(func() error { return boom })
	}
	if cb.State() != StateOpen {
		t.Fatalf("precondition: want open, got %s", cb.State())
	}

	time.Sleep(60 * time.Millisecond) // past OpenDuration → next call probes (half-open)

	// HalfOpenAttempts=2 consecutive successes needed to close.
	if err := cb.Call(func() error { return nil }); err != nil {
		t.Fatalf("probe call err: %v", err)
	}
	if err := cb.Call(func() error { return nil }); err != nil {
		t.Fatalf("second probe err: %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("state=%s, want closed after recovery", cb.State())
	}
}

func TestHalfOpenReopensOnFailure(t *testing.T) {
	cb := New("t", testConfig())
	boom := errors.New("boom")
	for i := 0; i < 3; i++ {
		_ = cb.Call(func() error { return boom })
	}
	time.Sleep(60 * time.Millisecond) // → half-open on next call

	_ = cb.Call(func() error { return boom }) // a failure in half-open reopens
	if cb.State() != StateOpen {
		t.Fatalf("state=%s, want open (half-open failure must reopen)", cb.State())
	}
	st := cb.GetStats()
	if st.State != "open" || st.NextAttempt == nil {
		t.Errorf("stats=%+v, want open with NextAttempt set", st)
	}
}
