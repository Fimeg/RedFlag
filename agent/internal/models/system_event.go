package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// SystemEvent represents a unified event log entry for all system events
// This is a copy of the server model to avoid circular dependencies
type SystemEvent struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	AgentID      *uuid.UUID             `json:"agent_id,omitempty" db:"agent_id"` // Pointer to allow NULL for server events
	EventType    string                 `json:"event_type" db:"event_type"`       // e.g., 'agent_update', 'agent_startup', 'server_build'
	EventSubtype string                 `json:"event_subtype" db:"event_subtype"` // e.g., 'success', 'failed', 'info', 'warning'
	Severity     string                 `json:"severity" db:"severity"`           // 'info', 'warning', 'error', 'critical'
	Component    string                 `json:"component" db:"component"`         // 'agent', 'server', 'build', 'download', 'config', etc.
	Message      string                 `json:"message" db:"message"`
	Metadata     map[string]interface{} `json:"metadata,omitempty" db:"metadata"` // JSONB for structured data
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

// Event type constants
const (
	EventTypeAgentStartup      = "agent_startup"
	EventTypeAgentRegistration = "agent_registration"
	EventTypeAgentCheckIn      = "agent_checkin"
	EventTypeAgentScan         = "agent_scan"
	EventTypeAgentUpdate       = "agent_update"
	EventTypeAgentInstall      = "agent_install"
	EventTypeAgentConfig       = "agent_config"
	EventTypeAgentMigration    = "agent_migration"
	EventTypeAgentShutdown     = "agent_shutdown"
	EventTypeServerBuild       = "server_build"
	EventTypeServerDownload    = "server_download"
	EventTypeServerConfig      = "server_config"
	EventTypeServerAuth        = "server_auth"
	EventTypeDownload          = "download"
	EventTypeMigration         = "migration"
	EventTypeError             = "error"
	EventTypeAgentCrypto       = "agent_crypto"
	EventTypeAgentDocker       = "agent_docker"
)

// Event subtype constants
const (
	SubtypeSuccess            = "success"
	SubtypeFailed             = "failed"
	SubtypeInfo               = "info"
	SubtypeWarning            = "warning"
	SubtypeCritical           = "critical"
	SubtypeDownloadFailed     = "download_failed"
	SubtypeValidationFailed   = "validation_failed"
	SubtypeConfigCorrupted    = "config_corrupted"
	SubtypeMigrationNeeded    = "migration_needed"
	SubtypePanicRecovered     = "panic_recovered"
	SubtypeTokenExpired       = "token_expired"
	SubtypeNetworkTimeout     = "network_timeout"
	SubtypePermissionDenied   = "permission_denied"
	SubtypeServiceUnavailable = "service_unavailable"
	SubtypeDegraded           = "degraded"
	SubtypeRollback           = "rollback"
	SubtypeStarted            = "started"
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