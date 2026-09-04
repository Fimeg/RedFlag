package circuitbreaker

import (
	"fmt"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota // Normal operation
	StateOpen                // Circuit is open, failing fast
	StateHalfOpen            // Testing if service recovered
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

// Config holds circuit breaker configuration
type Config struct {
	FailureThreshold int           // Number of failures before opening
	FailureWindow    time.Duration // Time window to track failures
	OpenDuration     time.Duration // How long circuit stays open
	HalfOpenAttempts int           // Successful attempts needed to close from half-open
}

// CircuitBreaker implements the circuit breaker pattern for subsystems
type CircuitBreaker struct {
	name   string
	config Config

	mu               sync.RWMutex
	state            State
	failures         []time.Time // Timestamps of recent failures
	consecutiveSuccess int       // Consecutive successes in half-open state
	openedAt         time.Time   // When circuit was opened
}

// New creates a new circuit breaker
func New(name string, config Config) *CircuitBreaker {
	return &CircuitBreaker{
		name:     name,
		config:   config,
		state:    StateClosed,
		failures: make([]time.Time, 0),
	}
}

// Call executes the given function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	// Check if we can execute
	if err := cb.beforeCall(); err != nil {
		return err
	}

	// Execute the function
	err := fn()

	// Record the result
	cb.afterCall(err)

	return err
}

// beforeCall checks if the call should be allowed
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// Normal operation, allow call
		return nil

	case StateOpen:
		// Check if enough time has passed to try half-open
		if time.Since(cb.openedAt) >= cb.config.OpenDuration {
			cb.state = StateHalfOpen
			cb.consecutiveSuccess = 0
			return nil
		}
		// Circuit is still open, fail fast
		return fmt.Errorf("circuit breaker [%s] is OPEN (will retry at %s)",
			cb.name, cb.openedAt.Add(cb.config.OpenDuration).Format("15:04:05"))

	case StateHalfOpen:
		// In half-open state, allow limited attempts
		return nil

	default:
		return fmt.Errorf("unknown circuit breaker state")
	}
}

// afterCall records the result and updates state
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	if err != nil {
		// Record failure
		cb.recordFailure(now)

		// If in half-open, go back to open on any failure
		if cb.state == StateHalfOpen {
			cb.state = StateOpen
			cb.openedAt = now
			cb.consecutiveSuccess = 0
			return
		}

		// Check if we should open the circuit
		if cb.shouldOpen(now) {
			cb.state = StateOpen
			cb.openedAt = now
			cb.consecutiveSuccess = 0
		}
	} else {
		// Success
		switch cb.state {
		case StateHalfOpen:
			// Count consecutive successes
			cb.consecutiveSuccess++
			if cb.consecutiveSuccess >= cb.config.HalfOpenAttempts {
				// Enough successes, close the circuit
				cb.state = StateClosed
				cb.failures = make([]time.Time, 0)
				cb.consecutiveSuccess = 0
			}

		case StateClosed:
			// Clean up old failures on success
			cb.cleanupOldFailures(now)
		}
	}
}

// recordFailure adds a failure timestamp
func (cb *CircuitBreaker) recordFailure(now time.Time) {
	cb.failures = append(cb.failures, now)
	cb.cleanupOldFailures(now)
}

// cleanupOldFailures removes failures outside the window
func (cb *CircuitBreaker) cleanupOldFailures(now time.Time) {
	cutoff := now.Add(-cb.config.FailureWindow)
	validFailures := make([]time.Time, 0)

	for _, failTime := range cb.failures {
		if failTime.After(cutoff) {
			validFailures = append(validFailures, failTime)
		}
	}

	cb.failures = validFailures
}

// shouldOpen determines if circuit should open based on failures
func (cb *CircuitBreaker) shouldOpen(now time.Time) bool {
	cb.cleanupOldFailures(now)
	return len(cb.failures) >= cb.config.FailureThreshold
}

// State returns the current circuit breaker state (thread-safe)
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns current circuit breaker statistics
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
		nextAttempt := cb.openedAt.Add(cb.config.OpenDuration)
		stats.NextAttempt = &nextAttempt
	}

	return stats
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = make([]time.Time, 0)
	cb.consecutiveSuccess = 0
	cb.openedAt = time.Time{}
}

// Stats holds circuit breaker statistics
type Stats struct {
	Name               string
	State              string
	RecentFailures     int
	ConsecutiveSuccess int
	NextAttempt        *time.Time
}

// String returns a human-readable representation of the stats
func (s Stats) String() string {
	if s.NextAttempt != nil {
		return fmt.Sprintf("[%s] state=%s, failures=%d, next_attempt=%s",
			s.Name, s.State, s.RecentFailures, s.NextAttempt.Format("15:04:05"))
	}
	return fmt.Sprintf("[%s] state=%s, failures=%d, success=%d",
		s.Name, s.State, s.RecentFailures, s.ConsecutiveSuccess)
}
