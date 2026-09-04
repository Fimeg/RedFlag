package logging

// This file contains example code showing how to integrate the security logger
// into various parts of the server application.

import (
	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// Example of how to initialize the security logger in main.go
func ExampleInitializeSecurityLogger(cfg *config.Config, db *sqlx.DB) (*SecurityLogger, error) {
	// Convert config to security logger config
	secConfig := SecurityLogConfig{
		Enabled:           cfg.SecurityLogging.Enabled,
		Level:             cfg.SecurityLogging.Level,
		LogSuccesses:      cfg.SecurityLogging.LogSuccesses,
		FilePath:          cfg.SecurityLogging.FilePath,
		MaxSizeMB:         cfg.SecurityLogging.MaxSizeMB,
		MaxFiles:          cfg.SecurityLogging.MaxFiles,
		RetentionDays:     cfg.SecurityLogging.RetentionDays,
		LogToDatabase:     cfg.SecurityLogging.LogToDatabase,
		HashIPAddresses:   cfg.SecurityLogging.HashIPAddresses,
	}

	// Create the security logger
	securityLogger, err := NewSecurityLogger(secConfig, db)
	if err != nil {
		return nil, err
	}

	return securityLogger, nil
}

// Example of using the security logger in authentication handlers
func ExampleAuthHandler(securityLogger *SecurityLogger, clientIP string) {
	// Example: JWT validation failed
	securityLogger.LogAuthJWTValidationFailure(
		uuid.Nil, // Agent ID might not be known yet
		"invalid.jwt.token",
		"expired signature",
	)

	// Example: Unauthorized access attempt
	securityLogger.LogUnauthorizedAccessAttempt(
		clientIP,
		"/api/v1/admin/users",
		"insufficient privileges",
		uuid.Nil,
	)
}

// Example of using the security logger in command/verification handlers
func ExampleCommandVerificationHandler(securityLogger *SecurityLogger, agentID, commandID uuid.UUID, signature string) {
	// Simulate signature verification
	signatureValid := false // In real code, this would be actual verification result

	if !signatureValid {
		securityLogger.LogCommandVerificationFailure(
			agentID,
			commandID,
			"signature mismatch: expected X, got Y",
		)
	} else {
		// Only log success if configured to do so
		if securityLogger.config.LogSuccesses {
			event := models.NewSecurityEvent(
				"INFO",
				models.SecurityEventTypes.CmdSignatureVerificationSuccess,
				agentID,
				"Command signature verification succeeded",
			)
			event.WithDetail("command_id", commandID.String())
			securityLogger.Log(event)
		}
	}
}

// Example of using the security logger in update handlers
func ExampleUpdateHandler(securityLogger *SecurityLogger, agentID uuid.UUID, updateData []byte, signature string) {
	// Simulate update nonce validation
	nonceValid := false // In real code, this would be actual validation

	if !nonceValid {
		securityLogger.LogNonceValidationFailure(
			agentID,
			"12345678-1234-1234-1234-123456789012",
			"nonce not found in database",
		)
	}

	// Simulate signature verification
	signatureValid := false
	if !signatureValid {
		securityLogger.LogUpdateSignatureValidationFailure(
			agentID,
			"update-123",
			"invalid signature format",
		)
	}
}

// Example of using the security logger on agent registration
func ExampleAgentRegistrationHandler(securityLogger *SecurityLogger, clientIP string) {
	securityLogger.LogAgentRegistrationFailed(
		clientIP,
		"invalid registration token",
	)
}

// Example of checking if a private key is configured
func ExampleCheckPrivateKey(securityLogger *SecurityLogger, cfg *config.Config) {
	if cfg.SigningPrivateKey == "" {
		securityLogger.LogPrivateKeyNotConfigured()
	}
}