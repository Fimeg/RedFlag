package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/aggregator-project/aggregator-agent/internal/cache"
	"github.com/aggregator-project/aggregator-agent/internal/client"
	"github.com/aggregator-project/aggregator-agent/internal/config"
	"github.com/aggregator-project/aggregator-agent/internal/display"
	"github.com/aggregator-project/aggregator-agent/internal/installer"
	"github.com/aggregator-project/aggregator-agent/internal/scanner"
	"github.com/aggregator-project/aggregator-agent/internal/system"
	"github.com/google/uuid"
)

const (
	AgentVersion = "0.1.7" // Windows Update data enrichment: CVEs, MSRC severity, dates, version parsing
)

// getConfigPath returns the platform-specific config path
func getConfigPath() string {
	if runtime.GOOS == "windows" {
		return "C:\\ProgramData\\RedFlag\\config.json"
	}
	return "/etc/aggregator/config.json"
}

// getDefaultServerURL returns the default server URL with environment variable support
func getDefaultServerURL() string {
	// Check environment variable first
	if envURL := os.Getenv("REDFLAG_SERVER_URL"); envURL != "" {
		return envURL
	}

	// Platform-specific defaults
	if runtime.GOOS == "windows" {
		// For Windows, use a placeholder that prompts users to configure
		return "http://REPLACE_WITH_SERVER_IP:8080"
	}
	return "http://localhost:8080"
}

func main() {
	registerCmd := flag.Bool("register", false, "Register agent with server")
	scanCmd := flag.Bool("scan", false, "Scan for updates and display locally")
	statusCmd := flag.Bool("status", false, "Show agent status")
	listUpdatesCmd := flag.Bool("list-updates", false, "List detailed update information")
	serverURL := flag.String("server", getDefaultServerURL(), "Server URL")
	exportFormat := flag.String("export", "", "Export format: json, csv")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(getConfigPath())
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Handle registration
	if *registerCmd {
		// Validate server URL for Windows users
		if runtime.GOOS == "windows" && strings.Contains(*serverURL, "REPLACE_WITH_SERVER_IP") {
			fmt.Println("❌ CONFIGURATION REQUIRED!")
			fmt.Println("==================================================================")
			fmt.Println("Please configure the server URL before registering:")
			fmt.Println("")
			fmt.Println("Option 1 - Use the -server flag:")
			fmt.Printf("   redflag-agent.exe -register -server http://10.10.20.159:8080\n")
			fmt.Println("")
			fmt.Println("Option 2 - Use environment variable:")
			fmt.Println("   set REDFLAG_SERVER_URL=http://10.10.20.159:8080")
			fmt.Println("   redflag-agent.exe -register")
			fmt.Println("")
			fmt.Println("Option 3 - Create a .env file:")
			fmt.Println("   REDFLAG_SERVER_URL=http://10.10.20.159:8080")
			fmt.Println("==================================================================")
			os.Exit(1)
		}

		if err := registerAgent(cfg, *serverURL); err != nil {
			log.Fatal("Registration failed:", err)
		}
		fmt.Println("==================================================================")
		fmt.Println("🎉 AGENT REGISTRATION SUCCESSFUL!")
		fmt.Println("==================================================================")
		fmt.Printf("📋 Agent ID: %s\n", cfg.AgentID)
		fmt.Printf("🌐 Server: %s\n", cfg.ServerURL)
		fmt.Printf("⏱️  Check-in Interval: %ds\n", cfg.CheckInInterval)
		fmt.Println("==================================================================")
		fmt.Println("💡 Save this Agent ID for your records!")
		fmt.Println("🚀 You can now start the agent without flags")
		fmt.Println("")
		return
	}

	// Handle scan command
	if *scanCmd {
		if err := handleScanCommand(cfg, *exportFormat); err != nil {
			log.Fatal("Scan failed:", err)
		}
		return
	}

	// Handle status command
	if *statusCmd {
		if err := handleStatusCommand(cfg); err != nil {
			log.Fatal("Status command failed:", err)
		}
		return
	}

	// Handle list-updates command
	if *listUpdatesCmd {
		if err := handleListUpdatesCommand(cfg, *exportFormat); err != nil {
			log.Fatal("List updates failed:", err)
		}
		return
	}

	// Check if registered
	if !cfg.IsRegistered() {
		log.Fatal("Agent not registered. Run with -register flag first.")
	}

	// Start agent service
	if err := runAgent(cfg); err != nil {
		log.Fatal("Agent failed:", err)
	}
}

