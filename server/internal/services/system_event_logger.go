package services

import (
	"context"
	"log"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services/notifier"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// SystemEventLogger persists events into the system_events table and
// optionally dispatches them to external notification sinks.
// Used by the orchestrator (timeouts/workflow) to emit system_events with zero
// import coupling on the services package.
type SystemEventLogger struct {
	db         *sqlx.DB
	dispatcher *notifier.Dispatcher
}

// NewSystemEventLogger creates a SystemEventLogger.
func NewSystemEventLogger(db *sqlx.DB) *SystemEventLogger {
	return &SystemEventLogger{db: db}
}

// SetDispatcher attaches a notification dispatcher. May be nil (notifications
// disabled). Safe to call before or after first LogEvent.
func (l *SystemEventLogger) SetDispatcher(d *notifier.Dispatcher) {
	l.dispatcher = d
}

// LogEvent inserts a row into system_events and dispatches to external
// notification sinks if configured. Best-effort: failures are logged
// but never returned, so the caller never stalls on event emission.
func (l *SystemEventLogger) LogEvent(agentID *uuid.UUID, eventType, eventSubtype, severity, component, message string, metadata map[string]interface{}) {
	meta := models.JSONB(metadata)
	if meta == nil {
		meta = models.JSONB{}
	}

	id, err := uuid.NewV4()
	if err != nil {
		log.Printf("[ERROR] [server] [event_logger] uuid_generation_failed error=%v", err)
		return
	}

	query := `
		INSERT INTO system_events (id, agent_id, event_type, event_subtype, severity, component, message, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	if _, err := l.db.Exec(query, id, agentID, eventType, eventSubtype, severity, component, message, meta); err != nil {
		log.Printf("[ERROR] [server] [event_logger] insert_failed event_type=%s component=%s error=%v", eventType, component, err)
	}

	// Dispatch to external notification sinks (best-effort, fail-open).
	if l.dispatcher != nil {
		ne := eventToNotify(eventType, eventSubtype, severity, message, agentID)
		if ne.Class != "" {
			l.dispatcher.Dispatch(context.Background(), ne)
		}
	}
}

// eventToNotify maps a system event (type+subtype+severity) to a NotifyEvent
// with the appropriate event class. Returns a zero-value NotifyEvent
// (Class == "") if the event is not operator-notifiable.
func eventToNotify(eventType, eventSubtype, severity, message string, agentID *uuid.UUID) notifier.NotifyEvent {
	// agent_update events
	if eventType == "agent_update" {
		switch eventSubtype {
		case "timed_out", "failed":
			return notifier.NotifyEvent{
				Class:       notifier.ClassUpdateFailed,
				Severity:    severity,
				Title:       "Agent update " + eventSubtype,
				Message:     message,
				AgentID:     agentIDString(agentID),
				Fingerprint: "agent_update_" + eventSubtype + "_" + agentIDString(agentID),
			}
		}
	}

	// Command failures
	if eventType == "command_failed" {
		return notifier.NotifyEvent{
			Class:       notifier.ClassSecurity,
			Severity:    severity,
			Title:       "Command rejected by agent",
			Message:     message,
			AgentID:     agentIDString(agentID),
			Fingerprint: "command_failed_" + agentIDString(agentID),
		}
	}

	// Agent registration failures
	if eventType == "agent_registration" && eventSubtype == "failed" {
		return notifier.NotifyEvent{
			Class:       notifier.ClassSecurity,
			Severity:    severity,
			Title:       "Agent registration failed",
			Message:     message,
			AgentID:     agentIDString(agentID),
			Fingerprint: "registration_failed_" + agentIDString(agentID),
		}
	}

	// Refresh token security events — machine mismatch, token reuse.
	if eventType == "token_security" {
		return notifier.NotifyEvent{
			Class:       notifier.ClassSecurity,
			Severity:    severity,
			Title:       "Token security event",
			Message:     message,
			AgentID:     agentIDString(agentID),
			Fingerprint: "token_security_" + eventSubtype + "_" + agentIDString(agentID),
		}
	}

	// Heartbeat / agent offline detection.
	if eventType == "agent_heartbeat" && eventSubtype == "missed" {
		return notifier.NotifyEvent{
			Class:       notifier.ClassAgentOffline,
			Severity:    severity,
			Title:       "Agent missed heartbeat",
			Message:     message,
			AgentID:     agentIDString(agentID),
			Fingerprint: "missed_heartbeat_" + agentIDString(agentID),
		}
	}

	// Not a notifiable event class.
	return notifier.NotifyEvent{}
}

func agentIDString(agentID *uuid.UUID) string {
	if agentID == nil {
		return ""
	}
	return agentID.String()
}
