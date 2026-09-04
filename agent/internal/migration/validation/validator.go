package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/common"
	"github.com/Fimeg/RedFlag/agent/internal/event"
	"github.com/Fimeg/RedFlag/agent/internal/migration"
	"github.com/Fimeg/RedFlag/agent/internal/migration/pathutils"
	"github.com/gofrs/uuid/v5"
)

// FileValidator handles comprehensive file validation for migration
type FileValidator struct {
	pathManager *pathutils.PathManager
	logger      *event.TeeLogger
}

// NewFileValidator creates a new file validator
func NewFileValidator(pm *pathutils.PathManager, logger *event.TeeLogger) *FileValidator {
	return &FileValidator{
		pathManager: pm,
		logger:      logger,
	}
}

// ValidationResult holds validation results
type ValidationResult struct {
	Valid      bool              `json:"valid"`
	Errors     []string          `json:"errors"`
	Warnings   []string          `json:"warnings"`
	Inventory  *FileInventory    `json:"inventory"`
	Statistics *ValidationStats  `json:"statistics"`
}

// FileInventory represents validated files
type FileInventory struct {
	ValidFiles     []common.AgentFile `json:"valid_files"`
	InvalidFiles   []InvalidFile      `json:"invalid_files"`
	MissingFiles   []string           `json:"missing_files"`
	SkippedFiles   []SkippedFile      `json:"skipped_files"`
	Directories    []string           `json:"directories"`
}

// InvalidFile represents a file that failed validation
type InvalidFile struct {
	Path        string `json:"path"`
	Reason      string `json:"reason"`
	ErrorType   string `json:"error_type"` // "not_found", "permission", "traversal", "other"
	Expected    string `json:"expected"`
}

// SkippedFile represents a file that was intentionally skipped
type SkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ValidationStats holds statistics about validation
type ValidationStats struct {
	TotalFiles      int     `json:"total_files"`
	ValidFiles      int     `json:"valid_files"`
	InvalidFiles    int     `json:"invalid_files"`
	MissingFiles    int     `json:"missing_files"`
	SkippedFiles    int     `json:"skipped_files"`
	ValidationTime  int64   `json:"validation_time_ms"`
	TotalSizeBytes  int64   `json:"total_size_bytes"`
}

