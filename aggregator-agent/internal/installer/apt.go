package installer

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// APTInstaller handles APT package installations
type APTInstaller struct {
	executor *SecureCommandExecutor
}

// NewAPTInstaller creates a new APT installer
func NewAPTInstaller() *APTInstaller {
	return &APTInstaller{
		executor: NewSecureCommandExecutor(),
	}
}

// IsAvailable checks if APT is available on this system
func (i *APTInstaller) IsAvailable() bool {
	_, err := exec.LookPath("apt-get")
	return err == nil
}

// Install installs packages using APT
func (i *APTInstaller) Install(packageName string) (*InstallResult, error) {
	startTime := time.Now()

	// Update package cache first using secure executor
	updateResult, err := i.executor.ExecuteCommand("apt-get", []string{"update"})
	if err != nil {
		updateResult.DurationSeconds = int(time.Since(startTime).Seconds())
		updateResult.ErrorMessage = fmt.Sprintf("Failed to update APT cache: %v", err)
		return updateResult, fmt.Errorf("apt-get update failed: %w", err)
	}

	// Install package using secure executor
	installResult, err := i.executor.ExecuteCommand("apt-get", []string{"install", "-y", packageName})
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("APT install failed: %v", err),
			Stdout:         installResult.Stdout,
			Stderr:         installResult.Stderr,
			ExitCode:       installResult.ExitCode,
			DurationSeconds: duration,
		}, err
	}

	return &InstallResult{
		Success:        true,
		Stdout:         installResult.Stdout,
		Stderr:         installResult.Stderr,
		ExitCode:       installResult.ExitCode,
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

	// Update package cache first using secure executor
	updateResult, err := i.executor.ExecuteCommand("apt-get", []string{"update"})
	if err != nil {
		updateResult.DurationSeconds = int(time.Since(startTime).Seconds())
		updateResult.ErrorMessage = fmt.Sprintf("Failed to update APT cache: %v", err)
		return updateResult, fmt.Errorf("apt-get update failed: %w", err)
	}

	// Install all packages in one command using secure executor
	args := []string{"install", "-y"}
	args = append(args, packageNames...)
	installResult, err := i.executor.ExecuteCommand("apt-get", args)
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("APT install failed: %v", err),
			Stdout:         installResult.Stdout,
			Stderr:         installResult.Stderr,
			ExitCode:       installResult.ExitCode,
			DurationSeconds: duration,
		}, err
	}

	return &InstallResult{
		Success:        true,
		Stdout:         installResult.Stdout,
		Stderr:         installResult.Stderr,
		ExitCode:       installResult.ExitCode,
		DurationSeconds: duration,
		PackagesInstalled: packageNames,
	}, nil
}

// Upgrade upgrades all packages using APT
func (i *APTInstaller) Upgrade() (*InstallResult, error) {
	startTime := time.Now()

	// Update package cache first using secure executor
	updateResult, err := i.executor.ExecuteCommand("apt-get", []string{"update"})
	if err != nil {
		updateResult.DurationSeconds = int(time.Since(startTime).Seconds())
		updateResult.ErrorMessage = fmt.Sprintf("Failed to update APT cache: %v", err)
		return updateResult, fmt.Errorf("apt-get update failed: %w", err)
	}

	// Upgrade all packages using secure executor
	upgradeResult, err := i.executor.ExecuteCommand("apt-get", []string{"upgrade", "-y"})
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("APT upgrade failed: %v", err),
			Stdout:         upgradeResult.Stdout,
			Stderr:         upgradeResult.Stderr,
			ExitCode:       upgradeResult.ExitCode,
			DurationSeconds: duration,
		}, err
	}

	return &InstallResult{
		Success:        true,
		Stdout:         upgradeResult.Stdout,
		Stderr:         upgradeResult.Stderr,
		ExitCode:       upgradeResult.ExitCode,
		DurationSeconds: duration,
		Action:         "upgrade",
	}, nil
}

