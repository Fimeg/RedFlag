package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/config"
)

// SecurityEvent represents a security event on the agent side
// This is a simplified version of the server model to avoid circular dependencies
type SecurityEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"` // CRITICAL, WARNING, INFO, DEBUG
	EventType string                 `json:"event_type"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// SecurityLogConfig holds configuration for security logging on the agent
type SecurityLogConfig struct {
	Enabled      bool   `json:"enabled" env:"REDFLAG_AGENT_SECURITY_LOG_ENABLED" default:"true"`
	Level        string `json:"level" env:"REDFLAG_AGENT_SECURITY_LOG_LEVEL" default:"warning"` // none, error, warn, info, debug
	LogSuccesses bool   `json:"log_successes" env:"REDFLAG_AGENT_SECURITY_LOG_SUCCESSES" default:"false"`
	FilePath     string `json:"file_path" env:"REDFLAG_AGENT_SECURITY_LOG_PATH"` // Relative to agent data directory
	MaxSizeMB    int    `json:"max_size_mb" env:"REDFLAG_AGENT_SECURITY_LOG_MAX_SIZE" default:"50"`
	MaxFiles     int    `json:"max_files" env:"REDFLAG_AGENT_SECURITY_LOG_MAX_FILES" default:"5"`
	BatchSize    int    `json:"batch_size" env:"REDFLAG_AGENT_SECURITY_LOG_BATCH_SIZE" default:"10"`
	SendToServer bool   `json:"send_to_server" env:"REDFLAG_AGENT_SECURITY_LOG_SEND" default:"true"`
}

// SecurityLogger handles security event logging on the agent
type SecurityLogger struct {
	config     SecurityLogConfig
	logger     *log.Logger
	file       *os.File
	mu         sync.Mutex
	buffer     []*SecurityEvent
	fileCursor int
	flushTimer *time.Timer
	lastFlush  time.Time
	closed     bool
}

// SecurityEventTypes defines all possible security event types on the agent
var SecurityEventTypes = struct {
	CmdSignatureVerificationFailed    string
	CmdSignatureVerificationSuccess   string
	UpdateNonceInvalid                string
	UpdateSignatureVerificationFailed string
	MachineIDChangeDetected           string
	ConfigTamperingWarning            string
	UnauthorizedCommandAttempt        string
	KeyRotationDetected               string
}{
	CmdSignatureVerificationFailed:    "CMD_SIGNATURE_VERIFICATION_FAILED",
	CmdSignatureVerificationSuccess:   "CMD_SIGNATURE_VERIFICATION_SUCCESS",
	UpdateNonceInvalid:                "UPDATE_NONCE_INVALID",
	UpdateSignatureVerificationFailed: "UPDATE_SIGNATURE_VERIFICATION_FAILED",
	MachineIDChangeDetected:           "MACHINE_ID_CHANGE_DETECTED",
	ConfigTamperingWarning:            "CONFIG_TAMPERING_WARNING",
	UnauthorizedCommandAttempt:        "UNAUTHORIZED_COMMAND_ATTEMPT",
	KeyRotationDetected:               "KEY_ROTATION_DETECTED",
}

// NewSecurityLogger creates a new agent security logger
func NewSecurityLogger(agentConfig *config.Config, logDir string) (*SecurityLogger, error) {
	// Create default security log config
	secConfig := SecurityLogConfig{
		Enabled:      true,
		Level:        "warning",
		LogSuccesses: false,
		FilePath:     "security.log",
		MaxSizeMB:    50,
		MaxFiles:     5,
		BatchSize:    10,
		SendToServer: true,
	}

	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create security log directory: %w", err)
	}

	// Open log file
	logPath := filepath.Join(logDir, secConfig.FilePath)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open security log file: %w", err)
	}

	logger := &SecurityLogger{
		config:    secConfig,
		logger:    log.New(file, "[SECURITY] ", log.LstdFlags|log.LUTC),
		file:      file,
		buffer:    make([]*SecurityEvent, 0, secConfig.BatchSize),
		lastFlush: time.Now().UTC(),
	}

	// Start flush timer
	logger.flushTimer = time.AfterFunc(30*time.Second, logger.flushBuffer)

	return logger, nil
}

// Log writes a security event
func (sl *SecurityLogger) Log(event *SecurityEvent) error {
	if !sl.config.Enabled || sl.config.Level == "none" {
		return nil
	}

	// Skip successes unless configured to log them
	if !sl.config.LogSuccesses && event.EventType == SecurityEventTypes.CmdSignatureVerificationSuccess {
		return nil
	}

	// Filter by log level
	if !sl.shouldLogLevel(event.Level) {
		return nil
	}

	sl.mu.Lock()
	defer sl.mu.Unlock()

	if sl.closed {
		return fmt.Errorf("security logger is closed")
	}

	// Add prefix to distinguish security events
	event.Message = "SECURITY: " + event.Message

	criticalWritten := false
	if event.Level == "CRITICAL" {
		if err := sl.writeEvent(event); err != nil {
			log.Printf("[ERROR] [agent] [security] critical_event_write_failed error=%v", err)
		} else {
			criticalWritten = true
		}
	}

	// Add to buffer for eventual server delivery. File flushes advance
	// fileCursor; server delivery clears only after a successful POST.
	sl.buffer = append(sl.buffer, event)
	if criticalWritten && sl.fileCursor == len(sl.buffer)-1 {
		sl.fileCursor++
	}
	if len(sl.buffer) >= sl.config.BatchSize {
		sl.flushBufferUnsafe()
	}

	return nil
}

// LogCommandVerificationFailure logs a command signature verification failure
func (sl *SecurityLogger) LogCommandVerificationFailure(commandID string, reason string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "CRITICAL",
		EventType: SecurityEventTypes.CmdSignatureVerificationFailed,
		Message:   "Command signature verification failed",
		Details: map[string]interface{}{
			"command_id": commandID,
			"reason":     reason,
		},
	}

	_ = sl.Log(event)
}

// LogNonceValidationFailure logs a nonce validation failure
func (sl *SecurityLogger) LogNonceValidationFailure(nonce string, reason string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "WARNING",
		EventType: SecurityEventTypes.UpdateNonceInvalid,
		Message:   "Update nonce validation failed",
		Details: map[string]interface{}{
			"nonce":  nonce[:min(len(nonce), 16)] + "...", // Truncate for security
			"reason": reason,
		},
	}

	_ = sl.Log(event)
}

// LogUpdateSignatureVerificationFailure logs an update signature verification failure
func (sl *SecurityLogger) LogUpdateSignatureVerificationFailure(updateID string, reason string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "CRITICAL",
		EventType: SecurityEventTypes.UpdateSignatureVerificationFailed,
		Message:   "Update signature verification failed",
		Details: map[string]interface{}{
			"update_id": updateID,
			"reason":    reason,
		},
	}

	_ = sl.Log(event)
}

// LogMachineIDChangeDetected logs when machine ID changes
func (sl *SecurityLogger) LogMachineIDChangeDetected(oldID, newID string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "WARNING",
		EventType: SecurityEventTypes.MachineIDChangeDetected,
		Message:   "Machine ID change detected",
		Details: map[string]interface{}{
			"old_machine_id": oldID,
			"new_machine_id": newID,
		},
	}

	_ = sl.Log(event)
}

// LogConfigTamperingWarning logs when configuration tampering is suspected
func (sl *SecurityLogger) LogConfigTamperingWarning(configPath string, reason string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "WARNING",
		EventType: SecurityEventTypes.ConfigTamperingWarning,
		Message:   "Configuration file tampering detected",
		Details: map[string]interface{}{
			"config_file": configPath,
			"reason":      reason,
		},
	}

	_ = sl.Log(event)
}

// LogUnauthorizedCommandAttempt logs an attempt to run an unauthorized command
func (sl *SecurityLogger) LogUnauthorizedCommandAttempt(command string, reason string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "WARNING",
		EventType: SecurityEventTypes.UnauthorizedCommandAttempt,
		Message:   "Unauthorized command execution attempt",
		Details: map[string]interface{}{
			"command": command,
			"reason":  reason,
		},
	}

	_ = sl.Log(event)
}

// LogCommandVerificationSuccess logs a successful command signature verification
func (sl *SecurityLogger) LogCommandVerificationSuccess(commandID string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "INFO",
		EventType: SecurityEventTypes.CmdSignatureVerificationSuccess,
		Message:   "Command signature verified successfully",
		Details: map[string]interface{}{
			"command_id": commandID,
		},
	}

	_ = sl.Log(event)
}

// LogCommandVerificationFailed logs a failed command signature verification
func (sl *SecurityLogger) LogCommandVerificationFailed(commandID, reason string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "CRITICAL",
		EventType: SecurityEventTypes.CmdSignatureVerificationFailed,
		Message:   "Command signature verification failed",
		Details: map[string]interface{}{
			"command_id": commandID,
			"reason":     reason,
		},
	}

	_ = sl.Log(event)
}

// LogKeyRotationDetected logs when a new signing key is detected and cached.
// This occurs when a command arrives with a key_id not previously cached by the agent,
// indicating the server has rotated its signing key.
func (sl *SecurityLogger) LogKeyRotationDetected(keyID string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "INFO",
		EventType: SecurityEventTypes.KeyRotationDetected,
		Message:   "New signing key detected and cached",
		Details: map[string]interface{}{
			"key_id": keyID,
		},
	}

	_ = sl.Log(event)
}

// LogCommandSkipped logs when a command is skipped due to signing configuration
func (sl *SecurityLogger) LogCommandSkipped(commandID, reason string) {
	if sl == nil {
		return
	}

	event := &SecurityEvent{
		Timestamp: time.Now().UTC(),
		Level:     "INFO",
		EventType: "COMMAND_SKIPPED",
		Message:   "Command skipped due to signing configuration",
		Details: map[string]interface{}{
			"command_id": commandID,
			"reason":     reason,
		},
	}

	_ = sl.Log(event)
}

// GetBatch returns a batch of events for sending to server
func (sl *SecurityLogger) GetBatch() []*SecurityEvent {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if len(sl.buffer) == 0 {
		return nil
	}

	batch := make([]*SecurityEvent, len(sl.buffer))
	copy(batch, sl.buffer)

	return batch
}

// ClearBatch removes the oldest events after successful server delivery.
func (sl *SecurityLogger) ClearBatch(count int) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if count <= 0 {
		return
	}
	if count >= len(sl.buffer) {
		for i := range sl.buffer {
			sl.buffer[i] = nil
		}
		sl.buffer = sl.buffer[:0]
		sl.fileCursor = 0
		return
	}
	copy(sl.buffer, sl.buffer[count:])
	for i := len(sl.buffer) - count; i < len(sl.buffer); i++ {
		sl.buffer[i] = nil
	}
	sl.buffer = sl.buffer[:len(sl.buffer)-count]
	sl.fileCursor -= count
	if sl.fileCursor < 0 {
		sl.fileCursor = 0
	}
}

// writeEvent writes an event to the log file
func (sl *SecurityLogger) writeEvent(event *SecurityEvent) error {
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal security event: %w", err)
	}

	sl.logger.Println(string(jsonData))
	return nil
}

// flushBuffer flushes all buffered events to file
func (sl *SecurityLogger) flushBuffer() {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.flushBufferUnsafe()
}

// flushBufferUnsafe flushes buffer without acquiring lock (must be called with lock held)
func (sl *SecurityLogger) flushBufferUnsafe() {
	if sl.fileCursor > len(sl.buffer) {
		sl.fileCursor = 0
	}
	for sl.fileCursor < len(sl.buffer) {
		event := sl.buffer[sl.fileCursor]
		if err := sl.writeEvent(event); err != nil {
			log.Printf("[ERROR] [agent] [security] security_event_write_failed error=%v", err)
			break
		}
		sl.fileCursor++
	}

	if sl.fileCursor == len(sl.buffer) {
		sl.lastFlush = time.Now().UTC()
	}

	// Reset timer if not closed
	if !sl.closed && sl.flushTimer != nil {
		sl.flushTimer.Stop()
		sl.flushTimer.Reset(30 * time.Second)
	}
}

// shouldLogLevel checks if the event should be logged based on the configured level
func (sl *SecurityLogger) shouldLogLevel(eventLevel string) bool {
	levels := map[string]int{
		"NONE":    0,
		"ERROR":   1,
		"WARNING": 2,
		"INFO":    3,
		"DEBUG":   4,
	}

	configLevel := levels[strings.ToUpper(sl.config.Level)]
	eventLvl, exists := levels[eventLevel]
	if !exists {
		eventLvl = 2 // Default to WARNING
	}

	return eventLvl <= configLevel
}

// Close closes the security logger
func (sl *SecurityLogger) Close() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if sl.closed {
		return nil
	}

	// Stop flush timer
	if sl.flushTimer != nil {
		sl.flushTimer.Stop()
	}

	// Flush remaining events
	sl.flushBufferUnsafe()

	// Close file
	if sl.file != nil {
		err := sl.file.Close()
		sl.closed = true
		return err
	}

	sl.closed = true
	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
