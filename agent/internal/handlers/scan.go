package handlers

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/models"
	"github.com/Fimeg/RedFlag/agent/internal/orchestrator"
	"github.com/Fimeg/RedFlag/agent/internal/scanner"
)

// mapAny extracts a typed value from map[string]interface{} with zero fallback.
func mapAny[T any](v interface{}) T {
	r, _ := v.(T)
	return r
}

// reportLogWithAck reports a command log to the server and tracks it for acknowledgment.
// A log with no command ID (locally triggered scan — no signed server command behind it)
// is reported without ack tracking: there is no command completion to guarantee.
func reportLogWithAck(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, logReport client.LogReport) error {
	if logReport.CommandID == "" {
		return apiClient.ReportLog(cfg.AgentID, logReport)
	}

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

// HandleScanUpdates scans for ALL package updates across all available package managers
// This is the virtual "updates" subsystem that triggers apt, dnf, winget, and windows scans
func HandleScanUpdates(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, commandID string) error {
	log.Println("Scanning for all package updates...")

	ctx := context.Background()
	startTime := time.Now()
	var totalUpdates int
	var scanResults []string
	var localResults []orchestrator.ScanResult
	var errors []string

	// Detect OS and run appropriate scanners
	osType := runtime.GOOS

	// Linux: Try APT first, then DNF
	if osType == "linux" {
		// Try APT
		aptScanner := scanner.NewAPTScanner()
		if aptScanner.IsAvailable() {
			log.Println("[updates] Running APT scan...")
			result, err := orch.ScanSingle(ctx, "apt")
			localResults = append(localResults, result)
			if err != nil {
				errors = append(errors, fmt.Sprintf("APT: %v", err))
			} else {
				scanResults = append(scanResults, fmt.Sprintf("APT: %d updates", len(result.Updates)))
				totalUpdates += len(result.Updates)
				// RECONCILE-001: always report on a successful scan, even 0 updates,
				// so the server can close rows absent from this scan.
				if result.Status == "success" {
					report := client.UpdateReport{
						CommandID:     commandID,
						Timestamp:     time.Now().UTC(),
						Updates:       result.Updates,
						Ecosystem:     "apt",
						ScanSucceeded: true,
					}
					if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
						log.Printf("[WARNING] [agent] [updates] apt_report_failed error=%v", err)
					}
				}
			}
		}

		// Try DNF
		dnfScanner := scanner.NewDNFScanner()
		if dnfScanner.IsAvailable() {
			log.Println("[updates] Running DNF scan...")
			result, err := orch.ScanSingle(ctx, "dnf")
			localResults = append(localResults, result)
			if err != nil {
				errors = append(errors, fmt.Sprintf("DNF: %v", err))
			} else {
				scanResults = append(scanResults, fmt.Sprintf("DNF: %d updates", len(result.Updates)))
				totalUpdates += len(result.Updates)
				// RECONCILE-001: always report on a successful scan, even 0 updates,
				// so the server can close rows absent from this scan.
				if result.Status == "success" {
					report := client.UpdateReport{
						CommandID:     commandID,
						Timestamp:     time.Now().UTC(),
						Updates:       result.Updates,
						Ecosystem:     "dnf",
						ScanSucceeded: true,
					}
					if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
						log.Printf("[WARNING] [agent] [updates] dnf_report_failed error=%v", err)
					}
				}
			}
		}
	}

	// Windows: Try Windows Update and Winget
	if osType == "windows" {
		// Try Windows Update
		windowsScanner := scanner.NewWindowsUpdateScanner()
		if windowsScanner.IsAvailable() {
			log.Println("[updates] Running Windows Update scan...")
			result, err := orch.ScanSingle(ctx, "windows")
			localResults = append(localResults, result)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Windows: %v", err))
			} else {
				scanResults = append(scanResults, fmt.Sprintf("Windows: %d updates", len(result.Updates)))
				totalUpdates += len(result.Updates)
				// Report Windows updates
				if len(result.Updates) > 0 {
					report := client.UpdateReport{
						CommandID: commandID,
						Timestamp: time.Now().UTC(),
						Updates:   result.Updates,
					}
					if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
						log.Printf("[WARNING] [updates] Failed to report Windows updates: %v", err)
					}
				}
			}
		}

		// Try Winget
		wingetScanner := scanner.NewWingetScanner()
		if wingetScanner.IsAvailable() {
			log.Println("[updates] Running Winget scan...")
			result, err := orch.ScanSingle(ctx, "winget")
			localResults = append(localResults, result)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Winget: %v", err))
			} else {
				scanResults = append(scanResults, fmt.Sprintf("Winget: %d updates", len(result.Updates)))
				totalUpdates += len(result.Updates)
				// Report Winget updates
				if len(result.Updates) > 0 {
					report := client.UpdateReport{
						CommandID: commandID,
						Timestamp: time.Now().UTC(),
						Updates:   result.Updates,
					}
					if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
						log.Printf("[WARNING] [updates] Failed to report Winget updates: %v", err)
					}
				}
			}
		}
	}
	recordLocalScanResults(cfg, localResults, true)

	duration := time.Since(startTime)
	stdout := fmt.Sprintf("Package update scan completed in %.2f seconds\n\nResults:\n%s\n\nTotal updates found: %d",
		duration.Seconds(),
		strings.Join(scanResults, "\n"),
		totalUpdates)

	stderr := ""
	exitCode := 0
	if len(errors) > 0 {
		stderr = fmt.Sprintf("Errors encountered:\n%s", strings.Join(errors, "\n"))
		exitCode = 1
	}

	// Create history entry
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_updates",
		Result:          map[bool]string{true: "success", false: "partial_failure"}[exitCode == 0],
		Stdout:          stdout,
		Stderr:          stderr,
		ExitCode:        exitCode,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "Package Updates",
			"subsystem":       "updates",
			"total_updates":   fmt.Sprintf("%d", totalUpdates),
			"scanners_run":    fmt.Sprintf("%d", len(scanResults)),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [updates] report_log_failed: %v", err)
	} else {
		log.Printf("[INFO] [agent] [updates] scan completed: %d updates found", totalUpdates)
	}

	return nil
}

