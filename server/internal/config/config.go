package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Fimeg/RedFlag/server/internal/version"
)

// Config holds the application configuration
type Config struct {
	Server struct {
		Host      string `env:"REDFLAG_SERVER_HOST" default:"0.0.0.0"`
		Port      int    `env:"REDFLAG_SERVER_PORT" default:"8080"`
		PublicURL string `env:"REDFLAG_PUBLIC_URL"` // Optional: External URL for reverse proxy/load balancer
		TLS       struct {
			Enabled  bool   `env:"REDFLAG_TLS_ENABLED" default:"false"`
			CertFile string `env:"REDFLAG_TLS_CERT_FILE"`
			KeyFile  string `env:"REDFLAG_TLS_KEY_FILE"`
		}
	}
	Database struct {
		Host     string `env:"REDFLAG_DB_HOST" default:"localhost"`
		Port     int    `env:"REDFLAG_DB_PORT" default:"5432"`
		Database string `env:"REDFLAG_DB_NAME" default:"redflag"`
		Username string `env:"REDFLAG_DB_USER" default:"redflag"`
		Password string `env:"REDFLAG_DB_PASSWORD"`
	}
	Admin struct {
		Username  string `env:"REDFLAG_ADMIN_USER" default:"admin"`
		Email     string `env:"REDFLAG_ADMIN_EMAIL" default:"admin@example.com"`
		Password  string `env:"REDFLAG_ADMIN_PASSWORD"`
		JWTSecret string `env:"REDFLAG_JWT_SECRET"`
	}
	AgentRegistration struct {
		TokenExpiry string `env:"REDFLAG_TOKEN_EXPIRY" default:"24h"`
		MaxTokens   int    `env:"REDFLAG_MAX_TOKENS" default:"100"`
		MaxSeats    int    `env:"REDFLAG_MAX_SEATS" default:"50"`
	}
	CheckInInterval    int
	OfflineThreshold   int
	Timezone           string
	LatestAgentVersion string
	MinAgentVersion    string `env:"MIN_AGENT_VERSION" default:"0.1.26"`
	SigningPrivateKey   string `env:"REDFLAG_SIGNING_PRIVATE_KEY"`
	// At-rest symmetric keys (base64, 32-byte AES-256). Provisioned by
	// resolveAtRestKeys onto the single secret rail: Docker secret, then env,
	// then a persisted key file under the data dir. Never stored in the DB.
	SettingsEncryptionKey string `env:"REDFLAG_SETTINGS_ENCRYPTION_KEY"`
	TokenEncryptionKey    string `env:"REDFLAG_TOKEN_ENC_KEY"`
	BinaryStoragePath  string `env:"REDFLAG_BINARY_STORAGE_PATH" default:"./binaries"`
	DebugEnabled      bool   `env:"REDFLAG_DEBUG" default:"false"` // Enable debug logging
	SecurityLogging   struct {
		Enabled           bool   `env:"REDFLAG_SECURITY_LOG_ENABLED" default:"true"`
		Level             string `env:"REDFLAG_SECURITY_LOG_LEVEL" default:"warning"` // none, error, warn, info, debug
		LogSuccesses      bool   `env:"REDFLAG_SECURITY_LOG_SUCCESSES" default:"false"`
		FilePath          string `env:"REDFLAG_SECURITY_LOG_PATH" default:"/var/log/redflag/security.json"`
		MaxSizeMB         int    `env:"REDFLAG_SECURITY_LOG_MAX_SIZE" default:"100"`
		MaxFiles          int    `env:"REDFLAG_SECURITY_LOG_MAX_FILES" default:"10"`
		RetentionDays     int    `env:"REDFLAG_SECURITY_LOG_RETENTION" default:"90"`
		LogToDatabase     bool   `env:"REDFLAG_SECURITY_LOG_TO_DB" default:"true"`
		HashIPAddresses   bool   `env:"REDFLAG_SECURITY_LOG_HASH_IP" default:"true"`
	}
}

// IsDockerSecretsMode returns true if the application is running in Docker secrets mode
func IsDockerSecretsMode() bool {
	// Check if we're running in Docker and secrets are available
	if _, err := os.Stat("/run/secrets"); err == nil {
		// Also check if any RedFlag secrets exist
		if _, err := os.Stat("/run/secrets/redflag_admin_password"); err == nil {
			return true
		}
	}
	// Check environment variable override
	return os.Getenv("REDFLAG_SECRETS_MODE") == "true"
}

// getSecretPath returns the full path to a Docker secret file
func getSecretPath(secretName string) string {
	return filepath.Join("/run/secrets", secretName)
}

