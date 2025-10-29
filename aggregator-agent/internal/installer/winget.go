package installer

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// WingetInstaller handles winget package installation
type WingetInstaller struct{}

// NewWingetInstaller creates a new Winget installer
func NewWingetInstaller() *WingetInstaller {
	return &WingetInstaller{}
}

// IsAvailable checks if winget is available on this system
func (i *WingetInstaller) IsAvailable() bool {
	// Only available on Windows
	if runtime.GOOS != "windows" {
		return false
	}

	// Check if winget command exists
	_, err := exec.LookPath("winget")
	return err == nil
}

// GetPackageType returns the package type this installer handles
func (i *WingetInstaller) GetPackageType() string {
	return "winget"
}

// Install installs a specific winget package
func (i *WingetInstaller) Install(packageName string) (*InstallResult, error) {
	return i.installPackage(packageName, false)
}

// InstallMultiple installs multiple winget packages
func (i *WingetInstaller) InstallMultiple(packageNames []string) (*InstallResult, error) {
	if len(packageNames) == 0 {
		return &InstallResult{
			Success:      false,
			ErrorMessage: "No packages specified for installation",
		}, fmt.Errorf("no packages specified")
	}

	// For winget, we'll install packages one by one to better track results
	startTime := time.Now()
	result := &InstallResult{
		Success:           true,
		Action:            "install_multiple",
		PackagesInstalled: []string{},
		Stdout:            "",
		Stderr:            "",
		ExitCode:          0,
		DurationSeconds:   0,
	}

	var combinedStdout []string
	var combinedStderr []string

	for _, packageName := range packageNames {
		singleResult, err := i.installPackage(packageName, false)
		if err != nil {
			result.Success = false
			result.Stderr += fmt.Sprintf("Failed to install %s: %v\n", packageName, err)
			continue
		}

		if !singleResult.Success {
			result.Success = false
			if singleResult.Stderr != "" {
				combinedStderr = append(combinedStderr, fmt.Sprintf("%s: %s", packageName, singleResult.Stderr))
			}
			continue
		}

		result.PackagesInstalled = append(result.PackagesInstalled, packageName)
		if singleResult.Stdout != "" {
			combinedStdout = append(combinedStdout, fmt.Sprintf("%s: %s", packageName, singleResult.Stdout))
		}
	}

	result.Stdout = strings.Join(combinedStdout, "\n")
	result.Stderr = strings.Join(combinedStderr, "\n")
	result.DurationSeconds = int(time.Since(startTime).Seconds())

	if result.Success {
		result.ExitCode = 0
	} else {
		result.ExitCode = 1
	}

	return result, nil
}

// Upgrade upgrades all outdated winget packages
func (i *WingetInstaller) Upgrade() (*InstallResult, error) {
	if !i.IsAvailable() {
		return nil, fmt.Errorf("winget is not available on this system")
	}

	startTime := time.Now()

	// Get list of outdated packages first
	outdatedPackages, err := i.getOutdatedPackages()
	if err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to get outdated packages: %v", err),
		}, err
	}

	if len(outdatedPackages) == 0 {
		return &InstallResult{
			Success:          true,
			Action:           "upgrade",
			Stdout:           "No outdated packages found",
			ExitCode:         0,
			DurationSeconds:  int(time.Since(startTime).Seconds()),
			PackagesInstalled: []string{},
		}, nil
	}

	// Upgrade all outdated packages
	return i.upgradeAllPackages(outdatedPackages)
}

// DryRun performs a dry run installation to check what would be installed
func (i *WingetInstaller) DryRun(packageName string) (*InstallResult, error) {
	return i.installPackage(packageName, true)
}

// installPackage is the internal implementation for package installation
func (i *WingetInstaller) installPackage(packageName string, isDryRun bool) (*InstallResult, error) {
	if !i.IsAvailable() {
		return nil, fmt.Errorf("winget is not available on this system")
	}

	startTime := time.Now()
	result := &InstallResult{
		Success:     false,
		IsDryRun:    isDryRun,
		ExitCode:    0,
		DurationSeconds: 0,
	}

	// Build winget command
	var cmd *exec.Cmd
	if isDryRun {
		// For dry run, we'll check if the package would be upgraded
		cmd = exec.Command("winget", "show", "--id", packageName, "--accept-source-agreements")
		result.Action = "dry_run"
	} else {
		// Install the package with upgrade flag
		cmd = exec.Command("winget", "install", "--id", packageName,
			"--upgrade", "--accept-package-agreements", "--accept-source-agreements", "--force")
		result.Action = "install"
	}

	// Execute command
	output, err := cmd.CombinedOutput()
	result.Stdout = string(output)
	result.Stderr = ""
	result.DurationSeconds = int(time.Since(startTime).Seconds())

	if err != nil {
		result.ExitCode = 1
		result.ErrorMessage = fmt.Sprintf("Command failed: %v", err)

		// Check if this is a "no update needed" scenario
		if strings.Contains(strings.ToLower(string(output)), "no upgrade available") ||
		   strings.Contains(strings.ToLower(string(output)), "already installed") {
			result.Success = true
			result.Stdout = "Package is already up to date"
			result.ExitCode = 0
			result.ErrorMessage = ""
		}

		return result, nil
	}

	result.Success = true
	result.ExitCode = 0
	result.PackagesInstalled = []string{packageName}

	// Parse output to extract additional information
	if !isDryRun {
		result.Stdout = i.parseInstallOutput(string(output), packageName)
	}

	return result, nil
}

