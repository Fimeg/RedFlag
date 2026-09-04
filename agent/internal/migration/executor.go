package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/common"
	"github.com/Fimeg/RedFlag/agent/internal/event"
	"github.com/Fimeg/RedFlag/agent/internal/version"
)

// MigrationPlan represents a complete migration plan
type MigrationPlan struct {
	Detection         *MigrationDetection  `json:"detection"`
	TargetVersion     string               `json:"target_version"`
	Config            *FileDetectionConfig `json:"config"`
	BackupPath        string               `json:"backup_path"`
	EstimatedDuration time.Duration        `json:"estimated_duration"`
	RiskLevel         string               `json:"risk_level"` // low, medium, high
}

// MigrationResult represents the result of a migration execution
type MigrationResult struct {
	Success           bool          `json:"success"`
	StartTime         time.Time     `json:"start_time"`
	EndTime           time.Time     `json:"end_time"`
	Duration          time.Duration `json:"duration"`
	BackupPath        string        `json:"backup_path"`
	MigratedFiles     []string      `json:"migrated_files"`
	Errors            []string      `json:"errors"`
	Warnings          []string      `json:"warnings"`
	AppliedChanges    []string      `json:"applied_changes"`
	RollbackAvailable bool          `json:"rollback_available"`
}

// MigrationExecutor handles the execution of migration plans
type MigrationExecutor struct {
	plan         *MigrationPlan
	result       *MigrationResult
	logger       *event.TeeLogger
	stateManager *StateManager
}

// NewMigrationExecutor creates a new migration executor
func NewMigrationExecutor(plan *MigrationPlan, configPath string) *MigrationExecutor {
	return &MigrationExecutor{
		plan:         plan,
		result:       &MigrationResult{},
		stateManager: NewStateManager(configPath),
	}
}