func registerAgent(cfg *config.Config, serverURL string) error {
	// Get detailed system information
	sysInfo, err := system.GetSystemInfo(AgentVersion)
	if err != nil {
		log.Printf("Warning: Failed to get detailed system info: %v\n", err)
		// Fall back to basic detection
		hostname, _ := os.Hostname()
		osType, osVersion, osArch := client.DetectSystem()
		sysInfo = &system.SystemInfo{
			Hostname:       hostname,
			OSType:         osType,
			OSVersion:      osVersion,
			OSArchitecture: osArch,
			AgentVersion:   AgentVersion,
			Metadata:       make(map[string]string),
		}
	}

	apiClient := client.NewClient(serverURL, "")

	// Create metadata with system information
	metadata := map[string]string{
		"installation_time": time.Now().Format(time.RFC3339),
	}

	// Add system info to metadata
	if sysInfo.CPUInfo.ModelName != "" {
		metadata["cpu_model"] = sysInfo.CPUInfo.ModelName
	}
	if sysInfo.CPUInfo.Cores > 0 {
		metadata["cpu_cores"] = fmt.Sprintf("%d", sysInfo.CPUInfo.Cores)
	}
	if sysInfo.MemoryInfo.Total > 0 {
		metadata["memory_total"] = fmt.Sprintf("%d", sysInfo.MemoryInfo.Total)
	}
	if sysInfo.RunningProcesses > 0 {
		metadata["processes"] = fmt.Sprintf("%d", sysInfo.RunningProcesses)
	}
	if sysInfo.Uptime != "" {
		metadata["uptime"] = sysInfo.Uptime
	}

	// Add disk information
	for i, disk := range sysInfo.DiskInfo {
		if i == 0 {
			metadata["disk_mount"] = disk.Mountpoint
			metadata["disk_total"] = fmt.Sprintf("%d", disk.Total)
			metadata["disk_used"] = fmt.Sprintf("%d", disk.Used)
			break // Only add primary disk info
		}
	}

	req := client.RegisterRequest{
		Hostname:       sysInfo.Hostname,
		OSType:         sysInfo.OSType,
		OSVersion:      sysInfo.OSVersion,
		OSArchitecture: sysInfo.OSArchitecture,
		AgentVersion:   sysInfo.AgentVersion,
		Metadata:       metadata,
	}

	resp, err := apiClient.Register(req)
	if err != nil {
		return err
	}

	// Update configuration
	cfg.ServerURL = serverURL
	cfg.AgentID = resp.AgentID
	cfg.Token = resp.Token
	cfg.RefreshToken = resp.RefreshToken

	// Get check-in interval from server config
	if interval, ok := resp.Config["check_in_interval"].(float64); ok {
		cfg.CheckInInterval = int(interval)
	} else {
		cfg.CheckInInterval = 300 // Default 5 minutes
	}

	// Save configuration
	return cfg.Save(getConfigPath())
}

