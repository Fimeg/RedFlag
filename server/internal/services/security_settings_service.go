package services

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/Fimeg/RedFlag/server/internal/crypto"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

type SecuritySettingsService struct {
	settingsQueries *queries.SecuritySettingsQueries
	signingService  *SigningService
	encryptionKey   []byte
}

func isRetiredServerSetting(category, key string) bool {
	if category != "command_signing" {
		return false
	}
	switch key {
	case "enabled", "enforcement_mode", "algorithm":
		return true
	default:
		return false
	}
}

func retiredServerSettingError(category, key string) error {
	return fmt.Errorf("%s.%s was removed: signing uses the provisioned Ed25519 service and enforcement is agent-local and default-closed", category, key)
}

// NewSecuritySettingsService creates a new security settings service. The
// base64-encoded AES-256 key is provisioned by config.resolveAtRestKeys (the
// single secret rail), so there is no ephemeral fallback here: an absent or
// malformed key is a startup error, never a silently regenerated key that would
// orphan previously-encrypted settings.
func NewSecuritySettingsService(settingsQueries *queries.SecuritySettingsQueries, signingService *SigningService, encryptionKeyB64 string) (*SecuritySettingsService, error) {
	key, err := base64.StdEncoding.DecodeString(encryptionKeyB64)
	if err != nil {
		return nil, fmt.Errorf("invalid settings encryption key format: %w", err)
	}
	if len(key) != crypto.KeySize {
		return nil, fmt.Errorf("settings encryption key must be %d bytes, got %d", crypto.KeySize, len(key))
	}

	return &SecuritySettingsService{
		settingsQueries: settingsQueries,
		signingService:  signingService,
		encryptionKey:   key,
	}, nil
}

// GetSetting retrieves a security setting with proper priority resolution
func (s *SecuritySettingsService) GetSetting(category, key string) (interface{}, error) {
	// Existing databases can still contain the pre-doctrine row. Do not expose
	// it through either the single-setting or aggregate APIs: stale clients must
	// see that server authority as gone, not mistake old data for live policy.
	if isRetiredServerSetting(category, key) {
		return nil, retiredServerSettingError(category, key)
	}

	// Priority 1: Environment variables
	if envValue := s.getEnvironmentValue(category, key); envValue != nil {
		return envValue, nil
	}

	// Priority 2: Config file values (this would be implemented based on your config structure)
	if configValue := s.getConfigValue(category, key); configValue != nil {
		return configValue, nil
	}

	// Priority 3: Database settings
	if dbSetting, err := s.settingsQueries.GetSetting(category, key); err == nil && dbSetting != nil {
		var value interface{}
		if dbSetting.IsEncrypted {
			decrypted, err := s.decrypt(dbSetting.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt setting: %w", err)
			}
			if err := json.Unmarshal([]byte(decrypted), &value); err != nil {
				return nil, fmt.Errorf("failed to unmarshal decrypted setting: %w", err)
			}
		} else {
			if err := json.Unmarshal([]byte(dbSetting.Value), &value); err != nil {
				return nil, fmt.Errorf("failed to unmarshal setting: %w", err)
			}
		}
		return value, nil
	}

	// Priority 4: Hardcoded defaults
	if defaultValue := s.getDefaultValue(category, key); defaultValue != nil {
		return defaultValue, nil
	}

	return nil, fmt.Errorf("setting not found: %s.%s", category, key)
}