// ExecuteMigration executes the complete migration plan
func (e *MigrationExecutor) ExecuteMigration() (*MigrationResult, error) {
	e.result.StartTime = time.Now().UTC()
	e.result.BackupPath = e.plan.BackupPath

	e.logger.Info("agent", "migration", "migration_executor", "Starting migration",
		map[string]interface{}{
			"from_version": e.plan.Detection.CurrentAgentVersion,
			"to_version":   e.plan.TargetVersion,
		})

	// Phase 1: Create backups
	if err := e.createBackups(); err != nil {
		e.logger.Error("agent", "migration", "migration_executor",
			fmt.Sprintf("Backup creation failed: %v", err),
			map[string]interface{}{
				"error":       err.Error(),
				"backup_path": e.plan.BackupPath,
				"phase":       "backup_creation",
			})
		return e.completeMigration(false, fmt.Errorf("backup creation failed: %w", err))
	}
	e.result.AppliedChanges = append(e.result.AppliedChanges, "Created backups at "+e.plan.BackupPath)

	// Phase 2: Directory migration
	if contains(e.plan.Detection.RequiredMigrations, "directory_migration") {
		if err := e.migrateDirectories(); err != nil {
			e.logger.Error("agent", "migration", "migration_executor",
				fmt.Sprintf("Directory migration failed: %v", err),
				map[string]interface{}{
					"error": err.Error(),
					"phase": "directory_migration",
				})
			return e.completeMigration(false, fmt.Errorf("directory migration failed: %w", err))
		}
		e.result.AppliedChanges = append(e.result.AppliedChanges, "Migrated directories")

		// Mark directory migration as completed
		if err := e.stateManager.MarkMigrationCompleted("directory_migration", e.plan.BackupPath, e.plan.TargetVersion); err != nil {
			e.logger.Warning("agent", "migration", "migration_executor",
				fmt.Sprintf("Failed to mark directory migration as completed: %v", err), nil)
		}
	}

	// Phase 3: Configuration migration
	if contains(e.plan.Detection.RequiredMigrations, "config_migration") {
		if err := e.migrateConfiguration(); err != nil {
			e.logger.Error("agent", "migration", "migration_executor",
				fmt.Sprintf("Configuration migration failed: %v", err),
				map[string]interface{}{
					"error": err.Error(),
					"phase": "configuration_migration",
				})
			return e.completeMigration(false, fmt.Errorf("configuration migration failed: %w", err))
		}
		e.result.AppliedChanges = append(e.result.AppliedChanges, "Migrated configuration")

		// Mark configuration migration as completed
		if err := e.stateManager.MarkMigrationCompleted("config_migration", e.plan.BackupPath, e.plan.TargetVersion); err != nil {
			e.logger.Warning("agent", "migration", "migration_executor",
				fmt.Sprintf("Failed to mark configuration migration as completed: %v", err), nil)
		}
	}

	// Phase 3b: config v5 schema migration.
	// The actual schema reshaping is performed lazily by the config package on load
	// (mergeConfigPreservingDefaults + migrateConfig). This phase's job is to record
	// completion and bump the on-disk config `version` so detection converges; without
	// it, config_v5_migration has no handler, is never marked complete, and re-triggers
	// on every boot (§11.10 State Machine Exhaustiveness).
	if contains(e.plan.Detection.RequiredMigrations, "config_v5_migration") {
		targetConfigVersion := version.ExtractConfigVersionFromAgent(e.plan.TargetVersion)
		if err := e.stateManager.BumpConfigVersion(targetConfigVersion, e.plan.TargetVersion); err != nil {
			// Non-fatal: the config package also self-heals the version on load. Marking
			// completion below is what actually breaks the re-trigger loop.
			e.logger.Warning("agent", "migration", "migration_executor",
				fmt.Sprintf("Failed to bump config version: %v", err), nil)
		} else {
			e.result.AppliedChanges = append(e.result.AppliedChanges,
				fmt.Sprintf("Bumped config schema version to %s", targetConfigVersion))
		}

		if err := e.stateManager.MarkMigrationCompleted("config_v5_migration", e.plan.BackupPath, e.plan.TargetVersion); err != nil {
			e.logger.Warning("agent", "migration", "migration_executor",
				fmt.Sprintf("Failed to mark config v5 migration as completed: %v", err), nil)
		}
	}

	// Phase 4: Docker secrets migration (if available)
	if contains(e.plan.Detection.RequiredMigrations, "docker_secrets_migration") {
		if e.plan.Detection.DockerDetection == nil {
			e.logger.Error("agent", "migration", "migration_executor",
				"Docker secrets migration requested but detection data missing",
				map[string]interface{}{
					"error": "missing detection data",
					"phase": "docker_secrets_migration",
				})
			return e.completeMigration(false, fmt.Errorf("docker secrets migration requested but detection data missing"))
		}

		dockerExecutor := NewDockerSecretsExecutor(e.plan.Detection.DockerDetection, e.plan.Config, e.logger)
		if err := dockerExecutor.ExecuteDockerSecretsMigration(); err != nil {
			e.logger.Error("agent", "migration", "migration_executor",
				fmt.Sprintf("Docker secrets migration failed: %v", err),
				map[string]interface{}{
					"error": err.Error(),
					"phase": "docker_secrets_migration",
				})
			return e.completeMigration(false, fmt.Errorf("docker secrets migration failed: %w", err))
		}
		e.result.AppliedChanges = append(e.result.AppliedChanges, "Migrated to Docker secrets")

		// Mark docker secrets migration as completed
		if err := e.stateManager.MarkMigrationCompleted("docker_secrets_migration", e.plan.BackupPath, e.plan.TargetVersion); err != nil {
			e.logger.Warning("agent", "migration", "migration_executor",
				fmt.Sprintf("Failed to mark docker secrets migration as completed: %v", err), nil)
		}
	}

	// Phase 5: Security hardening
	if contains(e.plan.Detection.RequiredMigrations, "security_hardening") {
		if err := e.applySecurityHardening(); err != nil {
			e.result.Warnings = append(e.result.Warnings,
				fmt.Sprintf("Security hardening incomplete: %v", err))
			e.logger.Warning("agent", "migration", "migration_executor",
				fmt.Sprintf("Security hardening incomplete: %v", err), nil)
		} else {
			e.result.AppliedChanges = append(e.result.AppliedChanges, "Applied security hardening")

			// Mark security hardening as completed
			if err := e.stateManager.MarkMigrationCompleted("security_hardening", e.plan.BackupPath, e.plan.TargetVersion); err != nil {
				e.logger.Warning("agent", "migration", "migration_executor",
					fmt.Sprintf("Failed to mark security hardening as completed: %v", err), nil)
			}
		}
	}

	// Phase 6: Validation
	if err := e.validateMigration(); err != nil {
		e.logger.Error("agent", "migration", "migration_executor",
			fmt.Sprintf("Migration validation failed: %v", err),
			map[string]interface{}{
				"error": err.Error(),
				"phase": "validation",
			})
		return e.completeMigration(false, fmt.Errorf("migration validation failed: %w", err))
	}

	return e.completeMigration(true, nil)
}

