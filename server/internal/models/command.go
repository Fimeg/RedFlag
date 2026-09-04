package models

import (
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
)

// AgentCommand represents a command to be executed by an agent
type AgentCommand struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	AgentID        uuid.UUID  `json:"agent_id" db:"agent_id"`
	CommandType    string     `json:"command_type" db:"command_type"`
	Params         JSONB      `json:"params" db:"params"`
	Status         string     `json:"status" db:"status"`
	Source         string     `json:"source" db:"source"`
	Signature      string     `json:"signature,omitempty" db:"signature"`
	KeyID          string     `json:"key_id,omitempty" db:"key_id"`
	SignedAt       *time.Time `json:"signed_at,omitempty" db:"signed_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	IdempotencyKey *string    `json:"idempotency_key,omitempty" db:"idempotency_key"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	SentAt         *time.Time `json:"sent_at,omitempty" db:"sent_at"`
	ReceivedAt     *time.Time `json:"received_at,omitempty" db:"received_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	Result         JSONB      `json:"result,omitempty" db:"result"`
	RetriedFromID  *uuid.UUID `json:"retried_from_id,omitempty" db:"retried_from_id"`
	RetryCount     int        `json:"retry_count" db:"retry_count"`
}

// Validate checks if the command has all required fields
func (c *AgentCommand) Validate() error {
	if c.ID == uuid.Nil {
		return ErrCommandIDRequired
	}
	if c.AgentID == uuid.Nil {
		return ErrAgentIDRequired
	}
	if c.CommandType == "" {
		return ErrCommandTypeRequired
	}
	if c.Status == "" {
		return ErrStatusRequired
	}
	if c.Source != "manual" && c.Source != "system" {
		return ErrInvalidSource
	}
	return nil
}

// IsTerminal returns true if the command is in a terminal state
func (c *AgentCommand) IsTerminal() bool {
	return c.Status == "completed" || c.Status == "failed" || c.Status == "cancelled"
}

// CanRetry returns true if the command can be retried
func (c *AgentCommand) CanRetry() bool {
	return c.Status == "failed" && c.RetriedFromID == nil
}

// Predefined errors for validation
var (
	ErrCommandIDRequired = errors.New("command ID cannot be zero UUID")
	ErrAgentIDRequired   = errors.New("agent ID is required")
	ErrCommandTypeRequired = errors.New("command type is required")
	ErrStatusRequired    = errors.New("status is required")
	ErrInvalidSource     = errors.New("source must be 'manual' or 'system'")
)

// CommandsResponse is returned when an agent checks in for commands
type CommandsResponse struct {
	Commands             []CommandItem      `json:"commands"`
	RapidPolling         *RapidPollingConfig `json:"rapid_polling,omitempty"`
	AcknowledgedIDs      []string           `json:"acknowledged_ids,omitempty"` // Result IDs server has recorded
	ReceiptConfirmedIDs  []string           `json:"receipt_confirmed_ids,omitempty"` // Command IDs server transitioned sent->received this turn
	ConfirmedCommandIDs  []string           `json:"confirmed_command_ids,omitempty"` // Command IDs server has confirmed completed via ReportLog
}

// RapidPollingConfig contains rapid polling configuration for the agent
type RapidPollingConfig struct {
	Enabled bool   `json:"enabled"`
	Until   string `json:"until"` // ISO 8601 timestamp
}

// CommandItem represents a command in the response
type CommandItem struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Params    JSONB      `json:"params"`
	Signature string     `json:"signature,omitempty"`
	KeyID     string     `json:"key_id,omitempty"`
	SignedAt  *time.Time `json:"signed_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	AgentID   string     `json:"agent_id,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// Command types
const (
	CommandTypeCollectSpecs       = "collect_specs"
	CommandTypeInstallUpdate      = "install_updates"
	CommandTypeDryRunUpdate       = "dry_run_update"
	CommandTypeConfirmDependencies = "confirm_dependencies"
	CommandTypeRollback           = "rollback_update"
	CommandTypeUpdateAgent        = "update_agent"
	CommandTypeEnableHeartbeat    = "enable_heartbeat"
	CommandTypeDisableHeartbeat   = "disable_heartbeat"
	CommandTypeReboot             = "reboot"
	CommandTypeCaptureScreenshot  = "capture_screenshot"
	CommandTypeScanProcesses      = "scan_processes"
)

// Command statuses
const (
	CommandStatusPending   = "pending"
	CommandStatusSent      = "sent"
	CommandStatusReceived  = "received" // Agent confirmed receipt; not yet completed
	CommandStatusCompleted = "completed"
	CommandStatusFailed    = "failed"
	CommandStatusTimedOut  = "timed_out"
	CommandStatusCancelled = "cancelled"
	CommandStatusRunning   = "running"
)

// Command sources
const (
	CommandSourceManual = "manual" // User-initiated via UI
	CommandSourceSystem = "system" // Auto-triggered by system operations
)

// RequiresRapidPolling returns true when a command needs the agent in
// rapid-polling mode for fast turnaround. The dispatch chokepoint
// (AgentHandler.signAndCreateCommand) auto-queues enable_heartbeat
// (Source=system) ahead of these. A new command type that needs rapid polling
// must be added here; do not paper over it at the per-handler call site.
func RequiresRapidPolling(commandType string) bool {
	switch commandType {
	case CommandTypeUpdateAgent,
		CommandTypeInstallUpdate,
		CommandTypeDryRunUpdate,
		CommandTypeConfirmDependencies,
		CommandTypeRollback,
		CommandTypeReboot:
		return true
	}
	return false
}

// ActiveCommandInfo represents information about an active command for UI display
type ActiveCommandInfo struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	AgentID       uuid.UUID  `json:"agent_id" db:"agent_id"`
	CommandType   string     `json:"command_type" db:"command_type"`
	Params        JSONB      `json:"params" db:"params"`
	Status        string     `json:"status" db:"status"`
	Source        string     `json:"source" db:"source"`
	Signature     string     `json:"signature,omitempty" db:"signature"`
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
