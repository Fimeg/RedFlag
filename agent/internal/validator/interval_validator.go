package validator

import (
	"fmt"
)

// IntervalValidator provides bounds checking for agent and scanner intervals
type IntervalValidator struct {
	minCheckInSeconds int // 60 seconds (1 minute)
	maxCheckInSeconds int // 3600 seconds (1 hour)
	minScannerMinutes int // 1 minute
	maxScannerMinutes int // 1440 minutes (24 hours)
}

// NewIntervalValidator creates a validator with default bounds
func NewIntervalValidator() *IntervalValidator {
	return &IntervalValidator{
		minCheckInSeconds: 60,   // 1 minute minimum
		maxCheckInSeconds: 3600, // 1 hour maximum
		minScannerMinutes: 1,    // 1 minute minimum
		maxScannerMinutes: 1440, // 24 hours maximum
	}
}

// ValidateCheckInInterval checks if agent check-in interval is within bounds
func (v *IntervalValidator) ValidateCheckInInterval(seconds int) error {
	if seconds < v.minCheckInSeconds {
		return fmt.Errorf("check-in interval %d seconds below minimum %d seconds (1 minute)", 
			seconds, v.minCheckInSeconds)
	}
	if seconds > v.maxCheckInSeconds {
		return fmt.Errorf("check-in interval %d seconds above maximum %d seconds (1 hour)", 
			seconds, v.maxCheckInSeconds)
	}
	return nil
}

// ValidateScannerInterval checks if scanner interval is within bounds
func (v *IntervalValidator) ValidateScannerInterval(minutes int) error {
	if minutes < v.minScannerMinutes {
		return fmt.Errorf("scanner interval %d minutes below minimum %d minutes", 
			minutes, v.minScannerMinutes)
	}
	if minutes > v.maxScannerMinutes {
		return fmt.Errorf("scanner interval %d minutes above maximum %d minutes (24 hours)", 
			minutes, v.maxScannerMinutes)
	}
	return nil
}

// GetBounds returns the current validation bounds (for testing/monitoring)
func (v *IntervalValidator) GetBounds() (minCheckIn, maxCheckIn, minScanner, maxScanner int) {
	return v.minCheckInSeconds, v.maxCheckInSeconds, 
		   v.minScannerMinutes, v.maxScannerMinutes
}