// ValidateInventory performs comprehensive validation of file inventory
func (v *FileValidator) ValidateInventory(files []common.AgentFile, requiredPatterns []string) (*ValidationResult, error) {
	start := time.Now()
	result := &ValidationResult{
		Valid:     true,
		Errors:    []string{},
		Warnings:  []string{},
		Inventory: &FileInventory{
			ValidFiles:   []common.AgentFile{},
			InvalidFiles: []InvalidFile{},
			MissingFiles: []string{},
			SkippedFiles: []SkippedFile{},
			Directories:  []string{},
		},
		Statistics: &ValidationStats{},
	}

	// Group files by directory and collect statistics
	dirMap := make(map[string]bool)
	var totalSize int64

	for _, file := range files {
		result.Statistics.TotalFiles++

		// Skip log files (.log, .tmp) as they shouldn't be migrated
		if containsAny(file.Path, []string{"*.log", "*.tmp"}) {
			result.Inventory.SkippedFiles = append(result.Inventory.SkippedFiles, SkippedFile{
				Path:   file.Path,
				Reason: "Log/temp files are not migrated",
			})
			result.Statistics.SkippedFiles++
			continue
		}

		// Validate file path and existence
		if err := v.pathManager.ValidatePath(file.Path); err != nil {
			result.Valid = false
			result.Statistics.InvalidFiles++

			errorType := "other"
			reason := err.Error()
			if os.IsNotExist(err) {
				errorType = "not_found"
				reason = fmt.Sprintf("File does not exist: %s", file.Path)
			} else if os.IsPermission(err) {
				errorType = "permission"
				reason = fmt.Sprintf("Permission denied: %s", file.Path)
			}

			result.Errors = append(result.Errors, reason)
			result.Inventory.InvalidFiles = append(result.Inventory.InvalidFiles, InvalidFile{
				Path:      file.Path,
				Reason:    reason,
				ErrorType: errorType,
			})

			// Log the validation failure
			v.logger.Warning("agent", "migration", "migration_validator",
				reason,
				map[string]interface{}{
					"file_path":  file.Path,
					"error_type": errorType,
					"file_size":  file.Size,
				})
			continue
		}

		// Track directory
		dir := filepath.Dir(file.Path)
		if !dirMap[dir] {
			dirMap[dir] = true
			result.Inventory.Directories = append(result.Inventory.Directories, dir)
		}

		result.Inventory.ValidFiles = append(result.Inventory.ValidFiles, file)
		result.Statistics.ValidFiles++
		totalSize += file.Size
	}

	result.Statistics.TotalSizeBytes = totalSize

	// Check for required files
	for _, pattern := range requiredPatterns {
		found := false
		for _, file := range result.Inventory.ValidFiles {
			if matched, _ := filepath.Match(pattern, filepath.Base(file.Path)); matched {
				found = true
				break
			}
		}
		if !found {
			result.Valid = false
			missing := fmt.Sprintf("Required file pattern not found: %s", pattern)
			result.Errors = append(result.Errors, missing)
			result.Inventory.MissingFiles = append(result.Inventory.MissingFiles, pattern)
			result.Statistics.MissingFiles++

			// Log missing required file
			v.logger.Error("agent", "migration", "migration_validator",
				missing,
				map[string]interface{}{
					"required_pattern": pattern,
					"phase":            "validation",
				})
		}
	}

	result.Statistics.ValidationTime = time.Since(start).Milliseconds()

	// Log validation completion
	v.logger.Info("agent", "migration", "migration_validator",
		fmt.Sprintf("File validation completed: %d total, %d valid, %d invalid, %d skipped",
			result.Statistics.TotalFiles,
			result.Statistics.ValidFiles,
			result.Statistics.InvalidFiles,
			result.Statistics.SkippedFiles),
		map[string]interface{}{
			"total_files":  result.Statistics.TotalFiles,
			"valid_files":  result.Statistics.ValidFiles,
			"invalid_files": result.Statistics.InvalidFiles,
			"skipped_files": result.Statistics.SkippedFiles,
			"valid":        result.Valid,
		})

	return result, nil
}

// ValidateBackupLocation validates backup location is writable and safe
func (v *FileValidator) ValidateBackupLocation(backupPath string) error {
	// Normalize path
	normalized, err := v.pathManager.NormalizeToAbsolute(backupPath)
	if err != nil {
		return fmt.Errorf("invalid backup path: %w", err)
	}

	// Ensure backup path isn't in system directories
	if strings.HasPrefix(normalized, "/bin/") || strings.HasPrefix(normalized, "/sbin/") ||
		strings.HasPrefix(normalized, "/usr/bin/") || strings.HasPrefix(normalized, "/usr/sbin/") {
		return fmt.Errorf("backup path cannot be in system directory: %s", normalized)
	}

	// Ensure parent directory exists and is writable
	parent := filepath.Dir(normalized)
	if err := v.pathManager.EnsureDirectory(parent); err != nil {
		return fmt.Errorf("cannot create backup directory: %w", err)
	}

	// Test write permission (create a temp file)
	testFile := filepath.Join(parent, ".migration_test_"+uuid.Must(uuid.NewV4()).String()[:8])
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return fmt.Errorf("backup directory not writable: %w", err)
	}

	// Clean up test file
	_ = os.Remove(testFile)

	return nil
}

