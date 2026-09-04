package installer

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"
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

// DryRun performs a dry run installation to check dependencies
func (i *APTInstaller) DryRun(packageName, version string) (*InstallResult, error) {
	startTime := time.Now()

	runner, err := NewDiscoveryRunner("apt")
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] discovery_runner_create error=%w", err)
	}

	_, updateErr := runner.Run(context.Background(), "update")
	if updateErr != nil {
		log.Printf("Warning: APT update failed (continuing with dry run): %v", updateErr)
	}

	target := packageName
	if version != "" {
		target = packageName + "=" + version
	}

	result, err := runner.Run(context.Background(), "install", "--dry-run", "--yes", target)
	duration := int(time.Since(startTime).Seconds())

	// nil guard
	if result == nil {
		result = &RunResult{}
	}
	deps := i.parseDependenciesFromAPTOutput(result.Stdout, packageName)

	installResult := &InstallResult{
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		DurationSeconds: duration,
		IsDryRun:        true,
		Action:          "dry_run",
	}

	if err != nil {
		if len(deps) > 0 {
			installResult.Success = true
			installResult.Dependencies = deps
			return installResult, nil
		}
		installResult.Success = false
		installResult.ErrorMessage = fmt.Sprintf("APT dry run failed: %v", err)
		return installResult, err
	}

	installResult.Success = true
	installResult.Dependencies = deps
	return installResult, nil
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

// VerifyHash checks that the package's current candidate .deb hash matches the
// pinned expected value. The hash is read from the signed apt index (the same
// SHA256 apt itself verifies the download against), so this catches an artifact
// swapped under a fixed version since pinning. apt's own GPG verification of the
// index and package remains the integrity layer at install.
func (i *APTInstaller) VerifyHash(packageName, version, expectedSHA256 string) error {
	if expectedSHA256 == "" {
		return fmt.Errorf("no expected hash registered for package %q — hash verification is mandatory for capability-gated installs", packageName)
	}

	resolved, err := ResolveArtifactSHA256("apt", packageName, version)
	if err != nil {
		return fmt.Errorf("failed to resolve package artifact: %w", err)
	}

	if !strings.EqualFold(resolved.SHA256, expectedSHA256) {
		return fmt.Errorf("package hash mismatch: expected %s, got %s", expectedSHA256, resolved.SHA256)
	}

	log.Printf("[INFO] [agent] [installer] hash_verified package=%s", packageName)
	return nil
}
