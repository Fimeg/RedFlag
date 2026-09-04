package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/Fimeg/RedFlag/agent/internal/event"
	"github.com/Fimeg/RedFlag/agent/internal/version"
	"github.com/gofrs/uuid/v5"
)

var teeLogger *event.TeeLogger

// InitLogger sets the package-level TeeLogger for dual-output logging.
func InitLogger(l *event.TeeLogger) {
	teeLogger = l
}

// MigrationState tracks migration completion status (used by migration package)
type MigrationState struct {
	LastCompleted       map[string]time.Time `json:"last_completed"`
	AgentVersion        string               `json:"agent_version"`
	ConfigVersion       string               `json:"config_version"`
	Timestamp           time.Time            `json:"timestamp"`
	Success             bool                 `json:"success"`
	RollbackPath        string               `json:"rollback_path,omitempty"`
	CompletedMigrations []string             `json:"completed_migrations"`
}

// ProxyConfig holds proxy configuration
type ProxyConfig struct {
	Enabled  bool   `json:"enabled"`
	HTTP     string `json:"http,omitempty"`     // HTTP proxy URL
	HTTPS    string `json:"https,omitempty"`    // HTTPS proxy URL
	NoProxy  string `json:"no_proxy,omitempty"` // Comma-separated hosts to bypass proxy
	Username string `json:"username,omitempty"` // Proxy username (optional)
	Password string `json:"password,omitempty"` // Proxy password (optional)
}

// TLSConfig holds TLS/security configuration
type TLSConfig struct {
	InsecureSkipVerify bool   `json:"insecure_skip_verify"` // Skip TLS certificate verification
	CertFile           string `json:"cert_file,omitempty"`  // Client certificate file
	KeyFile            string `json:"key_file,omitempty"`   // Client key file
	CAFile             string `json:"ca_file,omitempty"`    // CA certificate file
}

// NetworkConfig holds network-related configuration
type NetworkConfig struct {
	Timeout     time.Duration `json:"timeout"`       // Request timeout
	RetryCount  int           `json:"retry_count"`   // Number of retries
	RetryDelay  time.Duration `json:"retry_delay"`   // Delay between retries
	MaxIdleConn int           `json:"max_idle_conn"` // Maximum idle connections
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `json:"level"`          // Log level (debug, info, warn, error)
	File       string `json:"file,omitempty"` // Log file path (optional)
	MaxSize    int    `json:"max_size"`       // Max log file size in MB
	MaxBackups int    `json:"max_backups"`    // Max number of log file backups
	MaxAge     int    `json:"max_age"`        // Max age of log files in days
}

// SecurityLogConfig holds configuration for security logging
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

// CommandSigningConfig holds configuration for command signature verification
type CommandSigningConfig struct {
	Enabled         bool   `json:"enabled" env:"REDFLAG_AGENT_COMMAND_SIGNING_ENABLED" default:"true"`
	EnforcementMode string `json:"enforcement_mode" env:"REDFLAG_AGENT_COMMAND_ENFORCEMENT_MODE" default:"strict"` // strict, warning, disabled
	// StaleKeyMaxAgeHours bounds how long the agent serves its cached server
	// public key while the server is unreachable (SEC-028). 0 = built-in default.
	// The agent clamps to a doctrinal ceiling regardless; this only tunes within
	// it. Delivered fleet-wide via GET /api/v1/agents/:id/config.
	StaleKeyMaxAgeHours int `json:"stale_key_max_age_hours,omitempty" env:"REDFLAG_AGENT_STALE_KEY_MAX_AGE_HOURS"`
}

// PollingConfig holds admin-adjustable polling resilience tuning. These shape
// the check-in jitter and the reconnect backoff curve. Zero values fall back to
// built-in defaults (see the resilience defaults in internal/agent/loop.go), so
// older config files without these fields keep working.
type PollingConfig struct {
	JitterMaxSeconds   int `json:"jitter_max_seconds,omitempty"`   // cap on proportional check-in jitter (default 30)
	BackoffBaseSeconds int `json:"backoff_base_seconds,omitempty"` // reconnect backoff floor (default 10)
	BackoffMaxSeconds  int `json:"backoff_max_seconds,omitempty"`  // reconnect backoff ceiling (default 300)
}

