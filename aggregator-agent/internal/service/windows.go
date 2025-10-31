//go:build windows

package service

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-agent/internal/client"
	"github.com/Fimeg/RedFlag/aggregator-agent/internal/config"
	"github.com/Fimeg/RedFlag/aggregator-agent/internal/installer"
	"github.com/Fimeg/RedFlag/aggregator-agent/internal/scanner"
	"github.com/Fimeg/RedFlag/aggregator-agent/internal/system"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

var (
	elog       debug.Log
	serviceName = "RedFlagAgent"
)

const (
	AgentVersion = "0.1.16" // Enhanced configuration system with proxy support and registration tokens
)

type redflagService struct {
	agent *config.Config
	stop  chan struct{}
}

func (s *redflagService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}

	// Initialize event logging
	var err error
	elog, err = eventlog.Open(serviceName)
	if err != nil {
		log.Printf("Failed to open event log: %v", err)
		elog = debug.New("RedFlagAgent")
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("Starting %s service", serviceName))

	// Create stop channel
	s.stop = make(chan struct{})

	// Start the agent logic in a goroutine
	go s.runAgent()

	// Signal that service is running
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	elog.Info(1, fmt.Sprintf("%s service is now running", serviceName))

	// Handle service control requests
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				elog.Info(1, fmt.Sprintf("Stopping %s service", serviceName))
				changes <- svc.Status{State: svc.StopPending}
				close(s.stop) // Signal agent to stop gracefully
				break loop
			case svc.Pause:
				elog.Info(1, fmt.Sprintf("Pausing %s service", serviceName))
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				elog.Info(1, fmt.Sprintf("Continuing %s service", serviceName))
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				elog.Error(1, fmt.Sprintf("Unexpected control request #%d", c))
			}
		case <-s.stop:
			break loop
		}
	}

	elog.Info(1, fmt.Sprintf("%s service stopped", serviceName))
	changes <- svc.Status{State: svc.Stopped}
	return
}