// renewTokenIfNeeded handles 401 errors by renewing the agent token using refresh token
func renewTokenIfNeeded(apiClient *client.Client, cfg *config.Config, err error) (*client.Client, error) {
	if err != nil && strings.Contains(err.Error(), "401 Unauthorized") {
		log.Printf("🔄 Access token expired - attempting renewal with refresh token...")

		// Check if we have a refresh token
		if cfg.RefreshToken == "" {
			log.Printf("❌ No refresh token available - re-registration required")
			return nil, fmt.Errorf("refresh token missing - please re-register agent")
		}

		// Create temporary client without token for renewal
		tempClient := client.NewClient(cfg.ServerURL, "")

		// Attempt to renew access token using refresh token
		if err := tempClient.RenewToken(cfg.AgentID, cfg.RefreshToken); err != nil {
			log.Printf("❌ Refresh token renewal failed: %v", err)
			log.Printf("💡 Refresh token may be expired (>90 days) - re-registration required")
			return nil, fmt.Errorf("refresh token renewal failed: %w - please re-register agent", err)
		}

		// Update config with new access token (agent ID and refresh token stay the same!)
		cfg.Token = tempClient.GetToken()

		// Save updated config
		if err := cfg.Save(getConfigPath()); err != nil {
			log.Printf("⚠️  Warning: Failed to save renewed access token: %v", err)
		}

		log.Printf("✅ Access token renewed successfully - agent ID maintained: %s", cfg.AgentID)
		return tempClient, nil
	}

	// Return original client if no 401 error
	return apiClient, nil
}

func runAgent(cfg *config.Config) error {
	log.Printf("🚩 RedFlag Agent v%s starting...\n", AgentVersion)
	log.Printf("==================================================================")
	log.Printf("📋 AGENT ID: %s", cfg.AgentID)
	log.Printf("🌐 SERVER: %s", cfg.ServerURL)
	log.Printf("⏱️  CHECK-IN INTERVAL: %ds", cfg.CheckInInterval)
	log.Printf("==================================================================")
	log.Printf("💡 Tip: Use this Agent ID to identify this agent in the web UI")
	log.Printf("")

	apiClient := client.NewClient(cfg.ServerURL, cfg.Token)

	// Initialize scanners
	aptScanner := scanner.NewAPTScanner()
	dnfScanner := scanner.NewDNFScanner()
	dockerScanner, _ := scanner.NewDockerScanner()
	windowsUpdateScanner := scanner.NewWindowsUpdateScanner()
	wingetScanner := scanner.NewWingetScanner()

	// System info tracking
	var lastSystemInfoUpdate time.Time
	const systemInfoUpdateInterval = 1 * time.Hour // Update detailed system info every hour

	// Main check-in loop
	for {
		// Add jitter to prevent thundering herd
		jitter := time.Duration(rand.Intn(30)) * time.Second
		time.Sleep(jitter)

		// Check if we need to send detailed system info update
		if time.Since(lastSystemInfoUpdate) >= systemInfoUpdateInterval {
			log.Printf("Updating detailed system information...")
			if err := reportSystemInfo(apiClient, cfg); err != nil {
				log.Printf("Failed to report system info: %v\n", err)
			} else {
				lastSystemInfoUpdate = time.Now()
				log.Printf("✓ System information updated\n")
			}
		}

		log.Printf("Checking in with server... (Agent v%s)", AgentVersion)

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

		// Get commands from server (with optional metrics)
		commands, err := apiClient.GetCommands(cfg.AgentID, metrics)
		if err != nil {
			// Try to renew token if we got a 401 error
			newClient, renewErr := renewTokenIfNeeded(apiClient, cfg, err)
			if renewErr != nil {
				log.Printf("Check-in unsuccessful and token renewal failed: %v\n", renewErr)
				time.Sleep(time.Duration(cfg.CheckInInterval) * time.Second)
				continue
			}

			// If token was renewed, update client and retry
			if newClient != apiClient {
				log.Printf("🔄 Retrying check-in with renewed token...")
				apiClient = newClient
				commands, err = apiClient.GetCommands(cfg.AgentID, metrics)
				if err != nil {
					log.Printf("Check-in unsuccessful even after token renewal: %v\n", err)
					time.Sleep(time.Duration(cfg.CheckInInterval) * time.Second)
					continue
				}
			} else {
				log.Printf("Check-in unsuccessful: %v\n", err)
				time.Sleep(time.Duration(cfg.CheckInInterval) * time.Second)
				continue
			}
		}

		if len(commands) == 0 {
			log.Printf("Check-in successful - no new commands")
		} else {
			log.Printf("Check-in successful - received %d command(s)", len(commands))
		}

		// Process each command
		for _, cmd := range commands {
			log.Printf("Processing command: %s (%s)\n", cmd.Type, cmd.ID)

			switch cmd.Type {
			case "scan_updates":
				if err := handleScanUpdates(apiClient, cfg, aptScanner, dnfScanner, dockerScanner, windowsUpdateScanner, wingetScanner, cmd.ID); err != nil {
					log.Printf("Error scanning updates: %v\n", err)
				}

			case "collect_specs":
				log.Println("Spec collection not yet implemented")

			case "dry_run_update":
				if err := handleDryRunUpdate(apiClient, cfg, cmd.ID, cmd.Params); err != nil {
					log.Printf("Error dry running update: %v\n", err)
				}

			case "install_updates":
				if err := handleInstallUpdates(apiClient, cfg, cmd.ID, cmd.Params); err != nil {
					log.Printf("Error installing updates: %v\n", err)
				}

			case "confirm_dependencies":
				if err := handleConfirmDependencies(apiClient, cfg, cmd.ID, cmd.Params); err != nil {
					log.Printf("Error confirming dependencies: %v\n", err)
				}

			default:
				log.Printf("Unknown command type: %s\n", cmd.Type)
			}
		}

		// Wait for next check-in
		time.Sleep(time.Duration(cfg.CheckInInterval) * time.Second)
	}
}

