package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Fimeg/RedFlag/server/internal/crypto"
)

// SecretsManager handles Docker secrets creation and management
type SecretsManager struct {
	secretsPath   string
	encryptionKey string
}

// NewSecretsManager creates a new secrets manager
func NewSecretsManager() *SecretsManager {
	secretsPath := getSecretsPath()
	return &SecretsManager{
		secretsPath: secretsPath,
	}
}

// CreateDockerSecrets creates Docker secrets from the provided secrets map
func (sm *SecretsManager) CreateDockerSecrets(secrets map[string]string) error {
	if len(secrets) == 0 {
		return nil
	}

	// Ensure secrets directory exists
	if err := os.MkdirAll(sm.secretsPath, 0755); err != nil {
		return fmt.Errorf("failed to create secrets directory: %w", err)
	}

	// Generate encryption key if not provided
	if sm.encryptionKey == "" {
		key, err := sm.GenerateEncryptionKey()
		if err != nil {
			return fmt.Errorf("failed to generate encryption key: %w", err)
		}
		sm.encryptionKey = key
	}

	// Create each secret
	for name, value := range secrets {
		if err := sm.createSecret(name, value); err != nil {
			return fmt.Errorf("failed to create secret %s: %w", name, err)
		}
	}

	return nil
}

// createSecret creates a single Docker secret
func (sm *SecretsManager) createSecret(name, value string) error {
	secretPath := filepath.Join(sm.secretsPath, name)

	// Encrypt sensitive values
	encryptedValue, err := sm.encryptSecret(value)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// Write secret file with restricted permissions
	if err := os.WriteFile(secretPath, encryptedValue, 0400); err != nil {
		return fmt.Errorf("failed to write secret file: %w", err)
	}

	return nil
}

// encryptSecret encrypts a secret value using the shared AES-256-GCM primitive.
// The key is the hex-encoded master key; output is nonce||ciphertext.
func (sm *SecretsManager) encryptSecret(value string) ([]byte, error) {
	keyBytes, err := hex.DecodeString(sm.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("invalid encryption key format: %w", err)
	}
	return crypto.Encrypt(keyBytes, []byte(value))
}

// decryptSecret reverses encryptSecret using the shared AES-256-GCM primitive.
func (sm *SecretsManager) decryptSecret(encryptedValue []byte) (string, error) {
	keyBytes, err := hex.DecodeString(sm.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("invalid encryption key format: %w", err)
	}
	plaintext, err := crypto.Decrypt(keyBytes, encryptedValue)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// GenerateEncryptionKey generates a new encryption key
func (sm *SecretsManager) GenerateEncryptionKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// SetEncryptionKey sets the master encryption key
func (sm *SecretsManager) SetEncryptionKey(key string) {
	sm.encryptionKey = key
}

// GetEncryptionKey returns the current encryption key
func (sm *SecretsManager) GetEncryptionKey() string {
	return sm.encryptionKey
}

// GetSecretsPath returns the current secrets path
func (sm *SecretsManager) GetSecretsPath() string {
	return sm.secretsPath
}

// ValidateSecrets validates that all required secrets exist
func (sm *SecretsManager) ValidateSecrets(requiredSecrets []string) error {
	for _, secretName := range requiredSecrets {
		secretPath := filepath.Join(sm.secretsPath, secretName)
		if _, err := os.Stat(secretPath); os.IsNotExist(err) {
			return fmt.Errorf("required secret not found: %s", secretName)
		}
	}
	return nil
}

// ListSecrets returns a list of all created secrets
func (sm *SecretsManager) ListSecrets() ([]string, error) {
	entries, err := os.ReadDir(sm.secretsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read secrets directory: %w", err)
	}

	var secrets []string
	for _, entry := range entries {
		if !entry.IsDir() {
			secrets = append(secrets, entry.Name())
		}
	}

	return secrets, nil
}

// RemoveSecret removes a Docker secret
func (sm *SecretsManager) RemoveSecret(name string) error {
	secretPath := filepath.Join(sm.secretsPath, name)
	return os.Remove(secretPath)
}

// Cleanup removes all secrets and the secrets directory
func (sm *SecretsManager) Cleanup() error {
	if _, err := os.Stat(sm.secretsPath); os.IsNotExist(err) {
		return nil
	}

	// Remove all files in the directory
	entries, err := os.ReadDir(sm.secretsPath)
	if err != nil {
		return fmt.Errorf("failed to read secrets directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			if err := os.Remove(filepath.Join(sm.secretsPath, entry.Name())); err != nil {
				return fmt.Errorf("failed to remove secret %s: %w", entry.Name(), err)
			}
		}
	}

	// Remove the directory itself
	return os.Remove(sm.secretsPath)
}

// getSecretsPath returns the platform-specific secrets path
func getSecretsPath() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\Docker\secrets`
	}
	return "/run/secrets"
}

// IsDockerEnvironment checks if running in Docker
func IsDockerEnvironment() bool {
	// Check for .dockerenv file
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check for Docker in cgroup
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if containsString(string(data), "docker") {
			return true
		}
	}

	return false
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr)))
}