func (s *redflagService) runAgent() {
	log.Printf("🚩 RedFlag Agent starting in service mode...")
	log.Printf("==================================================================")
	log.Printf("📋 AGENT ID: %s", s.agent.AgentID)
	log.Printf("🌐 SERVER: %s", s.agent.ServerURL)
	log.Printf("⏱️  CHECK-IN INTERVAL: %ds", s.agent.CheckInInterval)
	log.Printf("==================================================================")

	// Initialize API client
	apiClient := client.NewClient(s.agent.ServerURL, s.agent.Token)

	// Initialize scanners
	aptScanner := scanner.NewAPTScanner()
	dnfScanner := scanner.NewDNFScanner()
	dockerScanner, _ := scanner.NewDockerScanner()
	windowsUpdateScanner := scanner.NewWindowsUpdateScanner()
	wingetScanner := scanner.NewWingetScanner()

	// System info tracking
	var lastSystemInfoUpdate time.Time
	const systemInfoUpdateInterval = 1 * time.Hour // Update detailed system info every hour

	// Main check-in loop with service stop handling
	for {
		select {
		case <-s.stop:
			log.Printf("Received stop signal, shutting down gracefully...")
			elog.Info(1, "Agent shutting down gracefully")
			return
		default:
			// Add jitter to prevent thundering herd
			jitter := time.Duration(rand.Intn(30)) * time.Second
			time.Sleep(jitter)

			// Check if we need to send detailed system info update
			if time.Since(lastSystemInfoUpdate) >= systemInfoUpdateInterval {
				log.Printf("Updating detailed system information...")
				if err := s.reportSystemInfo(apiClient); err != nil {
					log.Printf("Failed to report system info: %v\n", err)
					elog.Error(1, fmt.Sprintf("Failed to report system info: %v", err))
				} else {
					lastSystemInfoUpdate = time.Now()
					log.Printf("✓ System information updated\n")
					elog.Info(1, "System information updated successfully")
				}
			}

			log.Printf("Checking in with server...")

			// Collect lightweight system metrics
			sysMetrics, err := system.GetLightweightMetrics()
			var metrics *client.SystemMetrics
			if err == nil {
				metrics = &client.SystemMetrics{
					CPUPercent:    sysMetrics.CPUPercent,
					MemoryPercent: sysMetrics.MemoryPercent,
					MemoryUsedGB:  sysMetrics.MemoryUsedGB,
					MemoryTotalGB: sysMetrics.MemoryTotalGB,
					DiskUsedGB:    sysMetrics.DiskUsedGB,
					DiskTotalGB:   sysMetrics.DiskTotalGB,
					DiskPercent:   sysMetrics.DiskPercent,
					Uptime:        sysMetrics.Uptime,
					Version:       AgentVersion,
				}
			}

			// Add heartbeat status to metrics metadata if available
			if metrics != nil && s.agent.RapidPollingEnabled {
				// Check if rapid polling is still valid
				if time.Now().Before(s.agent.RapidPollingUntil) {
					if metrics.Metadata == nil {
						metrics.Metadata = make(map[string]interface{})
					}
					metrics.Metadata["rapid_polling_enabled"] = true
					metrics.Metadata["rapid_polling_until"] = s.agent.RapidPollingUntil.Format(time.RFC3339)
					metrics.Metadata["rapid_polling_duration_minutes"] = int(time.Until(s.agent.RapidPollingUntil).Minutes())
				} else {
					// Heartbeat expired, disable it
					s.agent.RapidPollingEnabled = false
					s.agent.RapidPollingUntil = time.Time{}
				}
			}

			// Get commands from server (with optional metrics)
			commands, err := apiClient.GetCommands(s.agent.AgentID, metrics)
			if err != nil {
				// Try to renew token if we got a 401 error
				newClient, renewErr := s.renewTokenIfNeeded(apiClient, err)
				if renewErr != nil {
					log.Printf("Check-in unsuccessful and token renewal failed: %v\n", renewErr)
					elog.Error(1, fmt.Sprintf("Check-in failed and token renewal failed: %v", renewErr))
					time.Sleep(time.Duration(s.getCurrentPollingInterval()) * time.Second)
					continue
				}
				// If token was renewed, update client and retry
				if newClient != apiClient {
					log.Printf("🔄 Retrying check-in with renewed token...")
					elog.Info(1, "Retrying check-in with renewed token")
					apiClient = newClient
					commands, err = apiClient.GetCommands(s.agent.AgentID, metrics)
					if err != nil {
						log.Printf("Check-in unsuccessful even after token renewal: %v\n", err)
						elog.Error(1, fmt.Sprintf("Check-in failed after token renewal: %v", err))
						time.Sleep(time.Duration(s.getCurrentPollingInterval()) * time.Second)
						continue
					}
				} else {
					log.Printf("Check-in unsuccessful: %v\n", err)
					elog.Error(1, fmt.Sprintf("Check-in unsuccessful: %v", err))
					time.Sleep(time.Duration(s.getCurrentPollingInterval()) * time.Second)
					continue
				}
			}

			if len(commands) == 0 {
				log.Printf("Check-in successful - no new commands")
				elog.Info(1, "Check-in successful - no new commands")
			} else {
				log.Printf("Check-in successful - received %d command(s)", len(commands))
				elog.Info(1, fmt.Sprintf("Check-in successful - received %d command(s)", len(commands)))
			}

			// Process each command with full implementation
			for _, cmd := range commands {
				log.Printf("Processing command: %s (%s)\n", cmd.Type, cmd.ID)
				elog.Info(1, fmt.Sprintf("Processing command: %s (%s)", cmd.Type, cmd.ID))

				switch cmd.Type {
				case "scan_updates":
					if err := s.handleScanUpdates(apiClient, aptScanner, dnfScanner, dockerScanner, windowsUpdateScanner, wingetScanner, cmd.ID); err != nil {
						log.Printf("Error scanning updates: %v\n", err)
						elog.Error(1, fmt.Sprintf("Error scanning updates: %v", err))
					}
				case "collect_specs":
					log.Println("Spec collection not yet implemented")
				case "dry_run_update":
					if err := s.handleDryRunUpdate(apiClient, cmd.ID, cmd.Params); err != nil {
						log.Printf("Error dry running update: %v\n", err)
						elog.Error(1, fmt.Sprintf("Error dry running update: %v", err))
					}
				case "install_updates":
					if err := s.handleInstallUpdates(apiClient, cmd.ID, cmd.Params); err != nil {
						log.Printf("Error installing updates: %v\n", err)
						elog.Error(1, fmt.Sprintf("Error installing updates: %v", err))
					}
				case "confirm_dependencies":
					if err := s.handleConfirmDependencies(apiClient, cmd.ID, cmd.Params); err != nil {
						log.Printf("Error confirming dependencies: %v\n", err)
						elog.Error(1, fmt.Sprintf("Error confirming dependencies: %v", err))
					}
				case "enable_heartbeat":
					if err := s.handleEnableHeartbeat(apiClient, cmd.ID, cmd.Params); err != nil {
						log.Printf("[Heartbeat] Error enabling heartbeat: %v\n", err)
						elog.Error(1, fmt.Sprintf("Error enabling heartbeat: %v", err))
					}
				case "disable_heartbeat":
					if err := s.handleDisableHeartbeat(apiClient, cmd.ID); err != nil {
						log.Printf("[Heartbeat] Error disabling heartbeat: %v\n", err)
						elog.Error(1, fmt.Sprintf("Error disabling heartbeat: %v", err))
					}
				default:
					log.Printf("Unknown command type: %s\n", cmd.Type)
					elog.Error(1, fmt.Sprintf("Unknown command type: %s", cmd.Type))
				}
			}

			// Wait for next check-in with stop signal checking
			select {
			case <-s.stop:
				log.Printf("Received stop signal during wait, shutting down gracefully...")
				elog.Info(1, "Agent shutting down gracefully during wait period")
				return
			case <-time.After(time.Duration(s.getCurrentPollingInterval()) * time.Second):
				// Continue to next iteration
			}
		}
	}
}

// RunService executes the agent as a Windows service
func RunService(cfg *config.Config) error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open event log: %w", err)
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("Starting %s service", serviceName))

	s := &redflagService{
		agent: cfg,
	}

	// Run as service
	if err := svc.Run(serviceName, s); err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", serviceName, err))
		return fmt.Errorf("service failed: %w", err)
	}

	elog.Info(1, fmt.Sprintf("%s service stopped", serviceName))
	return nil
}

// IsService returns true if running as Windows service
func IsService() bool {
	isService, _ := svc.IsWindowsService()
	return isService
}

