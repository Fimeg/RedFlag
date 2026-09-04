package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/cache"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/display"
	"github.com/Fimeg/RedFlag/agent/internal/orchestrator"
	"github.com/Fimeg/RedFlag/agent/internal/scanner"
	"github.com/Fimeg/RedFlag/agent/internal/system"
	"github.com/Fimeg/RedFlag/agent/internal/version"
)

// ReportLogWithAck reports a command log to the server and tracks it for acknowledgment
func ReportLogWithAck(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, logReport client.LogReport) error {
	// Track this command result as pending acknowledgment
	ackTracker.Add(logReport.CommandID)

	// Save acknowledgment state immediately
	if err := ackTracker.Save(); err != nil {
		log.Printf("Warning: Failed to save acknowledgment for command %s: %v", logReport.CommandID, err)
	}

	// Report the log to the server
	if err := apiClient.ReportLog(cfg.AgentID, logReport); err != nil {
		// If reporting failed, increment retry count but don't remove from pending
		ackTracker.IncrementRetry(logReport.CommandID)
		return err
	}

	return nil
}

// ScanCommand performs a local scan and displays results
func ScanCommand(cfg *config.Config, exportFormat string) error {
	// Initialize scanners
	aptScanner := scanner.NewAPTScanner()
	dnfScanner := scanner.NewDNFScanner()
	dockerScanner, _ := orchestrator.NewDockerScanner()
	windowsUpdateScanner := scanner.NewWindowsUpdateScanner()
	wingetScanner := scanner.NewWingetScanner()

	fmt.Println("Scanning for updates...")
	var allUpdates []client.UpdateReportItem
	var scanResults []orchestrator.ScanResult

	// Scan APT updates
	if aptScanner.IsAvailable() {
		fmt.Println("  - Scanning APT packages...")
		result := runLocalScanner("apt", aptScanner.Scan)
		scanResults = append(scanResults, result)
		if result.Error != nil {
			fmt.Printf("    APT scan failed: %v\n", result.Error)
		} else {
			fmt.Printf("    ✓ Found %d APT updates\n", len(result.Updates))
			allUpdates = append(allUpdates, result.Updates...)
		}
	}

	// Scan DNF updates
	if dnfScanner.IsAvailable() {
		fmt.Println("  - Scanning DNF packages...")
		result := runLocalScanner("dnf", dnfScanner.Scan)
		scanResults = append(scanResults, result)
		if result.Error != nil {
			fmt.Printf("    DNF scan failed: %v\n", result.Error)
		} else {
			fmt.Printf("    ✓ Found %d DNF updates\n", len(result.Updates))
			allUpdates = append(allUpdates, result.Updates...)
		}
	}

	// Scan Docker updates
	if dockerScanner != nil && dockerScanner.IsAvailable() {
		fmt.Println("  - Scanning Docker images...")
		result := runLocalScanner("docker", dockerScanner.Scan)
		scanResults = append(scanResults, result)
		if result.Error != nil {
			fmt.Printf("    Docker scan failed: %v\n", result.Error)
		} else {
			fmt.Printf("    ✓ Found %d Docker image updates\n", len(result.Updates))
			allUpdates = append(allUpdates, result.Updates...)
		}
	}

	// Scan Windows updates
	if windowsUpdateScanner.IsAvailable() {
		fmt.Println("  - Scanning Windows updates...")
		result := runLocalScanner("windows", windowsUpdateScanner.Scan)
		scanResults = append(scanResults, result)
		if result.Error != nil {
			fmt.Printf("    Windows Update scan failed: %v\n", result.Error)
		} else {
			fmt.Printf("    ✓ Found %d Windows updates\n", len(result.Updates))
			allUpdates = append(allUpdates, result.Updates...)
		}
	}

	// Scan Winget packages
	if wingetScanner.IsAvailable() {
		fmt.Println("  - Scanning Winget packages...")
		result := runLocalScanner("winget", wingetScanner.Scan)
		scanResults = append(scanResults, result)
		if result.Error != nil {
			fmt.Printf("    Winget scan failed: %v\n", result.Error)
		} else {
			fmt.Printf("    ✓ Found %d Winget package updates\n", len(result.Updates))
			allUpdates = append(allUpdates, result.Updates...)
		}
	}

	recordLocalFullScan(cfg, allUpdates, scanResults)

	// Display results
	fmt.Println()
	return display.PrintScanResults(allUpdates, exportFormat)
}

