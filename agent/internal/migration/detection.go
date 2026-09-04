package migration

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/common"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/Fimeg/RedFlag/agent/internal/version"
)

// AgentFileInventory represents all files associated with an agent installation
type AgentFileInventory struct {
	ConfigFiles       []common.AgentFile `json:"config_files"`
	StateFiles        []common.AgentFile `json:"state_files"`
	BinaryFiles       []common.AgentFile `json:"binary_files"`
	LogFiles          []common.AgentFile `json:"log_files"`
	CertificateFiles  []common.AgentFile `json:"certificate_files"`
	OldDirectoryPaths []string           `json:"old_directory_paths"`
	NewDirectoryPaths []string           `json:"new_directory_paths"`
}

// MigrationDetection represents the result of migration detection
type MigrationDetection struct {
	CurrentAgentVersion     string              `json:"current_agent_version"`
	CurrentConfigVersion    int                 `json:"current_config_version"`
	RequiresMigration       bool                `json:"requires_migration"`
	RequiredMigrations      []string            `json:"required_migrations"`
	MissingSecurityFeatures []string            `json:"missing_security_features"`
	Inventory               *AgentFileInventory `json:"inventory"`
	DockerDetection         *DockerDetection    `json:"docker_detection,omitempty"`
	DetectionTime           time.Time           `json:"detection_time"`
}

// SecurityFeature represents a security feature that may be missing
type SecurityFeature struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Enabled     bool   `json:"enabled"`
}

// FileDetectionConfig holds configuration for file detection
type FileDetectionConfig struct {
	OldConfigPath    string
	OldStatePath     string
	NewConfigPath    string
	NewStatePath     string
	BackupDirPattern string
}

// NewFileDetectionConfig creates a default detection configuration
func NewFileDetectionConfig() *FileDetectionConfig {
	return &FileDetectionConfig{
		OldConfigPath:    constants.LegacyConfigPath,
		OldStatePath:     constants.LegacyStatePath,
		NewConfigPath:    constants.GetAgentConfigDir(),
		NewStatePath:     constants.GetAgentStateDir(),
		BackupDirPattern: constants.GetMigrationBackupDir() + "/%d",
	}
}

// DetectMigrationRequirements scans for existing agent installations and determines migration needs
func DetectMigrationRequirements(config *FileDetectionConfig) (*MigrationDetection, error) {
	detection := &MigrationDetection{
		DetectionTime: time.Now().UTC(),
		Inventory:     &AgentFileInventory{},
	}

	// Scan for existing installations
	inventory, err := scanAgentFiles(config)
	if err != nil {
		return nil, fmt.Errorf("failed to scan agent files: %w", err)
	}
	detection.Inventory = inventory

	// Detect version information
	version, configVersion, err := detectVersionInfo(inventory)
	if err != nil {
		return nil, fmt.Errorf("failed to detect version: %w", err)
	}
	detection.CurrentAgentVersion = version
	detection.CurrentConfigVersion = configVersion

	// Identify required migrations
	requiredMigrations := determineRequiredMigrations(detection, config)
	detection.RequiredMigrations = requiredMigrations
	detection.RequiresMigration = len(requiredMigrations) > 0

	// Identify missing security features
	missingFeatures := identifyMissingSecurityFeatures(detection)
	detection.MissingSecurityFeatures = missingFeatures

	// Detect Docker secrets requirements if in Docker environment
	if IsDockerEnvironment() {
		dockerDetection, err := DetectDockerSecretsRequirements(config)
		if err != nil {
			return nil, fmt.Errorf("failed to detect Docker secrets requirements: %w", err)
		}
		detection.DockerDetection = dockerDetection
	}

	return detection, nil
}

// scanAgentFiles scans for agent-related files in old and new locations
func scanAgentFiles(config *FileDetectionConfig) (*AgentFileInventory, error) {
	inventory := &AgentFileInventory{
		OldDirectoryPaths: []string{config.OldConfigPath, config.OldStatePath},
		NewDirectoryPaths: []string{config.NewConfigPath, config.NewStatePath},
	}

	// Define file patterns to look for
	filePatterns := map[string][]string{
		"config": {
			"config.json",
			"agent.key",
			"server.key",
			"ca.crt",
		},
		"state": {
			"pending_acks.json",
			"public_key.cache",
			"last_scan.json",
			"metrics.json",
		},
		"binary": {
			"redflag-agent",
			"redflag-agent.exe",
		},
		"log": {
			"redflag-agent.log",
			"redflag-agent.*.log",
		},
		"certificate": {
			"*.crt",
			"*.key",
			"*.pem",
		},
	}

	// Scan both old and new directory paths
	allPaths := append(inventory.OldDirectoryPaths, inventory.NewDirectoryPaths...)
	for _, dirPath := range allPaths {
		if _, err := os.Stat(dirPath); err == nil {
			files, err := scanDirectory(dirPath, filePatterns)
			if err != nil {
				return nil, fmt.Errorf("failed to scan directory %s: %w", dirPath, err)
			}

			// Categorize files
			for _, file := range files {
				switch {
				case ContainsAny(file.Path, filePatterns["config"]):
					inventory.ConfigFiles = append(inventory.ConfigFiles, file)
				case ContainsAny(file.Path, filePatterns["state"]):
					inventory.StateFiles = append(inventory.StateFiles, file)
				case ContainsAny(file.Path, filePatterns["binary"]):
					inventory.BinaryFiles = append(inventory.BinaryFiles, file)
				case ContainsAny(file.Path, filePatterns["log"]):
					inventory.LogFiles = append(inventory.LogFiles, file)
				case ContainsAny(file.Path, filePatterns["certificate"]):
					inventory.CertificateFiles = append(inventory.CertificateFiles, file)
				}
			}
		}
	}

	return inventory, nil
}

