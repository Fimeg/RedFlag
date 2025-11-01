package system

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SystemInfo contains detailed system information
type SystemInfo struct {
	Hostname       string            `json:"hostname"`
	OSType         string            `json:"os_type"`
	OSVersion      string            `json:"os_version"`
	OSArchitecture string            `json:"os_architecture"`
	AgentVersion   string            `json:"agent_version"`
	IPAddress      string            `json:"ip_address"`
	CPUInfo        CPUInfo           `json:"cpu_info"`
	MemoryInfo     MemoryInfo        `json:"memory_info"`
	DiskInfo       []DiskInfo        `json:"disk_info"`
	RunningProcesses int             `json:"running_processes"`
	Uptime         string            `json:"uptime"`
	RebootRequired bool              `json:"reboot_required"`
	RebootReason   string            `json:"reboot_reason"`
	Metadata       map[string]string `json:"metadata"`
}

// CPUInfo contains CPU information
type CPUInfo struct {
	ModelName string `json:"model_name"`
	Cores     int    `json:"cores"`
	Threads   int    `json:"threads"`
}

// MemoryInfo contains memory information
type MemoryInfo struct {
	Total     uint64  `json:"total"`
	Available uint64  `json:"available"`
	Used      uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

// DiskInfo contains disk information for modular storage management
type DiskInfo struct {
	Mountpoint    string  `json:"mountpoint"`
	Total         uint64  `json:"total"`
	Available     uint64  `json:"available"`
	Used          uint64  `json:"used"`
	UsedPercent   float64 `json:"used_percent"`
	Filesystem    string  `json:"filesystem"`
	IsRoot        bool    `json:"is_root"`        // Primary system disk
	IsLargest     bool    `json:"is_largest"`     // Largest storage disk
	DiskType      string  `json:"disk_type"`      // SSD, HDD, NVMe, etc.
	Device        string  `json:"device"`         // Block device name
}

// GetSystemInfo collects detailed system information
func GetSystemInfo(agentVersion string) (*SystemInfo, error) {
	info := &SystemInfo{
		AgentVersion: agentVersion,
		Metadata:     make(map[string]string),
	}

	// Get basic system info
	info.OSType = runtime.GOOS
	info.OSArchitecture = runtime.GOARCH

	// Get hostname
	if hostname, err := exec.Command("hostname").Output(); err == nil {
		info.Hostname = strings.TrimSpace(string(hostname))
	}

	// Get IP address
	if ip, err := getIPAddress(); err == nil {
		info.IPAddress = ip
	}

	// Get OS version info
	if info.OSType == "linux" {
		info.OSVersion = getLinuxDistroInfo()
	} else if info.OSType == "windows" {
		info.OSVersion = getWindowsInfo()
	} else if info.OSType == "darwin" {
		info.OSVersion = getMacOSInfo()
	}

	// Get CPU info
	if cpu, err := getCPUInfo(); err == nil {
		info.CPUInfo = *cpu
	}

	// Get memory info
	if mem, err := getMemoryInfo(); err == nil {
		info.MemoryInfo = *mem
	}

	// Get disk info
	if disks, err := getDiskInfo(); err == nil {
		info.DiskInfo = disks
	}

	// Get process count
	if procs, err := getProcessCount(); err == nil {
		info.RunningProcesses = procs
	}

	// Get uptime
	if uptime, err := getUptime(); err == nil {
		info.Uptime = uptime
	}

	// Add hardware information for Windows
	if runtime.GOOS == "windows" {
		if hardware := getWindowsHardwareInfo(); len(hardware) > 0 {
			for key, value := range hardware {
				info.Metadata[key] = value
			}
		}
	}

	// Check if system requires reboot
	rebootRequired, rebootReason := checkRebootRequired()
	info.RebootRequired = rebootRequired
	info.RebootReason = rebootReason

	// Add collection timestamp
	info.Metadata["collected_at"] = time.Now().Format(time.RFC3339)

	return info, nil
}

// getLinuxDistroInfo parses /etc/os-release for distro information
func getLinuxDistroInfo() string {
	if data, err := exec.Command("cat", "/etc/os-release").Output(); err == nil {
		lines := strings.Split(string(data), "\n")
		prettyName := ""
		version := ""

		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				prettyName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			}
			if strings.HasPrefix(line, "VERSION_ID=") {
				version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}

		if prettyName != "" {
			return prettyName
		}

		// Fallback to parsing ID and VERSION_ID
		id := ""
		for _, line := range lines {
			if strings.HasPrefix(line, "ID=") {
				id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
			}
		}

		if id != "" {
			if version != "" {
				return strings.Title(id) + " " + version
			}
			return strings.Title(id)
		}
	}

	// Try other methods
	if data, err := exec.Command("lsb_release", "-d", "-s").Output(); err == nil {
		return strings.TrimSpace(string(data))
	}

	return "Linux"
}


