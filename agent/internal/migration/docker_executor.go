package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/common"
	"github.com/Fimeg/RedFlag/agent/internal/event"
)

// DockerSecretsExecutor handles the execution of Docker secrets migration
type DockerSecretsExecutor struct {
	detection  *DockerDetection
	config     *FileDetectionConfig
	logger     *event.TeeLogger
	encryption string
}

// NewDockerSecretsExecutor creates a new Docker secrets executor
func NewDockerSecretsExecutor(detection *DockerDetection, config *FileDetectionConfig, logger *event.TeeLogger) *DockerSecretsExecutor {
	return &DockerSecretsExecutor{
		detection: detection,
		config:    config,
		logger:    logger,
	}
}

// ExecuteDockerSecretsMigration performs the Docker secrets migration
func (e *DockerSecretsExecutor) ExecuteDockerSecretsMigration() error {
	if !e.detection.DockerAvailable {
		return fmt.Errorf("docker secrets not available")
	}

	if !e.detection.MigrateToSecrets {
		e.logger.Info("agent", "docker", "docker_executor", "No secrets to migrate", nil)
		return nil
	}

	e.logger.Info("agent", "docker", "docker_executor", "Starting Docker secrets migration", nil)

	// Generate encryption key for config files
	encKey, err := GenerateEncryptionKey()
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}
	e.encryption = encKey

	// Create backup before migration
	if err := e.createSecretsBackup(); err != nil {
		return fmt.Errorf("failed to create secrets backup: %w", err)
	}

	// Migrate each secret file
	for _, secretFile := range e.detection.SecretFiles {
		if err := e.migrateSecretFile(secretFile); err != nil {
			e.logger.Warning("agent", "docker", "docker_executor",
				fmt.Sprintf("Failed to migrate secret file %s: %v", secretFile.Path, err),
				map[string]interface{}{
					"file_path": secretFile.Path,
					"error":     err.Error(),
				})
			continue
		}
	}

	// Create Docker secrets configuration
	if err := e.createDockerConfig(); err != nil {
		return fmt.Errorf("failed to create Docker config: %w", err)
	}

	// Remove original secret files
	if err := e.removeOriginalSecrets(); err != nil {
		return fmt.Errorf("failed to remove original secrets: %w", err)
	}

	e.logger.Info("agent", "docker", "docker_executor", "Docker secrets migration completed successfully", nil)
	e.logger.Info("agent", "docker", "docker_executor", "Encryption key generated",
		map[string]interface{}{
			"key_hint": "key generated - save securely for decryption",
		})

	return nil
}

// createSecretsBackup creates a backup of secret files before migration
func (e *DockerSecretsExecutor) createSecretsBackup() error {
	timestamp := time.Now().Format("2006-01-02-150405")
	backupDir := fmt.Sprintf("/etc/redflag.backup.secrets.%s", timestamp)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	for _, secretFile := range e.detection.SecretFiles {
		backupPath := filepath.Join(backupDir, filepath.Base(secretFile.Path))
		if err := copySecretFile(secretFile.Path, backupPath); err != nil {
			e.logger.Warning("agent", "docker", "docker_executor",
				fmt.Sprintf("Failed to backup secret file %s: %v", secretFile.Path, err),
				map[string]interface{}{
					"file_path": secretFile.Path,
					"error":     err.Error(),
				})
		} else {
			e.logger.Info("agent", "docker", "docker_executor", "Backed up secret file",
				map[string]interface{}{
					"from": secretFile.Path,
					"to":   backupPath,
				})
		}
	}

	return nil
}

