package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
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
	AgentVersion = "0.1.0"
	ConfigPath   = "/etc/aggregator/config.json"
)

func main() {
	registerCmd := flag.Bool("register", false, "Register agent with server")
	scanCmd := flag.Bool("scan", false, "Scan for updates and display locally")
	statusCmd := flag.Bool("status", false, "Show agent status")
	listUpdatesCmd := flag.Bool("list-updates", false, "List detailed update information")
	serverURL := flag.String("server", "http://localhost:8080", "Server URL")
	exportFormat := flag.String("export", "", "Export format: json, csv")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(ConfigPath)
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Handle registration
	if *registerCmd {
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

	// Get check-in interval from server config
	if interval, ok := resp.Config["check_in_interval"].(float64); ok {
		cfg.CheckInInterval = int(interval)
	} else {
		cfg.CheckInInterval = 300 // Default 5 minutes
	}

	// Save configuration
	return cfg.Save(ConfigPath)
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

	// Main check-in loop
	for {
		// Add jitter to prevent thundering herd
		jitter := time.Duration(rand.Intn(30)) * time.Second
		time.Sleep(jitter)

		log.Println("Checking in with server...")

		// Get commands from server
		commands, err := apiClient.GetCommands(cfg.AgentID)
		if err != nil {
			log.Printf("Error getting commands: %v\n", err)
			time.Sleep(time.Duration(cfg.CheckInInterval) * time.Second)
			continue
		}

		// Process each command
		for _, cmd := range commands {
			log.Printf("Processing command: %s (%s)\n", cmd.Type, cmd.ID)

			switch cmd.Type {
			case "scan_updates":
				if err := handleScanUpdates(apiClient, cfg, aptScanner, dnfScanner, dockerScanner, cmd.ID); err != nil {
					log.Printf("Error scanning updates: %v\n", err)
				}

			case "collect_specs":
				log.Println("Spec collection not yet implemented")

			case "install_updates":
				if err := handleInstallUpdates(apiClient, cfg, cmd.ID, cmd.Params); err != nil {
					log.Printf("Error installing updates: %v\n", err)
				}

			default:
				log.Printf("Unknown command type: %s\n", cmd.Type)
			}
		}

		// Wait for next check-in
		time.Sleep(time.Duration(cfg.CheckInInterval) * time.Second)
	}
}

func handleScanUpdates(apiClient *client.Client, cfg *config.Config, aptScanner *scanner.APTScanner, dnfScanner *scanner.DNFScanner, dockerScanner *scanner.DockerScanner, commandID string) error {
	log.Println("Scanning for updates...")

	var allUpdates []client.UpdateReportItem

	// Scan APT updates
	if aptScanner.IsAvailable() {
		log.Println("  - Scanning APT packages...")
		updates, err := aptScanner.Scan()
		if err != nil {
			log.Printf("    APT scan failed: %v\n", err)
		} else {
			log.Printf("    Found %d APT updates\n", len(updates))
			allUpdates = append(allUpdates, updates...)
		}
	}

	// Scan DNF updates
	if dnfScanner.IsAvailable() {
		log.Println("  - Scanning DNF packages...")
		updates, err := dnfScanner.Scan()
		if err != nil {
			log.Printf("    DNF scan failed: %v\n", err)
		} else {
			log.Printf("    Found %d DNF updates\n", len(updates))
			allUpdates = append(allUpdates, updates...)
		}
	}

	// Scan Docker updates
	if dockerScanner != nil && dockerScanner.IsAvailable() {
		log.Println("  - Scanning Docker images...")
		updates, err := dockerScanner.Scan()
		if err != nil {
			log.Printf("    Docker scan failed: %v\n", err)
		} else {
			log.Printf("    Found %d Docker image updates\n", len(updates))
			allUpdates = append(allUpdates, updates...)
		}
	}

	// Report to server
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

	return nil
}

// handleScanCommand performs a local scan and displays results
func handleScanCommand(cfg *config.Config, exportFormat string) error {
	// Initialize scanners
	aptScanner := scanner.NewAPTScanner()
	dnfScanner := scanner.NewDNFScanner()
	dockerScanner, _ := scanner.NewDockerScanner()

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
	targetVersion := ""

	if pt, ok := params["package_type"].(string); ok {
		packageType = pt
	}
	if pn, ok := params["package_name"].(string); ok {
		packageName = pn
	}
	if tv, ok := params["target_version"].(string); ok {
		targetVersion = tv
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
			if key != "package_type" && key != "target_version" {
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