// getMacOSInfo gets macOS version information
func getMacOSInfo() string {
	if cmd, err := exec.LookPath("sw_vers"); err == nil {
		if data, err := exec.Command(cmd, "-productVersion").Output(); err == nil {
			version := strings.TrimSpace(string(data))
			return "macOS " + version
		}
	}

	return "macOS"
}

// getCPUInfo gets CPU information
func getCPUInfo() (*CPUInfo, error) {
	cpu := &CPUInfo{}

	if runtime.GOOS == "linux" {
		if data, err := exec.Command("cat", "/proc/cpuinfo").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			cores := 0
			for _, line := range lines {
				if strings.HasPrefix(line, "model name") {
					cpu.ModelName = strings.TrimPrefix(line, "model name\t: ")
				}
				if strings.HasPrefix(line, "processor") {
					cores++
				}
			}
			cpu.Cores = cores
			cpu.Threads = cores
		}
	} else if runtime.GOOS == "darwin" {
		if cmd, err := exec.LookPath("sysctl"); err == nil {
			if data, err := exec.Command(cmd, "-n", "hw.ncpu").Output(); err == nil {
				if cores, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
					cpu.Cores = cores
					cpu.Threads = cores
				}
			}
		}
	} else if runtime.GOOS == "windows" {
		return getWindowsCPUInfo()
	}

	return cpu, nil
}

// getMemoryInfo gets memory information
func getMemoryInfo() (*MemoryInfo, error) {
	mem := &MemoryInfo{}

	if runtime.GOOS == "linux" {
		if data, err := exec.Command("cat", "/proc/meminfo").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					switch fields[0] {
					case "MemTotal:":
						if total, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
							mem.Total = total * 1024 // Convert from KB to bytes
						}
					case "MemAvailable:":
						if available, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
							mem.Available = available * 1024
						}
					}
				}
			}
			mem.Used = mem.Total - mem.Available
			if mem.Total > 0 {
				mem.UsedPercent = float64(mem.Used) / float64(mem.Total) * 100
			}
		}
	} else if runtime.GOOS == "windows" {
		return getWindowsMemoryInfo()
	}

	return mem, nil
}