// loadFromSecrets reads configuration from Docker secrets
func loadFromSecrets(cfg *Config) error {
	// Note: For Docker secrets, we need to map environment variables differently
	// Docker secrets appear as files that contain the secret value
	fmt.Printf("[CONFIG] Loading configuration from Docker secrets\n")

	// Load sensitive values from Docker secrets
	if password, err := readSecretFile("redflag_admin_password"); err == nil && password != "" {
		cfg.Admin.Password = password
		fmt.Printf("[CONFIG] [OK] Admin password loaded from Docker secret\n")
	}

	if jwtSecret, err := readSecretFile("redflag_jwt_secret"); err == nil && jwtSecret != "" {
		cfg.Admin.JWTSecret = jwtSecret
		fmt.Printf("[CONFIG] [OK] JWT secret loaded from Docker secret\n")
	}

	if dbPassword, err := readSecretFile("redflag_db_password"); err == nil && dbPassword != "" {
		cfg.Database.Password = dbPassword
		fmt.Printf("[CONFIG] [OK] Database password loaded from Docker secret\n")
	}

	if signingKey, err := readSecretFile("redflag_signing_private_key"); err == nil && signingKey != "" {
		cfg.SigningPrivateKey = signingKey
		fmt.Printf("[CONFIG] [OK] Signing private key loaded from Docker secret (%d characters)\n", len(signingKey))
	}

	// For other configuration, fall back to environment variables
	// This allows mixing secrets (for sensitive data) with env vars (for non-sensitive config)
	return loadFromEnv(cfg, true)
}

// loadFromEnv reads configuration from environment variables
// If skipSensitive=true, it won't override values that might have come from secrets
func loadFromEnv(cfg *Config, skipSensitive bool) error {
	if !skipSensitive {
		fmt.Printf("[CONFIG] Loading configuration from environment variables\n")
	}

	// Parse server configuration
	if !skipSensitive || cfg.Server.Host == "" {
		cfg.Server.Host = getEnv("REDFLAG_SERVER_HOST", "0.0.0.0")
	}
	serverPort, _ := strconv.Atoi(getEnv("REDFLAG_SERVER_PORT", "8080"))
	cfg.Server.Port = serverPort
	cfg.Server.PublicURL = getEnv("REDFLAG_PUBLIC_URL", "") // Optional external URL
	cfg.Server.TLS.Enabled = getEnv("REDFLAG_TLS_ENABLED", "false") == "true"
	cfg.Server.TLS.CertFile = getEnv("REDFLAG_TLS_CERT_FILE", "")
	cfg.Server.TLS.KeyFile = getEnv("REDFLAG_TLS_KEY_FILE", "")

	// Parse database configuration
	cfg.Database.Host = getEnv("REDFLAG_DB_HOST", "localhost")
	dbPort, _ := strconv.Atoi(getEnv("REDFLAG_DB_PORT", "5432"))
	cfg.Database.Port = dbPort
	cfg.Database.Database = getEnv("REDFLAG_DB_NAME", "redflag")
	cfg.Database.Username = getEnv("REDFLAG_DB_USER", "redflag")

	// Only load password from env if we're not skipping sensitive data
	if !skipSensitive {
		cfg.Database.Password = getEnv("REDFLAG_DB_PASSWORD", "")
	}

	// Parse admin configuration
	cfg.Admin.Username = getEnv("REDFLAG_ADMIN_USER", "admin")
	if !skipSensitive {
		cfg.Admin.Password = getEnv("REDFLAG_ADMIN_PASSWORD", "")
		cfg.Admin.JWTSecret = getEnv("REDFLAG_JWT_SECRET", "")
	}

	// Parse agent registration configuration
	cfg.AgentRegistration.TokenExpiry = getEnv("REDFLAG_TOKEN_EXPIRY", "24h")
	maxTokens, _ := strconv.Atoi(getEnv("REDFLAG_MAX_TOKENS", "100"))
	cfg.AgentRegistration.MaxTokens = maxTokens
	maxSeats, _ := strconv.Atoi(getEnv("REDFLAG_MAX_SEATS", "50"))
	cfg.AgentRegistration.MaxSeats = maxSeats

	// Parse legacy configuration for backwards compatibility
	checkInInterval, _ := strconv.Atoi(getEnv("CHECK_IN_INTERVAL", "300"))
	offlineThreshold, _ := strconv.Atoi(getEnv("OFFLINE_THRESHOLD", "600"))
	cfg.CheckInInterval = checkInInterval
	cfg.OfflineThreshold = offlineThreshold
	cfg.Timezone = getEnv("TIMEZONE", "UTC")
	cfg.LatestAgentVersion = getEnv("LATEST_AGENT_VERSION", version.AgentVersion)
	cfg.MinAgentVersion = getEnv("MIN_AGENT_VERSION", "0.1.26")

	cfg.BinaryStoragePath = getEnv("REDFLAG_BINARY_STORAGE_PATH", "./binaries")

	if !skipSensitive {
		cfg.SigningPrivateKey = getEnv("REDFLAG_SIGNING_PRIVATE_KEY", "")
	}

	return nil
}