// InstallService installs the agent as a Windows service
func InstallService() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
	}

	// Create service with proper configuration
	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName: "RedFlag Update Agent",
		Description: "RedFlag agent for automated system updates and monitoring",
		StartType:   mgr.StartAutomatic,
		Dependencies: []string{"Tcpip", "Dnscache"},
	})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	// Set recovery actions
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 120 * time.Second},
	}, 0); err != nil {
		return fmt.Errorf("failed to set recovery actions: %w", err)
	}

	log.Printf("Service %s installed successfully", serviceName)
	return nil
}

// RemoveService removes the Windows service
func RemoveService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", serviceName)
	}
	defer s.Close()

	// Stop service if running
	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	if status.State != svc.Stopped {
		if _, err := s.Control(svc.Stop); err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}
		log.Printf("Stopping service...")
		time.Sleep(5 * time.Second) // Wait for service to stop
	}

	// Delete service
	if err := s.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	log.Printf("Service %s removed successfully", serviceName)
	return nil
}

// StartService starts the Windows service
func StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", serviceName)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	log.Printf("Service %s started successfully", serviceName)
	return nil
}

// StopService stops the Windows service
func StopService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", serviceName)
	}
	defer s.Close()

	if _, err := s.Control(svc.Stop); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	log.Printf("Service %s stopped successfully", serviceName)
	return nil
}

// ServiceStatus returns the current status of the Windows service
func ServiceStatus() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", serviceName)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	state := "UNKNOWN"
	switch status.State {
	case svc.Stopped:
		state = "STOPPED"
	case svc.StartPending:
		state = "STARTING"
	case svc.Running:
		state = "RUNNING"
	case svc.StopPending:
		state = "STOPPING"
	case svc.Paused:
		state = "PAUSED"
	case svc.PausePending:
		state = "PAUSING"
	case svc.ContinuePending:
		state = "RESUMING"
	}

	log.Printf("Service %s status: %s", serviceName, state)
	return nil
}

// Helper functions - these implement the same functionality as in main.go but adapted for service mode

// getCurrentPollingInterval returns the appropriate polling interval based on rapid mode
func (s *redflagService) getCurrentPollingInterval() int {
	// Check if rapid polling mode is active and not expired
	if s.agent.RapidPollingEnabled && time.Now().Before(s.agent.RapidPollingUntil) {
		return 5 // Rapid polling: 5 seconds
	}

	// Check if rapid polling has expired and clean up
	if s.agent.RapidPollingEnabled && time.Now().After(s.agent.RapidPollingUntil) {
		s.agent.RapidPollingEnabled = false
		s.agent.RapidPollingUntil = time.Time{}
		// Save the updated config to clean up expired rapid mode
		configPath := s.getConfigPath()
		if err := s.agent.Save(configPath); err != nil {
			log.Printf("Warning: Failed to cleanup expired rapid polling mode: %v", err)
		}
	}

	return s.agent.CheckInInterval // Normal polling: 5 minutes (300 seconds) by default
}

// getConfigPath returns the platform-specific config path
func (s *redflagService) getConfigPath() string {
	return "C:\\ProgramData\\RedFlag\\config.json"
}

// renewTokenIfNeeded handles 401 errors by renewing the agent token using refresh token
func (s *redflagService) renewTokenIfNeeded(apiClient *client.Client, err error) (*client.Client, error) {
	if err != nil && strings.Contains(err.Error(), "401 Unauthorized") {
		log.Printf("🔄 Access token expired - attempting renewal with refresh token...")
		elog.Info(1, "Access token expired - attempting renewal with refresh token")

		// Check if we have a refresh token
		if s.agent.RefreshToken == "" {
			log.Printf("❌ No refresh token available - re-registration required")
			elog.Error(1, "No refresh token available - re-registration required")
			return nil, fmt.Errorf("refresh token missing - please re-register agent")
		}

		// Create temporary client without token for renewal
		tempClient := client.NewClient(s.agent.ServerURL, "")

		// Attempt to renew access token using refresh token
		if err := tempClient.RenewToken(s.agent.AgentID, s.agent.RefreshToken); err != nil {
			log.Printf("❌ Refresh token renewal failed: %v", err)
			elog.Error(1, fmt.Sprintf("Refresh token renewal failed: %v", err))
			log.Printf("💡 Refresh token may be expired (>90 days) - re-registration required")
			return nil, fmt.Errorf("refresh token renewal failed: %w - please re-register agent", err)
		}

		// Update config with new access token (agent ID and refresh token stay the same!)
		s.agent.Token = tempClient.GetToken()

		// Save updated config
		configPath := s.getConfigPath()
		if err := s.agent.Save(configPath); err != nil {
			log.Printf("⚠️  Warning: Failed to save renewed access token: %v", err)
			elog.Error(1, fmt.Sprintf("Failed to save renewed access token: %v", err))
		}

		log.Printf("✅ Access token renewed successfully - agent ID maintained: %s", s.agent.AgentID)
		elog.Info(1, fmt.Sprintf("Access token renewed successfully - agent ID maintained: %s", s.agent.AgentID))
		return tempClient, nil
	}

	// Return original client if no 401 error
	return apiClient, nil
}

