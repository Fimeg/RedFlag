// Package event provides event helper functions and buffering for the RedFlag agent.
//
// ETHOS Compliance:
// (1) Errors are History, Not /dev/null
//     - All failures are captured as events
//     - Events include full context and metadata
//     - Events are buffered for offline scenarios
//
// (3) Assume Failure; Build for Resilience
//     - Best-effort event buffering (fails gracefully if buffer unavailable)
//     - Events persist to disk for recovery

package event

import (
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/models"
	"github.com/gofrs/uuid/v5"
)

// ScannerEventType represents a scanner operation event
type ScannerEventType string

const (
	// Event types for agent operations
	EventTypeAgentScan    = "agent_scan"
	EventTypeAgentConfig  = "agent_config"
	EventTypeAgentCheckIn = "agent_checkin"
	EventTypeAgentOffline = "agent_offline"
)

// ScanResult indicates the outcome of a scan operation
type ScanResult string

const (
	ScanResultSuccess ScanResult = "success"
	ScanResultFailed  ScanResult = "failed"
	ScanResultTimeout ScanResult = "timeout"
	ScanResultSkipped ScanResult = "skipped"
)

// BufferSystemEvent records a SystemEvent into the operational event buffer.
// A nil buffer degrades to local-only behavior; callers should still log or
// report command results through their normal channel.
func BufferSystemEvent(buffer *Buffer, agentID uuid.UUID, eventType, eventSubtype, severity, component, message string, metadata map[string]interface{}) {
	if buffer == nil {
		return
	}

	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}

	evt := &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    eventType,
		EventSubtype: eventSubtype,
		Severity:     severity,
		Component:    component,
		Message:      message,
		Metadata:     metadata,
		CreatedAt:    time.Now().UTC(),
	}
	if err := buffer.BufferEvent(evt); err != nil {
		log.Printf("[WARNING] [agent] [event] buffer_system_event_failed error=%v", err)
	}
}

// NewScanFailureEvent creates a SystemEvent for a failed scan operation
//
// Parameters:
//   - scannerName: The name of the scanner that failed (e.g., "apt", "docker")
//   - err: The error that occurred
//   - duration: How long the scan took before failing
//   - agentID: The agent UUID (empty for client-side creation without binding)
//
// Returns a SystemEvent ready to buffer or send to the server.
func NewScanFailureEvent(scannerName string, err error, duration time.Duration, agentID uuid.UUID) *models.SystemEvent {
	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}

	// Determine severity based on error type and scanner
	severity := models.SeverityWarning
	if isCriticalScanner(scannerName) {
		severity = models.SeverityCritical
	}

	return &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    EventTypeAgentScan,
		EventSubtype: models.SubtypeFailed,
		Severity:     severity,
		Component:    scannerName + "_scanner",
		Message:      fmt.Sprintf("%s scan failed after %v: %s", scannerName, duration.Round(time.Millisecond), err.Error()),
		Metadata: map[string]interface{}{
			"scanner":     scannerName,
			"error":       err.Error(),
			"duration_ms": duration.Milliseconds(),
			"result":      string(ScanResultFailed),
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		},
		CreatedAt: time.Now().UTC(),
	}
}

// NewScanSuccessEvent creates a SystemEvent for a successful scan operation
//
// Parameters:
//   - scannerName: The name of the scanner
//   - updateCount: Number of updates found
//   - duration: How long the scan took
//   - agentID: The agent UUID (empty for client-side creation without binding)
func NewScanSuccessEvent(scannerName string, updateCount int, duration time.Duration, agentID uuid.UUID) *models.SystemEvent {
	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}

	message := fmt.Sprintf("%s scan completed in %v", scannerName, duration.Round(time.Millisecond))
	if updateCount > 0 {
		message = fmt.Sprintf("%s scan found %d update(s) in %v", scannerName, updateCount, duration.Round(time.Millisecond))
	}

	return &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    EventTypeAgentScan,
		EventSubtype: models.SubtypeSuccess,
		Severity:     models.SeverityInfo,
		Component:    scannerName + "_scanner",
		Message:      message,
		Metadata: map[string]interface{}{
			"scanner":      scannerName,
			"update_count": updateCount,
			"duration_ms":  duration.Milliseconds(),
			"result":       string(ScanResultSuccess),
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		},
		CreatedAt: time.Now().UTC(),
	}
}

