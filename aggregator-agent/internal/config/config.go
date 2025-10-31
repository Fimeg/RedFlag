package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ProxyConfig holds proxy configuration
type ProxyConfig struct {
	Enabled  bool   `json:"enabled"`
	HTTP     string `json:"http,omitempty"`      // HTTP proxy URL
	HTTPS    string `json:"https,omitempty"`     // HTTPS proxy URL
	NoProxy  string `json:"no_proxy,omitempty"`  // Comma-separated hosts to bypass proxy
	Username string `json:"username,omitempty"` // Proxy username (optional)
	Password string `json:"password,omitempty"` // Proxy password (optional)
}

// TLSConfig holds TLS/security configuration
type TLSConfig struct {
	InsecureSkipVerify bool   `json:"insecure_skip_verify"` // Skip TLS certificate verification
	CertFile           string `json:"cert_file,omitempty"`   // Client certificate file
	KeyFile            string `json:"key_file,omitempty"`    // Client key file
	CAFile             string `json:"ca_file,omitempty"`     // CA certificate file
}

// NetworkConfig holds network-related configuration
type NetworkConfig struct {
	Timeout     time.Duration `json:"timeout"`      // Request timeout
	RetryCount  int           `json:"retry_count"`   // Number of retries
	RetryDelay  time.Duration `json:"retry_delay"`   // Delay between retries
	MaxIdleConn int           `json:"max_idle_conn"` // Maximum idle connections
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `json:"level"`       // Log level (debug, info, warn, error)
	File       string `json:"file,omitempty"` // Log file path (optional)
	MaxSize    int    `json:"max_size"`    // Max log file size in MB
	MaxBackups int    `json:"max_backups"` // Max number of log file backups
	MaxAge     int    `json:"max_age"`     // Max age of log files in days
}

// Config holds agent configuration
type Config struct {
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

	// Network Configuration
	Network NetworkConfig `json:"network,omitempty"`

	// Proxy Configuration
	Proxy ProxyConfig `json:"proxy,omitempty"`

	// Security Configuration
	TLS TLSConfig `json:"tls,omitempty"`

	// Logging Configuration
	Logging LoggingConfig `json:"logging,omitempty"`

	// Agent Metadata
	Tags         []string          `json:"tags,omitempty"`         // User-defined tags
	Metadata     map[string]string `json:"metadata,omitempty"`     // Custom metadata
	DisplayName  string            `json:"display_name,omitempty"` // Human-readable name
	Organization string            `json:"organization,omitempty"` // Organization/group
}

// Load reads configuration from multiple sources with priority order:
// 1. CLI flags
// 2. Environment variables
// 3. Configuration file
// 4. Default values
func Load(configPath string, cliFlags *CLIFlags) (*Config, error) {
	// Start with defaults
	config := getDefaultConfig()

	// Load from config file if it exists
	if fileConfig, err := loadFromFile(configPath); err == nil {
		mergeConfig(config, fileConfig)
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
	LogLevel         string
	ConfigFile        string
	Tags              []string
	Organization      string
	DisplayName       string
	InsecureTLS       bool
}

// getDefaultConfig returns default configuration values
func getDefaultConfig() *Config {
	return &Config{
		ServerURL:         "http://localhost:8080",
		CheckInInterval:   300, // 5 minutes
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
		Tags:      []string{},
		Metadata:  make(map[string]string),
	}
}

// loadFromFile reads configuration from file
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
			return getDefaultConfig(), nil // Return defaults if file doesn't exist
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
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

// IsRegistered checks if the agent is registered
func (c *Config) IsRegistered() bool {
	return c.AgentID != uuid.Nil && c.Token != ""
}

// NeedsRegistration checks if the agent needs to register with a token
func (c *Config) NeedsRegistration() bool {
	return c.RegistrationToken != "" && c.AgentID == uuid.Nil
}

// HasRegistrationToken checks if the agent has a registration token
func (c *Config) HasRegistrationToken() bool {
	return c.RegistrationToken != ""
}
