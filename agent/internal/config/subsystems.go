package config

import "time"

// SubsystemConfig holds configuration for individual subsystems
type SubsystemConfig struct {
	// Execution settings
	Enabled bool          `json:"enabled"`
	Timeout time.Duration `json:"timeout"` // Timeout for this subsystem

	// Interval for this subsystem (in minutes)
	// This controls how often the server schedules scans for this subsystem
	IntervalMinutes int `json:"interval_minutes,omitempty"`

	// Circuit breaker settings
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
}

// CircuitBreakerConfig holds circuit breaker settings for subsystems
type CircuitBreakerConfig struct {
	// Enabled controls whether circuit breaker is active
	Enabled bool `json:"enabled"`

	// FailureThreshold is the number of consecutive failures before opening the circuit
	FailureThreshold int `json:"failure_threshold"`

	// FailureWindow is the time window to track failures (e.g., 3 failures in 10 minutes)
	FailureWindow time.Duration `json:"failure_window"`

	// OpenDuration is how long the circuit stays open before attempting recovery
	OpenDuration time.Duration `json:"open_duration"`

	// HalfOpenAttempts is the number of test attempts in half-open state before fully closing
	HalfOpenAttempts int `json:"half_open_attempts"`
}

// SubsystemsConfig holds all subsystem configurations
type SubsystemsConfig struct {
	System  SubsystemConfig `json:"system"`  // System metrics scanner
	Updates SubsystemConfig `json:"updates"` // Virtual subsystem for package update scheduling
	APT     SubsystemConfig `json:"apt"`
	DNF     SubsystemConfig `json:"dnf"`
	Pacman  SubsystemConfig `json:"pacman"`
	Docker  SubsystemConfig `json:"docker"`
	Windows SubsystemConfig `json:"windows"`
	Winget  SubsystemConfig `json:"winget"`
	Storage SubsystemConfig `json:"storage"`
}

// GetDefaultSubsystemsConfig returns default subsystem configurations
func GetDefaultSubsystemsConfig() SubsystemsConfig {
	// Default circuit breaker config
	defaultCB := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 3,                // 3 consecutive failures
		FailureWindow:    10 * time.Minute, // within 10 minutes
		OpenDuration:     30 * time.Minute, // circuit open for 30 min
		HalfOpenAttempts: 2,                // 2 successful attempts to close circuit
	}

	// Aggressive circuit breaker for Windows Update (known to be slow/problematic)
	windowsCB := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 2, // Only 2 failures
		FailureWindow:    15 * time.Minute,
		OpenDuration:     60 * time.Minute, // Open for 1 hour
		HalfOpenAttempts: 3,
	}

	return SubsystemsConfig{
		System: SubsystemConfig{
			Enabled:         true,             // System scanner always available
			Timeout:         10 * time.Second, // System info should be fast
			IntervalMinutes: 5,                // Default: 5 minutes
			CircuitBreaker:  defaultCB,
		},
		Updates: SubsystemConfig{
			Enabled:         true,                                 // Virtual subsystem for package update scheduling
			Timeout:         0,                                    // Not used - delegates to individual package scanners
			IntervalMinutes: 720,                                  // Default: 12 hours (more reasonable for update checks)
			CircuitBreaker:  CircuitBreakerConfig{Enabled: false}, // No circuit breaker for virtual subsystem
		},
		APT: SubsystemConfig{
			Enabled:         true,
			Timeout:         30 * time.Second,
			IntervalMinutes: 15, // Default: 15 minutes
			CircuitBreaker:  defaultCB,
		},
		DNF: SubsystemConfig{
			Enabled:         true,
			Timeout:         15 * time.Minute, // TODO: Make scanner timeouts user-adjustable via settings. DNF operations can take a long time on large systems
			IntervalMinutes: 15,               // Default: 15 minutes
			CircuitBreaker:  defaultCB,
		},
		Pacman: SubsystemConfig{
			Enabled:         true,
			Timeout:         5 * time.Minute, // checkupdates syncs repo DBs into a temp dir; usually fast but can lag on slow mirrors
			IntervalMinutes: 15,              // Default: 15 minutes
			CircuitBreaker:  defaultCB,
		},
		Docker: SubsystemConfig{
			Enabled:         true,
			Timeout:         60 * time.Second, // Registry queries can be slow
			IntervalMinutes: 15,               // Default: 15 minutes
			CircuitBreaker:  defaultCB,
		},
		Windows: SubsystemConfig{
			Enabled:         true,
			Timeout:         10 * time.Minute, // Windows Update can be VERY slow
			IntervalMinutes: 15,               // Default: 15 minutes
			CircuitBreaker:  windowsCB,
		},
		Winget: SubsystemConfig{
			Enabled:         true,
			Timeout:         2 * time.Minute, // Winget has multiple retry strategies
			IntervalMinutes: 15,              // Default: 15 minutes
			CircuitBreaker:  defaultCB,
		},
		Storage: SubsystemConfig{
			Enabled:         true,
			Timeout:         10 * time.Second, // Disk info should be fast
			IntervalMinutes: 5,                // Default: 5 minutes
			CircuitBreaker:  defaultCB,
		},
	}
}
