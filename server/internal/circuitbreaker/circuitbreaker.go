// Package circuitbreaker is the server-side circuit breaker (SCALE-001 S8).
// It mirrors the agent's breaker (agent/internal/circuitbreaker) — same states,
// config, and semantics — so the two halves of RedFlag behave identically. The
// server uses it to wrap fragile outbound dependencies (OSV.dev, repology.org):
// when an upstream is failing or slow, the breaker opens and calls fail fast
// instead of piling goroutines and timing out one by one. Callers decide the
// fail direction; the supply-chain path fails OPEN (sovereignty) on an open
// breaker, exactly as it does on a transport error.
package circuitbreaker

import (
	"fmt"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	StateClosed   State = iota // Normal operation
	StateOpen                  // Failing fast
	StateHalfOpen              // Probing recovery
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration.
type Config struct {
	FailureThreshold int           // Failures within the window before opening
	FailureWindow    time.Duration // Window over which failures are counted
	OpenDuration     time.Duration // How long the circuit stays open before probing
	HalfOpenAttempts int           // Consecutive successes needed to close from half-open
}

// CircuitBreaker implements the breaker pattern for a single dependency.
type CircuitBreaker struct {
	name   string
	config Config

	mu                 sync.RWMutex
	state              State
	failures           []time.Time
	consecutiveSuccess int
	openedAt           time.Time
}

// New creates a closed circuit breaker.
func New(name string, config Config) *CircuitBreaker {
	return &CircuitBreaker{
		name:     name,
		config:   config,
		state:    StateClosed,
		failures: make([]time.Time, 0),
	}
}

// Call runs fn under breaker protection. When the breaker is open it returns the
// breaker error without invoking fn.
func (cb *CircuitBreaker) Call(fn func() error) error {
	if err := cb.beforeCall(); err != nil {
		return err
	}
	err := fn()
	cb.afterCall(err)
	return err
}

func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		if time.Since(cb.openedAt) >= cb.config.OpenDuration {
			cb.state = StateHalfOpen
			cb.consecutiveSuccess = 0
			return nil
		}
		return fmt.Errorf("circuit breaker [%s] is OPEN (will retry at %s)",
			cb.name, cb.openedAt.Add(cb.config.OpenDuration).Format("15:04:05"))
	case StateHalfOpen:
		return nil
	default:
		return fmt.Errorf("unknown circuit breaker state")
	}
}

func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	if err != nil {
		cb.recordFailure(now)
		if cb.state == StateHalfOpen {
			cb.state = StateOpen
			cb.openedAt = now
			cb.consecutiveSuccess = 0
			return
		}
		if cb.shouldOpen(now) {
			cb.state = StateOpen
			cb.openedAt = now
			cb.consecutiveSuccess = 0
		}
		return
	}

	switch cb.state {
	case StateHalfOpen:
		cb.consecutiveSuccess++
		if cb.consecutiveSuccess >= cb.config.HalfOpenAttempts {
			cb.state = StateClosed
			cb.failures = make([]time.Time, 0)
			cb.consecutiveSuccess = 0
		}
	case StateClosed:
		cb.cleanupOldFailures(now)
	}
}

func (cb *CircuitBreaker) recordFailure(now time.Time) {
	cb.failures = append(cb.failures, now)
	cb.cleanupOldFailures(now)
}

func (cb *CircuitBreaker) cleanupOldFailures(now time.Time) {
	cutoff := now.Add(-cb.config.FailureWindow)
	valid := make([]time.Time, 0, len(cb.failures))
	for _, t := range cb.failures {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	cb.failures = valid
}

func (cb *CircuitBreaker) shouldOpen(now time.Time) bool {
	cb.cleanupOldFailures(now)
	return len(cb.failures) >= cb.config.FailureThreshold
}

// State returns the current state (thread-safe).
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats is a point-in-time view for health/metrics (OBS-001).
type Stats struct {
	Name               string     `json:"name"`
	State              string     `json:"state"`
	RecentFailures     int        `json:"recent_failures"`
	ConsecutiveSuccess int        `json:"consecutive_success"`
	NextAttempt        *time.Time `json:"next_attempt,omitempty"`
}

// GetStats returns the current breaker statistics (thread-safe).
func (cb *CircuitBreaker) GetStats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	stats := Stats{
		Name:               cb.name,
		State:              cb.state.String(),
		RecentFailures:     len(cb.failures),
		ConsecutiveSuccess: cb.consecutiveSuccess,
	}
	if cb.state == StateOpen && !cb.openedAt.IsZero() {
		next := cb.openedAt.Add(cb.config.OpenDuration)
		stats.NextAttempt = &next
	}
	return stats
}

// Reset forces the breaker back to closed (manual recovery / tests).
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = make([]time.Time, 0)
	cb.consecutiveSuccess = 0
	cb.openedAt = time.Time{}
}