// PreValidate validates all conditions before migration starts
func (v *FileValidator) PreValidate(detection *migration.MigrationDetection, backupPath string) (*ValidationResult, error) {
	v.logger.Info("agent", "migration", "migration_validator",
		"Starting comprehensive migration validation",
		map[string]interface{}{
			"agent_version":  detection.CurrentAgentVersion,
			"config_version": detection.CurrentConfigVersion,
		})

	// Collect all files from inventory
	allFiles := v.collectAllFiles(detection.Inventory)

	// Define required patterns based on migration needs
	requiredPatterns := []string{
		"config.json", // Config is essential
		// Note: agent.key files are generated if missing
	}

	// Validate inventory
	result, err := v.ValidateInventory(allFiles, requiredPatterns)
	if err != nil {
		v.logger.Error("agent", "migration", "migration_validator",
			fmt.Sprintf("Validation failed: %v", err),
			map[string]interface{}{
				"error": err.Error(),
				"phase": "pre_validation",
			})
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Validate backup location
	if err := v.ValidateBackupLocation(backupPath); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Backup location invalid: %v", err))

		v.logger.Error("agent", "migration", "migration_validator",
			fmt.Sprintf("Backup validation failed: %v", err),
			map[string]interface{}{
				"backup_path": backupPath,
				"error":       err.Error(),
				"phase":       "validation",
			})
	}

	// Validate new directories can be created (but don't create them yet)
	newDirs := []string{
		v.pathManager.GetConfig().NewConfigPath,
		v.pathManager.GetConfig().NewStatePath,
	}
	for _, dir := range newDirs {
		normalized, err := v.pathManager.NormalizeToAbsolute(dir)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Invalid new directory %s: %v", dir, err))
			continue
		}

		// Check if parent is writable
		parent := filepath.Dir(normalized)
		if _, err := os.Stat(parent); err != nil {
			if os.IsNotExist(err) {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Parent directory for %s does not exist: %s", dir, parent))
			}
		}
	}

	// Log final validation status
	v.logger.Info("agent", "migration", "migration_validator",
		fmt.Sprintf("Pre-validation completed: %s", func() string {
			if result.Valid {
				return "PASSED"
			}
			return "FAILED"
		}()),
		map[string]interface{}{
			"errors_count":   len(result.Errors),
			"warnings_count": len(result.Warnings),
			"files_valid":    result.Statistics.ValidFiles,
			"files_invalid":  result.Statistics.InvalidFiles,
			"files_skipped":  result.Statistics.SkippedFiles,
		})

	return result, nil
}

// collectAllFiles collects all files from the migration inventory
func (v *FileValidator) collectAllFiles(inventory *migration.AgentFileInventory) []common.AgentFile {
	var allFiles []common.AgentFile
	if inventory != nil {
		allFiles = append(allFiles, inventory.ConfigFiles...)
		allFiles = append(allFiles, inventory.StateFiles...)
		allFiles = append(allFiles, inventory.BinaryFiles...)
		allFiles = append(allFiles, inventory.LogFiles...)
		allFiles = append(allFiles, inventory.CertificateFiles...)
	}
	return allFiles
}

// containsAny checks if path matches any of the patterns
func containsAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}

// ValidateFileForBackup validates a single file before backup
func (v *FileValidator) ValidateFileForBackup(file common.AgentFile) error {
	// Check if file exists
	if _, err := os.Stat(file.Path); err != nil {
		if os.IsNotExist(err) {
			v.logger.Warning("agent", "migration", "migration_validator",
				fmt.Sprintf("Skipping backup of non-existent file: %s", file.Path),
				map[string]interface{}{
					"file_path": file.Path,
					"phase":     "backup",
				})
			return fmt.Errorf("file does not exist: %s", file.Path)
		}
		return fmt.Errorf("failed to access file %s: %w", file.Path, err)
	}

	// Additional validation for sensitive files
	if strings.Contains(file.Path, ".key") || strings.Contains(file.Path, "config") {
		// Key files should be readable only by owner
		info, err := os.Stat(file.Path)
		if err == nil {
			perm := info.Mode().Perm()
			// Check if others have read permission
			if perm&0004 != 0 {
				v.logger.Warning("agent", "migration", "migration_validator",
					fmt.Sprintf("Sensitive file has world-readable permissions: %s (0%o)", file.Path, perm),
					map[string]interface{}{
						"file_path":   file.Path,
						"permissions": perm,
					})
			}
		}
	}

	return nil
}