// HandleScanStorage scans disk usage metrics only
func HandleScanStorage(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, commandID string) error {
	log.Println("Scanning storage...")

	ctx := context.Background()
	startTime := time.Now()

	result, err := orch.ScanSingle(ctx, "storage")
	if err != nil {
		return fmt.Errorf("failed to scan storage: %w", err)
	}
	recordLocalScanResult(cfg, result, false)

	results := []orchestrator.ScanResult{result}
	stdout, stderr, exitCode := orchestrator.FormatScanSummary(results)

	duration := time.Since(startTime)
	stdout += fmt.Sprintf("\nStorage scan completed in %.2f seconds\n", duration.Seconds())

	// Report storage metrics to server using dedicated endpoint
	if len(result.Updates) > 0 {
		metricItems := make([]models.StorageMetric, 0, len(result.Updates))
		for _, u := range result.Updates {
			m := u.Metadata
			metricItems = append(metricItems, models.StorageMetric{
				Mountpoint:     u.PackageName,
				Device:         u.RepositorySource,
				DiskType:       mapAny[string](m["disk_type"]),
				Filesystem:     mapAny[string](m["filesystem"]),
				TotalBytes:     mapAny[int64](m["total_bytes"]),
				UsedBytes:      mapAny[int64](m["used_bytes"]),
				AvailableBytes: mapAny[int64](m["available_bytes"]),
				UsedPercent:    mapAny[float64](m["used_percent"]),
				IsRoot:         mapAny[bool](m["is_root"]),
				IsLargest:      mapAny[bool](m["is_largest"]),
				Severity:       u.Severity,
			})
		}

		report := models.StorageMetricReport{
			AgentID:   cfg.AgentID,
			CommandID: commandID,
			Timestamp: time.Now().UTC(),
			Metrics:   metricItems,
		}

		if err := apiClient.ReportStorageMetrics(cfg.AgentID, report); err != nil {
			return fmt.Errorf("failed to report storage metrics: %w", err)
		}

		log.Printf("[INFO] [storage] Successfully reported %d storage metrics to server\n", len(result.Updates))
	}

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_storage",
		Result:          map[bool]string{true: "success", false: "failure"}[exitCode == 0],
		Stdout:          stdout,
		Stderr:          stderr,
		ExitCode:        exitCode,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "Disk Usage",
			"subsystem":       "storage",
			"metrics_count":   fmt.Sprintf("%d", len(result.Updates)),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [storage] report_log_failed: %v", err)
		log.Printf("[HISTORY] [agent] [storage] report_log_failed error=\"%v\" timestamp=%s", err, time.Now().UTC().Format(time.RFC3339))
	} else {
		log.Printf("[INFO] [agent] [storage] history_log_created command_id=%s timestamp=%s", commandID, time.Now().UTC().Format(time.RFC3339))
		log.Printf("[HISTORY] [agent] [scan_storage] log_created agent_id=%s command_id=%s result=%s timestamp=%s", cfg.AgentID, commandID, map[bool]string{true: "success", false: "failure"}[exitCode == 0], time.Now().UTC().Format(time.RFC3339))
	}

	return nil
}