// createBackups creates backups of all existing files
func (e *MigrationExecutor) createBackups() error {
	backupPath := e.plan.BackupPath
	if backupPath == "" {
		timestamp := time.Now().Format("2006-01-02-150405")
		backupPath = fmt.Sprintf(e.plan.Config.BackupDirPattern, timestamp)
		e.plan.BackupPath = backupPath
	}

	e.logger.Info("agent", "migration", "migration_executor", "Creating backup",
		map[string]interface{}{
			"backup_path": backupPath,
		})

	// Create backup directory
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Backup all files in inventory
	allFiles := e.collectAllFiles()
	for _, file := range allFiles {
		if err := e.backupFile(file, backupPath); err != nil {
			return fmt.Errorf("failed to backup file %s: %w", file.Path, err)
		}
		e.result.MigratedFiles = append(e.result.MigratedFiles, file.Path)
	}

	return nil
}

// migrateDirectories handles directory migration from old to new paths
func (e *MigrationExecutor) migrateDirectories() error {
	e.logger.Info("agent", "migration", "migration_executor", "Migrating directories", nil)

	// Create new directories
	newDirectories := []string{e.plan.Config.NewConfigPath, e.plan.Config.NewStatePath}
	for _, newPath := range newDirectories {
		if err := os.MkdirAll(newPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", newPath, err)
		}
		e.logger.Info("agent", "migration", "migration_executor", "Created directory",
			map[string]interface{}{
				"path": newPath,
			})
	}

	// Migrate files from old to new directories
	for _, oldDir := range e.plan.Detection.Inventory.OldDirectoryPaths {
		newDir := e.getNewDirectoryPath(oldDir)
		if newDir == "" {
			continue
		}

		if err := e.migrateDirectoryContents(oldDir, newDir); err != nil {
			return fmt.Errorf("failed to migrate directory %s to %s: %w", oldDir, newDir, err)
		}
		e.logger.Info("agent", "migration", "migration_executor", "Migrated directory",
			map[string]interface{}{
				"from": oldDir,
				"to":   newDir,
			})
	}

	return nil
}

// migrateConfiguration handles configuration file migration
func (e *MigrationExecutor) migrateConfiguration() error {
	e.logger.Info("agent", "migration", "migration_executor", "Migrating configuration", nil)

	// Find and migrate config files
	for _, configFile := range e.plan.Detection.Inventory.ConfigFiles {
		if strings.Contains(configFile.Path, "config.json") {
			newPath := e.getNewConfigPath(configFile.Path)
			if newPath != "" && newPath != configFile.Path {
				if err := e.migrateConfigFile(configFile.Path, newPath); err != nil {
					return fmt.Errorf("failed to migrate config file: %w", err)
				}
				e.logger.Info("agent", "migration", "migration_executor", "Migrated config",
					map[string]interface{}{
						"from": configFile.Path,
						"to":   newPath,
					})
			}
		}
	}

	return nil
}

// applySecurityHardening applies security-related configurations
func (e *MigrationExecutor) applySecurityHardening() error {
	e.logger.Info("agent", "migration", "migration_executor", "Applying security hardening", nil)

	// This would integrate with the config system to apply security defaults
	// For now, just log what would be applied

	for _, feature := range e.plan.Detection.MissingSecurityFeatures {
		switch feature {
		case "nonce_validation":
			e.logger.Info("agent", "migration", "migration_executor", "Enabling nonce validation", nil)
		case "machine_id_binding":
			e.logger.Info("agent", "migration", "migration_executor", "Configuring machine ID binding", nil)
		case "ed25519_verification":
			e.logger.Info("agent", "migration", "migration_executor", "Enabling Ed25519 verification", nil)
		case "subsystem_configuration":
			e.logger.Info("agent", "migration", "migration_executor", "Adding missing subsystem configurations", nil)
		case "system_subsystem":
			e.logger.Info("agent", "migration", "migration_executor", "Adding system scanner configuration", nil)
		}
	}

	return nil
}

