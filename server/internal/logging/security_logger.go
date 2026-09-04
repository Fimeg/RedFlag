package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"gopkg.in/natefinch/lumberjack.v2"
)

// SecurityLogConfig holds configuration for security logging
type SecurityLogConfig struct {
	Enabled           bool   `yaml:"enabled" env:"REDFLAG_SECURITY_LOG_ENABLED" default:"true"`
	Level             string `yaml:"level" env:"REDFLAG_SECURITY_LOG_LEVEL" default:"warning"` // none, error, warn, info, debug
	LogSuccesses      bool   `yaml:"log_successes" env:"REDFLAG_SECURITY_LOG_SUCCESSES" default:"false"`
	FilePath          string `yaml:"file_path" env:"REDFLAG_SECURITY_LOG_PATH" default:"/var/log/redflag/security.json"`
	MaxSizeMB         int    `yaml:"max_size_mb" env:"REDFLAG_SECURITY_LOG_MAX_SIZE" default:"100"`
	MaxFiles          int    `yaml:"max_files" env:"REDFLAG_SECURITY_LOG_MAX_FILES" default:"10"`
	RetentionDays     int    `yaml:"retention_days" env:"REDFLAG_SECURITY_LOG_RETENTION" default:"90"`
	LogToDatabase     bool   `yaml:"log_to_database" env:"REDFLAG_SECURITY_LOG_TO_DB" default:"true"`
	HashIPAddresses   bool   `yaml:"hash_ip_addresses" env:"REDFLAG_SECURITY_LOG_HASH_IP" default:"true"`
}

// Sink receives a copy of every persisted security event (e.g. the Wazuh
// emitter, INTEG-001). Implementations must never block: writeEvent runs on
// the journal path and the journal is the source of truth — a sink is a mirror.
type Sink interface {
	Emit(event *models.SecurityEvent)
}

// SecurityLogger handles structured security event logging
type SecurityLogger struct {
	config    SecurityLogConfig
	logger    *log.Logger
	db        *sqlx.DB
	lumberjack *lumberjack.Logger
	mu        sync.RWMutex
	buffer    chan *models.SecurityEvent
	bufferSize int
	stopChan   chan struct{}
	wg         sync.WaitGroup
	sink       Sink
}

// SetSink attaches an event mirror. Call before the logger is in use.
func (sl *SecurityLogger) SetSink(s Sink) {
	sl.sink = s
}

// NewSecurityLogger creates a new security logger instance
func NewSecurityLogger(config SecurityLogConfig, db *sqlx.DB) (*SecurityLogger, error) {
	if !config.Enabled || config.Level == "none" {
		return &SecurityLogger{
			config: config,
			logger: log.New(os.Stdout, "[SECURITY] ", log.LstdFlags|log.LUTC),
		}, nil
	}

	// Ensure log directory exists
	logDir := filepath.Dir(config.FilePath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create security log directory: %w", err)
	}

	// Setup rotating file writer
	lumberjack := &lumberjack.Logger{
		Filename:   config.FilePath,
		MaxSize:    config.MaxSizeMB,
		MaxBackups: config.MaxFiles,
		MaxAge:     config.RetentionDays,
		Compress:   true,
	}

	logger := &SecurityLogger{
		config:     config,
		logger:     log.New(lumberjack, "", 0), // No prefix, we'll add timestamps ourselves
		db:         db,
		lumberjack: lumberjack,
		buffer:     make(chan *models.SecurityEvent, 1000),
		bufferSize: 1000,
		stopChan:   make(chan struct{}),
	}

	// Start background processor
	logger.wg.Add(1)
	go logger.processEvents()

	return logger, nil
}

// Log writes a security event
func (sl *SecurityLogger) Log(event *models.SecurityEvent) error {
	if !sl.config.Enabled || sl.config.Level == "none" {
		return nil
	}

	// Skip successes unless configured to log them
	if !sl.config.LogSuccesses && event.EventType == models.SecurityEventTypes.CmdSignatureVerificationSuccess {
		return nil
	}

	// Filter by log level
	if !sl.shouldLogLevel(event.Level) {
		return nil
	}

	// Hash IP addresses if configured
	if sl.config.HashIPAddresses && event.IPAddress != "" {
		event.HashIPAddress()
	}

	// Try to send to buffer (non-blocking)
	select {
	case sl.buffer <- event:
	default:
		// Buffer full, log directly synchronously
		return sl.writeEvent(event)
	}

	return nil
}