func handleScanUpdates(apiClient *client.Client, cfg *config.Config, aptScanner *scanner.APTScanner, dnfScanner *scanner.DNFScanner, dockerScanner *scanner.DockerScanner, windowsUpdateScanner *scanner.WindowsUpdateScanner, wingetScanner *scanner.WingetScanner, commandID string) error {
	log.Println("Scanning for updates...")

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
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d APT updates", len(updates))
			log.Printf("    %s\n", resultMsg)
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
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d DNF updates", len(updates))
			log.Printf("    %s\n", resultMsg)
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
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d Docker image updates", len(updates))
			log.Printf("    %s\n", resultMsg)
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
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d Windows updates", len(updates))
			log.Printf("    %s\n", resultMsg)
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
			scanErrors = append(scanErrors, errorMsg)
		} else {
			resultMsg := fmt.Sprintf("Found %d Winget package updates", len(updates))
			log.Printf("    %s\n", resultMsg)
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
	if err := apiClient.ReportLog(cfg.AgentID, logReport); err != nil {
		log.Printf("Failed to report scan log: %v\n", err)
		// Continue anyway - updates are more important
	}

	// Report updates to server if any were found
	if len(allUpdates) > 0 {
		report := client.UpdateReport{
			CommandID: commandID,
			Timestamp: time.Now(),
			Updates:   allUpdates,
		}

		if err := apiClient.ReportUpdates(cfg.AgentID, report); err != nil {
			return fmt.Errorf("failed to report updates: %w", err)
		}

		log.Printf("✓ Reported %d updates to server\n", len(allUpdates))
	} else {
		log.Println("✓ No updates found")
	}

	// Return error if there were any scan failures
	if len(scanErrors) > 0 && len(allUpdates) == 0 {
		return fmt.Errorf("all scanners failed: %s", strings.Join(scanErrors, "; "))
	}

	return nil
}