// reportSystemInfo collects and reports detailed system information to the server
func (s *redflagService) reportSystemInfo(apiClient *client.Client) error {
	// Collect detailed system information
	sysInfo, err := system.GetSystemInfo(AgentVersion)
	if err != nil {
		return fmt.Errorf("failed to get system info: %w", err)
	}

	// Create system info report
	report := client.SystemInfoReport{
		Timestamp:   time.Now(),
		CPUModel:     sysInfo.CPUInfo.ModelName,
		CPUCores:     sysInfo.CPUInfo.Cores,
		CPUThreads:   sysInfo.CPUInfo.Threads,
		MemoryTotal:  sysInfo.MemoryInfo.Total,
		DiskTotal:    uint64(0),
		DiskUsed:     uint64(0),
		IPAddress:    sysInfo.IPAddress,
		Processes:    sysInfo.RunningProcesses,
		Uptime:       sysInfo.Uptime,
		Metadata:     make(map[string]interface{}),
	}

	// Add primary disk info
	if len(sysInfo.DiskInfo) > 0 {
		primaryDisk := sysInfo.DiskInfo[0]
		report.DiskTotal = primaryDisk.Total
		report.DiskUsed = primaryDisk.Used
		report.Metadata["disk_mount"] = primaryDisk.Mountpoint
		report.Metadata["disk_filesystem"] = primaryDisk.Filesystem
	}

	// Add collection timestamp and additional metadata
	report.Metadata["collected_at"] = time.Now().Format(time.RFC3339)
	report.Metadata["hostname"] = sysInfo.Hostname
	report.Metadata["os_type"] = sysInfo.OSType
	report.Metadata["os_version"] = sysInfo.OSVersion
	report.Metadata["os_architecture"] = sysInfo.OSArchitecture

	// Add any existing metadata from system info
	for key, value := range sysInfo.Metadata {
		report.Metadata[key] = value
	}

	// Report to server
	if err := apiClient.ReportSystemInfo(s.agent.AgentID, report); err != nil {
		return fmt.Errorf("failed to report system info: %w", err)
	}

	return nil
}

// Command handling functions - these need to be fully implemented

func (s *redflagService) handleScanUpdates(apiClient *client.Client, aptScanner *scanner.APTScanner, dnfScanner *scanner.DNFScanner, dockerScanner *scanner.DockerScanner, windowsUpdateScanner *scanner.WindowsUpdateScanner, wingetScanner *scanner.WingetScanner, commandID string) error {
	log.Println("Scanning for updates...")
	elog.Info(1, "Starting update scan")

	var allUpdates []client.UpdateReportItem
	var scanErrors []string
	var scanResults []string

	// Scan APT updates
	if aptScanner.IsAvailable() {
		log.Println("  - Scanning APT packages...")
		updates, err := aptScanner.Scan()
		if err != nil {
			errorMsg := fmt.Sprintf("APT scan failed: %v", err)
			log.Printf("    %s\n", errorMsg)
			elog.Error(1, errorMsg)
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d APT updates", len(updates))
			log.Printf("    %s\n", resultMsg)
			elog.Info(1, resultMsg)
			scanResults = append(scanResults, resultMsg)
			allUpdates = append(allUpdates, updates...)
		}
	} else {
		scanResults = append(scanResults, "APT scanner not available")
	}

	// Scan DNF updates
	if dnfScanner.IsAvailable() {
		log.Println("  - Scanning DNF packages...")
		updates, err := dnfScanner.Scan()
		if err != nil {
			errorMsg := fmt.Sprintf("DNF scan failed: %v", err)
			log.Printf("    %s\n", errorMsg)
			elog.Error(1, errorMsg)
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d DNF updates", len(updates))
			log.Printf("    %s\n", resultMsg)
			elog.Info(1, resultMsg)
			scanResults = append(scanResults, resultMsg)
			allUpdates = append(allUpdates, updates...)
		}
	} else {
		scanResults = append(scanResults, "DNF scanner not available")
	}

	// Scan Docker updates
	if dockerScanner != nil && dockerScanner.IsAvailable() {
		log.Println("  - Scanning Docker images...")
		updates, err := dockerScanner.Scan()
		if err != nil {
			errorMsg := fmt.Sprintf("Docker scan failed: %v", err)
			log.Printf("    %s\n", errorMsg)
			elog.Error(1, errorMsg)
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d Docker image updates", len(updates))
			log.Printf("    %s\n", resultMsg)
			elog.Info(1, resultMsg)
			scanResults = append(scanResults, resultMsg)
			allUpdates = append(allUpdates, updates...)
		}
	} else {
		scanResults = append(scanResults, "Docker scanner not available")
	}

	// Scan Windows updates
	if windowsUpdateScanner.IsAvailable() {
		log.Println("  - Scanning Windows updates...")
		updates, err := windowsUpdateScanner.Scan()
		if err != nil {
			errorMsg := fmt.Sprintf("Windows Update scan failed: %v", err)
			log.Printf("    %s\n", errorMsg)
			elog.Error(1, errorMsg)
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d Windows updates", len(updates))
			log.Printf("    %s\n", resultMsg)
			elog.Info(1, resultMsg)
			scanResults = append(scanResults, resultMsg)
			allUpdates = append(allUpdates, updates...)
		}
	} else {
		scanResults = append(scanResults, "Windows Update scanner not available")
	}

	// Scan Winget packages
	if wingetScanner.IsAvailable() {
		log.Println("  - Scanning Winget packages...")
		updates, err := wingetScanner.Scan()
		if err != nil {
			errorMsg := fmt.Sprintf("Winget scan failed: %v", err)
			log.Printf("    %s\n", errorMsg)
			elog.Error(1, errorMsg)
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d Winget package updates", len(updates))
			log.Printf("    %s\n", resultMsg)
			elog.Info(1, resultMsg)
			scanResults = append(scanResults, resultMsg)
			allUpdates = append(allUpdates, updates...)
		}
	} else {
		scanResults = append(scanResults, "Winget scanner not available")
	}

	// Report scan results to server (both successes and failures)
	success := len(allUpdates) > 0 || len(scanErrors) == 0
	var combinedOutput string

	// Combine all scan results
	if len(scanResults) > 0 {
		combinedOutput += "Scan Results:\n" + strings.Join(scanResults, "\n")
	}
	if len(scanErrors) > 0 {
		if combinedOutput != "" {
			combinedOutput += "\n"
		}
		combinedOutput += "Scan Errors:\n" + strings.Join(scanErrors, "\n")
	}
	if len(allUpdates) > 0 {
		if combinedOutput != "" {
			combinedOutput += "\n"
		}
		combinedOutput += fmt.Sprintf("Total Updates Found: %d", len(allUpdates))
	}

	// Create scan log entry
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "scan_updates",
		Result:          map[bool]string{true: "success", false: "failure"}[success],
		Stdout:          combinedOutput,
		Stderr:          strings.Join(scanErrors, "\n"),
		ExitCode:        map[bool]int{true: 0, false: 1}[success],
		DurationSeconds: 0, // Could track scan duration if needed
	}

	// Report the scan log
	if err := apiClient.ReportLog(s.agent.AgentID, logReport); err != nil {
		log.Printf("Failed to report scan log: %v\n", err)
		elog.Error(1, fmt.Sprintf("Failed to report scan log: %v", err))
		// Continue anyway - updates are more important
	}

	// Report updates to server if any were found
	if len(allUpdates) > 0 {
		report := client.UpdateReport{
			CommandID: commandID,
			Timestamp: time.Now(),
			Updates:   allUpdates,
		}

		if err := apiClient.ReportUpdates(s.agent.AgentID, report); err != nil {
			return fmt.Errorf("failed to report updates: %w", err)
		}

		log.Printf("✓ Reported %d updates to server\n", len(allUpdates))
		elog.Info(1, fmt.Sprintf("Reported %d updates to server", len(allUpdates)))
	} else {
		log.Println("✓ No updates found")
		elog.Info(1, "No updates found")
	}

	// Return error if there were any scan failures
	if len(scanErrors) > 0 && len(allUpdates) == 0 {
		return fmt.Errorf("all scanners failed: %s", strings.Join(scanErrors, "; "))
	}

	return nil
}