// HandleScanSystem scans system metrics (CPU, memory, processes, uptime)
func HandleScanSystem(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, commandID string) error {
	log.Println("Scanning system metrics...")

	ctx := context.Background()
	startTime := time.Now()

	result, err := orch.ScanSingle(ctx, "system")
	if err != nil {
		return fmt.Errorf("failed to scan system: %w", err)
	}
	recordLocalScanResult(cfg, result, false)

	results := []orchestrator.ScanResult{result}
	stdout, stderr, exitCode := orchestrator.FormatScanSummary(results)

	duration := time.Since(startTime)
	stdout += fmt.Sprintf("\nSystem scan completed in %.2f seconds\n", duration.Seconds())

	// Report system metrics to server using dedicated endpoint
	if len(result.Updates) > 0 {
		metricItems := make([]client.MetricsReportItem, 0, len(result.Updates))
		for _, u := range result.Updates {
			metricItems = append(metricItems, client.MetricsReportItem{
				PackageType:      u.PackageType,
				PackageName:      u.PackageName,
				CurrentVersion:   u.CurrentVersion,
				AvailableVersion: u.AvailableVersion,
				Severity:         u.Severity,
				RepositorySource: u.RepositorySource,
				Metadata:         u.Metadata,
			})
		}

		report := client.MetricsReport{
			CommandID: commandID,
			Timestamp: time.Now().UTC(),
			Metrics:   metricItems,
		}

		if err := apiClient.ReportMetrics(cfg.AgentID, report); err != nil {
			return fmt.Errorf("failed to report system metrics: %w", err)
		}

		log.Printf("[INFO] [agent] [system] Reported %d system metrics to server\n", len(result.Updates))
	}

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_system",
		Result:          map[bool]string{true: "success", false: "failure"}[exitCode == 0],
		Stdout:          stdout,
		Stderr:          stderr,
		ExitCode:        exitCode,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "System Metrics",
			"subsystem":       "system",
			"metrics_count":   fmt.Sprintf("%d", len(result.Updates)),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [system] report_log_failed: %v", err)
		log.Printf("[HISTORY] [agent] [system] report_log_failed error=\"%v\" timestamp=%s", err, time.Now().UTC().Format(time.RFC3339))
	} else {
		log.Printf("[INFO] [agent] [system] history_log_created command_id=%s timestamp=%s", commandID, time.Now().UTC().Format(time.RFC3339))
		log.Printf("[HISTORY] [agent] [scan_system] log_created agent_id=%s command_id=%s result=%s timestamp=%s", cfg.AgentID, commandID, map[bool]string{true: "success", false: "failure"}[exitCode == 0], time.Now().UTC().Format(time.RFC3339))
	}

	return nil
}