// getDiskInfo gets disk information for mounted filesystems with enhanced detection
func getDiskInfo() ([]DiskInfo, error) {
	var disks []DiskInfo

	if runtime.GOOS == "windows" {
		return getWindowsDiskInfo()
	} else {
		if cmd, err := exec.LookPath("df"); err == nil {
			if data, err := exec.Command(cmd, "-h", "--output=target,size,used,avail,pcent,source").Output(); err == nil {
				lines := strings.Split(string(data), "\n")

				// First pass: collect all valid disks
				var rawDisks []DiskInfo
				for i, line := range lines {
					if i == 0 || strings.TrimSpace(line) == "" {
						continue // Skip header and empty lines
					}

					fields := strings.Fields(line)
					if len(fields) >= 6 {
						mountpoint := fields[0]
						filesystem := fields[5]

						// Filter out pseudo-filesystems and only show physical/important mounts
						// Skip tmpfs, devtmpfs, overlay, squashfs, etc.
						if strings.HasPrefix(filesystem, "tmpfs") ||
							strings.HasPrefix(filesystem, "devtmpfs") ||
							strings.HasPrefix(filesystem, "overlay") ||
							strings.HasPrefix(filesystem, "squashfs") ||
							strings.HasPrefix(filesystem, "udev") ||
							strings.HasPrefix(filesystem, "proc") ||
							strings.HasPrefix(filesystem, "sysfs") ||
							strings.HasPrefix(filesystem, "cgroup") ||
							strings.HasPrefix(filesystem, "devpts") ||
							strings.HasPrefix(filesystem, "securityfs") ||
							strings.HasPrefix(filesystem, "pstore") ||
							strings.HasPrefix(filesystem, "bpf") ||
							strings.HasPrefix(filesystem, "configfs") ||
							strings.HasPrefix(filesystem, "fusectl") ||
							strings.HasPrefix(filesystem, "hugetlbfs") ||
							strings.HasPrefix(filesystem, "mqueue") ||
							strings.HasPrefix(filesystem, "debugfs") ||
							strings.HasPrefix(filesystem, "tracefs") {
							continue // Skip virtual/pseudo filesystems
						}

						// Skip container/snap mounts unless they're important
						if strings.Contains(mountpoint, "/snap/") ||
							strings.Contains(mountpoint, "/var/lib/docker") ||
							strings.Contains(mountpoint, "/run") {
							continue
						}

						disk := DiskInfo{
							Mountpoint: mountpoint,
							Filesystem: filesystem,
							Device:     filesystem,
						}

						// Parse sizes (df outputs in human readable format, we'll parse the numeric part)
						if total, err := parseSize(fields[1]); err == nil {
							disk.Total = total
						}
						if used, err := parseSize(fields[2]); err == nil {
							disk.Used = used
						}
						if available, err := parseSize(fields[3]); err == nil {
							disk.Available = available
						}
						if total, err := strconv.ParseFloat(strings.TrimSuffix(fields[4], "%"), 64); err == nil {
							disk.UsedPercent = total
						}

						rawDisks = append(rawDisks, disk)
					}
				}

				// Second pass: enhance with disk type detection and set flags
				var largestSize uint64 = 0
				var largestIndex int = -1

				for i := range rawDisks {
					// Detect root filesystem
					if rawDisks[i].Mountpoint == "/" || rawDisks[i].Mountpoint == "C:" {
						rawDisks[i].IsRoot = true
					}

					// Track largest disk
					if rawDisks[i].Total > largestSize {
						largestSize = rawDisks[i].Total
						largestIndex = i
					}

					// Detect disk type
					rawDisks[i].DiskType = detectDiskType(rawDisks[i].Device)
				}

				// Set largest disk flag
				if largestIndex >= 0 {
					rawDisks[largestIndex].IsLargest = true
				}

				disks = rawDisks
			}
		}
	}

	return disks, nil
}

// detectDiskType determines the type of storage device (SSD, HDD, NVMe, etc.)
func detectDiskType(device string) string {
	if device == "" {
		return "Unknown"
	}

	// Extract base device name (remove partition numbers like /dev/sda1 -> /dev/sda)
	baseDevice := device
	if strings.Contains(device, "/dev/") {
		parts := strings.Fields(device)
		if len(parts) > 0 {
			baseDevice = parts[0]
			// Remove partition numbers for common patterns
			re := strings.NewReplacer("/dev/sda", "/dev/sda", "/dev/sdb", "/dev/sdb", "/dev/nvme0n1", "/dev/nvme0n1")
			baseDevice = re.Replace(baseDevice)

			// More robust partition removal
			if matches := regexp.MustCompile(`^(/dev/sd[a-z]|/dev/nvme\d+n\d|/dev/hd[a-z])\d*$`).FindStringSubmatch(baseDevice); len(matches) > 1 {
				baseDevice = matches[1]
			}
		}
	}

	// Check for NVMe
	if strings.Contains(baseDevice, "nvme") {
		return "NVMe"
	}

	// Check for SSD indicators using lsblk
	if cmd, err := exec.LookPath("lsblk"); err == nil {
		if data, err := exec.Command(cmd, "-d", "-o", "rota,NAME", baseDevice).Output(); err == nil {
			output := string(data)
			if strings.Contains(output, "0") && strings.Contains(output, baseDevice[strings.LastIndex(baseDevice, "/")+1:]) {
				return "SSD" // rota=0 indicates non-rotating (SSD)
			} else if strings.Contains(output, "1") && strings.Contains(output, baseDevice[strings.LastIndex(baseDevice, "/")+1:]) {
				return "HDD" // rota=1 indicates rotating (HDD)
			}
		}
	}

	// Fallback detection based on device name patterns
	if strings.Contains(baseDevice, "sd") || strings.Contains(baseDevice, "hd") {
		return "HDD" // Traditional naming for SATA/IDE drives
	}

	return "Unknown"
}