// handleScanCommand performs a local scan and displays results
func handleScanCommand(cfg *config.Config, exportFormat string) error {
	// Initialize scanners
	aptScanner := scanner.NewAPTScanner()
	dnfScanner := scanner.NewDNFScanner()
	dockerScanner, _ := scanner.NewDockerScanner()
	windowsUpdateScanner := scanner.NewWindowsUpdateScanner()
	wingetScanner := scanner.NewWingetScanner()

	fmt.Println("🔍 Scanning for updates...")
	var allUpdates []client.UpdateReportItem

	// Scan APT updates
	if aptScanner.IsAvailable() {
		fmt.Println("  - Scanning APT packages...")
		updates, err := aptScanner.Scan()
		if err != nil {
			fmt.Printf("    ⚠️  APT scan failed: %v\n", err)
		} else {
			fmt.Printf("    ✓ Found %d APT updates\n", len(updates))
			allUpdates = append(allUpdates, updates...)
		}
	}

	// Scan DNF updates
	if dnfScanner.IsAvailable() {
		fmt.Println("  - Scanning DNF packages...")
		updates, err := dnfScanner.Scan()
		if err != nil {
			fmt.Printf("    ⚠️  DNF scan failed: %v\n", err)
		} else {
			fmt.Printf("    ✓ Found %d DNF updates\n", len(updates))
			allUpdates = append(allUpdates, updates...)
		}
	}

	// Scan Docker updates
	if dockerScanner != nil && dockerScanner.IsAvailable() {
		fmt.Println("  - Scanning Docker images...")
		updates, err := dockerScanner.Scan()
		if err != nil {
			fmt.Printf("    ⚠️  Docker scan failed: %v\n", err)
		} else {
			fmt.Printf("    ✓ Found %d Docker image updates\n", len(updates))
			allUpdates = append(allUpdates, updates...)
		}
	}

	// Scan Windows updates
	if windowsUpdateScanner.IsAvailable() {
		fmt.Println("  - Scanning Windows updates...")
		updates, err := windowsUpdateScanner.Scan()
		if err != nil {
			fmt.Printf("    ⚠️  Windows Update scan failed: %v\n", err)
		} else {
			fmt.Printf("    ✓ Found %d Windows updates\n", len(updates))
			allUpdates = append(allUpdates, updates...)
		}
	}

	// Scan Winget packages
	if wingetScanner.IsAvailable() {
		fmt.Println("  - Scanning Winget packages...")
		updates, err := wingetScanner.Scan()
		if err != nil {
			fmt.Printf("    ⚠️  Winget scan failed: %v\n", err)
		} else {
			fmt.Printf("    ✓ Found %d Winget package updates\n", len(updates))
			allUpdates = append(allUpdates, updates...)
		}
	}

	// Load and update cache
	localCache, err := cache.Load()
	if err != nil {
		fmt.Printf("⚠️  Warning: Failed to load cache: %v\n", err)
		localCache = &cache.LocalCache{}
	}

	// Update cache with scan results
	localCache.UpdateScanResults(allUpdates)
	if cfg.IsRegistered() {
		localCache.SetAgentInfo(cfg.AgentID, cfg.ServerURL)
		localCache.SetAgentStatus("online")
	}

	// Save cache
	if err := localCache.Save(); err != nil {
		fmt.Printf("⚠️  Warning: Failed to save cache: %v\n", err)
	}

	// Display results
	fmt.Println()
	return display.PrintScanResults(allUpdates, exportFormat)
}

// handleStatusCommand displays agent status information
func handleStatusCommand(cfg *config.Config) error {
	// Load cache
	localCache, err := cache.Load()
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	// Determine status
	agentStatus := "offline"
	if cfg.IsRegistered() {
		agentStatus = "online"
	}
	if localCache.AgentStatus != "" {
		agentStatus = localCache.AgentStatus
	}

	// Use cached info if available, otherwise use config
	agentID := cfg.AgentID.String()
	if localCache.AgentID != (uuid.UUID{}) {
		agentID = localCache.AgentID.String()
	}

	serverURL := cfg.ServerURL
	if localCache.ServerURL != "" {
		serverURL = localCache.ServerURL
	}

	// Display status
	display.PrintAgentStatus(
		agentID,
		serverURL,
		localCache.LastCheckIn,
		localCache.LastScanTime,
		localCache.UpdateCount,
		agentStatus,
	)

	return nil
}

