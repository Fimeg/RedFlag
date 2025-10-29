package models

import (
	"time"

	"github.com/google/uuid"
)

// AgentCommand represents a command to be executed by an agent
type AgentCommand struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	AgentID        uuid.UUID  `json:"agent_id" db:"agent_id"`
	CommandType    string     `json:"command_type" db:"command_type"`
	Params         JSONB      `json:"params" db:"params"`
	Status         string     `json:"status" db:"status"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	SentAt         *time.Time `json:"sent_at,omitempty" db:"sent_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	Result         JSONB      `json:"result,omitempty" db:"result"`
	RetriedFromID  *uuid.UUID `json:"retried_from_id,omitempty" db:"retried_from_id"`
}

// CommandsResponse is returned when an agent checks in for commands
type CommandsResponse struct {
	Commands       []CommandItem `json:"commands"`
	RapidPolling   *RapidPollingConfig `json:"rapid_polling,omitempty"`
}

// RapidPollingConfig contains rapid polling configuration for the agent
type RapidPollingConfig struct {
	Enabled bool   `json:"enabled"`
	Until   string `json:"until"` // ISO 8601 timestamp
}

// CommandItem represents a command in the response
type CommandItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Params JSONB  `json:"params"`
}

// Command types
const (
	CommandTypeScanUpdates        = "scan_updates"
	CommandTypeCollectSpecs       = "collect_specs"
	CommandTypeInstallUpdate      = "install_updates"
	CommandTypeDryRunUpdate       = "dry_run_update"
	CommandTypeConfirmDependencies = "confirm_dependencies"
	CommandTypeRollback           = "rollback_update"
	CommandTypeUpdateAgent        = "update_agent"
	CommandTypeEnableHeartbeat    = "enable_heartbeat"
	CommandTypeDisableHeartbeat   = "disable_heartbeat"
)

// Command statuses
const (
	CommandStatusPending   = "pending"
	CommandStatusSent      = "sent"
	CommandStatusCompleted = "completed"
	CommandStatusFailed    = "failed"
	CommandStatusTimedOut  = "timed_out"
	CommandStatusCancelled = "cancelled"
	CommandStatusRunning   = "running"
)

// ActiveCommandInfo represents information about an active command for UI display
type ActiveCommandInfo struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	AgentID       uuid.UUID  `json:"agent_id" db:"agent_id"`
	CommandType   string     `json:"command_type" db:"command_type"`
	Params        JSONB      `json:"params" db:"params"`
	Status        string     `json:"status" db:"status"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	SentAt        *time.Time `json:"sent_at,omitempty" db:"sent_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	Result        JSONB      `json:"result,omitempty" db:"result"`
	AgentHostname string     `json:"agent_hostname" db:"agent_hostname"`
	PackageName   string     `json:"package_name" db:"package_name"`
	PackageType   string     `json:"package_type" db:"package_type"`
	RetriedFromID *uuid.UUID `json:"retried_from_id,omitempty" db:"retried_from_id"`
	IsRetry       bool       `json:"is_retry" db:"is_retry"`
	HasBeenRetried bool      `json:"has_been_retried" db:"has_been_retried"`
	RetryCount    int        `json:"retry_count" db:"retry_count"`
}