// readSecretFile reads a Docker secret from /run/secrets/ directory
func readSecretFile(secretName string) (string, error) {
	path := getSecretPath(secretName)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret %s from %s: %w", secretName, path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// loadEnvFile applies KEY=VALUE lines from a flat config file into the process
// environment, without overriding anything already set — the same precedence
// docker-compose's own `env_file:` directive gives the container. Native
// (non-docker) installs have no compose layer to do this injection, so a
// service-adjacent config file is the only way to hand the binary its
// settings; this keeps that path additive and inert everywhere else.
func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, alreadySet := os.LookupEnv(key); !alreadySet {
			os.Setenv(key, value)
		}
	}
	return nil
}

// loadNativeConfigFile looks for a config file for non-docker deployments.
// REDFLAG_CONFIG_FILE takes an explicit path; otherwise it checks the
// per-OS default install-config location and no-ops if nothing is there —
// existing docker deployments (env vars already set by env_file:) never hit
// either path, so this cannot change their behavior.
func loadNativeConfigFile() {
	path := os.Getenv("REDFLAG_CONFIG_FILE")
	if path == "" {
		if runtime.GOOS == "windows" {
			path = filepath.Join(os.Getenv("ProgramData"), "RedFlag", "redflag.env")
		} else {
			path = "/etc/redflag/server.env"
		}
	}
	if _, err := os.Stat(path); err != nil {
		return
	}
	if err := loadEnvFile(path); err != nil {
		fmt.Printf("[CONFIG] [WARN] found %s but failed to read it: %v\n", path, err)
		return
	}
	fmt.Printf("[CONFIG] Loaded native config file: %s\n", path)
}

// Load reads configuration from Docker secrets or environment variables
func Load() (*Config, error) {
	loadNativeConfigFile()

	// Check if we're in Docker secrets mode
	cfg := &Config{}
	if IsDockerSecretsMode() {
		fmt.Printf("[CONFIG] Detected Docker secrets mode\n")
		if err := loadFromSecrets(cfg); err != nil {
			return nil, fmt.Errorf("failed to load configuration from secrets: %w", err)
		}
	} else {
		// Default to environment variable mode
		if err := loadFromEnv(cfg, false); err != nil {
			return nil, fmt.Errorf("failed to load configuration from environment: %w", err)
		}
	}

	// Provision at-rest encryption keys on the single secret rail.
	if err := resolveAtRestKeys(cfg); err != nil {
		return nil, fmt.Errorf("failed to provision at-rest encryption keys: %w", err)
	}

	return cfg, nil
}

// resolveAtRestKeys fills the symmetric at-rest keys that were not already
// supplied via env (loadFromEnv) or Docker secret (loadFromSecrets). It is the
// single provisioning path for these keys: Docker secret -> env var -> a
// persisted key file under the data dir, generated once on first boot.
//
// Persisting the generated key to disk (never the database) means a DB dump
// alone yields only ciphertext, and a restart never silently orphans previously
// encrypted data — the failure mode that the old ephemeral-generate fallback had.
func resolveAtRestKeys(cfg *Config) error {
	var err error
	if cfg.SettingsEncryptionKey == "" {
		if cfg.SettingsEncryptionKey, err = loadOrCreateKey("redflag_settings_encryption_key"); err != nil {
			return fmt.Errorf("settings encryption key: %w", err)
		}
	}
	if cfg.TokenEncryptionKey == "" {
		if cfg.TokenEncryptionKey, err = loadOrCreateKey("redflag_token_enc_key"); err != nil {
			return fmt.Errorf("token encryption key: %w", err)
		}
	}
	return nil
}

// loadOrCreateKey resolves one base64 AES-256 key by name. Order: Docker secret
// file, then the upper-cased env var, then a persisted file under the data dir
// (REDFLAG_DATA_DIR, default /app/data). If none exist a 32-byte key is
// generated and written 0600 so it survives restarts.
func loadOrCreateKey(name string) (string, error) {
	if v, err := readSecretFile(name); err == nil && v != "" {
		return v, nil
	}
	if v := os.Getenv(strings.ToUpper(name)); v != "" {
		return v, nil
	}

	dir := getEnv("REDFLAG_DATA_DIR", "/app/data")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create data dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, name+".key")
	if data, err := os.ReadFile(path); err == nil {
		if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
			return trimmed, nil
		}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		return "", fmt.Errorf("persist key %s: %w", path, err)
	}
	fmt.Printf("[CONFIG] [OK] generated and persisted %s.key (first boot)\n", name)
	return encoded, nil
}


// RunSetupWizard is deprecated - configuration is now handled via web interface
func RunSetupWizard() error {
	return fmt.Errorf("CLI setup wizard is deprecated. Please use the web interface at http://localhost:8080/setup for configuration")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GenerateSecurePassword generates a secure password (16 characters)
func GenerateSecurePassword() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	// Use alphanumeric characters for better UX
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 16)
	for i := range result {
		result[i] = chars[int(bytes[i])%len(chars)]
	}
	return string(result)
}

// GenerateSecureToken generates a cryptographically secure random token
func GenerateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
