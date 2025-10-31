//go:build !windows

package service

import (
	"fmt"
	"runtime"
	"github.com/Fimeg/RedFlag/aggregator-agent/internal/config"
)

// Stub implementations for non-Windows platforms

// RunService executes the agent as a Windows service (stub for non-Windows)
func RunService(cfg *config.Config) error {
	return fmt.Errorf("Windows service mode is only available on Windows, current OS: %s", runtime.GOOS)
}

// IsService returns true if running as Windows service (stub for non-Windows)
func IsService() bool {
	return false
}

// InstallService installs the agent as a Windows service (stub for non-Windows)
func InstallService() error {
	return fmt.Errorf("Windows service installation is only available on Windows, current OS: %s", runtime.GOOS)
}

// RemoveService removes the Windows service (stub for non-Windows)
func RemoveService() error {
	return fmt.Errorf("Windows service removal is only available on Windows, current OS: %s", runtime.GOOS)
}

// StartService starts the Windows service (stub for non-Windows)
func StartService() error {
	return fmt.Errorf("Windows service management is only available on Windows, current OS: %s", runtime.GOOS)
}

// StopService stops the Windows service (stub for non-Windows)
func StopService() error {
	return fmt.Errorf("Windows service management is only available on Windows, current OS: %s", runtime.GOOS)
}

// ServiceStatus returns the current status of the Windows service (stub for non-Windows)
func ServiceStatus() error {
	return fmt.Errorf("Windows service management is only available on Windows, current OS: %s", runtime.GOOS)
}

// RunConsole runs the agent in console mode with signal handling
func RunConsole(cfg *config.Config) error {
	// For non-Windows, just run normally
	return fmt.Errorf("Console mode is handled by main application logic on %s", runtime.GOOS)
}