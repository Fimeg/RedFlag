package notifier

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseEventClasses(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{"empty", "", 0},
		{"single known", "security", 1},
		{"two known", "security,agent_offline", 2},
		{"with dashes", "agent-offline,update-failed", 2},
		{"mixed case and spaces", " Security , THREAT ", 2},
		{"unknown dropped", "security,bogus,threat", 2},
		{"all unknown", "foo,bar", 0},
		{"all five", "security,agent_offline,update_failed,threat,eol", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEventClasses(tt.raw)
			if len(got) != tt.want {
				t.Errorf("ParseEventClasses(%q) = %d classes, want %d: %v", tt.raw, len(got), tt.want, got)
			}
		})
	}
}

func TestSeverityPasses(t *testing.T) {
	tests := []struct {
		filter   string
		severity string
		want     bool
	}{
		// Empty/any filter passes everything
		{"", "info", true},
		{"info,warning,error,critical", "info", true},
		// Exact match
		{"error,critical", "error", true},
		{"error,critical", "critical", true},
		// Below threshold
		{"error,critical", "warning", false},
		{"error,critical", "info", false},
		// Single filter
		{"warning", "warning", true},
		{"warning", "error", false},
	}

	for _, tt := range tests {
		t.Run(tt.filter+"_"+tt.severity, func(t *testing.T) {
			got := SeverityPasses(tt.filter, tt.severity)
			if got != tt.want {
				t.Errorf("SeverityPasses(%q, %q) = %v, want %v", tt.filter, tt.severity, got, tt.want)
			}
		})
	}
}

func TestDispatcherDedup(t *testing.T) {
	d := NewDispatcher(5 * time.Second) // long window so dups are caught

	cs := &countingSink{}
	d.Register(cs, ClassSecurity)

	event := NotifyEvent{
		Class:       ClassSecurity,
		Severity:    "error",
		Title:       "Test",
		Message:     "test message",
		Fingerprint: "test-dedup-fp",
	}

	// First dispatch — goes through, goroutine fires Send.
	d.Dispatch(context.Background(), event)
	// Second dispatch — dedup map hit, goroutine NOT fired.
	d.Dispatch(context.Background(), event)
	// Third — same.
	d.Dispatch(context.Background(), event)

	// Let goroutines settle.
	time.Sleep(20 * time.Millisecond)

	if cs.count.Load() != 1 {
		t.Errorf("expected exactly 1 delivery (dedup suppressed the rest), got %d", cs.count.Load())
	}
}

func TestDispatcherNoDedupDifferentFP(t *testing.T) {
	d := NewDispatcher(5 * time.Second)

	cs := &countingSink{}
	d.Register(cs, ClassSecurity)

	d.Dispatch(context.Background(), NotifyEvent{
		Class: ClassSecurity, Severity: "error", Title: "A", Message: "msg A", Fingerprint: "fp-a",
	})
	d.Dispatch(context.Background(), NotifyEvent{
		Class: ClassSecurity, Severity: "error", Title: "B", Message: "msg B", Fingerprint: "fp-b",
	})

	time.Sleep(20 * time.Millisecond)

	if cs.count.Load() != 2 {
		t.Errorf("expected 2 deliveries for different fingerprints, got %d", cs.count.Load())
	}
}

type countingSink struct {
	count atomic.Int64
}

func (s *countingSink) ID() string { return "test" }
func (s *countingSink) Send(_ context.Context, _ NotifyEvent) error {
	s.count.Add(1)
	return nil
}
