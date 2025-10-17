package scanner

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/aggregator-project/aggregator-agent/internal/client"
)

// WingetPackage represents a single package from winget output
type WingetPackage struct {
	Name           string `json:"Name"`
	ID             string `json:"Id"`
	Version        string `json:"Version"`
	Available      string `json:"Available"`
	Source         string `json:"Source"`
	IsPinned       bool   `json:"IsPinned"`
	PinReason      string `json:"PinReason,omitempty"`
}

// WingetScanner scans for Windows package updates using winget
type WingetScanner struct{}

// NewWingetScanner creates a new Winget scanner
func NewWingetScanner() *WingetScanner {
	return &WingetScanner{}
}

// IsAvailable checks if winget is available on this system
func (s *WingetScanner) IsAvailable() bool {
	// Only available on Windows
	if runtime.GOOS != "windows" {
		return false
	}

	// Check if winget command exists
	_, err := exec.LookPath("winget")
	return err == nil
}

// Scan scans for available winget package updates
func (s *WingetScanner) Scan() ([]client.UpdateReportItem, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("winget is not available on this system")
	}

	// Try multiple approaches with proper error handling
	var lastErr error

	// Method 1: Standard winget list with JSON output
	if updates, err := s.scanWithJSON(); err == nil {
		return updates, nil
	} else {
		lastErr = err
		fmt.Printf("Winget JSON scan failed: %v\n", err)
	}

	// Method 2: Fallback to basic winget list without JSON
	if updates, err := s.scanWithBasicOutput(); err == nil {
		return updates, nil
	} else {
		lastErr = fmt.Errorf("both winget scan methods failed: %v (last error)", err)
		fmt.Printf("Winget basic scan failed: %v\n", err)
	}

	// Method 3: Check if this is a known Winget issue and provide helpful error
	if isKnownWingetError(lastErr) {
		return nil, fmt.Errorf("winget encountered a known issue (exit code %s). This may be due to Windows Update service or system configuration. Try running 'winget upgrade' manually to resolve", getExitCode(lastErr))
	}

	return nil, lastErr
}

// scanWithJSON attempts to scan using JSON output (most reliable)
func (s *WingetScanner) scanWithJSON() ([]client.UpdateReportItem, error) {
	// Run winget list command to get outdated packages
	// Using --output json for structured output
	cmd := exec.Command("winget", "list", "--outdated", "--accept-source-agreements", "--output", "json")

	// Use CombinedOutput to capture both stdout and stderr for better error handling
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check for specific exit codes that might be transient
		if isTransientError(err) {
			return nil, fmt.Errorf("winget temporary failure: %w", err)
		}
		return nil, fmt.Errorf("failed to run winget list: %w (output: %s)", err, string(output))
	}

	// Parse JSON output
	var packages []WingetPackage
	if err := json.Unmarshal(output, &packages); err != nil {
		return nil, fmt.Errorf("failed to parse winget JSON output: %w (output: %s)", err, string(output))
	}

	var updates []client.UpdateReportItem

	// Convert each package to our UpdateReportItem format
	for _, pkg := range packages {
		// Skip if no available update
		if pkg.Available == "" || pkg.Available == pkg.Version {
			continue
		}

		updateItem := s.parseWingetPackage(pkg)
		updates = append(updates, *updateItem)
	}

	return updates, nil
}

// scanWithBasicOutput falls back to parsing text output
func (s *WingetScanner) scanWithBasicOutput() ([]client.UpdateReportItem, error) {
	cmd := exec.Command("winget", "list", "--outdated", "--accept-source-agreements")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run winget list basic: %w", err)
	}

	// Simple text parsing fallback
	return s.parseWingetTextOutput(string(output))
}