// HandleScanDocker scans Docker image inventory and container enrichment data.
// Uses the InventoryScanner path for image inventory, and the existing
// DockerReport path for container/stack/engine enrichment.
func HandleScanDocker(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, commandID string) error {
	log.Println("Scanning Docker images...")

	ctx := context.Background()
	startTime := time.Now()

	// Use inventory scanner path for image data
	invResult, err := orch.ScanInventorySingle(ctx, "docker")
	if err != nil {
		return fmt.Errorf("failed to scan Docker inventory: %w", err)
	}

	duration := time.Since(startTime)

	// Report inventory items to the new inventory endpoint
	if len(invResult.Items) > 0 {
		invReport := client.InventoryReport{
			CommandID:     commandID,
			Timestamp:     time.Now().UTC(),
			Ecosystem:     "docker",
			Items:         invResult.Items,
			ScanSucceeded: invResult.Status == "success",
		}
		if err := apiClient.ReportInventory(cfg.AgentID, invReport); err != nil {
			log.Printf("[WARNING] [agent] [docker] report_inventory_failed error=%v", err)
		} else {
			log.Printf("[INFO] [agent] [docker] Reported %d Docker images to inventory", len(invResult.Items))
		}
	}

	// Collect container/stack enrichment data (existing DockerReport path)
	updateCount := 0
	for _, item := range invResult.Items {
		if hasUpdate, ok := item.Metadata["has_update"].(bool); ok && hasUpdate {
			updateCount++
		}
	}

	dockerScanner, err := orchestrator.NewDockerScanner()
	if err != nil {
		log.Printf("[WARN] [agent] [docker] could not create scanner for enrichment: %v", err)
	} else {
		defer dockerScanner.Close()

		containers, err := dockerScanner.ScanContainers()
		if err != nil {
			log.Printf("[WARN] [agent] [docker] container scan failed: %v", err)
		} else {
			report := client.DockerReport{
				CommandID:     commandID,
				Timestamp:     time.Now().UTC(),
				Containers:    containers,
				Stacks:        dockerScanner.ScanStacks(containers),
				EngineVersion: dockerScanner.GetEngineVersion(),
			}
			if err := apiClient.ReportDockerImages(cfg.AgentID, report); err != nil {
				log.Printf("[WARNING] [agent] [docker] report_enrichment_failed error=%v", err)
			} else {
				log.Printf("[INFO] [agent] [docker] Reported %d containers, %d stacks for enrichment",
					len(report.Containers), len(report.Stacks))
			}
		}
	}

	// Build log report
	stdout := fmt.Sprintf("Docker inventory scan completed in %.2f seconds\n\nItems found: %d\nUpdates available: %d",
		duration.Seconds(), len(invResult.Items), updateCount)
	stderr := ""
	exitCode := 0
	if invResult.Status == "failed" {
		stderr = fmt.Sprintf("Error: %v", invResult.Error)
		exitCode = 1
	}

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_docker",
		Result:          map[bool]string{true: "success", false: "failure"}[exitCode == 0],
		Stdout:          stdout,
		Stderr:          stderr,
		ExitCode:        exitCode,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "Docker Images",
			"subsystem":       "docker",
			"images_count":    fmt.Sprintf("%d", len(invResult.Items)),
			"updates_found":   fmt.Sprintf("%d", updateCount),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [docker] report_log_failed: %v", err)
		log.Printf("[HISTORY] [agent] [docker] report_log_failed error=\"%v\" timestamp=%s", err, time.Now().UTC().Format(time.RFC3339))
	} else {
		log.Printf("[INFO] [agent] [docker] history_log_created command_id=%s timestamp=%s", commandID, time.Now().UTC().Format(time.RFC3339))
		log.Printf("[HISTORY] [agent] [scan_docker] log_created agent_id=%s command_id=%s result=%s timestamp=%s", cfg.AgentID, commandID, map[bool]string{true: "success", false: "failure"}[exitCode == 0], time.Now().UTC().Format(time.RFC3339))
	}

	return nil
}

// HandleScanAPT scans APT package updates only
func HandleScanAPT(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, commandID string) error {
	log.Println("Scanning APT packages...")

	ctx := context.Background()
	startTime := time.Now()

	// Execute APT scanner
	result, err := orch.ScanSingle(ctx, "apt")
	if err != nil {
		return fmt.Errorf("failed to scan APT: %w", err)
	}
	recordLocalScanResult(cfg, result, true)

	// Format results
	results := []orchestrator.ScanResult{result}
	stdout, stderr, exitCode := orchestrator.FormatScanSummary(results)

	duration := time.Since(startTime)
	stdout += fmt.Sprintf("\nAPT scan completed in %.2f seconds\n", duration.Seconds())

	// Report APT updates to server.
	// RECONCILE-001: always report on a successful scan, even when 0 updates are
	// found. An empty reported set + ScanSucceeded=true tells the server this
	// ecosystem is fully patched — it must close all tracked non-resting rows.
	// Declare updates at function scope for ReportLog access.
	var updates []client.UpdateReportItem
	if result.Status == "success" {
		updates = result.Updates
		report := client.UpdateReport{
			CommandID:     commandID,
			Timestamp:     time.Now().UTC(),
			Updates:       updates,
			Ecosystem:     "apt",
			ScanSucceeded: true,
		}

		if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
			log.Printf("[WARNING] [agent] [apt] report_updates_failed error=%v", err)
			// Do not return error here — the scan succeeded locally; the report failure
			// is a transport problem. ReportLog below will still record the scan.
		} else {
			log.Printf("[INFO] [agent] [apt] reported %d APT updates to server", len(updates))
		}
	}

	// Create history entry for unified view with proper formatting
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_apt",
		Result:          map[bool]string{true: "success", false: "failure"}[exitCode == 0],
		Stdout:          stdout,
		Stderr:          stderr,
		ExitCode:        exitCode,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "APT Packages",
			"subsystem":       "apt",
			"updates_found":   fmt.Sprintf("%d", len(updates)),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [apt] report_log_failed: %v", err)
		log.Printf("[HISTORY] [agent] [apt] report_log_failed error=\"%v\" timestamp=%s", err, time.Now().UTC().Format(time.RFC3339))
	} else {
		log.Printf("[INFO] [agent] [apt] history_log_created command_id=%s timestamp=%s", commandID, time.Now().UTC().Format(time.RFC3339))
		log.Printf("[HISTORY] [agent] [scan_apt] log_created agent_id=%s command_id=%s result=%s timestamp=%s", cfg.AgentID, commandID, map[bool]string{true: "success", false: "failure"}[exitCode == 0], time.Now().UTC().Format(time.RFC3339))
	}

	return nil
}