// ProcessExplorerConfig holds limits for process detail data collection.
// Zero values use the built-in defaults.
type ProcessExplorerConfig struct {
	MaxOpenFiles      int `json:"max_open_files,omitempty"`      // Cap on open files per process (default 2000)
	MaxSockets        int `json:"max_sockets,omitempty"`         // Cap on open sockets per process (default 500)
	MaxPipes          int `json:"max_pipes,omitempty"`           // Cap on open pipes per process (default 500)
	MaxMemoryMap      int `json:"max_memory_map,omitempty"`      // Cap on memory map entries per process (default 2000)
	MaxNamespaces     int `json:"max_namespaces,omitempty"`      // Cap on namespace entries per process (default 50)
	MaxEnvKeys        int `json:"max_env_keys,omitempty"`        // Cap on environment variable keys per process (default 200)
	MaxListeningPorts int `json:"max_listening_ports,omitempty"` // Cap on listening ports per process (default 100)
}

// Config holds agent configuration
type Config struct {
	// Version Information
	Version      string `json:"version,omitempty"`       // Config schema version
	AgentVersion string `json:"agent_version,omitempty"` // Agent binary version

	// Server Configuration
	ServerURL         string `json:"server_url"`
	RegistrationToken string `json:"registration_token,omitempty"` // One-time registration token

	// Agent Authentication
	AgentID      uuid.UUID `json:"agent_id"`
	Token        string    `json:"token"`         // Short-lived access token (24h)
	RefreshToken string    `json:"refresh_token"` // Long-lived refresh token (90d)

	// Agent Behavior
	CheckInInterval int `json:"check_in_interval"`

	// Rapid polling mode for faster response during operations
	RapidPollingEnabled bool      `json:"rapid_polling_enabled"`
	RapidPollingUntil   time.Time `json:"rapid_polling_until"`

	// Polling resilience tuning (admin-adjustable; server authority)
	Polling PollingConfig `json:"polling,omitempty"`

	// Degraded mode for operation after repeated failures
	DegradedMode bool `json:"degraded_mode"`

	// Network Configuration
	Network NetworkConfig `json:"network,omitempty"`

	// Proxy Configuration
	Proxy ProxyConfig `json:"proxy,omitempty"`

	// Security Configuration
	TLS TLSConfig `json:"tls,omitempty"`

	// Logging Configuration
	Logging LoggingConfig `json:"logging,omitempty"`

	// Security Logging Configuration
	SecurityLogging SecurityLogConfig `json:"security_logging,omitempty"`

	// Command Signing Configuration
	CommandSigning CommandSigningConfig `json:"command_signing,omitempty"`

	// Package Hash Verification Configuration
	PackageHashes map[string]string `json:"package_hashes,omitempty"` // package_name:expected_sha256

	// Agent Metadata
	Tags         []string          `json:"tags,omitempty"`         // User-defined tags
	Metadata     map[string]string `json:"metadata,omitempty"`     // Custom metadata
	DisplayName  string            `json:"display_name,omitempty"` // Human-readable name
	Organization string            `json:"organization,omitempty"` // Organization/group

	// OS Type (linux, windows, darwin)
	OSType string `json:"os_type,omitempty"`

	// OS holds OS-specific information
	OS OS `json:"os,omitempty"`

	// Subsystem Configuration
	Subsystems SubsystemsConfig `json:"subsystems,omitempty"` // Scanner subsystem configs

	// Kernel Enforcement Configuration
	KernelEnforcement KernelEnforcementConfig `json:"kernel_enforcement,omitempty"`

	// Desktop App Configuration
	Desktop DesktopConfig `json:"desktop,omitempty"`

	// Process Explorer Configuration
	ProcessExplorer ProcessExplorerConfig `json:"process_explorer,omitempty"`

	// Migration State
	MigrationState *MigrationState `json:"migration_state,omitempty"` // Migration completion tracking
}