// UpdatePackage updates a specific package using APT
func (i *APTInstaller) UpdatePackage(packageName string) (*InstallResult, error) {
	startTime := time.Now()

	// Update specific package using secure executor
	updateResult, err := i.executor.ExecuteCommand("apt-get", []string{"install", "--only-upgrade", "-y", packageName})
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("APT update failed: %v", err),
			Stdout:         updateResult.Stdout,
			Stderr:         updateResult.Stderr,
			ExitCode:       updateResult.ExitCode,
			DurationSeconds: duration,
		}, err
	}

	return &InstallResult{
		Success:        true,
		Stdout:         updateResult.Stdout,
		Stderr:         updateResult.Stderr,
		ExitCode:       updateResult.ExitCode,
		DurationSeconds: duration,
		PackagesInstalled: []string{packageName},
		Action:         "update",
	}, nil
}

// DryRun performs a dry run installation to check dependencies
func (i *APTInstaller) DryRun(packageName string) (*InstallResult, error) {
	startTime := time.Now()

	// Update package cache first using secure executor
	updateResult, err := i.executor.ExecuteCommand("apt-get", []string{"update"})
	if err != nil {
		updateResult.DurationSeconds = int(time.Since(startTime).Seconds())
		updateResult.ErrorMessage = fmt.Sprintf("Failed to update APT cache: %v", err)
		updateResult.IsDryRun = true
		return updateResult, fmt.Errorf("apt-get update failed: %w", err)
	}

	// Perform dry run installation using secure executor
	installResult, err := i.executor.ExecuteCommand("apt-get", []string{"install", "--dry-run", "--yes", packageName})
	duration := int(time.Since(startTime).Seconds())

	// Parse dependencies from the output
	dependencies := i.parseDependenciesFromAPTOutput(installResult.Stdout, packageName)

	if err != nil {
		// APT dry run may return non-zero exit code even for successful dependency resolution
		// so we check if we were able to parse dependencies
		if len(dependencies) > 0 {
			return &InstallResult{
				Success:        true,
				Stdout:         installResult.Stdout,
				Stderr:         installResult.Stderr,
				ExitCode:       installResult.ExitCode,
				DurationSeconds: duration,
				Dependencies:    dependencies,
				IsDryRun:        true,
				Action:          "dry_run",
			}, nil
		}

		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("APT dry run failed: %v", err),
			Stdout:         installResult.Stdout,
			Stderr:         installResult.Stderr,
			ExitCode:       installResult.ExitCode,
			DurationSeconds: duration,
			IsDryRun:        true,
			Action:          "dry_run",
		}, err
	}

	return &InstallResult{
		Success:        true,
		Stdout:         installResult.Stdout,
		Stderr:         installResult.Stderr,
		ExitCode:       installResult.ExitCode,
		DurationSeconds: duration,
		Dependencies:    dependencies,
		IsDryRun:        true,
		Action:          "dry_run",
	}, nil
}

// parseDependenciesFromAPTOutput extracts dependency package names from APT dry run output
func (i *APTInstaller) parseDependenciesFromAPTOutput(output string, packageName string) []string {
	var dependencies []string

	// Regex patterns to find dependencies in APT output
	patterns := []*regexp.Regexp{
		// Match "The following additional packages will be installed:" section
		regexp.MustCompile(`(?s)The following additional packages will be installed:(.*?)(\n\n|\z)`),
		// Match "The following NEW packages will be installed:" section
		regexp.MustCompile(`(?s)The following NEW packages will be installed:(.*?)(\n\n|\z)`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(output)
		if len(matches) > 1 {
			// Extract package names from the matched section
			packageLines := strings.Split(matches[1], "\n")
			for _, line := range packageLines {
				line = strings.TrimSpace(line)
				// Skip empty lines and section headers
				if line != "" && !strings.Contains(line, "will be installed") && !strings.Contains(line, "packages") {
					// Extract package names (they're typically space-separated)
					packages := strings.Fields(line)
					for _, pkg := range packages {
						pkg = strings.TrimSpace(pkg)
						// Filter out common non-package words
						if pkg != "" && !strings.Contains(pkg, "recommended") &&
						   !strings.Contains(pkg, "suggested") && !strings.Contains(pkg, "following") {
							dependencies = append(dependencies, pkg)
						}
					}
				}
			}
		}
	}

	// Remove duplicates and filter out the original package
	uniqueDeps := make([]string, 0)
	seen := make(map[string]bool)
	for _, dep := range dependencies {
		if dep != packageName && !seen[dep] {
			seen[dep] = true
			uniqueDeps = append(uniqueDeps, dep)
		}
	}

	return uniqueDeps
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