// HandleScanDNF scans DNF package updates only
func HandleScanDNF(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, commandID string) error {
	log.Println("Scanning DNF packages...")

	ctx := context.Background()
	startTime := time.Now()

	// Execute DNF scanner
	result, err := orch.ScanSingle(ctx, "dnf")
	if err != nil {
		return fmt.Errorf("failed to scan DNF: %w", err)
	}
	recordLocalScanResult(cfg, result, true)

	// Format results
	results := []orchestrator.ScanResult{result}
	stdout, stderr, exitCode := orchestrator.FormatScanSummary(results)

	duration := time.Since(startTime)
	stdout += fmt.Sprintf("\nDNF scan completed in %.2f seconds\n", duration.Seconds())

	// Report DNF updates to server.
	// RECONCILE-001: always report on a successful scan, even when 0 updates are
	// found. An empty reported set + ScanSucceeded=true tells the server this
	// ecosystem is fully patched — it must close all tracked non-resting rows.
	// Declare updates at function scope for ReportLog access.
	var updates []client.UpdateReportItem
	if result.Status == "success" {
		updates = result.Updates
		report := client.UpdateReport{
			CommandID:     commandID,
			Timestamp:     time.Now().UTC(),
			Updates:       updates,
			Ecosystem:     "dnf",
			ScanSucceeded: true,
		}

		if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
			log.Printf("[WARNING] [agent] [dnf] report_updates_failed error=%v", err)
			// Do not return error here — the scan succeeded locally; the report failure
			// is a transport problem. ReportLog below will still record the scan.
		} else {
			log.Printf("[INFO] [agent] [dnf] reported %d DNF updates to server", len(updates))
		}
	}

	// Create history entry for unified view with proper formatting
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_dnf",
		Result:          map[bool]string{true: "success", false: "failure"}[exitCode == 0],
		Stdout:          stdout,
		Stderr:          stderr,
		ExitCode:        exitCode,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "DNF Packages",
			"subsystem":       "dnf",
			"updates_found":   fmt.Sprintf("%d", len(updates)),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [dnf] report_log_failed: %v", err)
		log.Printf("[HISTORY] [agent] [dnf] report_log_failed error=\"%v\" timestamp=%s", err, time.Now().UTC().Format(time.RFC3339))
	} else {
		log.Printf("[INFO] [agent] [dnf] history_log_created command_id=%s timestamp=%s", commandID, time.Now().UTC().Format(time.RFC3339))
		log.Printf("[HISTORY] [agent] [scan_dnf] log_created agent_id=%s command_id=%s result=%s timestamp=%s", cfg.AgentID, commandID, map[bool]string{true: "success", false: "failure"}[exitCode == 0], time.Now().UTC().Format(time.RFC3339))
	}

	return nil
}