// LogCommandVerificationFailure logs a command signature verification failure
func (sl *SecurityLogger) LogCommandVerificationFailure(agentID, commandID uuid.UUID, reason string) {
	event := models.NewSecurityEvent("CRITICAL", models.SecurityEventTypes.CmdSignatureVerificationFailed, agentID, "Command signature verification failed")
	event.WithDetail("command_id", commandID.String())
	event.WithDetail("reason", reason)

	_ = sl.Log(event)
}

// LogUpdateSignatureValidationFailure logs an update signature validation failure
func (sl *SecurityLogger) LogUpdateSignatureValidationFailure(agentID uuid.UUID, updateID string, reason string) {
	event := models.NewSecurityEvent("CRITICAL", models.SecurityEventTypes.UpdateSignatureVerificationFailed, agentID, "Update signature validation failed")
	event.WithDetail("update_id", updateID)
	event.WithDetail("reason", reason)

	_ = sl.Log(event)
}

// LogCommandSigned logs successful command signing
func (sl *SecurityLogger) LogCommandSigned(cmd *models.AgentCommand) {
	event := models.NewSecurityEvent("INFO", models.SecurityEventTypes.CmdSigned, cmd.AgentID, "Command signed successfully")
	event.WithDetail("command_id", cmd.ID.String())
	event.WithDetail("command_type", cmd.CommandType)
	event.WithDetail("signature_present", cmd.Signature != "")

	_ = sl.Log(event)
}

// LogNonceValidationFailure logs a nonce validation failure
func (sl *SecurityLogger) LogNonceValidationFailure(agentID uuid.UUID, nonce string, reason string) {
	event := models.NewSecurityEvent("WARNING", models.SecurityEventTypes.UpdateNonceInvalid, agentID, "Update nonce validation failed")
	event.WithDetail("nonce", nonce)
	event.WithDetail("reason", reason)

	_ = sl.Log(event)
}

// LogMachineIDMismatch logs a machine ID mismatch
func (sl *SecurityLogger) LogMachineIDMismatch(agentID uuid.UUID, expected, actual string) {
	event := models.NewSecurityEvent("WARNING", models.SecurityEventTypes.MachineIDMismatch, agentID, "Machine ID mismatch detected")
	event.WithDetail("expected_machine_id", expected)
	event.WithDetail("actual_machine_id", actual)

	_ = sl.Log(event)
}

// LogAuthJWTValidationFailure logs a JWT validation failure
func (sl *SecurityLogger) LogAuthJWTValidationFailure(agentID uuid.UUID, token string, reason string) {
	event := models.NewSecurityEvent("WARNING", models.SecurityEventTypes.AuthJWTValidationFailed, agentID, "JWT authentication failed")
	event.WithDetail("reason", reason)
	if len(token) > 0 {
		event.WithDetail("token_preview", token[:min(len(token), 20)]+"...")
	}

	_ = sl.Log(event)
}

// LogPrivateKeyNotConfigured logs when private key is not configured
func (sl *SecurityLogger) LogPrivateKeyNotConfigured() {
	event := models.NewSecurityEvent("CRITICAL", models.SecurityEventTypes.PrivateKeyNotConfigured, uuid.Nil, "Private signing key not configured")
	event.WithDetail("component", "server")

	_ = sl.Log(event)
}

// LogUnsignedCommandRejected logs when an unsigned command is rejected in strict mode
// BUG-014: Strict signing mode - reject unsigned commands (ETHOS #2 Security is Non-Negotiable)
func (sl *SecurityLogger) LogUnsignedCommandRejected(cmd *models.AgentCommand) {
	event := models.NewSecurityEvent("ERROR", models.SecurityEventTypes.CmdSignatureVerificationFailed, cmd.AgentID, "Unsigned command rejected in strict signing mode")
	event.WithDetail("command_id", cmd.ID.String())
	event.WithDetail("command_type", cmd.CommandType)
	event.WithDetail("reason", "signing service not available - command rejected")

	_ = sl.Log(event)
}

// LogAgentRegistrationFailed logs an agent registration failure
func (sl *SecurityLogger) LogAgentRegistrationFailed(ip string, reason string) {
	event := models.NewSecurityEvent("WARNING", models.SecurityEventTypes.AgentRegistrationFailed, uuid.Nil, "Agent registration failed")
	event.WithIPAddress(ip)
	event.WithDetail("reason", reason)

	_ = sl.Log(event)
}

// LogUnauthorizedAccessAttempt logs an unauthorized access attempt
func (sl *SecurityLogger) LogUnauthorizedAccessAttempt(ip, endpoint, reason string, agentID uuid.UUID) {
	event := models.NewSecurityEvent("WARNING", models.SecurityEventTypes.UnauthorizedAccessAttempt, agentID, "Unauthorized access attempt")
	event.WithIPAddress(ip)
	event.WithDetail("endpoint", endpoint)
	event.WithDetail("reason", reason)

	_ = sl.Log(event)
}