// scanDirectory scans a directory for files matching specific patterns
func scanDirectory(dirPath string, patterns map[string][]string) ([]common.AgentFile, error) {
	var files []common.AgentFile

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Calculate checksum
		checksum, err := calculateFileChecksum(path)
		if err != nil {
			// Skip files we can't read
			return nil
		}

		file := common.AgentFile{
			Path:         path,
			Size:         info.Size(),
			ModifiedTime: info.ModTime(),
			Checksum:     checksum,
			Required:     isRequiredFile(path, patterns),
			Migrate:      shouldMigrateFile(path, patterns),
			Description:  getFileDescription(path),
		}

		// Try to detect version from filename or content
		if version := detectFileVersion(path, info); version != "" {
			file.Version = version
		}

		files = append(files, file)
		return nil
	})

	return files, err
}

// detectVersionInfo attempts to detect agent and config versions from files
func detectVersionInfo(inventory *AgentFileInventory) (string, int, error) {
	var detectedVersion string
	configVersion := 0

	// Try to read config file for version information
	for _, configFile := range inventory.ConfigFiles {
		if strings.Contains(configFile.Path, "config.json") {
			version, cfgVersion, err := readConfigVersion(configFile.Path)
			if err == nil {
				detectedVersion = version
				configVersion = cfgVersion
				break
			}
		}
	}

	// If no version found in config, try binary files
	if detectedVersion == "" {
		for _, binaryFile := range inventory.BinaryFiles {
			if version := detectBinaryVersion(binaryFile.Path); version != "" {
				detectedVersion = version
				break
			}
		}
	}

	// Default to unknown if nothing found
	if detectedVersion == "" {
		detectedVersion = "unknown"
	}

	return detectedVersion, configVersion, nil
}

// readConfigVersion reads version information from a config file
func readConfigVersion(configPath string) (string, int, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", 0, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", 0, err
	}

	// Try to extract version info
	var agentVersion string
	var cfgVersion int

	if version, ok := config["agent_version"].(string); ok {
		agentVersion = version
	}
	// The `version` field has historically been serialized both as a JSON number
	// (install template writes `"version": 5`) and as a string (config.Config types
	// it as a string; the agent rewrites it that way on save). Detection must read
	// both, or the version gate misfires and re-triggers migrations forever.
	cfgVersion = parseConfigVersion(config["version"])

	return agentVersion, cfgVersion, nil
}

// parseConfigVersion coerces the config `version` field to an int regardless of
// whether it was stored as a JSON number or a string.
func parseConfigVersion(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n
		}
	}
	return 0
}