func (s *redflagService) handleDryRunUpdate(apiClient *client.Client, commandID string, params map[string]interface{}) error {
	log.Println("Performing dry run update...")
	elog.Info(1, "Starting dry run update")

	// Parse parameters
	packageType := ""
	packageName := ""

	if pt, ok := params["package_type"].(string); ok {
		packageType = pt
	}
	if pn, ok := params["package_name"].(string); ok {
		packageName = pn
	}

	// Validate parameters
	if packageType == "" || packageName == "" {
		err := fmt.Errorf("package_type and package_name parameters are required")
		elog.Error(1, err.Error())
		return err
	}

	// Create installer based on package type
	inst, err := installer.InstallerFactory(packageType)
	if err != nil {
		err := fmt.Errorf("failed to create installer for package type %s: %w", packageType, err)
		elog.Error(1, err.Error())
		return err
	}

	// Check if installer is available
	if !inst.IsAvailable() {
		err := fmt.Errorf("%s installer is not available on this system", packageType)
		elog.Error(1, err.Error())
		return err
	}

	// Perform dry run
	log.Printf("Dry running package: %s (type: %s)", packageName, packageType)
	elog.Info(1, fmt.Sprintf("Dry running package: %s (type: %s)", packageName, packageType))

	result, err := inst.DryRun(packageName)
	if err != nil {
		// Report dry run failure
		logReport := client.LogReport{
			CommandID:       commandID,
			Action:          "dry_run",
			Result:          "failed",
			Stdout:          "",
			Stderr:          fmt.Sprintf("Dry run error: %v", err),
			ExitCode:        1,
			DurationSeconds: 0,
		}

		if reportErr := apiClient.ReportLog(s.agent.AgentID, logReport); reportErr != nil {
			log.Printf("Failed to report dry run failure: %v\n", reportErr)
			elog.Error(1, fmt.Sprintf("Failed to report dry run failure: %v", reportErr))
		}

		return fmt.Errorf("dry run failed: %w", err)
	}

	// Convert installer.InstallResult to client.InstallResult for reporting
	clientResult := &client.InstallResult{
		Success:          result.Success,
		ErrorMessage:     result.ErrorMessage,
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		DurationSeconds: result.DurationSeconds,
		Action:          result.Action,
		PackagesInstalled: result.PackagesInstalled,
		ContainersUpdated: result.ContainersUpdated,
		Dependencies:    result.Dependencies,
		IsDryRun:        true,
	}

	// Report dependencies back to server
	depReport := client.DependencyReport{
		PackageName:   packageName,
		PackageType:   packageType,
		Dependencies:  result.Dependencies,
		UpdateID:      params["update_id"].(string),
		DryRunResult:  clientResult,
	}

	if reportErr := apiClient.ReportDependencies(s.agent.AgentID, depReport); reportErr != nil {
		log.Printf("Failed to report dependencies: %v\n", reportErr)
		elog.Error(1, fmt.Sprintf("Failed to report dependencies: %v", reportErr))
		return fmt.Errorf("failed to report dependencies: %w", reportErr)
	}

	// Report dry run success
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "dry_run",
		Result:          "success",
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		DurationSeconds: result.DurationSeconds,
	}

	if len(result.Dependencies) > 0 {
		logReport.Stdout += fmt.Sprintf("\nDependencies found: %v", result.Dependencies)
	}

	if reportErr := apiClient.ReportLog(s.agent.AgentID, logReport); reportErr != nil {
		log.Printf("Failed to report dry run success: %v\n", reportErr)
		elog.Error(1, fmt.Sprintf("Failed to report dry run success: %v", reportErr))
	}

	if result.Success {
		log.Printf("✓ Dry run completed successfully in %d seconds\n", result.DurationSeconds)
		elog.Info(1, fmt.Sprintf("Dry run completed successfully in %d seconds", result.DurationSeconds))
		if len(result.Dependencies) > 0 {
			log.Printf("  Dependencies found: %v\n", result.Dependencies)
			elog.Info(1, fmt.Sprintf("Dependencies found: %v", result.Dependencies))
		} else {
			log.Printf("  No additional dependencies found\n")
			elog.Info(1, "No additional dependencies found")
		}
	} else {
		log.Printf("✗ Dry run failed after %d seconds\n", result.DurationSeconds)
		elog.Error(1, fmt.Sprintf("Dry run failed after %d seconds: %s", result.DurationSeconds, result.ErrorMessage))
	}

	return nil
}

