package installer

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/aggregator-project/aggregator-agent/internal/client"
)

// DNFInstaller handles DNF package installations
type DNFInstaller struct{}

// NewDNFInstaller creates a new DNF installer
func NewDNFInstaller() *DNFInstaller {
	return &DNFInstaller{}
}

// IsAvailable checks if DNF is available on this system
func (i *DNFInstaller) IsAvailable() bool {
	_, err := exec.LookPath("dnf")
	return err == nil
}

// Install installs packages using DNF
func (i *DNFInstaller) Install(packageName string) (*InstallResult, error) {
	startTime := time.Now()

	// Refresh package cache first
	refreshCmd := exec.Command("sudo", "dnf", "refresh", "-y")
	if output, err := refreshCmd.CombinedOutput(); err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to refresh DNF cache: %v\nStdout: %s", err, string(output)),
			DurationSeconds: int(time.Since(startTime).Seconds()),
		}, fmt.Errorf("dnf refresh failed: %w", err)
	}

	// Install package
	installCmd := exec.Command("sudo", "dnf", "install", "-y", packageName)
	output, err := installCmd.CombinedOutput()
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("DNF install failed: %v", err),
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
	}, nil
}

// InstallMultiple installs multiple packages using DNF
func (i *DNFInstaller) InstallMultiple(packageNames []string) (*InstallResult, error) {
	if len(packageNames) == 0 {
		return &InstallResult{
			Success:      false,
			ErrorMessage: "No packages specified for installation",
		}, fmt.Errorf("no packages specified")
	}

	startTime := time.Now()

	// Refresh package cache first
	refreshCmd := exec.Command("sudo", "dnf", "refresh", "-y")
	if output, err := refreshCmd.CombinedOutput(); err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to refresh DNF cache: %v\nStdout: %s", err, string(output)),
			DurationSeconds: int(time.Since(startTime).Seconds()),
		}, fmt.Errorf("dnf refresh failed: %w", err)
	}

	// Install all packages in one command
	args := []string{"install", "-y"}
	args = append(args, packageNames...)
	installCmd := exec.Command("sudo", "dnf", args...)
	output, err := installCmd.CombinedOutput()
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("DNF install failed: %v", err),
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

// Upgrade upgrades all packages using DNF
func (i *DNFInstaller) Upgrade() (*InstallResult, error) {
	startTime := time.Now()

	// Refresh package cache first
	refreshCmd := exec.Command("sudo", "dnf", "refresh", "-y")
	if output, err := refreshCmd.CombinedOutput(); err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to refresh DNF cache: %v\nStdout: %s", err, string(output)),
			DurationSeconds: int(time.Since(startTime).Seconds()),
		}, fmt.Errorf("dnf refresh failed: %w", err)
	}

	// Upgrade all packages
	upgradeCmd := exec.Command("sudo", "dnf", "upgrade", "-y")
	output, err := upgradeCmd.CombinedOutput()
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("DNF upgrade failed: %v", err),
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
func (i *DNFInstaller) GetPackageType() string {
	return "dnf"
}