// handleListUpdatesCommand displays detailed update information
func handleListUpdatesCommand(cfg *config.Config, exportFormat string) error {
	// Load cache
	localCache, err := cache.Load()
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	// Check if we have cached scan results
	if len(localCache.Updates) == 0 {
		fmt.Println("📋 No cached scan results found.")
		fmt.Println("💡 Run '--scan' first to discover available updates.")
		return nil
	}

	// Warn if cache is old
	if localCache.IsExpired(24 * time.Hour) {
		fmt.Printf("⚠️  Scan results are %s old. Run '--scan' for latest results.\n\n",
			formatTimeSince(localCache.LastScanTime))
	}

	// Display detailed results
	return display.PrintDetailedUpdates(localCache.Updates, exportFormat)
}

// handleInstallUpdates handles install_updates command
func handleInstallUpdates(apiClient *client.Client, cfg *config.Config, commandID string, params map[string]interface{}) error {
	log.Println("Installing updates...")

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
		return fmt.Errorf("package_type parameter is required")
	}

	// Create installer based on package type
	inst, err := installer.InstallerFactory(packageType)
	if err != nil {
		return fmt.Errorf("failed to create installer for package type %s: %w", packageType, err)
	}

	// Check if installer is available
	if !inst.IsAvailable() {
		return fmt.Errorf("%s installer is not available on this system", packageType)
	}

	var result *installer.InstallResult
	var action string

	// Perform installation based on what's specified
	if packageName != "" {
		action = "install"
		log.Printf("Installing package: %s (type: %s)", packageName, packageType)
		result, err = inst.Install(packageName)
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
			result, err = inst.InstallMultiple(packageNames)
		} else {
			// Upgrade all packages if no specific packages named
			action = "upgrade"
			log.Printf("Upgrading all packages (type: %s)", packageType)
			result, err = inst.Upgrade()
		}
	} else {
		// Upgrade all packages if no specific packages named
		action = "upgrade"
		log.Printf("Upgrading all packages (type: %s)", packageType)
		result, err = inst.Upgrade()
	}

	if err != nil {
		// Report installation failure
		logReport := client.LogReport{
			CommandID:       commandID,
			Action:          action,
			Result:          "failed",
			Stdout:          "",
			Stderr:          fmt.Sprintf("Installation error: %v", err),
			ExitCode:        1,
			DurationSeconds: 0,
		}

		if reportErr := apiClient.ReportLog(cfg.AgentID, logReport); reportErr != nil {
			log.Printf("Failed to report installation failure: %v\n", reportErr)
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

	if reportErr := apiClient.ReportLog(cfg.AgentID, logReport); reportErr != nil {
		log.Printf("Failed to report installation success: %v\n", reportErr)
	}

	if result.Success {
		log.Printf("✓ Installation completed successfully in %d seconds\n", result.DurationSeconds)
		if len(result.PackagesInstalled) > 0 {
			log.Printf("  Packages installed: %v\n", result.PackagesInstalled)
		}
	} else {
		log.Printf("✗ Installation failed after %d seconds\n", result.DurationSeconds)
		log.Printf("  Error: %s\n", result.ErrorMessage)
	}

	return nil
}

// handleDryRunUpdate handles dry_run_update command
func handleDryRunUpdate(apiClient *client.Client, cfg *config.Config, commandID string, params map[string]interface{}) error {
	log.Println("Performing dry run update...")

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
		return fmt.Errorf("package_type and package_name parameters are required")
	}

	// Create installer based on package type
	inst, err := installer.InstallerFactory(packageType)
	if err != nil {
		return fmt.Errorf("failed to create installer for package type %s: %w", packageType, err)
	}

	// Check if installer is available
	if !inst.IsAvailable() {
		return fmt.Errorf("%s installer is not available on this system", packageType)
	}

	// Perform dry run
	log.Printf("Dry running package: %s (type: %s)", packageName, packageType)
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

		if reportErr := apiClient.ReportLog(cfg.AgentID, logReport); reportErr != nil {
			log.Printf("Failed to report dry run failure: %v\n", reportErr)
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

	if reportErr := apiClient.ReportDependencies(cfg.AgentID, depReport); reportErr != nil {
		log.Printf("Failed to report dependencies: %v\n", reportErr)
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

	if reportErr := apiClient.ReportLog(cfg.AgentID, logReport); reportErr != nil {
		log.Printf("Failed to report dry run success: %v\n", reportErr)
	}

	if result.Success {
		log.Printf("✓ Dry run completed successfully in %d seconds\n", result.DurationSeconds)
		if len(result.Dependencies) > 0 {
			log.Printf("  Dependencies found: %v\n", result.Dependencies)
		} else {
			log.Printf("  No additional dependencies found\n")
		}
	} else {
		log.Printf("✗ Dry run failed after %d seconds\n", result.DurationSeconds)
		log.Printf("  Error: %s\n", result.ErrorMessage)
	}

	return nil
}

// handleConfirmDependencies handles confirm_dependencies command
func handleConfirmDependencies(apiClient *client.Client, cfg *config.Config, commandID string, params map[string]interface{}) error {
	log.Println("Installing update with confirmed dependencies...")

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
		return fmt.Errorf("package_type and package_name parameters are required")
	}

	// Create installer based on package type
	inst, err := installer.InstallerFactory(packageType)
	if err != nil {
		return fmt.Errorf("failed to create installer for package type %s: %w", packageType, err)
	}

	// Check if installer is available
	if !inst.IsAvailable() {
		return fmt.Errorf("%s installer is not available on this system", packageType)
	}

	var result *installer.InstallResult
	var action string

	// Perform installation with dependencies
	if len(dependencies) > 0 {
		action = "install_with_dependencies"
		log.Printf("Installing package with dependencies: %s (dependencies: %v)", packageName, dependencies)
		// Install main package + dependencies
		allPackages := append([]string{packageName}, dependencies...)
		result, err = inst.InstallMultiple(allPackages)
	} else {
		action = "install"
		log.Printf("Installing package: %s (no dependencies)", packageName)
		result, err = inst.Install(packageName)
	}

	if err != nil {
		// Report installation failure
		logReport := client.LogReport{
			CommandID:       commandID,
			Action:          action,
			Result:          "failed",
			Stdout:          "",
			Stderr:          fmt.Sprintf("Installation error: %v", err),
			ExitCode:        1,
			DurationSeconds: 0,
		}

		if reportErr := apiClient.ReportLog(cfg.AgentID, logReport); reportErr != nil {
			log.Printf("Failed to report installation failure: %v\n", reportErr)
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

	if reportErr := apiClient.ReportLog(cfg.AgentID, logReport); reportErr != nil {
		log.Printf("Failed to report installation success: %v\n", reportErr)
	}

	if result.Success {
		log.Printf("✓ Installation with dependencies completed successfully in %d seconds\n", result.DurationSeconds)
		if len(result.PackagesInstalled) > 0 {
			log.Printf("  Packages installed: %v\n", result.PackagesInstalled)
		}
	} else {
		log.Printf("✗ Installation with dependencies failed after %d seconds\n", result.DurationSeconds)
		log.Printf("  Error: %s\n", result.ErrorMessage)
	}

	return nil
}

// reportSystemInfo collects and reports detailed system information to the server
func reportSystemInfo(apiClient *client.Client, cfg *config.Config) error {
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
	if err := apiClient.ReportSystemInfo(cfg.AgentID, report); err != nil {
		return fmt.Errorf("failed to report system info: %w", err)
	}

	return nil
}

// formatTimeSince formats a duration as "X time ago"
func formatTimeSince(t time.Time) string {
	duration := time.Since(t)
	if duration < time.Minute {
		return fmt.Sprintf("%d seconds ago", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	} else {
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	}
}
