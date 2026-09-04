package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// SystemEvent represents a unified event log entry for all system events
// This implements the unified event logging system from docs/ERROR_FLOW_AUDIT.md
type SystemEvent struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	AgentID      *uuid.UUID             `json:"agent_id,omitempty" db:"agent_id"` // Pointer to allow NULL for server events
	EventType    string                 `json:"event_type" db:"event_type"`       // e.g., 'agent_update', 'agent_startup', 'server_build'
	EventSubtype string                 `json:"event_subtype" db:"event_subtype"` // e.g., 'success', 'failed', 'info', 'warning'
	Severity     string                 `json:"severity" db:"severity"`           // 'info', 'warning', 'error', 'critical'
	Component    string                 `json:"component" db:"component"`         // 'agent', 'server', 'build', 'download', 'config', etc.
	Message      string                 `json:"message" db:"message"`
	Metadata     JSONB                  `json:"metadata,omitempty" db:"metadata"` // JSONB; Value/Scan defined in agent.go
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	// Narrative is a server-rendered, operator-facing summary of the event.
	// Populated by handlers before returning to the UI; never persisted.
	// See services/event_renderer.go for the renderer.
	Narrative string `json:"narrative,omitempty" db:"-"`
}

// Event type constants
const (
	EventTypeAgentStartup       = "agent_startup"
	EventTypeAgentRegistration  = "agent_registration"
	EventTypeAgentUnregistration = "agent_unregistration"
	EventTypeAgentDeleted       = "agent_deleted"
	EventTypeAgentCheckIn       = "agent_checkin"
	EventTypeAgentScan          = "agent_scan"
	EventTypeAgentUpdate        = "agent_update"
	EventTypeAgentConfig        = "agent_config"
	EventTypeAgentMigration     = "agent_migration"
	EventTypeAgentShutdown      = "agent_shutdown"
	EventTypeRegistrationToken  = "registration_token"
	EventTypeCommandCreated     = "command_created"
	EventTypeCommandCompleted   = "command_completed"
	EventTypeCommandFailed      = "command_failed"
	EventTypeServerBuild        = "server_build"
	EventTypeServerDownload     = "server_download"
	EventTypeServerConfig       = "server_config"
	EventTypeServerAuth         = "server_auth"
	EventTypeDownload           = "download"
	EventTypeMigration          = "migration"
	EventTypeError             = "error"
)

// Event subtype constants
const (
	SubtypeSuccess     = "success"
	SubtypeFailed      = "failed"
	SubtypeInfo        = "info"
	SubtypeWarning     = "warning"
	SubtypeCritical    = "critical"
	SubtypeDownloadFailed     = "download_failed"
	SubtypeValidationFailed   = "validation_failed"
	SubtypeConfigCorrupted    = "config_corrupted"
	SubtypeMigrationNeeded    = "migration_needed"
	SubtypePanicRecovered     = "panic_recovered"
	SubtypeTokenExpired       = "token_expired"
	SubtypeNetworkTimeout     = "network_timeout"
	SubtypePermissionDenied   = "permission_denied"
	SubtypeServiceUnavailable = "service_unavailable"
)

// Severity constants
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityError    = "error"
	SeverityCritical = "critical"
)

// Component constants
const (
	ComponentAgent     = "agent"
	ComponentServer    = "server"
	ComponentBuild     = "build"
	ComponentDownload  = "download"
	ComponentConfig    = "config"
	ComponentDatabase  = "database"
	ComponentNetwork   = "network"
	ComponentSecurity  = "security"
	ComponentMigration = "migration"
)