func (s *redflagService) handleInstallUpdates(apiClient *client.Client, commandID string, params map[string]interface{}) error {
	log.Println("Installing updates...")
	elog.Info(1, "Starting update installation")

	// Parse parameters
	packageType := ""
	packageName := ""

	if pt, ok := params["package_type"].(string); ok {
		packageType = pt
	}
	if pn, ok := params["package_name"].(string); ok {
		packageName = pn
	}

	// Validate package type
	if packageType == "" {
		err := fmt.Errorf("package_type parameter is required")
		elog.Error(1, err.Error())
		return err
	}

	// Create installer based on package type
	inst, err := installer.InstallerFactory(packageType)
	if err != nil {
		err := fmt.Errorf("failed to create installer for package type %s: %w", packageType, err)
		elog.Error(1, err.Error())
		return err
	}

	// Check if installer is available
	if !inst.IsAvailable() {
		err := fmt.Errorf("%s installer is not available on this system", packageType)
		elog.Error(1, err.Error())
		return err
	}

	var result *installer.InstallResult
	var action string

	// Perform installation based on what's specified
	if packageName != "" {
		action = "update"
		log.Printf("Updating package: %s (type: %s)", packageName, packageType)
		elog.Info(1, fmt.Sprintf("Updating package: %s (type: %s)", packageName, packageType))
		result, err = inst.UpdatePackage(packageName)
	} else if len(params) > 1 {
		// Multiple packages might be specified in various ways
		var packageNames []string
		for key, value := range params {
			if key != "package_type" {
				if name, ok := value.(string); ok && name != "" {
					packageNames = append(packageNames, name)
				}
			}
		}
		if len(packageNames) > 0 {
			action = "install_multiple"
			log.Printf("Installing multiple packages: %v (type: %s)", packageNames, packageType)
			elog.Info(1, fmt.Sprintf("Installing multiple packages: %v (type: %s)", packageNames, packageType))
			result, err = inst.InstallMultiple(packageNames)
		} else {
			// Upgrade all packages if no specific packages named
			action = "upgrade"
			log.Printf("Upgrading all packages (type: %s)", packageType)
			elog.Info(1, fmt.Sprintf("Upgrading all packages (type: %s)", packageType))
			result, err = inst.Upgrade()
		}
	} else {
		// Upgrade all packages if no specific packages named
		action = "upgrade"
		log.Printf("Upgrading all packages (type: %s)", packageType)
		elog.Info(1, fmt.Sprintf("Upgrading all packages (type: %s)", packageType))
		result, err = inst.Upgrade()
	}

	if err != nil {
		// Report installation failure with actual command output
		logReport := client.LogReport{
			CommandID:       commandID,
			Action:          action,
			Result:          "failed",
			Stdout:          result.Stdout,
			Stderr:          result.Stderr,
			ExitCode:        result.ExitCode,
			DurationSeconds: result.DurationSeconds,
		}

		if reportErr := apiClient.ReportLog(s.agent.AgentID, logReport); reportErr != nil {
			log.Printf("Failed to report installation failure: %v\n", reportErr)
			elog.Error(1, fmt.Sprintf("Failed to report installation failure: %v", reportErr))
		}

		return fmt.Errorf("installation failed: %w", err)
	}

	// Report installation success
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          result.Action,
		Result:          "success",
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		DurationSeconds: result.DurationSeconds,
	}

	// Add additional metadata to the log report
	if len(result.PackagesInstalled) > 0 {
		logReport.Stdout += fmt.Sprintf("\nPackages installed: %v", result.PackagesInstalled)
	}

	if reportErr := apiClient.ReportLog(s.agent.AgentID, logReport); reportErr != nil {
		log.Printf("Failed to report installation success: %v\n", reportErr)
		elog.Error(1, fmt.Sprintf("Failed to report installation success: %v", reportErr))
	}

	if result.Success {
		log.Printf("✓ Installation completed successfully in %d seconds\n", result.DurationSeconds)
		elog.Info(1, fmt.Sprintf("Installation completed successfully in %d seconds", result.DurationSeconds))
		if len(result.PackagesInstalled) > 0 {
			log.Printf("  Packages installed: %v\n", result.PackagesInstalled)
			elog.Info(1, fmt.Sprintf("Packages installed: %v", result.PackagesInstalled))
		}
	} else {
		log.Printf("✗ Installation failed after %d seconds\n", result.DurationSeconds)
		elog.Error(1, fmt.Sprintf("Installation failed after %d seconds: %s", result.DurationSeconds, result.ErrorMessage))
	}

	return nil
}