// determineRequiredMigrations determines what migrations are needed
func determineRequiredMigrations(detection *MigrationDetection, config *FileDetectionConfig) []string {
	var migrations []string

	// Check migration state to skip already completed migrations
	configPath := filepath.Join(config.NewConfigPath, "config.json")
	stateManager := NewStateManager(configPath)

	// Check if old directories exist
	for _, oldDir := range detection.Inventory.OldDirectoryPaths {
		if _, err := os.Stat(oldDir); err == nil {
			// Check if directory migration was already completed
			completed, err := stateManager.IsMigrationCompleted("directory_migration")
			if err == nil && !completed {
				migrations = append(migrations, "directory_migration")
			}
			break
		}
	}

	// Check for legacy installation (old path migration)
	hasLegacyDirs := false
	for _, oldDir := range detection.Inventory.OldDirectoryPaths {
		if _, err := os.Stat(oldDir); err == nil {
			hasLegacyDirs = true
			break
		}
	}

	// Legacy migration: always migrate if old directories exist
	if hasLegacyDirs {
		if detection.CurrentConfigVersion < 4 {
			// Check if already completed
			completed, err := stateManager.IsMigrationCompleted("config_migration")
			if err == nil && !completed {
				migrations = append(migrations, "config_migration")
			}
		}

		// Check if Docker secrets migration is needed (v5)
		if detection.CurrentConfigVersion < 5 {
			// Check if already completed
			completed, err := stateManager.IsMigrationCompleted("config_v5_migration")
			if err == nil && !completed {
				migrations = append(migrations, "config_v5_migration")
			}
		}
	} else {
		// Version-based migration: compare current config version with expected
		// This handles upgrades for agents already in correct location
		// Use version package for single source of truth
		agentVersion := version.Version
		expectedConfigVersionStr := version.ExtractConfigVersionFromAgent(agentVersion)
		// Convert to int for comparison (e.g., "6" -> 6)
		expectedConfigVersion := 6 // Default fallback
		if expectedConfigInt, err := strconv.Atoi(expectedConfigVersionStr); err == nil {
			expectedConfigVersion = expectedConfigInt
		}

		// If config file exists but version is old, migrate
		if detection.CurrentConfigVersion < expectedConfigVersion {
			if detection.CurrentConfigVersion < 4 {
				// Check if already completed
				completed, err := stateManager.IsMigrationCompleted("config_migration")
				if err == nil && !completed {
					migrations = append(migrations, "config_migration")
				}
			}

			// Check if Docker secrets migration is needed (v5)
			if detection.CurrentConfigVersion < 5 {
				// Check if already completed
				completed, err := stateManager.IsMigrationCompleted("config_v5_migration")
				if err == nil && !completed {
					migrations = append(migrations, "config_v5_migration")
				}
			}
		}
	}

	// Check if Docker secrets migration is needed
	if detection.DockerDetection != nil && detection.DockerDetection.MigrateToSecrets {
		// Check if already completed
		completed, err := stateManager.IsMigrationCompleted("docker_secrets_migration")
		if err == nil && !completed {
			migrations = append(migrations, "docker_secrets_migration")
		}
	}

	// Check if security features need to be applied
	if len(detection.MissingSecurityFeatures) > 0 {
		// Check if already completed
		completed, err := stateManager.IsMigrationCompleted("security_hardening")
		if err == nil && !completed {
			migrations = append(migrations, "security_hardening")
		}
	}

	return migrations
}

// identifyMissingSecurityFeatures identifies security features that need to be enabled
func identifyMissingSecurityFeatures(detection *MigrationDetection) []string {
	var missingFeatures []string

	// Check config for security features
	if detection.Inventory.ConfigFiles != nil {
		for _, configFile := range detection.Inventory.ConfigFiles {
			if strings.Contains(configFile.Path, "config.json") {
				features := checkConfigSecurityFeatures(configFile.Path)
				missingFeatures = append(missingFeatures, features...)
			}
		}
	}

	// Default missing features for old versions
	if detection.CurrentConfigVersion < 4 {
		missingFeatures = append(missingFeatures,
			"nonce_validation",
			"machine_id_binding",
			"ed25519_verification",
			"subsystem_configuration",
		)
	}

	return missingFeatures
}

// checkConfigSecurityFeatures checks a config file for security feature settings
func checkConfigSecurityFeatures(configPath string) []string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return []string{}
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return []string{}
	}

	var missingFeatures []string

	// Check for subsystem configuration
	if subsystems, ok := config["subsystems"].(map[string]interface{}); ok {
		if _, hasSystem := subsystems["system"]; !hasSystem {
			missingFeatures = append(missingFeatures, "system_subsystem")
		}
	} else {
		missingFeatures = append(missingFeatures, "subsystem_configuration")
	}

	// Check for machine ID
	if _, hasMachineID := config["machine_id"]; !hasMachineID {
		missingFeatures = append(missingFeatures, "machine_id_binding")
	}

	return missingFeatures
}

// Helper functions

func calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func ContainsAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}

func isRequiredFile(path string, patterns map[string][]string) bool {
	base := filepath.Base(path)
	return base == "config.json" || base == "pending_acks.json"
}

func shouldMigrateFile(path string, patterns map[string][]string) bool {
	return !ContainsAny(path, []string{"*.log", "*.tmp"})
}

func getFileDescription(path string) string {
	base := filepath.Base(path)
	switch {
	case base == "config.json":
		return "Agent configuration file"
	case base == "pending_acks.json":
		return "Pending command acknowledgments"
	case base == "public_key.cache":
		return "Server public key cache"
	case strings.Contains(base, ".log"):
		return "Agent log file"
	case strings.Contains(base, ".key"):
		return "Private key file"
	case strings.Contains(base, ".crt"):
		return "Certificate file"
	default:
		return "Agent file"
	}
}

func detectFileVersion(path string, info os.FileInfo) string {
	// Try to extract version from filename
	base := filepath.Base(path)
	if strings.Contains(base, "v0.1.") {
		// Extract version from filename like "redflag-agent-v0.1.22"
		parts := strings.Split(base, "v0.1.")
		if len(parts) > 1 {
			return "v0.1." + strings.Split(parts[1], "-")[0]
		}
	}
	return ""
}

func detectBinaryVersion(binaryPath string) string {
	// This would involve reading binary headers or executing with --version flag
	// For now, return empty
	return ""
}
