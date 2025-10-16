package system

import (
	"os/exec"
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

// DiskInfo contains disk information
type DiskInfo struct {
	Mountpoint string  `json:"mountpoint"`
	Total      uint64  `json:"total"`
	Available  uint64  `json:"available"`
	Used       uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
	Filesystem string  `json:"filesystem"`
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

// getWindowsInfo gets Windows version information
func getWindowsInfo() string {
	// Try using wmic for Windows version
	if cmd, err := exec.LookPath("wmic"); err == nil {
		if data, err := exec.Command(cmd, "os", "get", "Caption,Version").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.Contains(line, "Microsoft Windows") {
					return strings.TrimSpace(line)
				}
			}
		}
	}

	return "Windows"
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
	}

	return mem, nil
}

// getDiskInfo gets disk information for mounted filesystems
func getDiskInfo() ([]DiskInfo, error) {
	var disks []DiskInfo

	if cmd, err := exec.LookPath("df"); err == nil {
		if data, err := exec.Command(cmd, "-h", "--output=target,size,used,avail,pcent,source").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if i == 0 || strings.TrimSpace(line) == "" {
					continue // Skip header and empty lines
				}

				fields := strings.Fields(line)
				if len(fields) >= 6 {
					disk := DiskInfo{
						Mountpoint: fields[0],
						Filesystem: fields[5],
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

					disks = append(disks, disk)
				}
			}
		}
	}

	return disks, nil
}

// parseSize parses human readable size strings (like "1.5G" or "500M")
func parseSize(sizeStr string) (uint64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	if len(sizeStr) == 0 {
		return 0, nil
	}

	multiplier := uint64(1)
	unit := sizeStr[len(sizeStr)-1:]
	if unit == "G" || unit == "g" {
		multiplier = 1024 * 1024 * 1024
		sizeStr = sizeStr[:len(sizeStr)-1]
	} else if unit == "M" || unit == "m" {
		multiplier = 1024 * 1024
		sizeStr = sizeStr[:len(sizeStr)-1]
	} else if unit == "K" || unit == "k" {
		multiplier = 1024
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
	}

	return "127.0.0.1", nil
}