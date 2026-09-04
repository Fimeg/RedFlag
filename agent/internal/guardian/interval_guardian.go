package guardian

import (
	"fmt"
	"sync"
)

// IntervalGuardian protects against accidental check-in interval overrides
type IntervalGuardian struct {
	mu               sync.Mutex
	lastCheckInValue int
	violationCount   int
}

// NewIntervalGuardian creates a new guardian with zero violations
func NewIntervalGuardian() *IntervalGuardian {
	return &IntervalGuardian{
		lastCheckInValue: 0,
		violationCount:   0,
	}
}

// SetBaseline records the expected check-in interval
func (g *IntervalGuardian) SetBaseline(interval int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastCheckInValue = interval
}

// CheckForOverrideAttempt validates that proposed interval matches baseline
// Returns error if mismatch detected (indicating a regression)
func (g *IntervalGuardian) CheckForOverrideAttempt(currentBaseline, proposedValue int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if currentBaseline != proposedValue {
		g.violationCount++
		return fmt.Errorf("INTERVAL_OVERRIDE_DETECTED: baseline=%d, proposed=%d, violations=%d", 
			currentBaseline, proposedValue, g.violationCount)
	}
	return nil
}

// GetViolationCount returns total number of violations detected
func (g *IntervalGuardian) GetViolationCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.violationCount
}

// Reset clears violation count (use after legitimate config change)
func (g *IntervalGuardian) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.violationCount = 0
}

// GetBaseline returns current baseline value
func (g *IntervalGuardian) GetBaseline() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lastCheckInValue
}
