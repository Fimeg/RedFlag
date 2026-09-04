package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/common"
)

// NewBuildRequest represents a request for a new agent build
type NewBuildRequest struct {
	ServerURL        string                 `json:"server_url" binding:"required"`
	Environment      string                 `json:"environment" binding:"required"`
	AgentType        string                 `json:"agent_type" binding:"required,oneof=linux-server windows-workstation docker-host"`
	Organization     string                 `json:"organization" binding:"required"`
	RegistrationToken string                `json:"registration_token" binding:"required"`
	CustomSettings   map[string]interface{} `json:"custom_settings,omitempty"`
	DeploymentID     string                 `json:"deployment_id,omitempty"`
	AgentID          string                 `json:"agent_id,omitempty"` // For upgrades when preserving ID
}

// UpgradeBuildRequest represents a request for an agent upgrade
type UpgradeBuildRequest struct {
	ServerURL       string                 `json:"server_url" binding:"required"`
	Environment     string                 `json:"environment"`
	AgentType       string                 `json:"agent_type"`
	Organization    string                 `json:"organization"`
	CustomSettings  map[string]interface{} `json:"custom_settings,omitempty"`
	DeploymentID    string                 `json:"deployment_id,omitempty"`
	PreserveExisting bool                  `json:"preserve_existing"`
	DetectionPath   string                 `json:"detection_path,omitempty"`
}

// DetectionRequest represents a request to detect existing agent installation
type DetectionRequest struct {
	DetectionPath string `json:"detection_path,omitempty"`
}

// InstallationDetection represents the result of detecting an existing installation
type InstallationDetection struct {
	HasExistingAgent   bool                   `json:"has_existing_agent"`
	AgentID           string                 `json:"agent_id,omitempty"`
	CurrentVersion    string                 `json:"current_version,omitempty"`
	ConfigVersion     int                    `json:"config_version,omitempty"`
	RequiresMigration bool                   `json:"requires_migration"`
	Inventory         *AgentFileInventory    `json:"inventory,omitempty"`
	MigrationPlan     *MigrationDetection    `json:"migration_plan,omitempty"`
	DetectionPath     string                 `json:"detection_path"`
	DetectionTime     string                 `json:"detection_time"`
	RecommendedAction string                 `json:"recommended_action"`
}

// AgentFileInventory represents all files associated with an agent installation
type AgentFileInventory struct {
	ConfigFiles      []common.AgentFile `json:"config_files"`
	StateFiles       []common.AgentFile `json:"state_files"`
	BinaryFiles      []common.AgentFile `json:"binary_files"`
	LogFiles         []common.AgentFile `json:"log_files"`
	CertificateFiles []common.AgentFile `json:"certificate_files"`
	ExistingPaths    []string           `json:"existing_paths"`
	MissingPaths     []string           `json:"missing_paths"`
}

// MigrationDetection represents migration detection results (from existing migration system)
type MigrationDetection struct {
	CurrentAgentVersion      string            `json:"current_agent_version"`
	CurrentConfigVersion     int               `json:"current_config_version"`
	RequiresMigration        bool              `json:"requires_migration"`
	RequiredMigrations       []string          `json:"required_migrations"`
	MissingSecurityFeatures  []string          `json:"missing_security_features"`
	Inventory                *AgentFileInventory `json:"inventory"`
	DetectionTime            string            `json:"detection_time"`
}

// InstallationDetector handles detection of existing agent installations
type InstallationDetector struct{}

// NewInstallationDetector creates a new installation detector
func NewInstallationDetector() *InstallationDetector {
	return &InstallationDetector{}
}

// DetectExistingInstallation detects if there's an existing agent installation
func (id *InstallationDetector) DetectExistingInstallation(agentID string) (*InstallationDetection, error) {
	result := &InstallationDetection{
		HasExistingAgent: false,
		DetectionTime:    time.Now().UTC().Format(time.RFC3339),
		RecommendedAction: "new_installation",
	}

	if agentID != "" {
		result.HasExistingAgent = true
		result.AgentID = agentID
		result.RecommendedAction = "upgrade"
	}

	return result, nil
}

// scanDirectory scans a directory for agent-related files
func (id *InstallationDetector) scanDirectory(dirPath string) ([]common.AgentFile, error) {
	var files []common.AgentFile

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return files, nil // Directory doesn't exist, return empty
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Calculate checksum
		checksum, err := id.calculateChecksum(fullPath)
		if err != nil {
			checksum = ""
		}

		file := common.AgentFile{
			Path:         fullPath,
			Size:         info.Size(),
			ModifiedTime: info.ModTime(),
			Checksum:     checksum,
			Required:     id.isRequiredFile(entry.Name()),
			Migrate:      id.shouldMigrateFile(entry.Name()),
			Description:  id.getFileDescription(entry.Name()),
		}

		files = append(files, file)
	}

	return files, nil
}