func runLocalScanner(scannerName string, scan func() ([]client.UpdateReportItem, error)) orchestrator.ScanResult {
	startTime := time.Now()
	updates, err := scan()
	result := orchestrator.ScanResult{
		ScannerName: scannerName,
		Updates:     updates,
		Error:       err,
		Duration:    time.Since(startTime),
		Status:      "success",
	}
	if err != nil {
		result.Status = "failed"
	}
	return result
}

// StatusCommand displays agent status information
func StatusCommand(cfg *config.Config) error {
	fmt.Println("==================================================================")
	fmt.Println("🚩 RedFlag Agent Status")
	fmt.Println("==================================================================")

	// Agent information
	fmt.Printf("Agent ID: %s\n", cfg.AgentID)
	fmt.Printf("Server: %s\n", cfg.ServerURL)
	fmt.Printf("Check-in Interval: %ds\n", cfg.CheckInInterval)
	fmt.Printf("Version: %s\n", version.Version)

	// Registration status
	if cfg.IsRegistered() {
		fmt.Println("Registration: Registered")
	} else {
		fmt.Println("Registration: Not registered")
	}

	// System information
	sysInfo, err := system.GetSystemInfo(version.Version)
	if err == nil {
		fmt.Printf("Hostname: %s\n", sysInfo.Hostname)
		fmt.Printf("OS: %s %s (%s)\n", sysInfo.OSType, sysInfo.OSVersion, sysInfo.OSArchitecture)
		fmt.Printf("Uptime: %s\n", sysInfo.Uptime)
	}

	// Cache status
	localCache, err := cache.Load()
	if err == nil && !localCache.LastScanTime.IsZero() {
		fmt.Printf("Last Scan: %s\n", localCache.LastScanTime.Format(time.RFC3339))
		fmt.Printf("Updates Available: %d\n", len(localCache.Updates))
	}

	fmt.Println("==================================================================")
	return nil
}

// ListUpdatesCommand lists detailed update information
func ListUpdatesCommand(cfg *config.Config, exportFormat string) error {
	// Load cache to get last scan results
	localCache, err := cache.Load()
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	if localCache.LastScanTime.IsZero() {
		fmt.Println("No scan results available. Run -scan first.")
		return nil
	}

	updates := localCache.Updates
	if len(updates) == 0 {
		fmt.Println("No updates available. System is up to date!")
		return nil
	}

	// Handle export formats
	switch exportFormat {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(updates)

	case "csv":
		writer := csv.NewWriter(os.Stdout)
		// Write header
		writer.Write([]string{"Name", "Current Version", "Available Version", "Source", "Severity"})
		// Write data
		for _, u := range updates {
			writer.Write([]string{u.PackageName, u.CurrentVersion, u.AvailableVersion, u.RepositorySource, u.Severity})
		}
		writer.Flush()
		return writer.Error()

	default:
		// Table format
		fmt.Printf("Found %d updates:\n\n", len(updates))
		fmt.Printf("%-30s %-20s %-20s %-10s\n", "NAME", "CURRENT", "AVAILABLE", "SOURCE")
		fmt.Println(string(make([]byte, 80)))
		for _, u := range updates {
			fmt.Printf("%-30s %-20s %-20s %-10s\n",
				truncate(u.PackageName, 30),
				truncate(u.CurrentVersion, 20),
				truncate(u.AvailableVersion, 20),
				u.RepositorySource)
		}
	}

	return nil
}

// truncate truncates a string to max length with ellipsis
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
