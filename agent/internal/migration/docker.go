package migration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/common"
)

// DockerDetection represents Docker secrets detection results
type DockerDetection struct {
	DockerAvailable     bool     `json:"docker_available"`
	SecretsMountPath    string   `json:"secrets_mount_path"`
	RequiredSecrets     []string `json:"required_secrets"`
	ExistingSecrets     []string `json:"existing_secrets"`
	MigrateToSecrets    bool     `json:"migrate_to_secrets"`
	SecretFiles         []common.AgentFile `json:"secret_files"`
	DetectionTime       time.Time `json:"detection_time"`
}

// SecretFile represents a file that should be migrated to Docker secrets
type SecretFile struct {
	Name        string `json:"name"`
	SourcePath  string `json:"source_path"`
	SecretPath  string `json:"secret_path"`
	Encrypted   bool   `json:"encrypted"`
	Checksum    string `json:"checksum"`
	Size        int64  `json:"size"`
}

// DockerConfig holds Docker secrets configuration
type DockerConfig struct {
	Enabled       bool     `json:"enabled"`
	SecretsPath   string   `json:"secrets_path"`
	EncryptionKey string   `json:"encryption_key,omitempty"`
	Secrets       map[string]string `json:"secrets,omitempty"`
}

// GetDockerSecretsPath returns the platform-specific Docker secrets path
func GetDockerSecretsPath() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\Docker\secrets`
	}
	return "/run/secrets"
}

// DetectDockerSecretsRequirements detects if Docker secrets migration is needed
func DetectDockerSecretsRequirements(config *FileDetectionConfig) (*DockerDetection, error) {
	detection := &DockerDetection{
		DetectionTime:    time.Now().UTC(),
		SecretsMountPath: GetDockerSecretsPath(),
	}

	// Check if Docker secrets directory exists
	if _, err := os.Stat(detection.SecretsMountPath); err == nil {
		detection.DockerAvailable = true
		fmt.Printf("[DOCKER] Docker secrets mount path detected: %s\n", detection.SecretsMountPath)
	} else {
		fmt.Printf("[DOCKER] Docker secrets not available: %s\n", err)
		return detection, nil
	}

	// Scan for sensitive files that should be migrated to secrets
	secretFiles, err := scanSecretFiles(config)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for secret files: %w", err)
	}

	detection.SecretFiles = secretFiles
	detection.MigrateToSecrets = len(secretFiles) > 0

	// Identify required secrets
	detection.RequiredSecrets = identifyRequiredSecrets(secretFiles)

	// Check existing secrets
	detection.ExistingSecrets = scanExistingSecrets(detection.SecretsMountPath)

	return detection, nil
}

// scanSecretFiles scans for files containing sensitive data
func scanSecretFiles(config *FileDetectionConfig) ([]common.AgentFile, error) {
	var secretFiles []common.AgentFile

	// Define sensitive file patterns
	secretPatterns := []string{
		"agent.key",
		"server.key",
		"ca.crt",
		"*.pem",
		"*.key",
		"config.json", // Will be filtered for sensitive content
	}

	// Scan new directory paths for secret files
	for _, dirPath := range []string{config.NewConfigPath, config.NewStatePath} {
		if _, err := os.Stat(dirPath); err == nil {
			files, err := scanSecretDirectory(dirPath, secretPatterns)
			if err != nil {
				return nil, fmt.Errorf("failed to scan directory %s for secrets: %w", dirPath, err)
			}
			secretFiles = append(secretFiles, files...)
		}
	}

	return secretFiles, nil
}

// scanSecretDirectory scans a directory for files that may contain secrets
func scanSecretDirectory(dirPath string, patterns []string) ([]common.AgentFile, error) {
	var files []common.AgentFile

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Check if file matches secret patterns
		if !matchesSecretPattern(path, patterns) {
			// For config.json, check if it contains sensitive data
			if filepath.Base(path) == "config.json" {
				if hasSensitiveContent(path) {
					return addSecretFile(&files, path, info)
				}
			}
			return nil
		}

		return addSecretFile(&files, path, info)
	})

	return files, err
}

// addSecretFile adds a file to the secret files list
func addSecretFile(files *[]common.AgentFile, path string, info os.FileInfo) error {
	checksum, err := calculateFileChecksum(path)
	if err != nil {
		return nil // Skip files we can't read
	}

	file := common.AgentFile{
		Path:         path,
		Size:         info.Size(),
		ModifiedTime: info.ModTime(),
		Checksum:     checksum,
		Required:     true,
		Migrate:      true,
		Description:  getSecretFileDescription(path),
	}

	*files = append(*files, file)
	return nil
}

// matchesSecretPattern checks if a file path matches secret patterns
func matchesSecretPattern(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
	}
	return false
}

// hasSensitiveContent checks if a config file contains sensitive data
func hasSensitiveContent(configPath string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}

	// Check for sensitive fields
	sensitiveFields := []string{
		"password", "token", "key", "secret", "credential",
		"proxy", "tls", "certificate", "private",
	}

	for _, field := range sensitiveFields {
		if containsSensitiveField(config, field) {
			return true
		}
	}

	return false
}

// containsSensitiveField recursively checks for sensitive fields in config
func containsSensitiveField(config map[string]interface{}, field string) bool {
	for key, value := range config {
		if containsString(key, field) {
			return true
		}

		if nested, ok := value.(map[string]interface{}); ok {
			if containsSensitiveField(nested, field) {
				return true
			}
		}
	}
	return false
}

// containsString checks if a string contains a substring (case-insensitive)
func containsString(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}

// identifyRequiredSecrets identifies which secrets need to be created
func identifyRequiredSecrets(secretFiles []common.AgentFile) []string {
	var secrets []string
	for _, file := range secretFiles {
		secretName := filepath.Base(file.Path)
		if file.Path == "config.json" {
			secrets = append(secrets, "config.json.enc")
		} else {
			secrets = append(secrets, secretName)
		}
	}
	return secrets
}

// scanExistingSecrets scans the Docker secrets directory for existing secrets
func scanExistingSecrets(secretsPath string) []string {
	var secrets []string

	entries, err := os.ReadDir(secretsPath)
	if err != nil {
		return secrets
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			secrets = append(secrets, entry.Name())
		}
	}

	return secrets
}

// getSecretFileDescription returns a description for a secret file
func getSecretFileDescription(path string) string {
	base := filepath.Base(path)
	switch {
	case base == "agent.key":
		return "Agent private key"
	case base == "server.key":
		return "Server private key"
	case base == "ca.crt":
		return "Certificate authority certificate"
	case strings.Contains(base, ".key"):
		return "Private key file"
	case strings.Contains(base, ".crt") || strings.Contains(base, ".pem"):
		return "Certificate file"
	case base == "config.json":
		return "Configuration file with sensitive data"
	default:
		return "Secret file"
	}
}

// EncryptFile encrypts a file using AES-256-GCM
func EncryptFile(inputPath, outputPath, key string) error {
	// Generate key from passphrase
	keyBytes := sha256.Sum256([]byte(key))

	// Read input file
	plaintext, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Create cipher
	block, err := aes.NewCipher(keyBytes[:])
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Write encrypted file
	if err := os.WriteFile(outputPath, ciphertext, 0600); err != nil {
		return fmt.Errorf("failed to write encrypted file: %w", err)
	}

	return nil
}

// DecryptFile decrypts a file using AES-256-GCM
func DecryptFile(inputPath, outputPath, key string) error {
	// Generate key from passphrase
	keyBytes := sha256.Sum256([]byte(key))

	// Read encrypted file
	ciphertext, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file: %w", err)
	}

	// Create cipher
	block, err := aes.NewCipher(keyBytes[:])
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Check minimum length
	if len(ciphertext) < gcm.NonceSize() {
		return fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt: %w", err)
	}

	// Write decrypted file
	if err := os.WriteFile(outputPath, plaintext, 0600); err != nil {
		return fmt.Errorf("failed to write decrypted file: %w", err)
	}

	return nil
}

// GenerateEncryptionKey generates a random encryption key
func GenerateEncryptionKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// IsDockerEnvironment checks if running in Docker environment
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