// SetSetting updates a security setting with validation and audit logging
func (s *SecuritySettingsService) SetSetting(category, key string, value interface{}, userID uuid.UUID, reason string) error {
	// Validate the setting
	if err := s.ValidateSetting(category, key, value); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Serialize, and encrypt sensitive values, before persistence. SEC-024: the
	// encrypt/decrypt hooks existed but the write path stored plaintext; sensitive
	// settings now persist as ciphertext with is_encrypted set.
	stored, isEncrypted, err := s.serializeForStorage(category, key, value)
	if err != nil {
		return err
	}

	// Check if setting exists
	existing, err := s.settingsQueries.GetSetting(category, key)
	if err != nil {
		return fmt.Errorf("failed to check existing setting: %w", err)
	}

	var oldValue *string
	var settingID uuid.UUID

	if existing != nil {
		// Update existing setting
		updated, oldVal, err := s.settingsQueries.UpdateSetting(category, key, stored, isEncrypted, &userID)
		if err != nil {
			return fmt.Errorf("failed to update setting: %w", err)
		}
		oldValue = oldVal
		settingID = updated.ID
	} else {
		// Create new setting
		created, err := s.settingsQueries.CreateSetting(category, key, stored, isEncrypted, &userID)
		if err != nil {
			return fmt.Errorf("failed to create setting: %w", err)
		}
		settingID = created.ID
	}

	// Create audit log. Never write a sensitive value (old or new) to the audit
	// trail in plaintext — that would reintroduce the exposure we just closed.
	valueJSON, _ := json.Marshal(value)
	newAudit := string(valueJSON)
	oldAudit := stringOrNil(oldValue)
	if isEncrypted {
		newAudit = "[REDACTED]"
		oldAudit = "[REDACTED]"
	}
	if err := s.settingsQueries.CreateAuditLog(
		settingID,
		userID,
		"update",
		oldAudit,
		newAudit,
		reason,
	); err != nil {
		// Log error but don't fail the operation
		log.Printf("[WARNING] [server] [security] audit_log_create_failed error=%v", err)
	}

	return nil
}

// GetAllSettings retrieves all security settings organized by category
func (s *SecuritySettingsService) GetAllSettings() (map[string]map[string]interface{}, error) {
	// Get all default values first
	result := s.getDefaultSettings()

	// Override with database settings
	dbSettings, err := s.settingsQueries.GetAllSettings()
	if err != nil {
		return nil, fmt.Errorf("failed to get database settings: %w", err)
	}

	for _, setting := range dbSettings {
		if isRetiredServerSetting(setting.Category, setting.Key) {
			continue
		}

		var value interface{}
		if setting.IsEncrypted {
			decrypted, err := s.decrypt(setting.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt setting %s.%s: %w", setting.Category, setting.Key, err)
			}
			if err := json.Unmarshal([]byte(decrypted), &value); err != nil {
				return nil, fmt.Errorf("failed to unmarshal decrypted setting %s.%s: %w", setting.Category, setting.Key, err)
			}
		} else {
			if err := json.Unmarshal([]byte(setting.Value), &value); err != nil {
				return nil, fmt.Errorf("failed to unmarshal setting %s.%s: %w", setting.Category, setting.Key, err)
			}
		}

		if result[setting.Category] == nil {
			result[setting.Category] = make(map[string]interface{})
		}
		result[setting.Category][setting.Key] = value
	}

	// Override with config file settings
	for category, settings := range result {
		for key := range settings {
			if configValue := s.getConfigValue(category, key); configValue != nil {
				result[category][key] = configValue
			}
		}
	}

	// Override with environment variables
	for category, settings := range result {
		for key := range settings {
			if envValue := s.getEnvironmentValue(category, key); envValue != nil {
				result[category][key] = envValue
			}
		}
	}

	return result, nil
}

// GetSettingsByCategory retrieves all settings for a specific category
func (s *SecuritySettingsService) GetSettingsByCategory(category string) (map[string]interface{}, error) {
	allSettings, err := s.GetAllSettings()
	if err != nil {
		return nil, err
	}

	if categorySettings, exists := allSettings[category]; exists {
		return categorySettings, nil
	}

	return nil, fmt.Errorf("category not found: %s", category)
}