// parseSize parses human readable size strings (like "1.5G", "500M", "3.7T")
func parseSize(sizeStr string) (uint64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	if len(sizeStr) == 0 {
		return 0, nil
	}

	multiplier := uint64(1)
	unit := sizeStr[len(sizeStr)-1:]
	if unit == "T" || unit == "t" {
		multiplier = 1024 * 1024 * 1024 * 1024  // Terabyte
		sizeStr = sizeStr[:len(sizeStr)-1]
	} else if unit == "G" || unit == "g" {
		multiplier = 1024 * 1024 * 1024  // Gigabyte
		sizeStr = sizeStr[:len(sizeStr)-1]
	} else if unit == "M" || unit == "m" {
		multiplier = 1024 * 1024  // Megabyte
		sizeStr = sizeStr[:len(sizeStr)-1]
	} else if unit == "K" || unit == "k" {
		multiplier = 1024  // Kilobyte
		sizeStr = sizeStr[:len(sizeStr)-1]
	}

	size, err := strconv.ParseFloat(sizeStr, 64)
	if err != nil {
		return 0, err
	}

	return uint64(size * float64(multiplier)), nil
}

// getProcessCount gets the number of running processes
func getProcessCount() (int, error) {
	if runtime.GOOS == "linux" {
		if data, err := exec.Command("ps", "-e").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			return len(lines) - 1, nil // Subtract 1 for header
		}
	} else if runtime.GOOS == "darwin" {
		if data, err := exec.Command("ps", "-ax").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			return len(lines) - 1, nil // Subtract 1 for header
		}
	} else if runtime.GOOS == "windows" {
		return getWindowsProcessCount()
	}

	return 0, nil
}

// getUptime gets system uptime
func getUptime() (string, error) {
	if runtime.GOOS == "linux" {
		if data, err := exec.Command("uptime", "-p").Output(); err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	} else if runtime.GOOS == "darwin" {
		if data, err := exec.Command("uptime").Output(); err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	} else if runtime.GOOS == "windows" {
		return getWindowsUptime()
	}

	return "Unknown", nil
}

// getIPAddress gets the primary IP address
func getIPAddress() (string, error) {
	if runtime.GOOS == "linux" {
		// Try to get the IP from hostname -I
		if data, err := exec.Command("hostname", "-I").Output(); err == nil {
			ips := strings.Fields(string(data))
			if len(ips) > 0 {
				return ips[0], nil
			}
		}

		// Fallback to ip route
		if data, err := exec.Command("ip", "route", "get", "8.8.8.8").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.Contains(line, "src") {
					fields := strings.Fields(line)
					for i, field := range fields {
						if field == "src" && i+1 < len(fields) {
							return fields[i+1], nil
						}
					}
				}
			}
		}
	} else if runtime.GOOS == "windows" {
		return getWindowsIPAddress()
	}

	return "127.0.0.1", nil
}

// LightweightMetrics contains lightweight system metrics for regular check-ins
type LightweightMetrics struct {
	CPUPercent    float64
	MemoryPercent float64
	MemoryUsedGB  float64
	MemoryTotalGB float64
	// Root filesystem disk info (primary disk)
	DiskUsedGB    float64
	DiskTotalGB   float64
	DiskPercent   float64
	// Largest disk info (for systems with separate data partitions)
	LargestDiskUsedGB  float64
	LargestDiskTotalGB float64
	LargestDiskPercent float64
	LargestDiskMount   string
	Uptime        string
}

