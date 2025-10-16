package installer

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/aggregator-project/aggregator-agent/internal/client"
)

// APTInstaller handles APT package installations
type APTInstaller struct{}

// NewAPTInstaller creates a new APT installer
func NewAPTInstaller() *APTInstaller {
	return &APTInstaller{}
}

// IsAvailable checks if APT is available on this system
func (i *APTInstaller) IsAvailable() bool {
	_, err := exec.LookPath("apt-get")
	return err == nil
}

// Install installs packages using APT
func (i *APTInstaller) Install(packageName string) (*InstallResult, error) {
	startTime := time.Now()

	// Update package cache first
	updateCmd := exec.Command("sudo", "apt-get", "update")
	if output, err := updateCmd.CombinedOutput(); err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to update APT cache: %v\nStdout: %s", err, string(output)),
			DurationSeconds: int(time.Since(startTime).Seconds()),
		}, fmt.Errorf("apt-get update failed: %w", err)
	}

	// Install package
	installCmd := exec.Command("sudo", "apt-get", "install", "-y", packageName)
	output, err := installCmd.CombinedOutput()
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("APT install failed: %v", err),
			Stdout:         string(output),
			Stderr:         "",
			ExitCode:       getExitCode(err),
			DurationSeconds: duration,
		}, err
	}

	return &InstallResult{
		Success:        true,
		Stdout:         string(output),
		Stderr:         "",
		ExitCode:       0,
		DurationSeconds: duration,
		PackagesInstalled: []string{packageName},
	}, nil
}

// InstallMultiple installs multiple packages using APT
func (i *APTInstaller) InstallMultiple(packageNames []string) (*InstallResult, error) {
	if len(packageNames) == 0 {
		return &InstallResult{
			Success:      false,
			ErrorMessage: "No packages specified for installation",
		}, fmt.Errorf("no packages specified")
	}

	startTime := time.Now()

	// Update package cache first
	updateCmd := exec.Command("sudo", "apt-get", "update")
	if output, err := updateCmd.CombinedOutput(); err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to update APT cache: %v\nStdout: %s", err, string(output)),
			DurationSeconds: int(time.Since(startTime).Seconds()),
		}, fmt.Errorf("apt-get update failed: %w", err)
	}

	// Install all packages in one command
	args := []string{"install", "-y"}
	args = append(args, packageNames...)
	installCmd := exec.Command("sudo", "apt-get", args...)
	output, err := installCmd.CombinedOutput()
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("APT install failed: %v", err),
			Stdout:         string(output),
			Stderr:         "",
			ExitCode:       getExitCode(err),
			DurationSeconds: duration,
		}, err
	}

	return &InstallResult{
		Success:        true,
		Stdout:         string(output),
		Stderr:         "",
		ExitCode:       0,
		DurationSeconds: duration,
		PackagesInstalled: packageNames,
	}, nil
}

// Upgrade upgrades all packages using APT
func (i *APTInstaller) Upgrade() (*InstallResult, error) {
	startTime := time.Now()

	// Update package cache first
	updateCmd := exec.Command("sudo", "apt-get", "update")
	if output, err := updateCmd.CombinedOutput(); err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to update APT cache: %v\nStdout: %s", err, string(output)),
			DurationSeconds: int(time.Since(startTime).Seconds()),
		}, fmt.Errorf("apt-get update failed: %w", err)
	}

	// Upgrade all packages
	upgradeCmd := exec.Command("sudo", "apt-get", "upgrade", "-y")
	output, err := upgradeCmd.CombinedOutput()
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("APT upgrade failed: %v", err),
			Stdout:         string(output),
			Stderr:         "",
			ExitCode:       getExitCode(err),
			DurationSeconds: duration,
		}, err
	}

	return &InstallResult{
		Success:        true,
		Stdout:         string(output),
		Stderr:         "",
		ExitCode:       0,
		DurationSeconds: duration,
		Action:         "upgrade",
	}, nil
}

// GetPackageType returns type of packages this installer handles
func (i *APTInstaller) GetPackageType() string {
	return "apt"
}

// getExitCode extracts exit code from exec error
func getExitCode(err error) int {
	if err == nil {
		return 0
	}

	if exitError, ok := err.(*exec.ExitError); ok {
		return exitError.ExitCode()
	}

	return 1 // Default error code
}