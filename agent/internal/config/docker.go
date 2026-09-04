package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DockerSecretsConfig holds Docker secrets configuration
type DockerSecretsConfig struct {
	Enabled       bool              `json:"enabled"`
	SecretsPath   string            `json:"secrets_path"`
	EncryptionKey string            `json:"encryption_key,omitempty"`
	Secrets       map[string]string `json:"secrets,omitempty"`
}

// LoadDockerConfig loads Docker configuration if available
func LoadDockerConfig(configPath string) (*DockerSecretsConfig, error) {
	dockerConfigPath := filepath.Join(configPath, "docker.json")

	// Check if Docker config exists
	if _, err := os.Stat(dockerConfigPath); os.IsNotExist(err) {
		return &DockerSecretsConfig{Enabled: false}, nil
	}

	data, err := ioutil.ReadFile(dockerConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Docker config: %w", err)
	}

	var dockerConfig DockerSecretsConfig
	if err := json.Unmarshal(data, &dockerConfig); err != nil {
		return nil, fmt.Errorf("failed to parse Docker config: %w", err)
	}

	// Set default secrets path if not specified
	if dockerConfig.SecretsPath == "" {
		dockerConfig.SecretsPath = getDefaultSecretsPath()
	}

	return &dockerConfig, nil
}

// getDefaultSecretsPath returns the default Docker secrets path for the platform
func getDefaultSecretsPath() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\Docker\secrets`
	}
	return "/run/secrets"
}

// ReadSecret reads a secret from Docker secrets or falls back to file
func ReadSecret(secretName, fallbackPath string, dockerConfig *DockerSecretsConfig) ([]byte, error) {
	// Try Docker secrets first if enabled
	if dockerConfig != nil && dockerConfig.Enabled {
		secretPath := filepath.Join(dockerConfig.SecretsPath, secretName)
		if data, err := ioutil.ReadFile(secretPath); err == nil {
			if teeLogger != nil {
				teeLogger.Info("agent", "config", "docker_config", "read secret from docker", map[string]interface{}{
					"secret_name": secretName,
				})
			}
			return data, nil
		}
	}

	// Fall back to file system
	if fallbackPath != "" {
		if data, err := ioutil.ReadFile(fallbackPath); err == nil {
			if teeLogger != nil {
				teeLogger.Info("agent", "config", "docker_config", "read secret from file", map[string]interface{}{
					"file_path": fallbackPath,
				})
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("secret not found: %s", secretName)
}

// MergeConfigWithSecrets merges configuration with Docker secrets
func MergeConfigWithSecrets(config *Config, dockerConfig *DockerSecretsConfig) error {
	if dockerConfig == nil || !dockerConfig.Enabled {
		return nil
	}

	// If there's an encrypted config, decrypt and merge it
	if encryptedConfigPath, exists := dockerConfig.Secrets["config"]; exists {
		if err := mergeEncryptedConfig(config, encryptedConfigPath, dockerConfig.EncryptionKey); err != nil {
			return fmt.Errorf("failed to merge encrypted config: %w", err)
		}
	}

	// Apply other secrets to configuration
	if err := applySecretsToConfig(config, dockerConfig); err != nil {
		return fmt.Errorf("failed to apply secrets to config: %w", err)
	}

	return nil
}

// mergeEncryptedConfig decrypts and merges encrypted configuration
func mergeEncryptedConfig(config *Config, encryptedPath, encryptionKey string) error {
	if encryptionKey == "" {
		return fmt.Errorf("no encryption key available for encrypted config")
	}

	// Create temporary file for decrypted config
	tempPath := encryptedPath + ".tmp"
	defer os.Remove(tempPath)

	// Decrypt the config file
	// Note: This would need to import the migration package's DecryptFile function
	// For now, we'll assume the decryption happens elsewhere
	return fmt.Errorf("encrypted config merge not yet implemented")
}

// applySecretsToConfig applies Docker secrets to configuration fields
func applySecretsToConfig(config *Config, dockerConfig *DockerSecretsConfig) error {
	// Apply proxy secrets
	if proxyUsername, exists := dockerConfig.Secrets["proxy_username"]; exists {
		config.Proxy.Username = proxyUsername
	}
	if proxyPassword, exists := dockerConfig.Secrets["proxy_password"]; exists {
		config.Proxy.Password = proxyPassword
	}

	// Apply TLS secrets
	if certFile, exists := dockerConfig.Secrets["tls_cert"]; exists {
		config.TLS.CertFile = certFile
	}
	if keyFile, exists := dockerConfig.Secrets["tls_key"]; exists {
		config.TLS.KeyFile = keyFile
	}
	if caFile, exists := dockerConfig.Secrets["tls_ca"]; exists {
		config.TLS.CAFile = caFile
	}

	// Apply registration token
	if regToken, exists := dockerConfig.Secrets["registration_token"]; exists {
		config.RegistrationToken = regToken
	}

	return nil
}

// IsDockerEnvironment checks if the agent is running in Docker
func IsDockerEnvironment() bool {
	// Check for .dockerenv file
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check for Docker in cgroup
	if data, err := ioutil.ReadFile("/proc/1/cgroup"); err == nil {
		if contains(string(data), "docker") {
			return true
		}
	}

	return false
}

// SaveDockerConfig saves Docker configuration to disk
func SaveDockerConfig(dockerConfig *DockerSecretsConfig, configPath string) error {
	dockerConfigPath := filepath.Join(configPath, "docker.json")

	data, err := json.MarshalIndent(dockerConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Docker config: %w", err)
	}

	if err := ioutil.WriteFile(dockerConfigPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write Docker config: %w", err)
	}

	if teeLogger != nil {
		teeLogger.Info("agent", "config", "docker_config", "saved docker config", map[string]interface{}{
			"config_path": dockerConfigPath,
		})
	}
	return nil
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}