// ValidateSetting validates a security setting value
func (s *SecuritySettingsService) ValidateSetting(category, key string, value interface{}) error {
	switch fmt.Sprintf("%s.%s", category, key) {
	case "nonce_validation.timeout_seconds":
		if timeout, ok := value.(float64); ok {
			if timeout < 60 || timeout > 3600 {
				return fmt.Errorf("nonce timeout must be between 60 and 3600 seconds")
			}
		} else {
			return fmt.Errorf("nonce timeout must be a number")
		}

	case "command_signing.enforcement_mode":
		// Removed: command verification posture is agent-local and
		// default-closed. The server cannot weaken it remotely, so no
		// server-side knob is offered. Reject writes to surface stale clients.
		return retiredServerSettingError(category, key)

	case "command_signing.enabled", "command_signing.algorithm":
		// Removed: these settings were never consumed by the signing service.
		// Availability follows the provisioned Ed25519 key, not mutable DB state.
		return retiredServerSettingError(category, key)

	case "update_signing.enforcement_mode", "machine_binding.enforcement_mode":
		if mode, ok := value.(string); ok {
			validModes := []string{"strict", "warning", "disabled"}
			valid := false
			for _, m := range validModes {
				if mode == m {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("enforcement mode must be one of: strict, warning, disabled")
			}
		} else {
			return fmt.Errorf("enforcement mode must be a string")
		}

	case "signature_verification.log_retention_days":
		if days, ok := value.(float64); ok {
			if days < 1 || days > 365 {
				return fmt.Errorf("log retention must be between 1 and 365 days")
			}
		} else {
			return fmt.Errorf("log retention must be a number")
		}

	case "operational.jitter_max_seconds":
		if v, ok := value.(float64); ok {
			if v < 0 || v > 300 {
				return fmt.Errorf("jitter_max_seconds must be between 0 and 300")
			}
		} else {
			return fmt.Errorf("jitter_max_seconds must be a number")
		}

	case "operational.backoff_base_seconds":
		if v, ok := value.(float64); ok {
			if v < 1 || v > 300 {
				return fmt.Errorf("backoff_base_seconds must be between 1 and 300")
			}
		} else {
			return fmt.Errorf("backoff_base_seconds must be a number")
		}

	case "operational.backoff_max_seconds":
		if v, ok := value.(float64); ok {
			if v < 10 || v > 3600 {
				return fmt.Errorf("backoff_max_seconds must be between 10 and 3600")
			}
		} else {
			return fmt.Errorf("backoff_max_seconds must be a number")
		}

	case "operational.retention_metrics_days",
		"operational.retention_events_days",
		"operational.retention_history_days":
		if days, ok := value.(float64); ok {
			if days < 0 || days > 3650 {
				return fmt.Errorf("retention days must be between 0 (keep forever) and 3650")
			}
		} else {
			return fmt.Errorf("retention days must be a number")
		}

	case "supply_chain.min_package_age_hours":
		if hours, ok := value.(float64); ok {
			if hours < 0 || hours > 8760 {
				return fmt.Errorf("min_package_age_hours must be between 0 and 8760 (one year)")
			}
		} else {
			return fmt.Errorf("min_package_age_hours must be a number")
		}

	case "supply_chain.gate_enforcement":
		if mode, ok := value.(string); ok {
			switch strings.ToLower(mode) {
			case "off", "warn", "block":
			default:
				return fmt.Errorf("gate_enforcement must be one of: off, warn, block")
			}
		} else {
			return fmt.Errorf("gate_enforcement must be a string")
		}

	case "supply_chain.block_unknown_age":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("block_unknown_age must be a boolean")
		}

	case "supply_chain.soak_window_days":
		if days, ok := value.(float64); ok {
			if days < 0 || days > 365 {
				return fmt.Errorf("soak_window_days must be between 0 and 365")
			}
		} else {
			return fmt.Errorf("soak_window_days must be a number")
		}

	case "supply_chain.soak_enforcement":
		if mode, ok := value.(string); ok {
			switch strings.ToLower(mode) {
			case "off", "warn", "block":
			default:
				return fmt.Errorf("soak_enforcement must be one of: off, warn, block")
			}
		} else {
			return fmt.Errorf("soak_enforcement must be a string")
		}

	case "command_signing.stale_key_max_age_hours":
		// Bounded, never zero/infinite: forward-only requires a fail-closed
		// ceiling. 1h floor, 720h (30d) ceiling — mirrors the agent's doctrinal
		// clamp so the UI can't offer a value the agent would reject.
		if hours, ok := value.(float64); ok {
			if hours < 1 || hours > 720 {
				return fmt.Errorf("stale_key_max_age_hours must be between 1 and 720 (30 days)")
			}
		} else {
			return fmt.Errorf("stale_key_max_age_hours must be a number")
		}

	case "update_signing.algorithm":
		if algo, ok := value.(string); ok {
			if algo != "ed25519" {
				return fmt.Errorf("only ed25519 algorithm is currently supported")
			}
		} else {
			return fmt.Errorf("algorithm must be a string")
		}

	case "observability.metrics_enabled":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("metrics_enabled must be a boolean")
		}

	case "observability.metrics_token_hash":
		tokenHash, ok := value.(string)
		if !ok {
			return fmt.Errorf("metrics_token_hash must be a string")
		}
		if tokenHash == "" {
			return nil
		}
		decoded, err := hex.DecodeString(tokenHash)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("metrics_token_hash must be a SHA-256 hex digest")
		}
	}

	return nil
}