// NewScanTimeoutEvent creates a SystemEvent for a scan that timed out
func NewScanTimeoutEvent(scannerName string, duration time.Duration, timeout time.Duration, agentID uuid.UUID) *models.SystemEvent {
	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}

	return &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    EventTypeAgentScan,
		EventSubtype: models.SubtypeNetworkTimeout,
		Severity:     models.SeverityWarning,
		Component:    scannerName + "_scanner",
		Message:      fmt.Sprintf("%s scan timed out after %v (timeout: %v)", scannerName, duration.Round(time.Millisecond), timeout),
		Metadata: map[string]interface{}{
			"scanner":     scannerName,
			"duration_ms": duration.Milliseconds(),
			"timeout_ms":  timeout.Milliseconds(),
			"result":      string(ScanResultTimeout),
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		},
		CreatedAt: time.Now().UTC(),
	}
}

// NewConfigSyncEvent creates a SystemEvent for a config sync operation
func NewConfigSyncEvent(success bool, changes []string, attempt int, agentID uuid.UUID) *models.SystemEvent {
	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}

	subtype := models.SubtypeSuccess
	severity := models.SeverityInfo
	message := fmt.Sprintf("Config synced successfully (attempt %d)", attempt)

	if !success {
		subtype = models.SubtypeFailed
		severity = models.SeverityWarning
		message = fmt.Sprintf("Config sync failed (attempt %d)", attempt)
	} else if len(changes) > 0 {
		message = fmt.Sprintf("Config updated: %v (attempt %d)", changes, attempt)
	}

	return &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    EventTypeAgentConfig,
		EventSubtype: subtype,
		Severity:     severity,
		Component:    models.ComponentAgent,
		Message:      message,
		Metadata: map[string]interface{}{
			"success":   success,
			"changes":   changes,
			"attempt":   attempt,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
		CreatedAt: time.Now().UTC(),
	}
}

// NewOfflineEvent creates a SystemEvent when the agent goes offline
func NewOfflineEvent(reason string, lastCheckIn time.Time, agentID uuid.UUID) *models.SystemEvent {
	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}

	message := "Agent went offline"
	if reason != "" {
		message = fmt.Sprintf("Agent went offline: %s", reason)
	}

	var offlineDuration string
	if !lastCheckIn.IsZero() {
		offlineDuration = fmt.Sprintf("%v", time.Since(lastCheckIn).Round(time.Second))
	}

	return &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    EventTypeAgentOffline,
		EventSubtype: models.SubtypeTokenExpired, // Reusing existing subtype
		Severity:     models.SeverityWarning,
		Component:    models.ComponentAgent,
		Message:      message,
		Metadata: map[string]interface{}{
			"reason":           reason,
			"last_check_in":    lastCheckIn.Format(time.RFC3339),
			"offline_duration": offlineDuration,
			"timestamp":        time.Now().UTC().Format(time.RFC3339),
		},
		CreatedAt: time.Now().UTC(),
	}
}

// NewReconnectionEvent creates a SystemEvent when the agent reconnects
func NewReconnectionEvent(offlineDuration time.Duration, agentID uuid.UUID) *models.SystemEvent {
	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}

	return &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    EventTypeAgentCheckIn,
		EventSubtype: models.SubtypeSuccess,
		Severity:     models.SeverityInfo,
		Component:    models.ComponentAgent,
		Message:      fmt.Sprintf("Agent reconnected after %v offline", offlineDuration.Round(time.Second)),
		Metadata: map[string]interface{}{
			"offline_duration_ms": offlineDuration.Milliseconds(),
			"timestamp":           time.Now().UTC().Format(time.RFC3339),
		},
		CreatedAt: time.Now().UTC(),
	}
}

// NewCheckInEvent creates a SystemEvent for a successful check-in
func NewCheckInEvent(hasCommands bool, commandCount int, agentID uuid.UUID) *models.SystemEvent {
	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}

	subtype := models.SubtypeInfo
	message := "Check-in successful - no new commands"
	if hasCommands {
		subtype = models.SubtypeSuccess
		message = fmt.Sprintf("Check-in successful - received %d command(s)", commandCount)
	}

	return &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      agentIDPtr,
		EventType:    EventTypeAgentCheckIn,
		EventSubtype: subtype,
		Severity:     models.SeverityInfo,
		Component:    models.ComponentAgent,
		Message:      message,
		Metadata: map[string]interface{}{
			"has_commands":  hasCommands,
			"command_count": commandCount,
			"timestamp":     time.Now().UTC().Format(time.RFC3339),
		},
		CreatedAt: time.Now().UTC(),
	}
}

// isCriticalScanner returns true if the scanner is considered critical
// Critical scanner failures are logged with SeverityCritical
func isCriticalScanner(scannerName string) bool {
	// Update-related scanners that indicate potential security or update issues
	criticalScanners := map[string]bool{
		"apt":     true,  // Security patches
		"dnf":     true,  // Security patches
		"windows": true,  // Security patches
		"winget":  false, // Optional updates
		"docker":  false, // Optional feature
		"storage": false, // Monitoring only
		"system":  false, // Monitoring only
	}

	return criticalScanners[scannerName]
}