// parseWingetTextOutput parses winget text output as fallback
func (s *WingetScanner) parseWingetTextOutput(output string) ([]client.UpdateReportItem, error) {
	var updates []client.UpdateReportItem
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip header lines and empty lines
		if strings.HasPrefix(line, "Name") || strings.HasPrefix(line, "-") || line == "" {
			continue
		}

		// Simple parsing for tab or space-separated values
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			pkgName := fields[0]
			currentVersion := fields[1]
			availableVersion := fields[2]

			// Skip if no update available
			if availableVersion == currentVersion || availableVersion == "Unknown" {
				continue
			}

			update := client.UpdateReportItem{
				PackageType:      "winget",
				PackageName:      pkgName,
				CurrentVersion:   currentVersion,
				AvailableVersion: availableVersion,
				Severity:         s.determineSeverityFromName(pkgName),
				RepositorySource: "winget",
				PackageDescription: fmt.Sprintf("Update available for %s", pkgName),
				Metadata: map[string]interface{}{
					"package_manager": "winget",
					"detected_via":    "text_parser",
				},
			}
			updates = append(updates, update)
		}
	}

	return updates, nil
}

// isTransientError checks if the error might be temporary
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Common transient error patterns
	transientPatterns := []string{
		"network error",
		"timeout",
		"connection refused",
		"temporary failure",
		"service unavailable",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// isKnownWingetError checks for known Winget issues
func isKnownWingetError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for the specific exit code 0x8a150002
	if strings.Contains(errStr, "2316632066") || strings.Contains(errStr, "0x8a150002") {
		return true
	}

	// Other known Winget issues
	knownPatterns := []string{
		"winget is not recognized",
		"windows package manager",
		"windows app installer",
		"restarting your computer",
	}

	for _, pattern := range knownPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// getExitCode extracts exit code from error if available
func getExitCode(err error) string {
	if err == nil {
		return "unknown"
	}

	// Try to extract exit code from error message
	errStr := err.Error()
	if strings.Contains(errStr, "exit status") {
		// Extract exit status number
		parts := strings.Fields(errStr)
		for i, part := range parts {
			if part == "status" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}

	return "unknown"
}

// determineSeverityFromName provides basic severity detection for fallback
func (s *WingetScanner) determineSeverityFromName(name string) string {
	lowerName := strings.ToLower(name)

	// Security tools get higher priority
	if strings.Contains(lowerName, "antivirus") ||
	   strings.Contains(lowerName, "security") ||
	   strings.Contains(lowerName, "defender") ||
	   strings.Contains(lowerName, "firewall") {
		return "critical"
	}

	// Browsers and communication tools get high priority
	if strings.Contains(lowerName, "firefox") ||
	   strings.Contains(lowerName, "chrome") ||
	   strings.Contains(lowerName, "edge") ||
	   strings.Contains(lowerName, "browser") {
		return "high"
	}

	return "moderate"
}

// parseWingetPackage converts a WingetPackage to our UpdateReportItem format
func (s *WingetScanner) parseWingetPackage(pkg WingetPackage) *client.UpdateReportItem {
	// Determine severity based on package type and source
	severity := s.determineSeverity(pkg)

	// Categorize the package type
	packageCategory := s.categorizePackage(pkg.Name, pkg.Source)

	// Create metadata with winget-specific information
	metadata := map[string]interface{}{
		"package_id":     pkg.ID,
		"source":         pkg.Source,
		"category":       packageCategory,
		"is_pinned":      pkg.IsPinned,
		"pin_reason":     pkg.PinReason,
		"package_manager": "winget",
	}

	// Add additional metadata based on package source
	if pkg.Source == "winget" {
		metadata["repository_type"] = "community"
	} else if pkg.Source == "msstore" {
		metadata["repository_type"] = "microsoft_store"
	} else {
		metadata["repository_type"] = "custom"
	}

	// Create the update report item
	updateItem := &client.UpdateReportItem{
		PackageType:      "winget",
		PackageName:      pkg.Name,
		CurrentVersion:   pkg.Version,
		AvailableVersion: pkg.Available,
		Severity:         severity,
		RepositorySource: pkg.Source,
		Metadata:         metadata,
	}

	// Add description if available (would need additional winget calls)
	// For now, we'll use the package name as description
	updateItem.PackageDescription = fmt.Sprintf("Update available for %s from %s", pkg.Name, pkg.Source)

	return updateItem
}

// determineSeverity determines the severity of a package update based on various factors
func (s *WingetScanner) determineSeverity(pkg WingetPackage) string {
	name := strings.ToLower(pkg.Name)
	source := strings.ToLower(pkg.Source)

	// Security tools get higher priority
	if strings.Contains(name, "antivirus") ||
	   strings.Contains(name, "security") ||
	   strings.Contains(name, "firewall") ||
	   strings.Contains(name, "malware") ||
	   strings.Contains(name, "defender") ||
	   strings.Contains(name, "crowdstrike") ||
	   strings.Contains(name, "sophos") ||
	   strings.Contains(name, "symantec") {
		return "critical"
	}

	// Browsers and communication tools get high priority
	if strings.Contains(name, "firefox") ||
	   strings.Contains(name, "chrome") ||
	   strings.Contains(name, "edge") ||
	   strings.Contains(name, "browser") ||
	   strings.Contains(name, "zoom") ||
	   strings.Contains(name, "teams") ||
	   strings.Contains(name, "slack") ||
	   strings.Contains(name, "discord") {
		return "high"
	}

	// Development tools
	if strings.Contains(name, "visual studio") ||
	   strings.Contains(name, "vscode") ||
	   strings.Contains(name, "git") ||
	   strings.Contains(name, "docker") ||
	   strings.Contains(name, "nodejs") ||
	   strings.Contains(name, "python") ||
	   strings.Contains(name, "java") ||
	   strings.Contains(name, "powershell") {
		return "moderate"
	}

	// Microsoft Store apps might be less critical
	if source == "msstore" {
		return "low"
	}

	// Default severity
	return "moderate"
}

// categorizePackage categorizes the package based on name and source
func (s *WingetScanner) categorizePackage(name, source string) string {
	lowerName := strings.ToLower(name)

	// Development tools
	if strings.Contains(lowerName, "visual studio") ||
	   strings.Contains(lowerName, "vscode") ||
	   strings.Contains(lowerName, "intellij") ||
	   strings.Contains(lowerName, "sublime") ||
	   strings.Contains(lowerName, "notepad++") ||
	   strings.Contains(lowerName, "git") ||
	   strings.Contains(lowerName, "docker") ||
	   strings.Contains(lowerName, "nodejs") ||
	   strings.Contains(lowerName, "python") ||
	   strings.Contains(lowerName, "java") ||
	   strings.Contains(lowerName, "rust") ||
	   strings.Contains(lowerName, "go") ||
	   strings.Contains(lowerName, "github") ||
	   strings.Contains(lowerName, "postman") ||
	   strings.Contains(lowerName, "wireshark") {
		return "development"
	}

	// Security tools
	if strings.Contains(lowerName, "antivirus") ||
	   strings.Contains(lowerName, "security") ||
	   strings.Contains(lowerName, "firewall") ||
	   strings.Contains(lowerName, "malware") ||
	   strings.Contains(lowerName, "defender") ||
	   strings.Contains(lowerName, "crowdstrike") ||
	   strings.Contains(lowerName, "sophos") ||
	   strings.Contains(lowerName, "symantec") ||
	   strings.Contains(lowerName, "vpn") ||
	   strings.Contains(lowerName, "1password") ||
	   strings.Contains(lowerName, "bitwarden") ||
	   strings.Contains(lowerName, "lastpass") {
		return "security"
	}

	// Browsers
	if strings.Contains(lowerName, "firefox") ||
	   strings.Contains(lowerName, "chrome") ||
	   strings.Contains(lowerName, "edge") ||
	   strings.Contains(lowerName, "opera") ||
	   strings.Contains(lowerName, "brave") ||
	   strings.Contains(lowerName, "vivaldi") ||
	   strings.Contains(lowerName, "browser") {
		return "browser"
	}

	// Communication tools
	if strings.Contains(lowerName, "zoom") ||
	   strings.Contains(lowerName, "teams") ||
	   strings.Contains(lowerName, "slack") ||
	   strings.Contains(lowerName, "discord") ||
	   strings.Contains(lowerName, "telegram") ||
	   strings.Contains(lowerName, "whatsapp") ||
	   strings.Contains(lowerName, "skype") ||
	   strings.Contains(lowerName, "outlook") {
		return "communication"
	}

	// Media and entertainment
	if strings.Contains(lowerName, "vlc") ||
	   strings.Contains(lowerName, "spotify") ||
	   strings.Contains(lowerName, "itunes") ||
	   strings.Contains(lowerName, "plex") ||
	   strings.Contains(lowerName, "kodi") ||
	   strings.Contains(lowerName, "obs") ||
	   strings.Contains(lowerName, "streamlabs") {
		return "media"
	}

	// Productivity tools
	if strings.Contains(lowerName, "microsoft office") ||
	   strings.Contains(lowerName, "word") ||
	   strings.Contains(lowerName, "excel") ||
	   strings.Contains(lowerName, "powerpoint") ||
	   strings.Contains(lowerName, "adobe") ||
	   strings.Contains(lowerName, "photoshop") ||
	   strings.Contains(lowerName, "acrobat") ||
	   strings.Contains(lowerName, "notion") ||
	   strings.Contains(lowerName, "obsidian") ||
	   strings.Contains(lowerName, "typora") {
		return "productivity"
	}

	// System utilities
	if strings.Contains(lowerName, "7-zip") ||
	   strings.Contains(lowerName, "winrar") ||
	   strings.Contains(lowerName, "ccleaner") ||
	   strings.Contains(lowerName, "process") ||
	   strings.Contains(lowerName, "task manager") ||
	   strings.Contains(lowerName, "cpu-z") ||
	   strings.Contains(lowerName, "gpu-z") ||
	   strings.Contains(lowerName, "hwmonitor") {
		return "utility"
	}

	// Gaming
	if strings.Contains(lowerName, "steam") ||
	   strings.Contains(lowerName, "epic") ||
	   strings.Contains(lowerName, "origin") ||
	   strings.Contains(lowerName, "uplay") ||
	   strings.Contains(lowerName, "gog") ||
	   strings.Contains(lowerName, "discord") { // Discord is also gaming
		return "gaming"
	}

	// Default category
	return "application"
}

// GetPackageDetails retrieves detailed information about a specific winget package
func (s *WingetScanner) GetPackageDetails(packageID string) (*client.UpdateReportItem, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("winget is not available on this system")
	}

	// Run winget show command to get detailed package information
	cmd := exec.Command("winget", "show", "--id", packageID, "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run winget show: %w", err)
	}

	// Parse JSON output (winget show outputs a single package object)
	var pkg WingetPackage
	if err := json.Unmarshal(output, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse winget show output: %w", err)
	}

	// Convert to UpdateReportItem format
	updateItem := s.parseWingetPackage(pkg)
	return updateItem, nil
}

// GetInstalledPackages retrieves all installed packages via winget
func (s *WingetScanner) GetInstalledPackages() ([]WingetPackage, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("winget is not available on this system")
	}

	// Run winget list command to get all installed packages
	cmd := exec.Command("winget", "list", "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run winget list: %w", err)
	}

	// Parse JSON output
	var packages []WingetPackage
	if err := json.Unmarshal(output, &packages); err != nil {
		return nil, fmt.Errorf("failed to parse winget JSON output: %w", err)
	}

	return packages, nil
}