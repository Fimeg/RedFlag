package scanner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/installer"
)

// APTScanner scans for APT package updates
type APTScanner struct{}

// NewAPTScanner creates a new APT scanner
func NewAPTScanner() *APTScanner {
	return &APTScanner{}
}

func (s *APTScanner) Name() string { return "APT Update Scanner" }

// IsAvailable checks if APT is available on this system
func (s *APTScanner) IsAvailable() bool {
	_, err := exec.LookPath("apt")
	return err == nil
}

// Scan scans for available APT updates
func (s *APTScanner) Scan() ([]client.UpdateReportItem, error) {
	runner, err := installer.NewDiscoveryRunner("apt")
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [apt] discovery_runner_create error=%w", err)
	}
	// apt-get update refreshes the cache (best effort)
	runner.Run(context.Background(), "update")
	result, err := runner.Run(context.Background(), "list", "--upgradable")
	if err != nil {
		return nil, fmt.Errorf("failed to run apt list --upgradable: %w", err)
	}
	return parseAPTOutput([]byte(result.Stdout))
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