// HandleScanWindows scans Windows Updates only
func HandleScanWindows(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, commandID string) error {
	log.Println("Scanning Windows Updates...")

	ctx := context.Background()
	startTime := time.Now()

	// Execute Windows Update scanner
	result, err := orch.ScanSingle(ctx, "windows")
	if err != nil {
		return fmt.Errorf("failed to scan Windows Updates: %w", err)
	}
	recordLocalScanResult(cfg, result, true)

	// Format results
	results := []orchestrator.ScanResult{result}
	stdout, stderr, exitCode := orchestrator.FormatScanSummary(results)

	duration := time.Since(startTime)
	stdout += fmt.Sprintf("\nWindows Update scan completed in %.2f seconds\n", duration.Seconds())

	// Report Windows updates to server if any were found
	// Declare updates at function scope for ReportLog access
	var updates []client.UpdateReportItem
	if result.Status == "success" && len(result.Updates) > 0 {
		updates = result.Updates
		report := client.UpdateReport{
			CommandID: commandID,
			Timestamp: time.Now().UTC(),
			Updates:   updates,
		}

		if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
			return fmt.Errorf("failed to report Windows updates: %w", err)
		}

		log.Printf("[INFO] [agent] [windows] Successfully reported %d Windows updates to server\n", len(updates))
	}

	// Create history entry for unified view with proper formatting
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_windows",
		Result:          map[bool]string{true: "success", false: "failure"}[exitCode == 0],
		Stdout:          stdout,
		Stderr:          stderr,
		ExitCode:        exitCode,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "Windows Updates",
			"subsystem":       "windows",
			"updates_found":   fmt.Sprintf("%d", len(updates)),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [windows] report_log_failed: %v", err)
		log.Printf("[HISTORY] [agent] [windows] report_log_failed error=\"%v\" timestamp=%s", err, time.Now().UTC().Format(time.RFC3339))
	} else {
		log.Printf("[INFO] [agent] [windows] history_log_created command_id=%s timestamp=%s", commandID, time.Now().UTC().Format(time.RFC3339))
		log.Printf("[HISTORY] [agent] [scan_windows] log_created agent_id=%s command_id=%s result=%s timestamp=%s", cfg.AgentID, commandID, map[bool]string{true: "success", false: "failure"}[exitCode == 0], time.Now().UTC().Format(time.RFC3339))
	}

	return nil
}

// HandleScanWinget scans Winget package updates only
func HandleScanWinget(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, commandID string) error {
	log.Println("Scanning Winget packages...")

	ctx := context.Background()
	startTime := time.Now()

	// Execute Winget scanner
	result, err := orch.ScanSingle(ctx, "winget")
	if err != nil {
		return fmt.Errorf("failed to scan Winget: %w", err)
	}
	recordLocalScanResult(cfg, result, true)

	// Format results
	results := []orchestrator.ScanResult{result}
	stdout, stderr, exitCode := orchestrator.FormatScanSummary(results)

	duration := time.Since(startTime)
	stdout += fmt.Sprintf("\nWinget scan completed in %.2f seconds\n", duration.Seconds())

	// Report Winget updates to server if any were found
	// Declare updates at function scope for ReportLog access
	var updates []client.UpdateReportItem
	if result.Status == "success" && len(result.Updates) > 0 {
		updates = result.Updates
		report := client.UpdateReport{
			CommandID: commandID,
			Timestamp: time.Now().UTC(),
			Updates:   updates,
		}

		if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
			return fmt.Errorf("failed to report Winget updates: %w", err)
		}

		log.Printf("[INFO] [agent] [winget] Successfully reported %d Winget updates to server\n", len(updates))
	}

	// Create history entry for unified view with proper formatting
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_winget",
		Result:          map[bool]string{true: "success", false: "failure"}[exitCode == 0],
		Stdout:          stdout,
		Stderr:          stderr,
		ExitCode:        exitCode,
		DurationSeconds: int(duration.Seconds()),
		Metadata: map[string]string{
			"subsystem_label": "Winget Packages",
			"subsystem":       "winget",
			"updates_found":   fmt.Sprintf("%d", len(updates)),
		},
	}
	if err := reportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [winget] report_log_failed: %v", err)
		log.Printf("[HISTORY] [agent] [winget] report_log_failed error=\"%v\" timestamp=%s", err, time.Now().UTC().Format(time.RFC3339))
	} else {
		log.Printf("[INFO] [agent] [winget] history_log_created command_id=%s timestamp=%s", commandID, time.Now().UTC().Format(time.RFC3339))
		log.Printf("[HISTORY] [agent] [scan_winget] log_created agent_id=%s command_id=%s result=%s timestamp=%s", cfg.AgentID, commandID, map[bool]string{true: "success", false: "failure"}[exitCode == 0], time.Now().UTC().Format(time.RFC3339))
	}

	return nil
}