// migrateSecretFile migrates a single secret file to Docker secrets
func (e *DockerSecretsExecutor) migrateSecretFile(secretFile common.AgentFile) error {
	secretName := filepath.Base(secretFile.Path)
	secretPath := filepath.Join(e.detection.SecretsMountPath, secretName)

	// Handle config.json specially (encrypt it)
	if secretName == "config.json" {
		return e.migrateConfigFile(secretFile)
	}

	// Copy secret file to Docker secrets directory
	if err := copySecretFile(secretFile.Path, secretPath); err != nil {
		return fmt.Errorf("failed to copy secret to Docker mount: %w", err)
	}

	// Set secure permissions
	if err := os.Chmod(secretPath, 0400); err != nil {
		return fmt.Errorf("failed to set secret permissions: %w", err)
	}

	e.logger.Info("agent", "docker", "docker_executor", "Migrated secret",
		map[string]interface{}{
			"from": secretFile.Path,
			"to":   secretPath,
		})
	return nil
}

// migrateConfigFile handles special migration of config.json with encryption
func (e *DockerSecretsExecutor) migrateConfigFile(secretFile common.AgentFile) error {
	// Read original config
	configData, err := os.ReadFile(secretFile.Path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse config to separate sensitive from non-sensitive data
	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Split config into public and sensitive parts
	publicConfig, sensitiveConfig := e.splitConfig(config)

	// Write public config back to original location
	publicData, err := json.MarshalIndent(publicConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal public config: %w", err)
	}

	if err := os.WriteFile(secretFile.Path, publicData, 0644); err != nil {
		return fmt.Errorf("failed to write public config: %w", err)
	}

	// Encrypt sensitive config
	sensitiveData, err := json.MarshalIndent(sensitiveConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sensitive config: %w", err)
	}

	tempSensitivePath := secretFile.Path + ".sensitive"
	if err := os.WriteFile(tempSensitivePath, sensitiveData, 0600); err != nil {
		return fmt.Errorf("failed to write sensitive config: %w", err)
	}
	defer os.Remove(tempSensitivePath)

	// Encrypt sensitive config
	encryptedPath := filepath.Join(e.detection.SecretsMountPath, "config.json.enc")
	if err := EncryptFile(tempSensitivePath, encryptedPath, e.encryption); err != nil {
		return fmt.Errorf("failed to encrypt config: %w", err)
	}

	e.logger.Info("agent", "docker", "docker_executor", "Migrated config with encryption",
		map[string]interface{}{
			"original":       secretFile.Path,
			"public":         secretFile.Path,
			"encrypted":      encryptedPath,
		})

	return nil
}

// splitConfig splits configuration into public and sensitive parts
func (e *DockerSecretsExecutor) splitConfig(config map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	public := make(map[string]interface{})
	sensitive := make(map[string]interface{})

	sensitiveFields := []string{
		"password", "token", "key", "secret", "credential",
		"proxy", "tls", "certificate", "private",
	}

	for key, value := range config {
		if e.isSensitiveField(key, value, sensitiveFields) {
			sensitive[key] = value
		} else {
			public[key] = value
		}
	}

	return public, sensitive
}

// isSensitiveField checks if a field contains sensitive data
func (e *DockerSecretsExecutor) isSensitiveField(key string, value interface{}, sensitiveFields []string) bool {
	// Check key name
	for _, field := range sensitiveFields {
		if strings.Contains(strings.ToLower(key), strings.ToLower(field)) {
			return true
		}
	}

	// Check nested values
	if nested, ok := value.(map[string]interface{}); ok {
		for nKey, nValue := range nested {
			if e.isSensitiveField(nKey, nValue, sensitiveFields) {
				return true
			}
		}
	}

	return false
}

// createDockerConfig creates the Docker secrets configuration file
func (e *DockerSecretsExecutor) createDockerConfig() error {
	dockerConfig := DockerConfig{
		Enabled:       true,
		SecretsPath:   e.detection.SecretsMountPath,
		EncryptionKey: e.encryption,
		Secrets:       make(map[string]string),
	}

	// Map secret files to their Docker secret names
	for _, secretFile := range e.detection.SecretFiles {
		secretName := filepath.Base(secretFile.Path)
		if secretName == "config.json" {
			dockerConfig.Secrets["config"] = "config.json.enc"
		} else {
			dockerConfig.Secrets[secretName] = secretName
		}
	}

	// Write Docker config
	configPath := filepath.Join(e.config.NewConfigPath, "docker.json")
	configData, err := json.MarshalIndent(dockerConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Docker config: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return fmt.Errorf("failed to write Docker config: %w", err)
	}

	e.logger.Info("agent", "docker", "docker_executor", "Created Docker config",
		map[string]interface{}{
			"config_path": configPath,
		})
	return nil
}

// removeOriginalSecrets removes the original secret files after migration
func (e *DockerSecretsExecutor) removeOriginalSecrets() error {
	for _, secretFile := range e.detection.SecretFiles {
		// Don't remove config.json as it's been split into public part
		if filepath.Base(secretFile.Path) == "config.json" {
			continue
		}

		if err := os.Remove(secretFile.Path); err != nil {
			e.logger.Warning("agent", "docker", "docker_executor",
				fmt.Sprintf("Failed to remove original secret %s: %v", secretFile.Path, err),
				map[string]interface{}{
					"file_path": secretFile.Path,
					"error":     err.Error(),
				})
		} else {
			e.logger.Info("agent", "docker", "docker_executor", "Removed original secret",
				map[string]interface{}{
					"file_path": secretFile.Path,
				})
		}
	}

	return nil
}

// copySecretFile copies a file from src to dst (renamed to avoid conflicts)
func copySecretFile(src, dst string) error {
	// Read source file
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Write destination file
	return os.WriteFile(dst, data, 0644)
}

// ValidateDockerSecretsMigration validates that the Docker secrets migration was successful
func (e *DockerSecretsExecutor) ValidateDockerSecretsMigration() error {
	// Check that Docker secrets directory exists
	if _, err := os.Stat(e.detection.SecretsMountPath); err != nil {
		return fmt.Errorf("Docker secrets directory not accessible: %w", err)
	}

	// Check that all required secrets exist
	for _, secretName := range e.detection.RequiredSecrets {
		secretPath := filepath.Join(e.detection.SecretsMountPath, secretName)
		if _, err := os.Stat(secretPath); err != nil {
			return fmt.Errorf("required secret not found: %s", secretName)
		}
	}

	// Check that Docker config exists
	dockerConfigPath := filepath.Join(e.config.NewConfigPath, "docker.json")
	if _, err := os.Stat(dockerConfigPath); err != nil {
		return fmt.Errorf("Docker config not found: %w", err)
	}

	e.logger.Info("agent", "docker", "docker_executor", "Docker secrets migration validation successful", nil)
	return nil
}

// RollbackDockerSecretsMigration rolls back the Docker secrets migration
func (e *DockerSecretsExecutor) RollbackDockerSecretsMigration(backupDir string) error {
	e.logger.Warning("agent", "docker", "docker_executor", "Rolling back Docker secrets migration",
		map[string]interface{}{
			"backup_dir": backupDir,
		})

	// Restore original secret files from backup
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		backupPath := filepath.Join(backupDir, entry.Name())
		originalPath := filepath.Join(e.config.NewConfigPath, entry.Name())

		if err := copySecretFile(backupPath, originalPath); err != nil {
			e.logger.Error("agent", "docker", "docker_executor",
				fmt.Sprintf("Failed to restore %s: %v", entry.Name(), err),
				map[string]interface{}{
					"file_name": entry.Name(),
					"error":     err.Error(),
				})
		} else {
			e.logger.Info("agent", "docker", "docker_executor", "Restored secret",
				map[string]interface{}{
					"file_name": entry.Name(),
				})
		}
	}

	// Remove Docker config
	dockerConfigPath := filepath.Join(e.config.NewConfigPath, "docker.json")
	if err := os.Remove(dockerConfigPath); err != nil {
		e.logger.Error("agent", "docker", "docker_executor",
			fmt.Sprintf("Failed to remove Docker config: %v", err),
			map[string]interface{}{
				"config_path": dockerConfigPath,
				"error":       err.Error(),
			})
	}

	e.logger.Info("agent", "docker", "docker_executor", "Docker secrets migration rollback completed", nil)
	return nil
}