// categorizeFile categorizes a file into the appropriate inventory section
func (id *InstallationDetector) categorizeFile(file common.AgentFile, inventory *AgentFileInventory) {
	filename := filepath.Base(file.Path)

	switch {
	case filename == "config.json":
		inventory.ConfigFiles = append(inventory.ConfigFiles, file)
	case filename == "pending_acks.json" || filename == "public_key.cache" || filename == "last_scan.json" || filename == "metrics.json":
		inventory.StateFiles = append(inventory.StateFiles, file)
	case filename == "redflag-agent" || filename == "redflag-agent.exe":
		inventory.BinaryFiles = append(inventory.BinaryFiles, file)
	case strings.HasSuffix(filename, ".log"):
		inventory.LogFiles = append(inventory.LogFiles, file)
	case strings.HasSuffix(filename, ".crt") || strings.HasSuffix(filename, ".key") || strings.HasSuffix(filename, ".pem"):
		inventory.CertificateFiles = append(inventory.CertificateFiles, file)
	}
}

// extractAgentInfo extracts agent ID, version, and config version from config files
func (id *InstallationDetector) extractAgentInfo(inventory *AgentFileInventory) (string, string, int, error) {
	var agentID, version string
	var configVersion int

	// Look for config.json first
	for _, configFile := range inventory.ConfigFiles {
		if strings.Contains(configFile.Path, "config.json") {
			data, err := os.ReadFile(configFile.Path)
			if err != nil {
				continue
			}

			var config map[string]interface{}
			if err := json.Unmarshal(data, &config); err != nil {
				continue
			}

			// Extract agent ID
			if id, ok := config["agent_id"].(string); ok {
				agentID = id
			}

			// Extract version information
			if ver, ok := config["agent_version"].(string); ok {
				version = ver
			}
			if ver, ok := config["version"].(float64); ok {
				configVersion = int(ver)
			}

			break
		}
	}

	// If no agent ID found in config, we don't have a valid installation
	if agentID == "" {
		return "", "", 0, fmt.Errorf("no agent ID found in configuration")
	}

	return agentID, version, configVersion, nil
}

// determineMigrationRequired determines if migration is needed
func (id *InstallationDetector) determineMigrationRequired(inventory *AgentFileInventory) bool {
	// Check for old directory paths
	for _, configFile := range inventory.ConfigFiles {
		if strings.Contains(configFile.Path, "/etc/aggregator/") || strings.Contains(configFile.Path, "/var/lib/aggregator/") {
			return true
		}
	}

	for _, stateFile := range inventory.StateFiles {
		if strings.Contains(stateFile.Path, "/etc/aggregator/") || strings.Contains(stateFile.Path, "/var/lib/aggregator/") {
			return true
		}
	}

	// Check config version (older than v5 needs migration)
	for _, configFile := range inventory.ConfigFiles {
		if strings.Contains(configFile.Path, "config.json") {
			if _, _, configVersion, err := id.extractAgentInfo(inventory); err == nil {
				if configVersion < 5 {
					return true
				}
			}
		}
	}

	return false
}

// calculateChecksum calculates SHA256 checksum of a file
func (id *InstallationDetector) calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// isRequiredFile determines if a file is required for agent operation
func (id *InstallationDetector) isRequiredFile(filename string) bool {
	requiredFiles := []string{
		"config.json",
		"redflag-agent",
		"redflag-agent.exe",
	}

	for _, required := range requiredFiles {
		if filename == required {
			return true
		}
	}
	return false
}

// shouldMigrateFile determines if a file should be migrated
func (id *InstallationDetector) shouldMigrateFile(filename string) bool {
	migratableFiles := []string{
		"config.json",
		"pending_acks.json",
		"public_key.cache",
		"last_scan.json",
		"metrics.json",
	}

	for _, migratable := range migratableFiles {
		if filename == migratable {
			return true
		}
	}
	return false
}

// getFileDescription returns a human-readable description of a file
func (id *InstallationDetector) getFileDescription(filename string) string {
	descriptions := map[string]string{
		"config.json":       "Agent configuration file",
		"pending_acks.json": "Pending command acknowledgments",
		"public_key.cache":  "Server public key cache",
		"last_scan.json":    "Last scan results",
		"metrics.json":      "Agent metrics data",
		"redflag-agent":     "Agent binary",
		"redflag-agent.exe": "Windows agent binary",
	}

	if desc, ok := descriptions[filename]; ok {
		return desc
	}
	return "Agent file"
}