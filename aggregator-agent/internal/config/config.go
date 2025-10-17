package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Config holds agent configuration
type Config struct {
	ServerURL       string    `json:"server_url"`
	AgentID         uuid.UUID `json:"agent_id"`
	Token           string    `json:"token"`            // Short-lived access token (24h)
	RefreshToken    string    `json:"refresh_token"`    // Long-lived refresh token (90d)
	CheckInInterval int       `json:"check_in_interval"`
}

// Load reads configuration from file
func Load(configPath string) (*Config, error) {
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if file doesn't exist
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// Save writes configuration to file
func (c *Config) Save(configPath string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
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
