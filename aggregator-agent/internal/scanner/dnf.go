package scanner

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/aggregator-project/aggregator-agent/internal/client"
)

// DNFScanner scans for DNF/RPM package updates
type DNFScanner struct{}

// NewDNFScanner creates a new DNF scanner
func NewDNFScanner() *DNFScanner {
	return &DNFScanner{}
}

// IsAvailable checks if DNF is available on this system
func (s *DNFScanner) IsAvailable() bool {
	_, err := exec.LookPath("dnf")
	return err == nil
}

// Scan scans for available DNF updates
func (s *DNFScanner) Scan() ([]client.UpdateReportItem, error) {
	// Check for updates (don't update cache to avoid needing sudo)
	cmd := exec.Command("dnf", "check-update")
	output, err := cmd.Output()
	if err != nil {
		// dnf check-update returns exit code 100 when updates are available
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 100 {
			// Updates are available, continue processing
		} else {
			return nil, fmt.Errorf("failed to run dnf check-update: %w", err)
		}
	}

	return parseDNFOutput(output)
}

func parseDNFOutput(output []byte) ([]client.UpdateReportItem, error) {
	var updates []client.UpdateReportItem
	scanner := bufio.NewScanner(bytes.NewReader(output))

	// Regex to parse dnf check-update output:
	// package-name.version arch      new-version
	re := regexp.MustCompile(`^([^\s]+)\.([^\s]+)\s+([^\s]+)\s+([^\s]+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and header/footer
		if line == "" ||
		   strings.HasPrefix(line, "Last metadata") ||
		   strings.HasPrefix(line, "Dependencies") ||
		   strings.HasPrefix(line, "Obsoleting") ||
		   strings.Contains(line, "Upgraded") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) < 5 {
			continue
		}

		packageName := matches[1]
		arch := matches[2]
		repoAndVersion := matches[3]
		newVersion := matches[4]

		// Extract repository and current version from repoAndVersion
		// Format is typically: repo-version current-version
		parts := strings.Fields(repoAndVersion)
		var repository, currentVersion string

		if len(parts) >= 2 {
			repository = parts[0]
			currentVersion = parts[1]
		} else if len(parts) == 1 {
			repository = parts[0]
			// Try to get current version from rpm
			currentVersion = getInstalledVersion(packageName)
		}

		// Determine severity based on repository and update type
		severity := determineSeverity(repository, packageName, newVersion)

		update := client.UpdateReportItem{
			PackageType:      "dnf",
			PackageName:      packageName,
			CurrentVersion:   currentVersion,
			AvailableVersion: newVersion,
			Severity:         severity,
			RepositorySource: repository,
			Metadata: map[string]interface{}{
				"architecture": arch,
			},
		}

		updates = append(updates, update)
	}

	return updates, nil
}

// getInstalledVersion gets the currently installed version of a package
func getInstalledVersion(packageName string) string {
	cmd := exec.Command("rpm", "-q", "--queryformat", "%{VERSION}", packageName)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// determineSeverity determines the severity of an update based on repository and package information
func determineSeverity(repository, packageName, newVersion string) string {
	// Security updates
	if strings.Contains(strings.ToLower(repository), "security") ||
	   strings.Contains(strings.ToLower(repository), "updates") ||
	   strings.Contains(strings.ToLower(packageName), "security") ||
	   strings.Contains(strings.ToLower(packageName), "selinux") ||
	   strings.Contains(strings.ToLower(packageName), "crypto") ||
	   strings.Contains(strings.ToLower(packageName), "openssl") ||
	   strings.Contains(strings.ToLower(packageName), "gnutls") {
		return "critical"
	}

	// Kernel updates are important
	if strings.Contains(strings.ToLower(packageName), "kernel") {
		return "important"
	}

	// Core system packages
	if strings.Contains(strings.ToLower(packageName), "glibc") ||
	   strings.Contains(strings.ToLower(packageName), "systemd") ||
	   strings.Contains(strings.ToLower(packageName), "bash") ||
	   strings.Contains(strings.ToLower(packageName), "coreutils") {
		return "important"
	}

	// Development tools
	if strings.Contains(strings.ToLower(packageName), "gcc") ||
	   strings.Contains(strings.ToLower(packageName), "python") ||
	   strings.Contains(strings.ToLower(packageName), "nodejs") ||
	   strings.Contains(strings.ToLower(packageName), "java") ||
	   strings.Contains(strings.ToLower(packageName), "go") {
		return "moderate"
	}

	// Default severity
	return "low"
}