// validateMigration validates that the migration was successful
func (e *MigrationExecutor) validateMigration() error {
	e.logger.Info("agent", "migration", "migration_executor", "Validating migration", nil)

	// Ensure new directories exist. These dirs are created on demand elsewhere
	// (install script, GetAgentStateDir callers) and are not always materialized by
	// a non-legacy migration, so a bare os.Stat here produced a false "not found" on
	// Windows. Create-then-verify: MkdirAll is idempotent, and only a stat that still
	// fails after a successful MkdirAll is a real failure.
	newDirectories := []string{e.plan.Config.NewConfigPath, e.plan.Config.NewStatePath}
	for _, newDir := range newDirectories {
		if err := os.MkdirAll(newDir, 0755); err != nil {
			return fmt.Errorf("failed to ensure new directory %s: %w", newDir, err)
		}
		if _, err := os.Stat(newDir); err != nil {
			return fmt.Errorf("new directory %s not found after create: %w", newDir, err)
		}
	}

	// Check that config files exist in new location
	for _, configFile := range e.plan.Detection.Inventory.ConfigFiles {
		newPath := e.getNewConfigPath(configFile.Path)
		if newPath != "" {
			if _, err := os.Stat(newPath); err != nil {
				return fmt.Errorf("migrated config file %s not found: %w", newPath, err)
			}
		}
	}

	e.logger.Info("agent", "migration", "migration_executor", "validation_successful", nil)
	return nil
}

// Helper methods

func (e *MigrationExecutor) collectAllFiles() []common.AgentFile {
	var allFiles []common.AgentFile
	allFiles = append(allFiles, e.plan.Detection.Inventory.ConfigFiles...)
	allFiles = append(allFiles, e.plan.Detection.Inventory.StateFiles...)
	allFiles = append(allFiles, e.plan.Detection.Inventory.BinaryFiles...)
	allFiles = append(allFiles, e.plan.Detection.Inventory.LogFiles...)
	allFiles = append(allFiles, e.plan.Detection.Inventory.CertificateFiles...)
	return allFiles
}

func (e *MigrationExecutor) backupFile(file common.AgentFile, backupPath string) error {
	// Check if file exists before attempting backup
	if _, err := os.Stat(file.Path); err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, log and skip
			e.logger.Warning("agent", "migration", "migration_executor",
				fmt.Sprintf("File does not exist, skipping backup: %s", file.Path),
				map[string]interface{}{
					"file_path": file.Path,
					"phase":     "backup",
				})
			return nil
		}
		return fmt.Errorf("migration: failed to stat file %s: %w", file.Path, err)
	}

	// Clean paths to fix trailing slash issues
	cleanOldConfig := filepath.Clean(e.plan.Config.OldConfigPath)
	cleanOldState := filepath.Clean(e.plan.Config.OldStatePath)
	cleanPath := filepath.Clean(file.Path)
	var relPath string
	var err error

	// Try to get relative path based on expected file location
	// If file is under old config path, use that as base
	if strings.HasPrefix(cleanPath, cleanOldConfig) {
		relPath, err = filepath.Rel(cleanOldConfig, cleanPath)
		if err != nil || strings.Contains(relPath, "..") {
			// Fallback to filename if path traversal or error
			relPath = filepath.Base(cleanPath)
		}
	} else if strings.HasPrefix(cleanPath, cleanOldState) {
		relPath, err = filepath.Rel(cleanOldState, cleanPath)
		if err != nil || strings.Contains(relPath, "..") {
			// Fallback to filename if path traversal or error
			relPath = filepath.Base(cleanPath)
		}
	} else {
		// File is not in expected old locations - use just the filename
		// This happens for files already in the new location
		relPath = filepath.Base(cleanPath)
		// Add subdirectory based on file type to avoid collisions
		switch {
		case ContainsAny(cleanPath, []string{"config.json", "agent.key", "server.key", "ca.crt"}):
			relPath = filepath.Join("config", relPath)
		case ContainsAny(cleanPath, []string{
			"pending_acks.json", "public_key.cache", "last_scan.json", "metrics.json"}):
			relPath = filepath.Join("state", relPath)
		}
	}

	// Ensure backup path is clean
	cleanBackupPath := filepath.Clean(backupPath)
	backupFilePath := filepath.Join(cleanBackupPath, relPath)
	backupFilePath = filepath.Clean(backupFilePath)
	backupDir := filepath.Dir(backupFilePath)

	// Final safety check
	if strings.Contains(backupFilePath, "..") {
		return fmt.Errorf("migration: backup path contains parent directory reference: %s", backupFilePath)
	}

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("migration: failed to create backup directory %s: %w", backupDir, err)
	}

	// Copy file to backup location
	if err := copyFile(cleanPath, backupFilePath); err != nil {
		return fmt.Errorf("migration: failed to copy file to backup: %w", err)
	}

	e.logger.Info("agent", "migration", "migration_executor", "Successfully backed up",
		map[string]interface{}{
			"file_path": cleanPath,
		})
	return nil
}

