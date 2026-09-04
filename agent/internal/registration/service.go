package registration

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/Fimeg/RedFlag/agent/internal/crypto"
	"github.com/Fimeg/RedFlag/agent/internal/scanner"
	"github.com/Fimeg/RedFlag/agent/internal/system"
	"github.com/Fimeg/RedFlag/agent/internal/version"
)

// RegisterAgent registers the agent with the server
func RegisterAgent(cfg *config.Config, serverURL string) error {
	// Get detailed system information
	sysInfo, err := system.GetSystemInfo(version.Version)
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
			AgentVersion:   version.Version,
			Metadata:       make(map[string]string),
		}
	}

	// Use registration token from config if available
	apiClient := client.NewClient(serverURL, cfg.RegistrationToken)

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

	// Get machine ID for binding
	machineID, err := system.GetMachineID()
	if err != nil {
		return fmt.Errorf("machine_id_unavailable: %w - cannot register without consistent machine ID", err)
	}

	// Get embedded public key fingerprint
	publicKeyFingerprint := system.GetPublicKeyFingerprint()
	if publicKeyFingerprint == "" {
		log.Printf("Warning: No embedded public key fingerprint found")
	}

	// Detect available scanners for platform-specific subsystem creation
	availableScanners := scanner.DetectAvailable()
	log.Printf("[INFO] [agent] [registration] detected_scanners=%v", availableScanners)

	req := client.RegisterRequest{
		Hostname:             sysInfo.Hostname,
		OSType:               sysInfo.OSType,
		OSVersion:            sysInfo.OSVersion,
		OSArchitecture:       sysInfo.OSArchitecture,
		AgentVersion:         sysInfo.AgentVersion,
		MachineID:            machineID,
		PublicKeyFingerprint: publicKeyFingerprint,
		Metadata:             metadata,
		AvailableScanners:    availableScanners,
		DeviceType:           sysInfo.DeviceType,
		DeviceModel:          sysInfo.DeviceModel,
		OSDistro:             sysInfo.OSDistro,
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
	if err := cfg.Save(constants.GetAgentConfigPath()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Fetch and cache server public key for signature verification
	log.Println("Fetching server public key for update signature verification...")
	if err := FetchAndCachePublicKey(cfg.ServerURL); err != nil {
		log.Printf("Warning: Failed to fetch server public key: %v", err)
		log.Printf("Agent will not be able to verify update signatures")
		// Don't fail registration - key can be fetched later
	} else {
		log.Println("[INFO] [agent] [crypto] server_public_key_cached")
	}

	return nil
}

// FetchAndCachePublicKey fetches the server's Ed25519 public key and caches it locally
func FetchAndCachePublicKey(serverURL string) error {
	_, err := crypto.FetchAndCacheServerPublicKey(serverURL)
	return err
}