func (s *redflagService) handleConfirmDependencies(apiClient *client.Client, commandID string, params map[string]interface{}) error {
	log.Println("Installing update with confirmed dependencies...")
	elog.Info(1, "Starting dependency confirmation installation")

	// Parse parameters
	packageType := ""
	packageName := ""
	var dependencies []string

	if pt, ok := params["package_type"].(string); ok {
		packageType = pt
	}
	if pn, ok := params["package_name"].(string); ok {
		packageName = pn
	}
	if deps, ok := params["dependencies"].([]interface{}); ok {
		for _, dep := range deps {
			if depStr, ok := dep.(string); ok {
				dependencies = append(dependencies, depStr)
			}
		}
	}

	// Validate parameters
	if packageType == "" || packageName == "" {
		err := fmt.Errorf("package_type and package_name parameters are required")
		elog.Error(1, err.Error())
		return err
	}

	// Create installer based on package type
	inst, err := installer.InstallerFactory(packageType)
	if err != nil {
		err := fmt.Errorf("failed to create installer for package type %s: %w", packageType, err)
		elog.Error(1, err.Error())
		return err
	}

	// Check if installer is available
	if !inst.IsAvailable() {
		err := fmt.Errorf("%s installer is not available on this system", packageType)
		elog.Error(1, err.Error())
		return err
	}

	var result *installer.InstallResult
	var action string

	// Perform installation with dependencies
	if len(dependencies) > 0 {
		action = "install_with_dependencies"
		log.Printf("Installing package with dependencies: %s (dependencies: %v)", packageName, dependencies)
		elog.Info(1, fmt.Sprintf("Installing package with dependencies: %s (dependencies: %v)", packageName, dependencies))
		// Install main package + dependencies
		allPackages := append([]string{packageName}, dependencies...)
		result, err = inst.InstallMultiple(allPackages)
	} else {
		action = "upgrade"
		log.Printf("Installing package: %s (no dependencies)", packageName)
		elog.Info(1, fmt.Sprintf("Installing package: %s (no dependencies)", packageName))
		// Use UpdatePackage instead of Install to handle existing packages
		result, err = inst.UpdatePackage(packageName)
	}

	if err != nil {
		// Report installation failure with actual command output
		logReport := client.LogReport{
			CommandID:       commandID,
			Action:          action,
			Result:          "failed",
			Stdout:          result.Stdout,
			Stderr:          result.Stderr,
			ExitCode:        result.ExitCode,
			DurationSeconds: result.DurationSeconds,
		}

		if reportErr := apiClient.ReportLog(s.agent.AgentID, logReport); reportErr != nil {
			log.Printf("Failed to report installation failure: %v\n", reportErr)
			elog.Error(1, fmt.Sprintf("Failed to report installation failure: %v", reportErr))
		}

		return fmt.Errorf("installation failed: %w", err)
	}

	// Report installation success
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          result.Action,
		Result:          "success",
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		DurationSeconds: result.DurationSeconds,
	}

	// Add additional metadata to the log report
	if len(result.PackagesInstalled) > 0 {
		logReport.Stdout += fmt.Sprintf("\nPackages installed: %v", result.PackagesInstalled)
	}
	if len(dependencies) > 0 {
		logReport.Stdout += fmt.Sprintf("\nDependencies included: %v", dependencies)
	}

	if reportErr := apiClient.ReportLog(s.agent.AgentID, logReport); reportErr != nil {
		log.Printf("Failed to report installation success: %v\n", reportErr)
		elog.Error(1, fmt.Sprintf("Failed to report installation success: %v", reportErr))
	}

	if result.Success {
		log.Printf("✓ Installation with dependencies completed successfully in %d seconds\n", result.DurationSeconds)
		elog.Info(1, fmt.Sprintf("Installation with dependencies completed successfully in %d seconds", result.DurationSeconds))
		if len(result.PackagesInstalled) > 0 {
			log.Printf("  Packages installed: %v\n", result.PackagesInstalled)
			elog.Info(1, fmt.Sprintf("Packages installed: %v", result.PackagesInstalled))
		}
	} else {
		log.Printf("✗ Installation with dependencies failed after %d seconds\n", result.DurationSeconds)
		elog.Error(1, fmt.Sprintf("Installation with dependencies failed after %d seconds: %s", result.DurationSeconds, result.ErrorMessage))
	}

	return nil
}