// InitializeDefaultSettings creates default settings in the database if they don't exist
func (s *SecuritySettingsService) InitializeDefaultSettings() error {
	defaults := s.getDefaultSettings()

	for category, settings := range defaults {
		for key, value := range settings {
			existing, err := s.settingsQueries.GetSetting(category, key)
			if err != nil {
				return fmt.Errorf("failed to check existing setting %s.%s: %w", category, key, err)
			}

			if existing == nil {
				stored, isEncrypted, err := s.serializeForStorage(category, key, value)
				if err != nil {
					return fmt.Errorf("failed to serialize default setting %s.%s: %w", category, key, err)
				}
				if _, err := s.settingsQueries.CreateSetting(category, key, stored, isEncrypted, nil); err != nil {
					return fmt.Errorf("failed to create default setting %s.%s: %w", category, key, err)
				}
			}
		}
	}

	return nil
}

// Helper methods

func (s *SecuritySettingsService) getDefaultSettings() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		"command_signing": {
			// stale_key_max_age_hours bounds how long an offline agent keeps
			// trusting its cached server public key before it fails closed
			// (SEC-028). Operator policy within a doctrinal range — the agent
			// enforces a hard ceiling regardless, so this tunes but never disables
			// forward-only. Default 168h (7d).
			"stale_key_max_age_hours": 168.0,
		},
		"update_signing": {
			"enabled":          true,
			"enforcement_mode": "strict",
			"allow_unsigned":   false,
		},
		"nonce_validation": {
			"timeout_seconds":      600,
			"reject_expired":       true,
			"log_expired_attempts": true,
		},
		"machine_binding": {
			"enabled":          true,
			"enforcement_mode": "strict",
			"strict_action":    "reject",
		},
		"signature_verification": {
			"log_level":          "warn",
			"log_retention_days": 30,
			"log_failures":       true,
			"alert_on_failure":   true,
		},
		// Shai-Hulud-class defenses. min_package_age_hours is the soak window
		// applied at approval time; gate_enforcement chooses how strictly to
		// apply it: "off" disables the check, "warn" attaches a warning to the
		// approval response (default — sovereignty principle), "block" rejects
		// approval with 409. See docs (post-v0.2.0.0 writeup) for the threat
		// model.
		"supply_chain": {
			"min_package_age_hours": 24.0,
			"gate_enforcement":      "warn",
			// block_unknown_age fails the gate *closed* when a registry-backed
			// package (npm/PyPI) cannot be dated — a dark recency source is
			// itself a worm signal. Default false preserves the sovereignty
			// principle (infra hiccups never block legitimate work); operators
			// running a hostile-supply-chain posture can opt in.
			"block_unknown_age": false,
			// Version soak gate (GATE-005), applied on the install path. A
			// version must have been observed in the fleet for soak_window_days
			// before it is install-eligible; soak_enforcement chooses off/warn/
			// block. Default 14d/block holds the "newest version" signal that
			// zero-day supply-chain attacks ride on.
			"soak_window_days": 14.0,
			"soak_enforcement": "block",
		},
		"observability": {
			"metrics_enabled":    false,
			"metrics_token_hash": "",
		},
		"notify": {
			"ntfy_url":      "",
			"ntfy_events":   "",
			"ntfy_severity": "error,critical",
			"smtp_host":     "",
			"smtp_from":     "",
			"smtp_to":       "",
			"smtp_events":   "",
			"smtp_severity": "error,critical",
		},
	}
}

