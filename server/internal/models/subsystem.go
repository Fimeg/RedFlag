package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// AgentSubsystem represents a subsystem configuration for an agent
type AgentSubsystem struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	AgentID         uuid.UUID  `json:"agent_id" db:"agent_id"`
	Subsystem       string     `json:"subsystem" db:"subsystem"`
	Enabled         bool       `json:"enabled" db:"enabled"`
	IntervalMinutes int        `json:"interval_minutes" db:"interval_minutes"`
	AutoRun         bool       `json:"auto_run" db:"auto_run"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty" db:"last_run_at"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty" db:"next_run_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// SubsystemType represents the type of subsystem
type SubsystemType string

const (
	SubsystemStorage SubsystemType = "storage"
	SubsystemSystem  SubsystemType = "system"
	SubsystemDocker  SubsystemType = "docker"
)

// SubsystemConfig represents the configuration for updating a subsystem
type SubsystemConfig struct {
	Enabled         *bool `json:"enabled,omitempty"`
	IntervalMinutes *int  `json:"interval_minutes,omitempty"`
	AutoRun         *bool `json:"auto_run,omitempty"`
}

// SubsystemStats provides statistics about a subsystem's execution
type SubsystemStats struct {
	Subsystem       string     `json:"subsystem"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	IntervalMinutes int        `json:"interval_minutes"`
	AutoRun         bool       `json:"auto_run"`
	RunCount        int        `json:"run_count"`        // Total runs
	LastStatus      string     `json:"last_status"`      // Last command status
	LastDuration    int        `json:"last_duration"`    // Last run duration in seconds
}
