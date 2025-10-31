package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
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
		Password  string `env:"REDFLAG_ADMIN_PASSWORD"`
		JWTSecret string `env:"REDFLAG_JWT_SECRET"`
	}
	AgentRegistration struct {
		TokenExpiry string `env:"REDFLAG_TOKEN_EXPIRY" default:"24h"`
		MaxTokens   int    `env:"REDFLAG_MAX_TOKENS" default:"100"`
		MaxSeats    int    `env:"REDFLAG_MAX_SEATS" default:"50"`
	}
	CheckInInterval  int
	OfflineThreshold int
	Timezone         string
	LatestAgentVersion string
}

// Load reads configuration from environment variables only (immutable configuration)
func Load() (*Config, error) {
	fmt.Printf("[CONFIG] Loading configuration from environment variables\n")

	cfg := &Config{}

	// Parse server configuration
	cfg.Server.Host = getEnv("REDFLAG_SERVER_HOST", "0.0.0.0")
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
	cfg.Database.Password = getEnv("REDFLAG_DB_PASSWORD", "")

	// Parse admin configuration
	cfg.Admin.Username = getEnv("REDFLAG_ADMIN_USER", "admin")
	cfg.Admin.Password = getEnv("REDFLAG_ADMIN_PASSWORD", "")
	cfg.Admin.JWTSecret = getEnv("REDFLAG_JWT_SECRET", "")

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
	cfg.LatestAgentVersion = getEnv("LATEST_AGENT_VERSION", "0.1.16")

	// Handle missing secrets
	if cfg.Admin.Password == "" || cfg.Admin.JWTSecret == "" || cfg.Database.Password == "" {
		fmt.Printf("[WARNING] Missing required configuration (admin password, JWT secret, or database password)\n")
		fmt.Printf("[INFO] Run: ./redflag-server --setup to configure\n")
		return nil, fmt.Errorf("missing required configuration")
	}

	// Check if we're using bootstrap defaults that need to be replaced
	if cfg.Admin.Password == "changeme" || cfg.Admin.JWTSecret == "bootstrap-jwt-secret-replace-in-setup" || cfg.Database.Password == "redflag_bootstrap" {
		fmt.Printf("[INFO] Server running with bootstrap configuration - setup required\n")
		fmt.Printf("[INFO] Configure via web interface at: http://localhost:8080/setup\n")
		return nil, fmt.Errorf("bootstrap configuration detected - setup required")
	}

	// Validate JWT secret is not the development default
	if cfg.Admin.JWTSecret == "test-secret-for-development-only" {
		fmt.Printf("[SECURITY WARNING] Using development JWT secret\n")
		fmt.Printf("[INFO] Run: ./redflag-server --setup to configure production secrets\n")
	}

	return cfg, nil
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


func deriveJWTSecret(username, password string) string {
	// Derive JWT secret from admin credentials
	// This ensures JWT secret changes if admin password changes
	hash := sha256.Sum256([]byte(username + password + "redflag-jwt-2024"))
	return hex.EncodeToString(hash[:])
}

// GenerateSecureToken generates a cryptographically secure random token
func GenerateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
