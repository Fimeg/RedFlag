// Package constants provides centralized path definitions for the RedFlag agent.
// This package ensures consistency across all components and makes path management
// maintainable and testable.
package constants

import (
	"path/filepath"
	"runtime"
)

// Base directories
const (
	LinuxBaseDir   = "/var/lib/redflag"
	WindowsBaseDir = "C:\\ProgramData\\RedFlag"
)

// Subdirectory structure
const (
	AgentDir        = "agent"
	ServerDir       = "server"
	CacheSubdir     = "cache"
	StateSubdir     = "state"
	MigrationSubdir = "migration_backups"
)

// Config paths
const (
	LinuxConfigBase   = "/etc/redflag"
	WindowsConfigBase = "C:\\ProgramData\\RedFlag"
	ConfigFile        = "config.json"
)

// Log paths
const (
	LinuxLogBase = "/var/log/redflag"
	LogSubdir    = "logs"
	AgentLogFile = "agent.log"
)

// Legacy paths for migration
const (
	LegacyConfigPath = "/etc/aggregator/config.json"
	LegacyStatePath  = "/var/lib/aggregator"
)

// GetBaseDir returns platform-specific base directory
func GetBaseDir() string {
	if runtime.GOOS == "windows" {
		return WindowsBaseDir
	}
	return LinuxBaseDir
}

// GetAgentStateDir returns /var/lib/redflag/agent/state
func GetAgentStateDir() string {
	return filepath.Join(GetBaseDir(), AgentDir, StateSubdir)
}

// GetAgentCacheDir returns /var/lib/redflag/agent/cache
func GetAgentCacheDir() string {
	return filepath.Join(GetBaseDir(), AgentDir, CacheSubdir)
}

// GetMigrationBackupDir returns /var/lib/redflag/agent/migration_backups
func GetMigrationBackupDir() string {
	return filepath.Join(GetBaseDir(), AgentDir, MigrationSubdir)
}

// GetAgentConfigPath returns /etc/redflag/agent/config.json
func GetAgentConfigPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(WindowsConfigBase, AgentDir, ConfigFile)
	}
	return filepath.Join(LinuxConfigBase, AgentDir, ConfigFile)
}

// GetAgentConfigDir returns /etc/redflag/agent
func GetAgentConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(WindowsConfigBase, AgentDir)
	}
	return filepath.Join(LinuxConfigBase, AgentDir)
}

// GetServerPublicKeyPath returns /etc/redflag/server/server_public_key
func GetServerPublicKeyPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(WindowsConfigBase, ServerDir, "server_public_key")
	}
	return filepath.Join(LinuxConfigBase, ServerDir, "server_public_key")
}

// GetServerPublicKeyDir returns /etc/redflag/server (directory for server public keys)
func GetServerPublicKeyDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(WindowsConfigBase, ServerDir)
	}
	return filepath.Join(LinuxConfigBase, ServerDir)
}

// GetAgentLogDir returns the platform-specific agent log directory.
func GetAgentLogDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(WindowsBaseDir, LogSubdir)
	}
	return filepath.Join(LinuxLogBase, AgentDir)
}

// GetAgentLogPath returns the platform-specific primary agent log file.
func GetAgentLogPath() string {
	return filepath.Join(GetAgentLogDir(), AgentLogFile)
}

// GetLegacyAgentConfigPath returns legacy /etc/aggregator/config.json
func GetLegacyAgentConfigPath() string {
	return LegacyConfigPath
}

// GetLegacyAgentStatePath returns legacy /var/lib/aggregator
func GetLegacyAgentStatePath() string {
	return LegacyStatePath
}

// Staging directory for self-update binaries (pending-upgrade.bin etc.).
// Mirrors /var/lib/redflag/agent on Linux, C:\ProgramData\RedFlag\agent on Windows.
func GetAgentStagingDir() string {
	return filepath.Join(GetBaseDir(), AgentDir)
}

// GetAgentStagingPath returns the staging path for a self-update binary.
// suffix is e.g. "pending-upgrade.bin", "pending-helper.bin", "pending-desktop.bin".
func GetAgentStagingPath(suffix string) string {
	return filepath.Join(GetBaseDir(), AgentDir, suffix)
}

// Binary install paths — /usr/local/bin / C:\Program Files\RedFlag.
func GetBinaryInstallDir() string {
	if runtime.GOOS == "windows" {
		return `C:\Program Files\RedFlag`
	}
	return "/usr/local/bin"
}

func GetAgentBinaryPath() string {
	name := "redflag-agent"
	if runtime.GOOS == "windows" {
		name = "redflag-agent.exe"
	}
	return filepath.Join(GetBinaryInstallDir(), name)
}

func GetHelperBinaryPath() string {
	name := "redflag-helper"
	if runtime.GOOS == "windows" {
		name = "redflag-helper.exe"
	}
	return filepath.Join(GetBinaryInstallDir(), name)
}

func GetDesktopBinaryPath() string {
	name := "redflag-desktop"
	if runtime.GOOS == "windows" {
		name = "redflag-desktop.exe"
	}
	return filepath.Join(GetBinaryInstallDir(), name)
}
