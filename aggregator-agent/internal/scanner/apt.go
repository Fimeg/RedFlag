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

// APTScanner scans for APT package updates
type APTScanner struct{}

// NewAPTScanner creates a new APT scanner
func NewAPTScanner() *APTScanner {
	return &APTScanner{}
}

// IsAvailable checks if APT is available on this system
func (s *APTScanner) IsAvailable() bool {
	_, err := exec.LookPath("apt")
	return err == nil
}

// Scan scans for available APT updates
func (s *APTScanner) Scan() ([]client.UpdateReportItem, error) {
	// Update package cache (sudo may be required, but try anyway)
	updateCmd := exec.Command("apt-get", "update")
	updateCmd.Run() // Ignore errors since we might not have sudo

	// Get upgradable packages
	cmd := exec.Command("apt", "list", "--upgradable")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run apt list: %w", err)
	}

	return parseAPTOutput(output)
}

func parseAPTOutput(output []byte) ([]client.UpdateReportItem, error) {
	var updates []client.UpdateReportItem
	scanner := bufio.NewScanner(bytes.NewReader(output))

	// Regex to parse apt output:
	// package/repo version arch [upgradable from: old_version]
	re := regexp.MustCompile(`^([^\s/]+)/([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+\[upgradable from:\s+([^\]]+)\]`)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Listing...") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) < 6 {
			continue
		}

		packageName := matches[1]
		repository := matches[2]
		newVersion := matches[3]
		oldVersion := matches[5]

		// Determine severity (simplified - in production, query Ubuntu Security Advisories)
		severity := "moderate"
		if strings.Contains(repository, "security") {
			severity = "important"
		}

		update := client.UpdateReportItem{
			PackageType:      "apt",
			PackageName:      packageName,
			CurrentVersion:   oldVersion,
			AvailableVersion: newVersion,
			Severity:         severity,
			RepositorySource: repository,
			Metadata: map[string]interface{}{
				"architecture": matches[4],
			},
		}

		updates = append(updates, update)
	}

	return updates, nil
}
