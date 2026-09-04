// Package event provides event helper functions and buffering for the RedFlag agent.
//
// TeeLogger emits an ETHOS-tagged log.Printf AND a buffered SystemEvent in a
// single call. When the internal buffer is nil (e.g. during early boot or in
// tests), it degrades to log-only mode.
//
// ETHOS Compliance:
// (1) Errors are History, Not /dev/null
//     - Every TeeLogger call produces a log line (stderr/journald)
//     - Every TeeLogger call buffers a SystemEvent for server delivery
//     - Buffer failures are logged, never silently dropped
//
// (3) Assume Failure; Build for Resilience
//     - Nil buffer degrades gracefully to log-only
//     - Buffer write is best-effort; log line is the durability guarantee

package event

import (
	"log"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/models"
	"github.com/gofrs/uuid/v5"
)

// TeeLogger emits an ETHOS-tagged log.Printf AND a buffered SystemEvent
// in a single call. When the internal buffer is nil (e.g. during early boot
// or in test), it degrades to log-only mode.
type TeeLogger struct {
	buffer  *Buffer
	agentID uuid.UUID
}

// NewTeeLogger creates a TeeLogger. Pass a nil buffer for log-only mode.
func NewTeeLogger(buffer *Buffer, agentID uuid.UUID) *TeeLogger {
	return &TeeLogger{buffer: buffer, agentID: agentID}
}

// Buffer returns the operational event buffer used by this logger.
func (l *TeeLogger) Buffer() *Buffer {
	if l == nil {
		return nil
	}
	return l.buffer
}

// LogParams carries all structured fields for a dual-output event.
type LogParams struct {
	Level           string                 // ETHOS level tag: "INFO", "WARNING", "ERROR", "CRITICAL"
	System          string                 // ETHOS system tag: "agent"
	Component       string                 // ETHOS component tag: "migration", "crypto", "config"
	EventType       string                 // SystemEvent.EventType
	EventSubtype    string                 // SystemEvent.EventSubtype
	Severity        string                 // SystemEvent.Severity
	ServerComponent string                 // SystemEvent.Component
	Message         string                 // Human-readable message
	Metadata        map[string]interface{} // Structured key-value pairs
}

// Log is the core dual-output method.
func (l *TeeLogger) Log(params LogParams) {
	// Nil receiver: log-only mode (no buffer, no agent ID).
	if l == nil {
		log.Printf("[%s] [%s] [%s] %s", params.Level, params.System, params.Component, params.Message)
		return
	}

	// 1. ETHOS-tagged log line
	log.Printf("[%s] [%s] [%s] %s", params.Level, params.System, params.Component, params.Message)

	// 2. Buffer SystemEvent (best-effort, nil-safe)
	if l.buffer == nil {
		return
	}

	var agentIDPtr *uuid.UUID
	if l.agentID != uuid.Nil {
		agentIDPtr = &l.agentID
	}

	evt := &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    params.EventType,
		EventSubtype: params.EventSubtype,
		Severity:     params.Severity,
		Component:    params.ServerComponent,
		Message:      params.Message,
		Metadata:     params.Metadata,
		CreatedAt:    time.Now().UTC(),
	}

	if err := l.buffer.BufferEvent(evt); err != nil {
		log.Printf("[WARNING] [agent] [event] tee_buffer_failed error=%v", err)
	}
}

// Info emits an INFO-level dual-output event.
func (l *TeeLogger) Info(system, component, serverComponent, message string, metadata map[string]interface{}) {
	l.Log(LogParams{
		Level: "INFO", System: system, Component: component,
		EventType: deriveEventType(component), EventSubtype: models.SubtypeInfo,
		Severity: models.SeverityInfo, ServerComponent: serverComponent,
		Message: message, Metadata: metadata,
	})
}

// Warning emits a WARNING-level dual-output event.
func (l *TeeLogger) Warning(system, component, serverComponent, message string, metadata map[string]interface{}) {
	l.Log(LogParams{
		Level: "WARNING", System: system, Component: component,
		EventType: deriveEventType(component), EventSubtype: models.SubtypeWarning,
		Severity: models.SeverityWarning, ServerComponent: serverComponent,
		Message: message, Metadata: metadata,
	})
}

// Error emits an ERROR-level dual-output event.
func (l *TeeLogger) Error(system, component, serverComponent, message string, metadata map[string]interface{}) {
	l.Log(LogParams{
		Level: "ERROR", System: system, Component: component,
		EventType: deriveEventType(component), EventSubtype: models.SubtypeFailed,
		Severity: models.SeverityError, ServerComponent: serverComponent,
		Message: message, Metadata: metadata,
	})
}

// Critical emits a CRITICAL-level dual-output event.
func (l *TeeLogger) Critical(system, component, serverComponent, message string, metadata map[string]interface{}) {
	l.Log(LogParams{
		Level: "CRITICAL", System: system, Component: component,
		EventType: deriveEventType(component), EventSubtype: models.SubtypeCritical,
		Severity: models.SeverityCritical, ServerComponent: serverComponent,
		Message: message, Metadata: metadata,
	})
}

// deriveEventType maps an ETHOS component tag to a SystemEvent.EventType.
// Callers can always override by using Log() directly with explicit params.
func deriveEventType(component string) string {
	switch component {
	case "migration":
		return models.EventTypeAgentMigration
	case "crypto":
		return models.EventTypeAgentCrypto
	case "config":
		return models.EventTypeAgentConfig
	case "docker":
		return models.EventTypeAgentDocker
	case "installer":
		return models.EventTypeAgentInstall
	case "kernel":
		return models.EventTypeAgentStartup
	case "acknowledgment":
		return models.EventTypeAgentCheckIn
	case "receipt":
		return models.EventTypeAgentCheckIn
	case "localapi":
		return models.EventTypeAgentStartup
	case "loop":
		return models.EventTypeAgentStartup
	default:
		return models.EventTypeError
	}
}