func (s *SecuritySettingsService) getDefaultValue(category, key string) interface{} {
	defaults := s.getDefaultSettings()
	if cat, exists := defaults[category]; exists {
		if value, exists := cat[key]; exists {
			return value
		}
	}
	return nil
}

func (s *SecuritySettingsService) getEnvironmentValue(category, key string) interface{} {
	envKey := fmt.Sprintf("REDFLAG_%s_%s", strings.ToUpper(category), strings.ToUpper(key))
	envValue := os.Getenv(envKey)
	if envValue == "" {
		return nil
	}

	// Try to parse as boolean
	if strings.ToLower(envValue) == "true" {
		return true
	}
	if strings.ToLower(envValue) == "false" {
		return false
	}

	// Try to parse as number
	if num, err := strconv.ParseFloat(envValue, 64); err == nil {
		return num
	}

	// Return as string
	return envValue
}

func (s *SecuritySettingsService) getConfigValue(category, key string) interface{} {
	// This would be implemented based on your config structure
	// For now, returning nil to prioritize env vars and database
	return nil
}

func (s *SecuritySettingsService) isSensitiveSetting(category, key string) bool {
	// Define which settings are sensitive and should be encrypted
	sensitive := map[string]bool{
		"command_signing.private_key": true,
		"update_signing.private_key":  true,
		"machine_binding.server_key":  true,
		"encryption.master_key":       true,
	}

	settingKey := fmt.Sprintf("%s.%s", category, key)
	return sensitive[settingKey]
}

// serializeForStorage marshals a setting value to its JSON column form and, for
// sensitive keys, encrypts it. It returns the exact string to persist plus the
// is_encrypted flag. Non-sensitive settings store plain JSON exactly as before;
// sensitive settings store base64(AES-GCM(JSON)) so the GetSetting read path's
// decrypt-then-unmarshal step round-trips. SEC-024.
func (s *SecuritySettingsService) serializeForStorage(category, key string, value interface{}) (string, bool, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", false, fmt.Errorf("marshal setting value: %w", err)
	}
	if !s.isSensitiveSetting(category, key) {
		return string(raw), false, nil
	}
	enc, err := s.encrypt(string(raw))
	if err != nil {
		return "", false, fmt.Errorf("encrypt sensitive setting %s.%s: %w", category, key, err)
	}
	return enc, true, nil
}