// getOutdatedPackages retrieves a list of outdated packages
func (i *WingetInstaller) getOutdatedPackages() ([]string, error) {
	cmd := exec.Command("winget", "list", "--outdated", "--accept-source-agreements", "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get outdated packages: %w", err)
	}

	var packages []WingetPackage
	if err := json.Unmarshal(output, &packages); err != nil {
		return nil, fmt.Errorf("failed to parse winget output: %w", err)
	}

	var outdatedNames []string
	for _, pkg := range packages {
		if pkg.Available != "" && pkg.Available != pkg.Version {
			outdatedNames = append(outdatedNames, pkg.ID)
		}
	}

	return outdatedNames, nil
}

// upgradeAllPackages upgrades all specified packages
func (i *WingetInstaller) upgradeAllPackages(packageIDs []string) (*InstallResult, error) {
	startTime := time.Now()
	result := &InstallResult{
		Success:           true,
		Action:            "upgrade",
		PackagesInstalled: []string{},
		Stdout:            "",
		Stderr:            "",
		ExitCode:          0,
		DurationSeconds:   0,
	}

	var combinedStdout []string
	var combinedStderr []string

	for _, packageID := range packageIDs {
		upgradeResult, err := i.installPackage(packageID, false)
		if err != nil {
			result.Success = false
			combinedStderr = append(combinedStderr, fmt.Sprintf("Failed to upgrade %s: %v", packageID, err))
			continue
		}

		if !upgradeResult.Success {
			result.Success = false
			if upgradeResult.Stderr != "" {
				combinedStderr = append(combinedStderr, fmt.Sprintf("%s: %s", packageID, upgradeResult.Stderr))
			}
			continue
		}

		result.PackagesInstalled = append(result.PackagesInstalled, packageID)
		if upgradeResult.Stdout != "" {
			combinedStdout = append(combinedStdout, upgradeResult.Stdout)
		}
	}

	result.Stdout = strings.Join(combinedStdout, "\n")
	result.Stderr = strings.Join(combinedStderr, "\n")
	result.DurationSeconds = int(time.Since(startTime).Seconds())

	if result.Success {
		result.ExitCode = 0
	} else {
		result.ExitCode = 1
	}

	return result, nil
}

// parseInstallOutput parses and formats winget install output
func (i *WingetInstaller) parseInstallOutput(output, packageName string) string {
	lines := strings.Split(output, "\n")
	var relevantLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Include important status messages
		if strings.Contains(strings.ToLower(line), "successfully") ||
		   strings.Contains(strings.ToLower(line), "installed") ||
		   strings.Contains(strings.ToLower(line), "upgraded") ||
		   strings.Contains(strings.ToLower(line), "modified") ||
		   strings.Contains(strings.ToLower(line), "completed") ||
		   strings.Contains(strings.ToLower(line), "failed") ||
		   strings.Contains(strings.ToLower(line), "error") {
			relevantLines = append(relevantLines, line)
		}

		// Include download progress
		if strings.Contains(line, "Downloading") ||
		   strings.Contains(line, "Installing") ||
		   strings.Contains(line, "Extracting") {
			relevantLines = append(relevantLines, line)
		}
	}

	if len(relevantLines) == 0 {
		return fmt.Sprintf("Package %s installation completed", packageName)
	}

	return strings.Join(relevantLines, "\n")
}

// parseDependencies analyzes package dependencies (winget doesn't explicitly expose dependencies)
func (i *WingetInstaller) parseDependencies(packageName string) ([]string, error) {
	// Winget doesn't provide explicit dependency information in its basic output
	// This is a placeholder for future enhancement where we might parse
	// additional metadata or use Windows package management APIs

	// For now, we'll return empty dependencies as winget handles this automatically
	return []string{}, nil
}

// GetPackageInfo retrieves detailed information about a specific package
func (i *WingetInstaller) GetPackageInfo(packageID string) (map[string]interface{}, error) {
	if !i.IsAvailable() {
		return nil, fmt.Errorf("winget is not available on this system")
	}

	cmd := exec.Command("winget", "show", "--id", packageID, "--accept-source-agreements", "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get package info: %w", err)
	}

	var packageInfo map[string]interface{}
	if err := json.Unmarshal(output, &packageInfo); err != nil {
		return nil, fmt.Errorf("failed to parse package info: %w", err)
	}

	return packageInfo, nil
}

// IsPackageInstalled checks if a package is already installed
func (i *WingetInstaller) IsPackageInstalled(packageID string) (bool, string, error) {
	if !i.IsAvailable() {
		return false, "", fmt.Errorf("winget is not available on this system")
	}

	cmd := exec.Command("winget", "list", "--id", packageID, "--accept-source-agreements", "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		// Command failed, package is likely not installed
		return false, "", nil
	}

	var packages []WingetPackage
	if err := json.Unmarshal(output, &packages); err != nil {
		return false, "", fmt.Errorf("failed to parse package list: %w", err)
	}

	if len(packages) > 0 {
		return true, packages[0].Version, nil
	}

	return false, "", nil
}

// WingetPackage represents a winget package structure for JSON parsing
type WingetPackage struct {
	Name           string `json:"Name"`
	ID             string `json:"Id"`
	Version        string `json:"Version"`
	Available      string `json:"Available"`
	Source         string `json:"Source"`
	IsPinned       bool   `json:"IsPinned"`
	PinReason      string `json:"PinReason,omitempty"`
}

// UpdatePackage updates a specific winget package (alias for Install method)
func (i *WingetInstaller) UpdatePackage(packageName string) (*InstallResult, error) {
	// Winget uses same logic for updating as installing
	return i.Install(packageName)
}