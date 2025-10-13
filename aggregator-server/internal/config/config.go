package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	ServerPort       string
	DatabaseURL      string
	JWTSecret        string
	CheckInInterval  int
	OfflineThreshold int
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists (for development)
	_ = godotenv.Load()

	checkInInterval, _ := strconv.Atoi(getEnv("CHECK_IN_INTERVAL", "300"))
	offlineThreshold, _ := strconv.Atoi(getEnv("OFFLINE_THRESHOLD", "600"))

	return &Config{
		ServerPort:       getEnv("SERVER_PORT", "8080"),
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://aggregator:aggregator@localhost:5432/aggregator?sslmode=disable"),
		JWTSecret:        getEnv("JWT_SECRET", "change-me-in-production"),
		CheckInInterval:  checkInInterval,
		OfflineThreshold: offlineThreshold,
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