func (s *SecuritySettingsService) encrypt(value string) (string, error) {
	ciphertext, err := crypto.Encrypt(s.encryptionKey, []byte(value))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *SecuritySettingsService) decrypt(encryptedValue string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedValue)
	if err != nil {
		return "", err
	}
	plaintext, err := crypto.Decrypt(s.encryptionKey, data)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func stringOrNil(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetNonceTimeout returns the current nonce validation timeout in seconds
func (s *SecuritySettingsService) GetNonceTimeout() (int, error) {
	value, err := s.GetSetting("nonce_validation", "timeout_seconds")
	if err != nil {
		return 600, err // Return default on error
	}

	if timeout, ok := value.(float64); ok {
		return int(timeout), nil
	}

	return 600, nil // Return default if type is wrong
}

// GetEnforcementMode returns the enforcement mode for a given category
func (s *SecuritySettingsService) GetEnforcementMode(category string) (string, error) {
	value, err := s.GetSetting(category, "enforcement_mode")
	if err != nil {
		return "strict", err // Return default on error
	}

	if mode, ok := value.(string); ok {
		return mode, nil
	}

	return "strict", nil // Return default if type is wrong
}

// IsSignatureVerificationEnabled returns whether signature verification is enabled for a category
func (s *SecuritySettingsService) IsSignatureVerificationEnabled(category string) (bool, error) {
	value, err := s.GetSetting(category, "enabled")
	if err != nil {
		return true, err // Return default on error
	}

	if enabled, ok := value.(bool); ok {
		return enabled, nil
	}

	return true, nil // Return default if type is wrong
}

// GetAuditTrail returns recent security settings audit entries (F-E1-7 fix)
func (s *SecuritySettingsService) GetAuditTrail(limit int) ([]models.SecuritySettingAudit, error) {
	return s.settingsQueries.GetAllAuditLogs(limit)
}

// GetPolicyBool reads a policy.<key> setting as bool. On any error (missing,
// wrong type, decrypt failure) returns the supplied fallback. Hot path: never
// blocks on settings being absent. Callers should treat the fallback as the
// safe default for the gate.
func (s *SecuritySettingsService) GetPolicyBool(key string, fallback bool) bool {
	value, err := s.GetSetting("policy", key)
	if err != nil {
		return fallback
	}
	if b, ok := value.(bool); ok {
		return b
	}
	return fallback
}

// GetPolicyString reads a policy.<key> setting as a string. On error or type
// mismatch returns the supplied fallback. Used for enumerated policy values
// (e.g. auto_approve_max_severity) where a missing row means "use the default".
func (s *SecuritySettingsService) GetPolicyString(key, fallback string) string {
	value, err := s.GetSetting("policy", key)
	if err != nil {
		return fallback
	}
	if str, ok := value.(string); ok {
		return str
	}
	return fallback
}

// GetSupplyChainGateConfig resolves the approval-time package-age gate policy
// from the layered settings (env > config > DB > baked default, via GetSetting).
// This is what makes the supply_chain.* settings live: PackageAgeGateConfig()
// alone reads only env, so a DB/UI override would otherwise be inert. Callers
// that hold a settings service should prefer this; the free PackageAgeGateConfig
// remains the fallback when no service is wired.
func (s *SecuritySettingsService) GetSupplyChainGateConfig() (minAgeHours float64, enforcement string, blockUnknownAge bool) {
	// Start from the env/baked defaults so a missing service or DB row degrades
	// to exactly the legacy behavior.
	minAgeHours, enforcement = PackageAgeGateConfig()
	blockUnknownAge = false

	if v, err := s.GetSetting("supply_chain", "min_package_age_hours"); err == nil {
		if f, ok := v.(float64); ok && f >= 0 {
			minAgeHours = f
		}
	}
	if v, err := s.GetSetting("supply_chain", "gate_enforcement"); err == nil {
		if str, ok := v.(string); ok {
			switch strings.ToLower(str) {
			case "off", "warn", "block":
				enforcement = strings.ToLower(str)
			}
		}
	}
	if v, err := s.GetSetting("supply_chain", "block_unknown_age"); err == nil {
		if b, ok := v.(bool); ok {
			blockUnknownAge = b
		}
	}
	return
}

// GetSoakGateConfig resolves the install-path version-soak gate policy (GATE-005)
// from the layered settings (env > config > DB > baked default, via GetSetting),
// so an admin's DB/UI policy actually drives the gate. SoakGateConfig() alone
// reads only env; this is the live equivalent and the free function remains the
// fallback when no settings service is wired.
func (s *SecuritySettingsService) GetSoakGateConfig() (requiredDays float64, enforcement string) {
	requiredDays, enforcement = SoakGateConfig()

	if v, err := s.GetSetting("supply_chain", "soak_window_days"); err == nil {
		if f, ok := v.(float64); ok && f >= 0 {
			requiredDays = f
		}
	}
	if v, err := s.GetSetting("supply_chain", "soak_enforcement"); err == nil {
		if str, ok := v.(string); ok {
			switch strings.ToLower(str) {
			case "off", "warn", "block":
				enforcement = strings.ToLower(str)
			}
		}
	}
	return
}

// GetOperationalInt reads an operational.<key> setting as int. On error or
// type mismatch returns the supplied fallback. Used for runtime tuning where
// a missing row should not stop the service.
func (s *SecuritySettingsService) GetOperationalInt(key string, fallback int) int {
	value, err := s.GetSetting("operational", key)
	if err != nil {
		return fallback
	}
	if f, ok := value.(float64); ok {
		return int(f)
	}
	if i, ok := value.(int); ok {
		return i
	}
	return fallback
}