// DesktopConfig controls the native Qt/QML local-machine operations console.
// The agent service spawns the desktop binary as a child process when a
// desktop session is detected. The binary connects back to the agent's
// local API socket.
type DesktopConfig struct {
	Enabled         bool `json:"enabled"`           // Whether to auto-launch the desktop app
	MaxRestarts     int  `json:"max_restarts"`      // Max restarts before giving up (0 = unlimited)
	RestartDelaySec int  `json:"restart_delay_sec"` // Seconds between restart attempts
}

// Load reads configuration from multiple sources with priority order:
// 1. CLI flags
// 2. Environment variables
// 3. Configuration file
// 4. Default values
func Load(configPath string, cliFlags *CLIFlags) (*Config, error) {
	// Load existing config from file first
	config, err := loadFromFile(configPath)
	if err != nil {
		// Only use defaults if file doesn't exist or can't be read
		config = getDefaultConfig()
	} else {
		// Config loaded and merged with defaults. Persist any new keys that
		// the upgrade brought in but were not in the on-disk file — the loaded
		// struct has them via mergeConfigPreservingDefaults, but the file
		// does not. This catches new fields (desktop, polling, etc.) that
		// fresh installs get from the template but upgrades miss.
		persistNewDefaults(configPath, config)
	}

	// Override with environment variables
	mergeConfig(config, loadFromEnv())

	// Override with CLI flags (highest priority)
	if cliFlags != nil {
		mergeConfig(config, loadFromFlags(cliFlags))
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// CLIFlags holds command line flag values
type CLIFlags struct {
	ServerURL         string
	RegistrationToken string
	ProxyHTTP         string
	ProxyHTTPS        string
	ProxyNoProxy      string
	LogLevel          string
	ConfigFile        string
	Tags              []string
	Organization      string
	DisplayName       string
	InsecureTLS       bool
}

// getConfigVersionForAgent extracts the config version from the agent version
// Agent version format: v0.1.23.6 where the fourth octet (.6) maps to config version
func getConfigVersionForAgent(agentVersion string) string {
	// Strip 'v' prefix if present
	cleanVersion := strings.TrimPrefix(agentVersion, "v")

	// Split version parts
	parts := strings.Split(cleanVersion, ".")
	if len(parts) == 4 {
		// Return the fourth octet as the config version
		// v0.1.23.6 → "6"
		return parts[3]
	}

	// TODO: Integrate with global error logging system when available
	// For now, default to "6" to match current agent version
	return "6"
}

// getDefaultConfig returns default configuration values
func getDefaultConfig() *Config {
	// Use version package for single source of truth
	configVersion := version.ConfigVersion
	if configVersion == "dev" {
		// Fallback to extracting from agent version if not injected
		configVersion = version.ExtractConfigVersionFromAgent(version.Version)
	}

	return &Config{
		Version:         configVersion,   // Config schema version from version package
		AgentVersion:    version.Version, // Agent version from version package
		ServerURL:       "http://localhost:8080",
		CheckInInterval: 300, // 5 minutes

		// Server Authentication
		RegistrationToken: "",       // One-time registration token (embedded by install script)
		AgentID:           uuid.Nil, // Will be set during registration
		Token:             "",       // Will be set during registration
		RefreshToken:      "",       // Will be set during registration

		// Agent Behavior
		RapidPollingEnabled: false,
		RapidPollingUntil:   time.Time{},
		DegradedMode:        false,

		// Polling resilience tuning (defaults; operator/server may override)
		Polling: PollingConfig{
			JitterMaxSeconds:   30,
			BackoffBaseSeconds: 10,
			BackoffMaxSeconds:  300,
		},

		// Network Security
		Proxy: ProxyConfig{},
		TLS:   TLSConfig{},
		Network: NetworkConfig{
			Timeout:     30 * time.Second,
			RetryCount:  3,
			RetryDelay:  5 * time.Second,
			MaxIdleConn: 10,
		},
		Logging: LoggingConfig{
			Level:      "info",
			MaxSize:    100, // 100MB
			MaxBackups: 3,
			MaxAge:     28, // 28 days
		},
		SecurityLogging: SecurityLogConfig{
			Enabled:      true,
			Level:        "warning",
			LogSuccesses: false,
			FilePath:     "security.log",
			MaxSizeMB:    50,
			MaxFiles:     5,
			BatchSize:    10,
			SendToServer: true,
		},
		CommandSigning: CommandSigningConfig{
			Enabled:         true,
			EnforcementMode: "strict",
		},
		Subsystems: GetDefaultSubsystemsConfig(),
		ProcessExplorer: ProcessExplorerConfig{
			MaxOpenFiles:      2000,
			MaxSockets:        500,
			MaxPipes:          500,
			MaxMemoryMap:      2000,
			MaxNamespaces:     50,
			MaxEnvKeys:        200,
			MaxListeningPorts: 100,
		},
		Desktop: DesktopConfig{
			Enabled:         true,
			MaxRestarts:     3,
			RestartDelaySec: 5,
		},
		Tags:     []string{},
		Metadata: make(map[string]string),
	}
}

// loadFromFile reads configuration from file with backward compatibility migration
func loadFromFile(configPath string) (*Config, error) {
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file does not exist") // Return error so caller uses defaults
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Parse the existing config into a generic map to preserve all fields
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Create a new config with ALL defaults to fill missing fields
	config := getDefaultConfig()

	// Carefully merge the loaded config into our defaults
	// This preserves existing values while filling missing ones with defaults
	configJSON, err := json.Marshal(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal config: %w", err)
	}

	// Create a temporary config to hold loaded values
	tempConfig := &Config{}
	if err := json.Unmarshal(configJSON, &tempConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal temp config: %w", err)
	}

	// Merge loaded config into defaults (only non-zero values)
	mergeConfigPreservingDefaults(config, tempConfig)

	// Handle specific migrations for known breaking changes
	migrateConfig(config)

	return config, nil
}

// migrateConfig handles specific known migrations between config versions
func migrateConfig(cfg *Config) {
	// Save the registration token before migration
	savedRegistrationToken := cfg.RegistrationToken

	// Update config schema version to latest
	targetVersion := version.ConfigVersion
	if targetVersion == "dev" {
		// Fallback to extracting from agent version
		targetVersion = version.ExtractConfigVersionFromAgent(version.Version)
	}

	if cfg.Version != targetVersion {
		if teeLogger != nil {
			teeLogger.Info("agent", "config", "config", "migrating config schema", map[string]interface{}{
				"from_version": cfg.Version,
				"to_version":   targetVersion,
			})
		}
		cfg.Version = targetVersion
	}

	// Migration 1: Ensure minimum check-in interval (30 seconds)
	if cfg.CheckInInterval < 30 {
		if teeLogger != nil {
			teeLogger.Info("agent", "config", "config", "migrating check_in_interval to minimum 30 seconds", map[string]interface{}{
				"old_value": cfg.CheckInInterval,
			})
		}
		cfg.CheckInInterval = 300 // Default to 5 minutes for better performance
	}

	// Migration 2: Add missing subsystem fields with defaults
	// Check if subsystem is zero value (truly missing), not just has zero fields
	if cfg.Subsystems.System == (SubsystemConfig{}) {
		if teeLogger != nil {
			teeLogger.Info("agent", "config", "config", "adding missing subsystem", map[string]interface{}{
				"subsystem": "system",
			})
		}
		cfg.Subsystems.System = GetDefaultSubsystemsConfig().System
	}

	if cfg.Subsystems.Updates == (SubsystemConfig{}) {
		if teeLogger != nil {
			teeLogger.Info("agent", "config", "config", "adding missing subsystem", map[string]interface{}{
				"subsystem": "updates",
			})
		}
		cfg.Subsystems.Updates = GetDefaultSubsystemsConfig().Updates
	}

	// CRITICAL: Restore the registration token after migration
	// This ensures the token is never overwritten by migration logic
	if savedRegistrationToken != "" {
		cfg.RegistrationToken = savedRegistrationToken
	}
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv() *Config {
	config := &Config{}

	if serverURL := os.Getenv("REDFLAG_SERVER_URL"); serverURL != "" {
		config.ServerURL = serverURL
	}
	if token := os.Getenv("REDFLAG_REGISTRATION_TOKEN"); token != "" {
		config.RegistrationToken = token
	}
	if proxyHTTP := os.Getenv("REDFLAG_HTTP_PROXY"); proxyHTTP != "" {
		config.Proxy.Enabled = true
		config.Proxy.HTTP = proxyHTTP
	}
	if proxyHTTPS := os.Getenv("REDFLAG_HTTPS_PROXY"); proxyHTTPS != "" {
		config.Proxy.Enabled = true
		config.Proxy.HTTPS = proxyHTTPS
	}
	if noProxy := os.Getenv("REDFLAG_NO_PROXY"); noProxy != "" {
		config.Proxy.NoProxy = noProxy
	}
	if logLevel := os.Getenv("REDFLAG_LOG_LEVEL"); logLevel != "" {
		if config.Logging == (LoggingConfig{}) {
			config.Logging = LoggingConfig{}
		}
		config.Logging.Level = logLevel
	}
	if org := os.Getenv("REDFLAG_ORGANIZATION"); org != "" {
		config.Organization = org
	}
	if displayName := os.Getenv("REDFLAG_DISPLAY_NAME"); displayName != "" {
		config.DisplayName = displayName
	}

	// Security logging environment variables
	if secEnabled := os.Getenv("REDFLAG_AGENT_SECURITY_LOG_ENABLED"); secEnabled != "" {
		if config.SecurityLogging == (SecurityLogConfig{}) {
			config.SecurityLogging = SecurityLogConfig{}
		}
		config.SecurityLogging.Enabled = secEnabled == "true"
	}
	if secLevel := os.Getenv("REDFLAG_AGENT_SECURITY_LOG_LEVEL"); secLevel != "" {
		if config.SecurityLogging == (SecurityLogConfig{}) {
			config.SecurityLogging = SecurityLogConfig{}
		}
		config.SecurityLogging.Level = secLevel
	}
	if secLogSucc := os.Getenv("REDFLAG_AGENT_SECURITY_LOG_SUCCESSES"); secLogSucc != "" {
		if config.SecurityLogging == (SecurityLogConfig{}) {
			config.SecurityLogging = SecurityLogConfig{}
		}
		config.SecurityLogging.LogSuccesses = secLogSucc == "true"
	}
	if secPath := os.Getenv("REDFLAG_AGENT_SECURITY_LOG_PATH"); secPath != "" {
		if config.SecurityLogging == (SecurityLogConfig{}) {
			config.SecurityLogging = SecurityLogConfig{}
		}
		config.SecurityLogging.FilePath = secPath
	}

	return config
}

// loadFromFlags loads configuration from CLI flags
func loadFromFlags(flags *CLIFlags) *Config {
	config := &Config{}

	if flags.ServerURL != "" {
		config.ServerURL = flags.ServerURL
	}
	if flags.RegistrationToken != "" {
		config.RegistrationToken = flags.RegistrationToken
	}
	if flags.ProxyHTTP != "" || flags.ProxyHTTPS != "" {
		config.Proxy = ProxyConfig{
			Enabled: true,
			HTTP:    flags.ProxyHTTP,
			HTTPS:   flags.ProxyHTTPS,
			NoProxy: flags.ProxyNoProxy,
		}
	}
	if flags.LogLevel != "" {
		config.Logging = LoggingConfig{
			Level: flags.LogLevel,
		}
	}
	if len(flags.Tags) > 0 {
		config.Tags = flags.Tags
	}
	if flags.Organization != "" {
		config.Organization = flags.Organization
	}
	if flags.DisplayName != "" {
		config.DisplayName = flags.DisplayName
	}
	if flags.InsecureTLS {
		config.TLS = TLSConfig{
			InsecureSkipVerify: true,
		}
	}

	return config
}

// mergeConfig merges source config into target config (non-zero values only)
func mergeConfig(target, source *Config) {
	if source.ServerURL != "" {
		target.ServerURL = source.ServerURL
	}
	if source.RegistrationToken != "" {
		target.RegistrationToken = source.RegistrationToken
	}
	if source.CheckInInterval != 0 {
		target.CheckInInterval = source.CheckInInterval
	}
	if source.Polling.JitterMaxSeconds != 0 {
		target.Polling.JitterMaxSeconds = source.Polling.JitterMaxSeconds
	}
	if source.Polling.BackoffBaseSeconds != 0 {
		target.Polling.BackoffBaseSeconds = source.Polling.BackoffBaseSeconds
	}
	if source.Polling.BackoffMaxSeconds != 0 {
		target.Polling.BackoffMaxSeconds = source.Polling.BackoffMaxSeconds
	}
	if source.AgentID != uuid.Nil {
		target.AgentID = source.AgentID
	}
	if source.Token != "" {
		target.Token = source.Token
	}
	if source.RefreshToken != "" {
		target.RefreshToken = source.RefreshToken
	}

	// Merge nested configs
	if source.Network != (NetworkConfig{}) {
		target.Network = source.Network
	}
	if source.Proxy != (ProxyConfig{}) {
		target.Proxy = source.Proxy
	}
	if source.TLS != (TLSConfig{}) {
		target.TLS = source.TLS
	}
	if source.Logging != (LoggingConfig{}) {
		target.Logging = source.Logging
	}
	if source.SecurityLogging != (SecurityLogConfig{}) {
		target.SecurityLogging = source.SecurityLogging
	}
	if source.CommandSigning != (CommandSigningConfig{}) {
		target.CommandSigning = source.CommandSigning
	}

	// Merge metadata
	if source.Tags != nil {
		target.Tags = source.Tags
	}
	if source.Metadata != nil {
		if target.Metadata == nil {
			target.Metadata = make(map[string]string)
		}
		for k, v := range source.Metadata {
			target.Metadata[k] = v
		}
	}
	if source.DisplayName != "" {
		target.DisplayName = source.DisplayName
	}
	if source.Organization != "" {
		target.Organization = source.Organization
	}

	// Merge rapid polling settings
	target.RapidPollingEnabled = source.RapidPollingEnabled
	if !source.RapidPollingUntil.IsZero() {
		target.RapidPollingUntil = source.RapidPollingUntil
	}

	// Merge subsystems config
	if source.Subsystems != (SubsystemsConfig{}) {
		target.Subsystems = source.Subsystems
	}
}

// validateConfig validates configuration values
func validateConfig(config *Config) error {
	if config.ServerURL == "" {
		return fmt.Errorf("server_url is required")
	}
	if config.CheckInInterval < 30 {
		return fmt.Errorf("check_in_interval must be at least 30 seconds")
	}
	if config.CheckInInterval > 3600 {
		return fmt.Errorf("check_in_interval cannot exceed 3600 seconds (1 hour)")
	}
	if config.Network.Timeout <= 0 {
		return fmt.Errorf("network timeout must be positive")
	}
	if config.Network.RetryCount < 0 || config.Network.RetryCount > 10 {
		return fmt.Errorf("retry_count must be between 0 and 10")
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if config.Logging.Level != "" && !validLogLevels[config.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", config.Logging.Level)
	}

	return nil
}

// mergeMissingKeys recursively walks fresh and injects keys that are absent
// from raw into raw. Both maps are map[string]interface{} as parsed from JSON.
// Returns the total number of keys added (at any depth).
func mergeMissingKeys(raw, fresh map[string]interface{}, depth int) int {
	if depth > 16 {
		return 0 // safety limit — deeply nested config is pathological
	}
	added := 0
	for k, v := range fresh {
		rawVal, exists := raw[k]
		if !exists {
			raw[k] = v
			added++
			continue
		}
		// If both sides are maps, recurse to find missing sub-keys.
		rawMap, rawOk := rawVal.(map[string]interface{})
		freshMap, freshOk := v.(map[string]interface{})
		if rawOk && freshOk {
			added += mergeMissingKeys(rawMap, freshMap, depth+1)
		}
	}
	return added
}

// persistNewDefaults injects new keys from the merged config into the on-disk
// file without removing unknown keys (e.g. machine_id) that the Config struct
// does not carry. This is how upgrades get new config fields (desktop, polling,
// etc.) persisted without the installer re-running.
func persistNewDefaults(configPath string, cfg *Config) {
	existing, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(existing, &raw); err != nil {
		return
	}

	// Marshal the merged struct to a map to discover new-key candidates.
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	var fresh map[string]interface{}
	if err := json.Unmarshal(cfgJSON, &fresh); err != nil {
		return
	}

	// Inject keys present in the struct map but absent from the raw file.
	// Recurse into nested objects so new sub-fields (e.g. desktop.max_restarts
	// added in a newer version) are written even when the parent key exists.
	added := mergeMissingKeys(raw, fresh, 0)
	if added == 0 {
		return // already up to date
	}

	if teeLogger != nil {
		teeLogger.Info("agent", "config", "config", "persisting new default keys to config.json", map[string]interface{}{
			"added": added,
		})
	}

	merged, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(configPath, merged, 0600); err != nil {
		if teeLogger != nil {
			teeLogger.Warning("agent", "config", "config", "failed to persist new defaults", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}
}

// Save writes configuration to file
func (c *Config) Save(configPath string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create parent directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// SetDegradedMode sets the degraded mode flag and saves the config
func (c *Config) SetDegradedMode(enabled bool) error {
	c.DegradedMode = enabled
	return c.Save(constants.GetAgentConfigPath())
}

// IsRegistered checks if the agent is registered
func (c *Config) IsRegistered() bool {
	return c.AgentID != uuid.Nil && c.Token != ""
}

// IsStandalone reports whether this config has a stable local identity and no
// fleet credential. A partial fleet enrollment is not standalone: refresh or
// registration material must never silently become local mutation authority.
func (c *Config) IsStandalone() bool {
	return c.AgentID != uuid.Nil && c.Token == "" && c.RefreshToken == "" && c.RegistrationToken == ""
}

// InitializeStandalone gives a local-only Agent one durable UUID. It is
// idempotent, but refuses any fleet credential so provisioning cannot convert a
// fleet host into local authority by accident.
func (c *Config) InitializeStandalone() error {
	if c.IsRegistered() || c.Token != "" || c.RefreshToken != "" || c.RegistrationToken != "" {
		return fmt.Errorf("standalone identity refused: fleet enrollment material is present")
	}
	if c.AgentID != uuid.Nil {
		return nil
	}
	id, err := uuid.NewV4()
	if err != nil {
		return fmt.Errorf("generate standalone agent id: %w", err)
	}
	c.AgentID = id
	return nil
}

// OSType represents the operating system type
type OSType string

const (
	OSTypeLinux   OSType = "linux"
	OSTypeWindows OSType = "windows"
	OSTypeDarwin  OSType = "darwin"
)

// OS holds OS-specific information
type OS struct {
	Type    OSType `json:"type"`
	Arch    string `json:"arch,omitempty"`
	Version string `json:"version,omitempty"`
}

// GetOSType returns the OS type from the config
func (c *Config) GetOSType() OSType {
	if c.OS.Type != "" {
		return c.OS.Type
	}
	// Default to linux if not set
	return OSTypeLinux
}

// NeedsRegistration checks if the agent needs to register with a token
func (c *Config) NeedsRegistration() bool {
	return c.RegistrationToken != "" && c.AgentID == uuid.Nil
}

// HasRegistrationToken checks if the agent has a registration token
func (c *Config) HasRegistrationToken() bool {
	return c.RegistrationToken != ""
}

// mergeConfigPreservingDefaults merges source config into target config
// but only overwrites fields that are explicitly set (non-zero)
// This is different from mergeConfig which blindly copies non-zero values
func mergeConfigPreservingDefaults(target, source *Config) {
	// Server Configuration
	if source.ServerURL != "" && source.ServerURL != getDefaultConfig().ServerURL {
		target.ServerURL = source.ServerURL
	}
	// IMPORTANT: Never overwrite registration token if target already has one
	if source.RegistrationToken != "" && target.RegistrationToken == "" {
		target.RegistrationToken = source.RegistrationToken
	}

	// Agent Configuration
	if source.CheckInInterval != 0 {
		target.CheckInInterval = source.CheckInInterval
	}
	if source.Polling.JitterMaxSeconds != 0 {
		target.Polling.JitterMaxSeconds = source.Polling.JitterMaxSeconds
	}
	if source.Polling.BackoffBaseSeconds != 0 {
		target.Polling.BackoffBaseSeconds = source.Polling.BackoffBaseSeconds
	}
	if source.Polling.BackoffMaxSeconds != 0 {
		target.Polling.BackoffMaxSeconds = source.Polling.BackoffMaxSeconds
	}
	if source.AgentID != uuid.Nil {
		target.AgentID = source.AgentID
	}
	if source.Token != "" {
		target.Token = source.Token
	}
	if source.RefreshToken != "" {
		target.RefreshToken = source.RefreshToken
	}

	// Merge nested configs only if they're not default values
	if source.Network != (NetworkConfig{}) {
		target.Network = source.Network
	}
	if source.Proxy != (ProxyConfig{}) {
		target.Proxy = source.Proxy
	}
	if source.TLS != (TLSConfig{}) {
		target.TLS = source.TLS
	}
	if source.Logging != (LoggingConfig{}) && source.Logging.Level != "" {
		target.Logging = source.Logging
	}
	if source.SecurityLogging != (SecurityLogConfig{}) {
		target.SecurityLogging = source.SecurityLogging
	}
	if source.CommandSigning != (CommandSigningConfig{}) {
		target.CommandSigning = source.CommandSigning
	}

	// Merge metadata
	if source.Tags != nil && len(source.Tags) > 0 {
		target.Tags = source.Tags
	}
	if source.Metadata != nil {
		if target.Metadata == nil {
			target.Metadata = make(map[string]string)
		}
		for k, v := range source.Metadata {
			target.Metadata[k] = v
		}
	}
	if source.DisplayName != "" {
		target.DisplayName = source.DisplayName
	}
	if source.Organization != "" {
		target.Organization = source.Organization
	}

	// Merge rapid polling settings
	target.RapidPollingEnabled = source.RapidPollingEnabled
	if !source.RapidPollingUntil.IsZero() {
		target.RapidPollingUntil = source.RapidPollingUntil
	}

	// Merge subsystems config
	if source.Subsystems != (SubsystemsConfig{}) {
		target.Subsystems = source.Subsystems
	}

	// Desktop app config. A zero struct means the key is absent from the file —
	// keep the defaults. Any explicit setting (enabled, restart tuning) wins.
	if source.Desktop != (DesktopConfig{}) {
		target.Desktop = source.Desktop
	}

	// Version info
	if source.Version != "" {
		target.Version = source.Version
	}
	if source.AgentVersion != "" {
		target.AgentVersion = source.AgentVersion
	}
}