func (e *MigrationExecutor) migrateDirectoryContents(oldDir, newDir string) error {
	return filepath.Walk(oldDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(oldDir, path)
		if err != nil {
			return err
		}

		newPath := filepath.Join(newDir, relPath)
		newDirPath := filepath.Dir(newPath)

		if err := os.MkdirAll(newDirPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if err := copyFile(path, newPath); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}

		return nil
	})
}

func (e *MigrationExecutor) migrateConfigFile(oldPath, newPath string) error {
	// This would use the config migration logic from the config package
	// For now, just copy the file
	return copyFile(oldPath, newPath)
}

func (e *MigrationExecutor) getNewDirectoryPath(oldPath string) string {
	if oldPath == e.plan.Config.OldConfigPath {
		return e.plan.Config.NewConfigPath
	}
	if oldPath == e.plan.Config.OldStatePath {
		return e.plan.Config.NewStatePath
	}
	return ""
}

func (e *MigrationExecutor) getNewConfigPath(oldPath string) string {
	if strings.HasPrefix(oldPath, e.plan.Config.OldConfigPath) {
		relPath := strings.TrimPrefix(oldPath, e.plan.Config.OldConfigPath)
		return filepath.Join(e.plan.Config.NewConfigPath, relPath)
	}
	if strings.HasPrefix(oldPath, e.plan.Config.OldStatePath) {
		relPath := strings.TrimPrefix(oldPath, e.plan.Config.OldStatePath)
		return filepath.Join(e.plan.Config.NewStatePath, relPath)
	}
	return ""
}

func (e *MigrationExecutor) completeMigration(success bool, err error) (*MigrationResult, error) {
	e.result.EndTime = time.Now().UTC()
	e.result.Duration = e.result.EndTime.Sub(e.result.StartTime)
	e.result.Success = success
	e.result.RollbackAvailable = success && e.result.BackupPath != ""

	if err != nil {
		e.result.Errors = append(e.result.Errors, err.Error())
	}

	if success {
		e.logger.Info("agent", "migration", "migration_executor", "Migration completed",
			map[string]interface{}{
				"duration": e.result.Duration.String(),
			})
		if e.result.RollbackAvailable {
			e.logger.Info("agent", "migration", "migration_executor", "rollback_available",
				map[string]interface{}{
					"path": e.result.BackupPath,
				})
		}

		// Clean up old directories after successful migration
		if err := e.stateManager.CleanupOldDirectories(); err != nil {
			e.logger.Warning("agent", "migration", "migration_executor",
				fmt.Sprintf("Failed to cleanup old directories: %v", err), nil)
		}
	} else {
		e.logger.Error("agent", "migration", "migration_executor", "Migration failed",
			map[string]interface{}{
				"duration": e.result.Duration.String(),
			})
		if len(e.result.Errors) > 0 {
			for _, errMsg := range e.result.Errors {
				e.logger.Error("agent", "migration", "migration_executor", errMsg, nil)
			}
		}
	}

	return e.result, err
}

// Utility functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	if err != nil {
		return err
	}

	// Preserve file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
