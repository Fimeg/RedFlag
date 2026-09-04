package models

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
)

// SecurityEvent represents a security-related event that occurred
type SecurityEvent struct {
	Timestamp    time.Time              `json:"timestamp" db:"timestamp"`
	Level        string                 `json:"level" db:"level"` // CRITICAL, WARNING, INFO, DEBUG
	EventType    string                 `json:"event_type" db:"event_type"`
	AgentID      uuid.UUID              `json:"agent_id,omitempty" db:"agent_id"`
	Message      string                 `json:"message" db:"message"`
	TraceID      string                 `json:"trace_id,omitempty" db:"trace_id"`
	IPAddress    string                 `json:"ip_address,omitempty" db:"ip_address"`
	Details      map[string]interface{} `json:"details,omitempty" db:"details"` // JSON encoded
	Metadata     map[string]interface{} `json:"metadata,omitempty" db:"metadata"` // JSON encoded
}

// SecurityEventTypes defines all possible security event types
var SecurityEventTypes = struct {
	CmdSigned                        string
	CmdSignatureVerificationFailed   string
	CmdSignatureVerificationSuccess  string
	UpdateNonceInvalid               string
	UpdateSignatureVerificationFailed string
	MachineIDMismatch                string
	AuthJWTValidationFailed          string
	PrivateKeyNotConfigured          string
	AgentRegistrationFailed          string
	UnauthorizedAccessAttempt        string
	ConfigTamperingDetected          string
	AnomalousBehavior                string
	SecurityAdvisoryCleared          string
}{
	CmdSigned:                        "CMD_SIGNED",
	CmdSignatureVerificationFailed:   "CMD_SIGNATURE_VERIFICATION_FAILED",
	CmdSignatureVerificationSuccess:  "CMD_SIGNATURE_VERIFICATION_SUCCESS",
	UpdateNonceInvalid:               "UPDATE_NONCE_INVALID",
	UpdateSignatureVerificationFailed: "UPDATE_SIGNATURE_VERIFICATION_FAILED",
	MachineIDMismatch:                "MACHINE_ID_MISMATCH",
	AuthJWTValidationFailed:          "AUTH_JWT_VALIDATION_FAILED",
	PrivateKeyNotConfigured:          "PRIVATE_KEY_NOT_CONFIGURED",
	AgentRegistrationFailed:          "AGENT_REGISTRATION_FAILED",
	UnauthorizedAccessAttempt:        "UNAUTHORIZED_ACCESS_ATTEMPT",
	ConfigTamperingDetected:          "CONFIG_TAMPERING_DETECTED",
	AnomalousBehavior:                "ANOMALOUS_BEHAVIOR",
	SecurityAdvisoryCleared:          "SECURITY_ADVISORY_CLEARED",
}

// IsCritical returns true if the event is of critical severity
func (e *SecurityEvent) IsCritical() bool {
	return e.Level == "CRITICAL"
}

// IsWarning returns true if the event is a warning
func (e *SecurityEvent) IsWarning() bool {
	return e.Level == "WARNING"
}

// ShouldLogToDatabase determines if this event should be stored in the database
func (e *SecurityEvent) ShouldLogToDatabase(logToDatabase bool) bool {
	return logToDatabase && (e.IsCritical() || e.IsWarning())
}

// HashIPAddress hashes the IP address for privacy
func (e *SecurityEvent) HashIPAddress() {
	if e.IPAddress != "" {
		hash := sha256.Sum256([]byte(e.IPAddress))
		e.IPAddress = fmt.Sprintf("hashed:%x", hash[:8]) // Store first 8 bytes of hash
	}
}

// NewSecurityEvent creates a new security event with current timestamp
func NewSecurityEvent(level, eventType string, agentID uuid.UUID, message string) *SecurityEvent {
	return &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     level,
		EventType: eventType,
		AgentID:   agentID,
		Message:   message,
		Details:   make(map[string]interface{}),
		Metadata:  make(map[string]interface{}),
	}
}

// WithTrace adds a trace ID to the event
func (e *SecurityEvent) WithTrace(traceID string) *SecurityEvent {
	e.TraceID = traceID
	return e
}

// WithIPAddress adds an IP address to the event
func (e *SecurityEvent) WithIPAddress(ip string) *SecurityEvent {
	e.IPAddress = ip
	return e
}

// WithDetail adds a key-value detail to the event
func (e *SecurityEvent) WithDetail(key string, value interface{}) *SecurityEvent {
	e.Details[key] = value
	return e
}

// WithMetadata adds a key-value metadata to the event
func (e *SecurityEvent) WithMetadata(key string, value interface{}) *SecurityEvent {
	e.Metadata[key] = value
	return e
}