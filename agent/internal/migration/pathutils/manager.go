package pathutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathManager provides centralized path operations with validation
type PathManager struct {
	config *Config
}

// Config holds path configuration for migration
type Config struct {
	OldConfigPath    string
	OldStatePath     string
	NewConfigPath    string
	NewStatePath     string
	BackupDirPattern string
}

// NewPathManager creates a new path manager with cleaned configuration
func NewPathManager(config *Config) *PathManager {
	// Clean all paths to remove trailing slashes and normalize
	cleanConfig := &Config{
		OldConfigPath:    filepath.Clean(strings.TrimSpace(config.OldConfigPath)),
		OldStatePath:     filepath.Clean(strings.TrimSpace(config.OldStatePath)),
		NewConfigPath:    filepath.Clean(strings.TrimSpace(config.NewConfigPath)),
		NewStatePath:     filepath.Clean(strings.TrimSpace(config.NewStatePath)),
		BackupDirPattern: strings.TrimSpace(config.BackupDirPattern),
	}
	return &PathManager{config: cleanConfig}
}

// NormalizeToAbsolute ensures a path is absolute and cleaned
func (pm *PathManager) NormalizeToAbsolute(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Clean and make absolute
	cleaned := filepath.Clean(path)

	// Check for path traversal attempts
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path contains parent directory reference: %s", path)
	}

	// Ensure it's absolute
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("path must be absolute: %s", path)
	}

	return cleaned, nil
}

// ValidatePath validates a single path exists
func (pm *PathManager) ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Normalize path first
	normalized, err := pm.NormalizeToAbsolute(path)
	if err != nil {
		return err
	}

	// Check existence
	info, err := os.Stat(normalized)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", normalized)
		}
		return fmt.Errorf("failed to access path %s: %w", normalized, err)
	}

	// Additional validation for security
	if filepath.IsAbs(normalized) && strings.HasPrefix(normalized, "/etc/") {
		// Config files should be owned by root or agent user (checking basic permissions)
		if info.Mode().Perm()&0004 == 0 && info.Mode().Perm()&0002 == 0 {
			return fmt.Errorf("config file is not readable: %s", normalized)
		}
	}

	return nil
}

// EnsureDirectory creates directory if it doesn't exist
func (pm *PathManager) EnsureDirectory(path string) error {
	normalized, err := pm.NormalizeToAbsolute(path)
	if err != nil {
		return err
	}

	// Check if it exists and is a directory
	if info, err := os.Stat(normalized); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", normalized)
		}
		return nil
	}

	// Create directory with proper permissions
	if err := os.MkdirAll(normalized, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", normalized, err)
	}

	return nil
}

// GetRelativePath gets relative path from base directory
// Returns error if path would traverse outside base
func (pm *PathManager) GetRelativePath(basePath, fullPath string) (string, error) {
	normBase, err := pm.NormalizeToAbsolute(basePath)
	if err != nil {
		return "", fmt.Errorf("invalid base path: %w", err)
	}

	normFull, err := pm.NormalizeToAbsolute(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid full path: %w", err)
	}

	// Check if full path is actually under base path
	if !strings.HasPrefix(normFull, normBase) {
		// Not under base path, use filename-only approach
		return filepath.Base(normFull), nil
	}

	rel, err := filepath.Rel(normBase, normFull)
	if err != nil {
		return "", fmt.Errorf("failed to get relative path from %s to %s: %w", normBase, normFull, err)
	}

	// Final safety check
	if strings.Contains(rel, "..") {
		return filepath.Base(normFull), nil
	}

	return rel, nil
}

// JoinPath joins path components safely
func (pm *PathManager) JoinPath(base string, components ...string) string {
	// Ensure base is absolute and cleaned
	if absBase, err := pm.NormalizeToAbsolute(base); err == nil {
		base = absBase
	}

	// Clean all components
	cleanComponents := make([]string, len(components))
	for i, comp := range components {
		cleanComponents[i] = filepath.Clean(comp)
	}

	// Join all components
	result := filepath.Join(append([]string{base}, cleanComponents...)...)

	// Final safety check
	if strings.Contains(result, "..") {
		// Fallback to string-based join if path traversal detected
		return filepath.Join(base, filepath.Join(cleanComponents...))
	}

	return result
}

// GetConfig returns the path configuration
func (pm *PathManager) GetConfig() *Config {
	return pm.config
}

// ValidateConfig validates all configured paths
func (pm *PathManager) ValidateConfig() error {
	if pm.config.OldConfigPath == "" || pm.config.OldStatePath == "" {
		return fmt.Errorf("old paths cannot be empty")
	}

	if pm.config.NewConfigPath == "" || pm.config.NewStatePath == "" {
		return fmt.Errorf("new paths cannot be empty")
	}

	if pm.config.BackupDirPattern == "" {
		return fmt.Errorf("backup dir pattern cannot be empty")
	}

	// Validate paths are absolute
	paths := []string{
		pm.config.OldConfigPath,
		pm.config.OldStatePath,
		pm.config.NewConfigPath,
		pm.config.NewStatePath,
	}

	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("path must be absolute: %s", path)
		}
	}

	return nil
}

// GetNewPathForOldPath determines the new path for a file that was in an old location
func (pm *PathManager) GetNewPathForOldPath(oldPath string) (string, error) {
	// Validate old path
	normalizedOld, err := pm.NormalizeToAbsolute(oldPath)
	if err != nil {
		return "", fmt.Errorf("invalid old path: %w", err)
	}

	// Check if it's in old config path
	if strings.HasPrefix(normalizedOld, pm.config.OldConfigPath) {
		relPath, err := pm.GetRelativePath(pm.config.OldConfigPath, normalizedOld)
		if err != nil {
			return "", err
		}
		return pm.JoinPath(pm.config.NewConfigPath, relPath), nil
	}

	// Check if it's in old state path
	if strings.HasPrefix(normalizedOld, pm.config.OldStatePath) {
		relPath, err := pm.GetRelativePath(pm.config.OldStatePath, normalizedOld)
		if err != nil {
			return "", err
		}
		return pm.JoinPath(pm.config.NewStatePath, relPath), nil
	}

	// File is not in expected old locations, return as is
	return normalizedOld, nil
}