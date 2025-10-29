package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/term"
)

// Config holds the application configuration
type Config struct {
	Server struct {
		Host string `env:"REDFLAG_SERVER_HOST" default:"0.0.0.0"`
		Port int    `env:"REDFLAG_SERVER_PORT" default:"8080"`
		TLS  struct {
			Enabled   bool   `env:"REDFLAG_TLS_ENABLED" default:"false"`
			CertFile  string `env:"REDFLAG_TLS_CERT_FILE"`
			KeyFile   string `env:"REDFLAG_TLS_KEY_FILE"`
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

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists (for development)
	_ = godotenv.Load()

	cfg := &Config{}

	// Parse server configuration
	cfg.Server.Host = getEnv("REDFLAG_SERVER_HOST", "0.0.0.0")
	serverPort, _ := strconv.Atoi(getEnv("REDFLAG_SERVER_PORT", "8080"))
	cfg.Server.Port = serverPort
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

	// Validate JWT secret is not the development default
	if cfg.Admin.JWTSecret == "test-secret-for-development-only" {
		fmt.Printf("[SECURITY WARNING] Using development JWT secret\n")
		fmt.Printf("[INFO] Run: ./redflag-server --setup to configure production secrets\n")
	}

	return cfg, nil
}

// RunSetupWizard guides user through initial configuration
func RunSetupWizard() error {
	fmt.Printf("RedFlag Server Setup Wizard\n")
	fmt.Printf("===========================\n\n")

	// Admin credentials
	fmt.Printf("Admin Account Setup\n")
	fmt.Printf("--------------------\n")
	username := promptForInput("Admin username", "admin")
	password := promptForPassword("Admin password")

	// Database configuration
	fmt.Printf("\nDatabase Configuration\n")
	fmt.Printf("----------------------\n")
	dbHost := promptForInput("Database host", "localhost")
	dbPort, _ := strconv.Atoi(promptForInput("Database port", "5432"))
	dbName := promptForInput("Database name", "redflag")
	dbUser := promptForInput("Database user", "redflag")
	dbPassword := promptForPassword("Database password")

	// Server configuration
	fmt.Printf("\nServer Configuration\n")
	fmt.Printf("--------------------\n")
	serverHost := promptForInput("Server bind address", "0.0.0.0")
	serverPort, _ := strconv.Atoi(promptForInput("Server port", "8080"))

	// Agent limits
	fmt.Printf("\nAgent Registration\n")
	fmt.Printf("------------------\n")
	maxSeats, _ := strconv.Atoi(promptForInput("Maximum agent seats (security limit)", "50"))

	// Generate JWT secret from admin password
	jwtSecret := deriveJWTSecret(username, password)

	// Create .env file
	envContent := fmt.Sprintf(`# RedFlag Server Configuration
# Generated on %s

# Server Configuration
REDFLAG_SERVER_HOST=%s
REDFLAG_SERVER_PORT=%d
REDFLAG_TLS_ENABLED=false
# REDFLAG_TLS_CERT_FILE=
# REDFLAG_TLS_KEY_FILE=

# Database Configuration
REDFLAG_DB_HOST=%s
REDFLAG_DB_PORT=%d
REDFLAG_DB_NAME=%s
REDFLAG_DB_USER=%s
REDFLAG_DB_PASSWORD=%s

# Admin Configuration
REDFLAG_ADMIN_USER=%s
REDFLAG_ADMIN_PASSWORD=%s
REDFLAG_JWT_SECRET=%s

# Agent Registration
REDFLAG_TOKEN_EXPIRY=24h
REDFLAG_MAX_TOKENS=100
REDFLAG_MAX_SEATS=%d

# Legacy Configuration (for backwards compatibility)
SERVER_PORT=%d
DATABASE_URL=postgres://%s:%s@%s:%d/%s?sslmode=disable
JWT_SECRET=%s
CHECK_IN_INTERVAL=300
OFFLINE_THRESHOLD=600
TIMEZONE=UTC
LATEST_AGENT_VERSION=0.1.8
`, time.Now().Format("2006-01-02 15:04:05"), serverHost, serverPort,
		dbHost, dbPort, dbName, dbUser, dbPassword,
		username, password, jwtSecret, maxSeats,
		serverPort, dbUser, dbPassword, dbHost, dbPort, dbName, jwtSecret)

	// Write .env file
	if err := os.WriteFile(".env", []byte(envContent), 0600); err != nil {
		return fmt.Errorf("failed to write .env file: %w", err)
	}

	fmt.Printf("\n[OK] Configuration saved to .env file\n")
	fmt.Printf("[SECURITY] File permissions set to 0600 (owner read/write only)\n")
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("   1. Start database: %s:%d\n", dbHost, dbPort)
	fmt.Printf("   2. Create database: CREATE DATABASE %s;\n", dbName)
	fmt.Printf("   3. Run migrations: ./redflag-server --migrate\n")
	fmt.Printf("   4. Start server: ./redflag-server\n")
	fmt.Printf("\nServer will be available at: http://%s:%d\n", serverHost, serverPort)
	fmt.Printf("Admin interface: http://%s:%d/admin\n", serverHost, serverPort)

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func promptForInput(prompt, defaultValue string) string {
	fmt.Printf("%s [%s]: ", prompt, defaultValue)
	var input string
	fmt.Scanln(&input)
	if strings.TrimSpace(input) == "" {
		return defaultValue
	}
	return strings.TrimSpace(input)
}

func promptForPassword(prompt string) string {
	fmt.Printf("%s: ", prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		// Fallback to non-hidden input
		var input string
		fmt.Scanln(&input)
		return strings.TrimSpace(input)
	}
	fmt.Printf("\n")
	return strings.TrimSpace(string(password))
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