func (s *redflagService) handleEnableHeartbeat(apiClient *client.Client, commandID string, params map[string]interface{}) error {
	log.Printf("[Heartbeat] Enabling rapid polling with params: %v", params)

	// Parse duration parameter (default 60 minutes)
	durationMinutes := 60
	if duration, ok := params["duration_minutes"].(float64); ok {
		durationMinutes = int(duration)
	}

	// Update agent config
	s.agent.RapidPollingEnabled = true
	s.agent.RapidPollingUntil = time.Now().Add(time.Duration(durationMinutes) * time.Minute)

	// Save config
	configPath := s.getConfigPath()
	if err := s.agent.Save(configPath); err != nil {
		log.Printf("[Heartbeat] Warning: Failed to save config: %v", err)
	}

	// Create log report
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "enable_heartbeat",
		Result:          "success",
		Stdout:          fmt.Sprintf("Heartbeat enabled for %d minutes", durationMinutes),
		Stderr:          "",
		ExitCode:        0,
		DurationSeconds: 0,
	}

	if err := apiClient.ReportLog(s.agent.AgentID, logReport); err != nil {
		log.Printf("[Heartbeat] Failed to report heartbeat enable: %v", err)
	}

	// Send immediate check-in to update heartbeat status in UI
	log.Printf("[Heartbeat] Sending immediate check-in to update status")
	sysMetrics, err := system.GetLightweightMetrics()
	if err == nil {
		metrics := &client.SystemMetrics{
			CPUPercent:    sysMetrics.CPUPercent,
			MemoryPercent: sysMetrics.MemoryPercent,
			MemoryUsedGB:  sysMetrics.MemoryUsedGB,
			MemoryTotalGB: sysMetrics.MemoryTotalGB,
			DiskUsedGB:    sysMetrics.DiskUsedGB,
			DiskTotalGB:   sysMetrics.DiskTotalGB,
			DiskPercent:   sysMetrics.DiskPercent,
			Uptime:        sysMetrics.Uptime,
			Version:       AgentVersion,
		}

		// Include heartbeat metadata
		metrics.Metadata = map[string]interface{}{
			"rapid_polling_enabled": true,
			"rapid_polling_until":   s.agent.RapidPollingUntil.Format(time.RFC3339),
		}

		// Send immediate check-in with updated heartbeat status
		_, checkinErr := apiClient.GetCommands(s.agent.AgentID, metrics)
		if checkinErr != nil {
			log.Printf("[Heartbeat] Failed to send immediate check-in: %v", checkinErr)
		} else {
			log.Printf("[Heartbeat] Immediate check-in sent successfully")
		}
	}

	log.Printf("[Heartbeat] Rapid polling enabled successfully")
	return nil
}

func (s *redflagService) handleDisableHeartbeat(apiClient *client.Client, commandID string) error {
	log.Printf("[Heartbeat] Disabling rapid polling")

	// Update agent config to disable rapid polling
	s.agent.RapidPollingEnabled = false
	s.agent.RapidPollingUntil = time.Time{} // Zero value

	// Save config
	configPath := s.getConfigPath()
	if err := s.agent.Save(configPath); err != nil {
		log.Printf("[Heartbeat] Warning: Failed to save config: %v", err)
	}

	// Create log report
	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "disable_heartbeat",
		Result:          "success",
		Stdout:          "Heartbeat disabled",
		Stderr:          "",
		ExitCode:        0,
		DurationSeconds: 0,
	}

	if err := apiClient.ReportLog(s.agent.AgentID, logReport); err != nil {
		log.Printf("[Heartbeat] Failed to report heartbeat disable: %v", err)
	}

	// Send immediate check-in to update heartbeat status in UI
	log.Printf("[Heartbeat] Sending immediate check-in to update status")
	sysMetrics, err := system.GetLightweightMetrics()
	if err == nil {
		metrics := &client.SystemMetrics{
			CPUPercent:    sysMetrics.CPUPercent,
			MemoryPercent: sysMetrics.MemoryPercent,
			MemoryUsedGB:  sysMetrics.MemoryUsedGB,
			MemoryTotalGB: sysMetrics.MemoryTotalGB,
			DiskUsedGB:    sysMetrics.DiskUsedGB,
			DiskTotalGB:   sysMetrics.DiskTotalGB,
			DiskPercent:   sysMetrics.DiskPercent,
			Uptime:        sysMetrics.Uptime,
			Version:       AgentVersion,
		}

		// Include empty heartbeat metadata to explicitly show disabled state
		metrics.Metadata = map[string]interface{}{
			"rapid_polling_enabled": false,
			"rapid_polling_until":   "",
		}

		// Send immediate check-in with updated heartbeat status
		_, checkinErr := apiClient.GetCommands(s.agent.AgentID, metrics)
		if checkinErr != nil {
			log.Printf("[Heartbeat] Failed to send immediate check-in: %v", checkinErr)
		} else {
			log.Printf("[Heartbeat] Immediate check-in sent successfully")
		}
	}

	log.Printf("[Heartbeat] Rapid polling disabled successfully")
	return nil
}

// RunConsole runs the agent in console mode with signal handling
func RunConsole(cfg *config.Config) error {
	log.Printf("🚩 RedFlag Agent starting in console mode...")
	log.Printf("Press Ctrl+C to stop")

	// Handle console signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create stop channel for graceful shutdown
	stopChan := make(chan struct{})

	// Start agent in goroutine
	go func() {
		defer close(stopChan)
		log.Printf("Agent console mode running...")
		ticker := time.NewTicker(time.Duration(cfg.CheckInInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Printf("Checking in with server...")
			case <-stopChan:
				log.Printf("Shutting down console agent...")
				return
			}
		}
	}()

	// Wait for signal
	<-sigChan
	log.Printf("Received shutdown signal, stopping agent...")

	// Graceful shutdown
	close(stopChan)
	time.Sleep(2 * time.Second) // Allow cleanup

	log.Printf("Agent stopped")
	return nil
}