// GetLightweightMetrics collects lightweight system metrics for regular check-ins
// This is much faster than GetSystemInfo() and suitable for frequent calls
func GetLightweightMetrics() (*LightweightMetrics, error) {
	metrics := &LightweightMetrics{}

	// Get memory info
	if mem, err := getMemoryInfo(); err == nil {
		metrics.MemoryPercent = mem.UsedPercent
		metrics.MemoryUsedGB = float64(mem.Used) / (1024 * 1024 * 1024)
		metrics.MemoryTotalGB = float64(mem.Total) / (1024 * 1024 * 1024)
	}

	// Get disk info (both root and largest)
	if disks, err := getDiskInfo(); err == nil {
		var rootDisk *DiskInfo
		var largestDisk *DiskInfo

		for i, disk := range disks {
			// Find root filesystem
			if disk.Mountpoint == "/" || disk.Mountpoint == "C:" {
				rootDisk = &disks[i]
			}

			// Track largest disk
			if largestDisk == nil || disk.Total > largestDisk.Total {
				largestDisk = &disks[i]
			}
		}

		// Set root disk metrics (primary disk)
		if rootDisk != nil {
			metrics.DiskUsedGB = float64(rootDisk.Used) / (1024 * 1024 * 1024)
			metrics.DiskTotalGB = float64(rootDisk.Total) / (1024 * 1024 * 1024)
			metrics.DiskPercent = rootDisk.UsedPercent
		}

		// Set largest disk metrics (for data partitions like /home)
		if largestDisk != nil && (rootDisk == nil || largestDisk.Total > rootDisk.Total) {
			metrics.LargestDiskUsedGB = float64(largestDisk.Used) / (1024 * 1024 * 1024)
			metrics.LargestDiskTotalGB = float64(largestDisk.Total) / (1024 * 1024 * 1024)
			metrics.LargestDiskPercent = largestDisk.UsedPercent
			metrics.LargestDiskMount = largestDisk.Mountpoint
		}
	}

	// Get uptime
	if uptime, err := getUptime(); err == nil {
		metrics.Uptime = uptime
	}

	// Note: CPU percentage requires sampling over time, which is expensive
	// For now, we omit it from lightweight metrics
	// In the future, we could add a background goroutine to track CPU usage

	return metrics, nil
}
// checkRebootRequired checks if the system requires a reboot
func checkRebootRequired() (bool, string) {
	if runtime.GOOS == "linux" {
		return checkLinuxRebootRequired()
	} else if runtime.GOOS == "windows" {
		return checkWindowsRebootRequired()
	}
	return false, ""
}

// checkLinuxRebootRequired checks if a Linux system requires a reboot
func checkLinuxRebootRequired() (bool, string) {
	// Method 1: Check Debian/Ubuntu reboot-required file
	if err := exec.Command("test", "-f", "/var/run/reboot-required").Run(); err == nil {
		// File exists, reboot is required
		// Try to read the packages that require reboot
		if output, err := exec.Command("cat", "/var/run/reboot-required.pkgs").Output(); err == nil {
			packages := strings.TrimSpace(string(output))
			if packages != "" {
				// Truncate if too long
				if len(packages) > 200 {
					packages = packages[:200] + "..."
				}
				return true, "Packages: " + packages
			}
		}
		return true, "System updates require reboot"
	}

	// Method 2: Check RHEL/Fedora/Rocky using needs-restarting
	cmd := exec.Command("needs-restarting", "-r")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 means reboot is needed
			if exitErr.ExitCode() == 1 {
				return true, "Kernel or system libraries updated"
			}
		}
	}

	return false, ""
}

// checkWindowsRebootRequired checks if a Windows system requires a reboot
func checkWindowsRebootRequired() (bool, string) {
	// Check Windows Update pending reboot registry keys
	// HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired
	cmd := exec.Command("reg", "query", "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\WindowsUpdate\\Auto Update\\RebootRequired")
	if err := cmd.Run(); err == nil {
		return true, "Windows updates require reboot"
	}

	// Check Component Based Servicing pending reboot
	cmd = exec.Command("reg", "query", "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing\\RebootPending")
	if err := cmd.Run(); err == nil {
		return true, "Component updates require reboot"
	}

	return false, ""
}
