package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/denisbrodbeck/machineid"
)

// GetMachineID generates a unique machine identifier that persists across reboots
func GetMachineID() (string, error) {
	// Try machineid library first (cross-platform)
	id, err := machineid.ID()
	if err == nil && id != "" {
		// Hash the machine ID for consistency and privacy
		return hashMachineID(id), nil
	}

	// Fallback to OS-specific methods
	switch runtime.GOOS {
	case "linux":
		return getLinuxMachineID()
	case "windows":
		return getWindowsMachineID()
	case "darwin":
		return getDarwinMachineID()
	default:
		return generateGenericMachineID()
	}
}

// hashMachineID creates a consistent hash from machine ID
func hashMachineID(id string) string {
	hash := sha256.Sum256([]byte(id))
	return hex.EncodeToString(hash[:]) // Return full hash for uniqueness
}

// getLinuxMachineID tries multiple sources for Linux machine ID
func getLinuxMachineID() (string, error) {
	// Try /etc/machine-id first (systemd)
	if id, err := os.ReadFile("/etc/machine-id"); err == nil {
		idStr := strings.TrimSpace(string(id))
		if idStr != "" {
			return hashMachineID(idStr), nil
		}
	}

	// Try /var/lib/dbus/machine-id
	if id, err := os.ReadFile("/var/lib/dbus/machine-id"); err == nil {
		idStr := strings.TrimSpace(string(id))
		if idStr != "" {
			return hashMachineID(idStr), nil
		}
	}

	// Try DMI product UUID
	if id, err := os.ReadFile("/sys/class/dmi/id/product_uuid"); err == nil {
		idStr := strings.TrimSpace(string(id))
		if idStr != "" {
			return hashMachineID(idStr), nil
		}
	}

	// ARM: device-tree model, combined with /etc/machine-id when present.
	// ARM hardware has no DMI; device-tree model binds to the hardware and
	// /etc/machine-id distinguishes OS installs (dual-boot gets distinct IDs).
	if id := getDeviceTreeMachineID(); id != "" {
		return hashMachineID(id), nil
	}

	// ARM: SoC serial from /proc/cpuinfo (not all SoCs expose one)
	if serial := getARMSerial(); serial != "" {
		return hashMachineID(serial), nil
	}

	// Try /etc/hostname as last resort
	if hostname, err := os.ReadFile("/etc/hostname"); err == nil {
		hostnameStr := strings.TrimSpace(string(hostname))
		if hostnameStr != "" {
			return hashMachineID(hostnameStr + "-linux-fallback"), nil
		}
	}

	return generateGenericMachineID()
}

// getDeviceTreeMachineID builds an ID source from /proc/device-tree/model
// plus /etc/machine-id. Returns "" when device-tree is absent (x86, containers).
func getDeviceTreeMachineID() string {
	return deviceTreeMachineIDFrom("/proc/device-tree/model", "/etc/machine-id")
}

func deviceTreeMachineIDFrom(modelPath, machineIDPath string) string {
	model, err := os.ReadFile(modelPath)
	if err != nil {
		return ""
	}
	// device-tree strings are NUL-terminated
	modelStr := strings.TrimSpace(strings.Trim(string(model), "\x00"))
	if modelStr == "" {
		return ""
	}
	if id, err := os.ReadFile(machineIDPath); err == nil {
		if idStr := strings.TrimSpace(string(id)); idStr != "" {
			return modelStr + ":" + idStr
		}
	}
	// Hardware-bound but not install-unique; still better than hostname
	return modelStr + ":arm-device"
}

// getARMSerial reads a SoC serial number from /proc/cpuinfo when exposed.
func getARMSerial() string {
	return armSerialFrom("/proc/cpuinfo")
}

func armSerialFrom(cpuinfoPath string) string {
	data, err := os.ReadFile(cpuinfoPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Serial") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		serial := strings.TrimSpace(parts[1])
		// All-zero serials are placeholders, not identifiers
		if serial != "" && strings.Trim(serial, "0") != "" {
			return "arm-serial:" + serial
		}
	}
	return ""
}

// getWindowsMachineID gets Windows machine ID.
// Note: The primary machineid.ID() in GetMachineID() already tried the
// registry key HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid.
// If it failed, retrying returns the same error — go straight to generic.
// On Windows reinstall, MachineGuid changes. Use the rebind endpoint
// (POST /admin/agents/:id/rebind-machine-id) to recover without re-registration.
func getWindowsMachineID() (string, error) {
	return generateGenericMachineID()
}

// getDarwinMachineID gets macOS machine ID
func getDarwinMachineID() (string, error) {
	// Try machineid library platform-specific keys first
	if id, err := machineid.ID(); err == nil && id != "" {
		return hashMachineID(id), nil
	}

	// Fallback to generating generic ID
	return generateGenericMachineID()
}

// generateGenericMachineID creates a fallback machine ID from available system info
func generateGenericMachineID() (string, error) {
	// Combine hostname with other available info
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	// Create a reasonably unique ID from available system info
	idSource := fmt.Sprintf("%s-%s-%s", hostname, runtime.GOOS, runtime.GOARCH)
	return hashMachineID(idSource), nil
}

// GetEmbeddedPublicKey returns the embedded public key fingerprint
// This should be set at build time using ldflags
var EmbeddedPublicKey = "not-set-at-build-time"

// GetPublicKeyFingerprint returns the fingerprint of the embedded public key
func GetPublicKeyFingerprint() string {
	if EmbeddedPublicKey == "not-set-at-build-time" {
		return ""
	}

	// Return first 8 bytes as fingerprint
	if len(EmbeddedPublicKey) >= 16 {
		return EmbeddedPublicKey[:16]
	}
	return EmbeddedPublicKey
}