package installer

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// DNFInstaller handles DNF package installations
type DNFInstaller struct {
	executor *SecureCommandExecutor
}

// NewDNFInstaller creates a new DNF installer
func NewDNFInstaller() *DNFInstaller {
	return &DNFInstaller{
		executor: NewSecureCommandExecutor(),
	}
}

// IsAvailable checks if DNF is available on this system
func (i *DNFInstaller) IsAvailable() bool {
	_, err := exec.LookPath("dnf")
	return err == nil
}

// Install installs packages using DNF
func (i *DNFInstaller) Install(packageName string) (*InstallResult, error) {
	startTime := time.Now()

	// Refresh package cache first using secure executor
	refreshResult, err := i.executor.ExecuteCommand("dnf", []string{"makecache"})
	if err != nil {
		refreshResult.DurationSeconds = int(time.Since(startTime).Seconds())
		refreshResult.ErrorMessage = fmt.Sprintf("Failed to refresh DNF cache: %v", err)
		return refreshResult, fmt.Errorf("dnf refresh failed: %w", err)
	}

	// Install package using secure executor
	installResult, err := i.executor.ExecuteCommand("dnf", []string{"install", "-y", packageName})
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("DNF install failed: %v", err),
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

// InstallMultiple installs multiple packages using DNF
func (i *DNFInstaller) InstallMultiple(packageNames []string) (*InstallResult, error) {
	if len(packageNames) == 0 {
		return &InstallResult{
			Success:      false,
			ErrorMessage: "No packages specified for installation",
		}, fmt.Errorf("no packages specified")
	}

	startTime := time.Now()

	// Refresh package cache first using secure executor
	refreshResult, err := i.executor.ExecuteCommand("dnf", []string{"makecache"})
	if err != nil {
		refreshResult.DurationSeconds = int(time.Since(startTime).Seconds())
		refreshResult.ErrorMessage = fmt.Sprintf("Failed to refresh DNF cache: %v", err)
		return refreshResult, fmt.Errorf("dnf refresh failed: %w", err)
	}

	// Install all packages in one command using secure executor
	args := []string{"install", "-y"}
	args = append(args, packageNames...)
	installResult, err := i.executor.ExecuteCommand("dnf", args)
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("DNF install failed: %v", err),
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

// Upgrade upgrades all packages using DNF
func (i *DNFInstaller) Upgrade() (*InstallResult, error) {
	startTime := time.Now()

	// Refresh package cache first using secure executor
	refreshResult, err := i.executor.ExecuteCommand("dnf", []string{"makecache"})
	if err != nil {
		refreshResult.DurationSeconds = int(time.Since(startTime).Seconds())
		refreshResult.ErrorMessage = fmt.Sprintf("Failed to refresh DNF cache: %v", err)
		return refreshResult, fmt.Errorf("dnf refresh failed: %w", err)
	}

	// Upgrade all packages using secure executor
	upgradeResult, err := i.executor.ExecuteCommand("dnf", []string{"upgrade", "-y"})
	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		return &InstallResult{
			Success:        false,
			ErrorMessage:   fmt.Sprintf("DNF upgrade failed: %v", err),
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

// DryRun performs a dry run installation to check dependencies
func (i *DNFInstaller) DryRun(packageName string) (*InstallResult, error) {
	startTime := time.Now()

	// Attempt to refresh package cache, but don't fail if it doesn't work
	// (dry run can still work with slightly stale cache)
	refreshResult, refreshErr := i.executor.ExecuteCommand("dnf", []string{"makecache"})
	if refreshErr != nil {
		// Log refresh attempt but don't fail the dry run
		log.Printf("Warning: DNF makecache failed (continuing with dry run): %v", refreshErr)
	}
	_ = refreshResult // Discard refresh result intentionally

	// Perform dry run installation using secure executor
	installResult, err := i.executor.ExecuteCommand("dnf", []string{"install", "--assumeno", "--downloadonly", packageName})
	duration := int(time.Since(startTime).Seconds())

	// Parse dependencies from the output
	dependencies := i.parseDependenciesFromDNFOutput(installResult.Stdout, packageName)

	if err != nil {
		// DNF dry run may return non-zero exit code even for successful dependency resolution
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
			ErrorMessage:   fmt.Sprintf("DNF dry run failed: %v", err),
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

// parseDependenciesFromDNFOutput extracts dependency package names from DNF dry run output
func (i *DNFInstaller) parseDependenciesFromDNFOutput(output string, packageName string) []string {
	var dependencies []string

	// Regex patterns to find dependencies in DNF output
	patterns := []*regexp.Regexp{
		// Match "Installing dependencies:" section
		regexp.MustCompile(`(?s)Installing dependencies:(.*?)(\n\n|\z|Transaction Summary:)`),
		// Match "Dependencies resolved." section and package list
		regexp.MustCompile(`(?s)Dependencies resolved\.(.*?)(\n\n|\z|Transaction Summary:)`),
		// Match package installation lines
		regexp.MustCompile(`^\s*([a-zA-Z0-9][a-zA-Z0-9+._-]*)\s+[a-zA-Z0-9:.]+(?:\s+[a-zA-Z]+)?$`),
	}

	for _, pattern := range patterns {
		if strings.Contains(pattern.String(), "Installing dependencies:") {
			matches := pattern.FindStringSubmatch(output)
			if len(matches) > 1 {
				// Extract package names from the dependencies section
				lines := strings.Split(matches[1], "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" && !strings.Contains(line, "Dependencies") {
						pkg := i.extractPackageNameFromDNFLine(line)
						if pkg != "" {
							dependencies = append(dependencies, pkg)
						}
					}
				}
			}
		}
	}

	// Also look for transaction summary which lists all packages to be installed
	transactionPattern := regexp.MustCompile(`(?s)Transaction Summary:\s*\n\s*Install\s+(\d+) Packages?\s*\n((?:\s+\d+\s+[a-zA-Z0-9+._-]+\s+[a-zA-Z0-9:.]+.*\n?)*)`)
	matches := transactionPattern.FindStringSubmatch(output)
	if len(matches) > 2 {
		installLines := strings.Split(matches[2], "\n")
		for _, line := range installLines {
			line = strings.TrimSpace(line)
			if line != "" {
				pkg := i.extractPackageNameFromDNFLine(line)
				if pkg != "" && pkg != packageName {
					dependencies = append(dependencies, pkg)
				}
			}
		}
	}

	// Remove duplicates
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

// extractPackageNameFromDNFLine extracts package name from a DNF output line
func (i *DNFInstaller) extractPackageNameFromDNFLine(line string) string {
	// Remove architecture info if present
	if idx := strings.LastIndex(line, "."); idx > 0 {
		archSuffix := line[idx:]
		if strings.Contains(archSuffix, ".x86_64") || strings.Contains(archSuffix, ".noarch") ||
		   strings.Contains(archSuffix, ".i386") || strings.Contains(archSuffix, ".arm64") {
			line = line[:idx]
		}
	}

	// Extract package name (typically at the start of the line)
	fields := strings.Fields(line)
	if len(fields) > 0 {
		pkg := fields[0]
		// Remove version info if present
		if idx := strings.Index(pkg, "-"); idx > 0 {
			potentialName := pkg[:idx]
			// Check if this looks like a version (contains numbers)
			versionPart := pkg[idx+1:]
			if strings.Contains(versionPart, ".") || regexp.MustCompile(`\d`).MatchString(versionPart) {
				return potentialName
			}
		}
		return pkg
	}

	return ""
}

// GetPackageType returns type of packages this installer handles
func (i *DNFInstaller) GetPackageType() string {
	return "dnf"
}