// processEvents processes events from the buffer in the background
func (sl *SecurityLogger) processEvents() {
	defer sl.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	batch := make([]*models.SecurityEvent, 0, 100)

	for {
		select {
		case event := <-sl.buffer:
			batch = append(batch, event)
			if len(batch) >= 100 {
				sl.processBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				sl.processBatch(batch)
				batch = batch[:0]
			}
		case <-sl.stopChan:
			// Process any remaining events
			for len(sl.buffer) > 0 {
				batch = append(batch, <-sl.buffer)
			}
			if len(batch) > 0 {
				sl.processBatch(batch)
			}
			return
		}
	}
}

// processBatch processes a batch of events
func (sl *SecurityLogger) processBatch(events []*models.SecurityEvent) {
	for _, event := range events {
		_ = sl.writeEvent(event)
	}
}

// writeEvent writes an event to the configured outputs
func (sl *SecurityLogger) writeEvent(event *models.SecurityEvent) error {
	// Write to file
	if err := sl.writeToFile(event); err != nil {
		log.Printf("[ERROR] Failed to write security event to file: %v", err)
	}

	// Write to database if configured
	if sl.config.LogToDatabase && sl.db != nil && event.ShouldLogToDatabase(sl.config.LogToDatabase) {
		if err := sl.writeToDatabase(event); err != nil {
			log.Printf("[ERROR] Failed to write security event to database: %v", err)
		}
	}

	// Mirror to the attached sink (non-blocking by contract) after the
	// journal writes — the journal is authoritative, the sink is best-effort.
	if sl.sink != nil {
		sl.sink.Emit(event)
	}

	return nil
}

// writeToFile writes the event as JSON to the log file. The message and
// details are sanitized to strip ANSI escapes and control characters that
// could be used for log injection, and oversized payloads are truncated.
func (sl *SecurityLogger) writeToFile(event *models.SecurityEvent) error {
	safeEvent := *event
	safeEvent.Message = SanitizeForLog(event.Message)
	safeEvent.Details = SanitizeMap(event.Details)
	safeEvent.Metadata = SanitizeMap(event.Metadata)

	jsonData, err := ValidateJSONForLog(&safeEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal security event: %w", err)
	}

	sl.logger.Println(string(jsonData))
	return nil
}

// writeToDatabase writes the event to the database
func (sl *SecurityLogger) writeToDatabase(event *models.SecurityEvent) error {
	// Create security_events table if not exists
	if err := sl.ensureSecurityEventsTable(); err != nil {
		return fmt.Errorf("failed to ensure security_events table: %w", err)
	}

	// Encode details and metadata as JSON
	detailsJSON, _ := json.Marshal(event.Details)
	metadataJSON, _ := json.Marshal(event.Metadata)

	query := `
		INSERT INTO security_events (timestamp, level, event_type, agent_id, message, trace_id, ip_address, details, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := sl.db.Exec(query,
		event.Timestamp,
		event.Level,
		event.EventType,
		event.AgentID,
		event.Message,
		event.TraceID,
		event.IPAddress,
		detailsJSON,
		metadataJSON,
	)

	return err
}

// ensureSecurityEventsTable creates the security_events table if it doesn't exist
func (sl *SecurityLogger) ensureSecurityEventsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS security_events (
			id SERIAL PRIMARY KEY,
			timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
			level VARCHAR(20) NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			agent_id UUID,
			message TEXT NOT NULL,
			trace_id VARCHAR(100),
			ip_address VARCHAR(100),
			details JSONB,
			metadata JSONB,
			INDEX idx_security_events_timestamp (timestamp),
			INDEX idx_security_events_agent_id (agent_id),
			INDEX idx_security_events_level (level),
			INDEX idx_security_events_event_type (event_type)
		)`

	_, err := sl.db.Exec(query)
	return err
}

// Close closes the security logger and flushes any pending events
func (sl *SecurityLogger) Close() error {
	if sl.lumberjack != nil {
		close(sl.stopChan)
		sl.wg.Wait()
		if err := sl.lumberjack.Close(); err != nil {
			return err
		}
	}
	return nil
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

	configLevel := levels[sl.config.Level]
	eventLvl, exists := levels[eventLevel]
	if !exists {
		eventLvl = 2 // Default to WARNING
	}

	return eventLvl <= configLevel
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

