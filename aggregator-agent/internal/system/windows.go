//go:build windows
// +build windows

package system

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// getWindowsInfo gets detailed Windows version information using WMI
func getWindowsInfo() string {
	// Try using wmic for detailed Windows version info
	if cmd, err := exec.LookPath("wmic"); err == nil {
		// Get Caption (e.g., "Microsoft Windows 10 Pro")
		caption := ""
		if data, err := exec.Command(cmd, "os", "get", "Caption", "/value").Output(); err == nil {
			output := strings.TrimSpace(string(data))
			if strings.HasPrefix(output, "Caption=") {
				caption = strings.TrimPrefix(output, "Caption=")
				caption = strings.TrimSpace(caption)
			}
		}

		// Get Version and Build Number
		version := ""
		if data, err := exec.Command(cmd, "os", "get", "Version", "/value").Output(); err == nil {
			output := strings.TrimSpace(string(data))
			if strings.HasPrefix(output, "Version=") {
				version = strings.TrimPrefix(output, "Version=")
				version = strings.TrimSpace(version)
			}
		}

		// Combine caption and version for clean output
		if caption != "" && version != "" {
			return fmt.Sprintf("%s (Build %s)", caption, version)
		} else if caption != "" {
			return caption
		} else if version != "" {
			return fmt.Sprintf("Windows %s", version)
		}
	}

	// Fallback to basic version detection
	return "Windows"
}

// getWindowsCPUInfo gets detailed CPU information using WMI
func getWindowsCPUInfo() (*CPUInfo, error) {
	cpu := &CPUInfo{}

	// Try using wmic for CPU information
	if cmd, err := exec.LookPath("wmic"); err == nil {
		// Get CPU name with better error handling
		if data, err := exec.Command(cmd, "cpu", "get", "Name").Output(); err == nil {
			output := string(data)
			fmt.Printf("WMIC CPU Name output: '%s'\n", output) // Debug logging
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.Contains(line, "Name") {
					cpu.ModelName = line
					fmt.Printf("Found CPU model: '%s'\n", line) // Debug logging
					break
				}
			}
		} else {
			fmt.Printf("Failed to get CPU name via wmic: %v\n", err)
		}

		// Get number of cores
		if data, err := exec.Command(cmd, "cpu", "get", "NumberOfCores").Output(); err == nil {
			output := string(data)
			fmt.Printf("WMIC CPU Cores output: '%s'\n", output) // Debug logging
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.Contains(line, "NumberOfCores") {
					if cores, err := strconv.Atoi(line); err == nil {
						cpu.Cores = cores
						fmt.Printf("Found CPU cores: %d\n", cores) // Debug logging
					}
					break
				}
			}
		} else {
			fmt.Printf("Failed to get CPU cores via wmic: %v\n", err)
		}

		// Get number of logical processors (threads)
		if data, err := exec.Command(cmd, "cpu", "get", "NumberOfLogicalProcessors").Output(); err == nil {
			output := string(data)
			fmt.Printf("WMIC CPU Threads output: '%s'\n", output) // Debug logging
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.Contains(line, "NumberOfLogicalProcessors") {
					if threads, err := strconv.Atoi(line); err == nil {
						cpu.Threads = threads
						fmt.Printf("Found CPU threads: %d\n", threads) // Debug logging
					}
					break
				}
			}
		} else {
			fmt.Printf("Failed to get CPU threads via wmic: %v\n", err)
		}

		// If we couldn't get threads, assume it's equal to cores
		if cpu.Threads == 0 {
			cpu.Threads = cpu.Cores
		}
	} else {
		fmt.Printf("WMIC command not found, unable to get CPU info\n")
	}

	// Fallback to PowerShell if wmic failed
	if cpu.ModelName == "" {
		fmt.Printf("Attempting PowerShell fallback for CPU info...\n")
		if psCmd, err := exec.LookPath("powershell"); err == nil {
			// Get CPU info via PowerShell
			if data, err := exec.Command(psCmd, "-Command", "Get-CimInstance -ClassName Win32_Processor | Select-Object -First 1 Name,NumberOfCores,NumberOfLogicalProcessors | ConvertTo-Json").Output(); err == nil {
				fmt.Printf("PowerShell CPU output: '%s'\n", string(data))
				// Try to parse JSON output (simplified)
				output := string(data)
				if strings.Contains(output, "Name") {
					// Simple string extraction as fallback
					lines := strings.Split(output, "\n")
					for _, line := range lines {
						if strings.Contains(line, "Name") && strings.Contains(line, ":") {
							parts := strings.Split(line, ":")
							if len(parts) >= 2 {
								cpu.ModelName = strings.TrimSpace(strings.Trim(parts[1], " ,\""))
								fmt.Printf("Found CPU via PowerShell: '%s'\n", cpu.ModelName)
								break
							}
						}
					}
				}
			} else {
				fmt.Printf("PowerShell CPU info failed: %v\n", err)
			}
		}
	}

	return cpu, nil
}

// getWindowsMemoryInfo gets memory information using WMI
func getWindowsMemoryInfo() (*MemoryInfo, error) {
	mem := &MemoryInfo{}

	if cmd, err := exec.LookPath("wmic"); err == nil {
		// Get total memory in bytes
		if data, err := exec.Command(cmd, "computersystem", "get", "TotalPhysicalMemory").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" && !strings.Contains(line, "TotalPhysicalMemory") {
					if total, err := strconv.ParseUint(strings.TrimSpace(line), 10, 64); err == nil {
						mem.Total = total
					}
					break
				}
			}
		}

		// Get available memory using PowerShell (more accurate than wmic for available memory)
		if cmd, err := exec.LookPath("powershell"); err == nil {
			if data, err := exec.Command(cmd, "-Command",
				"(Get-Counter '\\Memory\\Available MBytes').CounterSamples.CookedValue").Output(); err == nil {
				if available, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil {
					mem.Available = uint64(available * 1024 * 1024) // Convert MB to bytes
				}
			}
		} else {
			// Fallback: estimate available memory (this is not very accurate)
			mem.Available = mem.Total / 4 // Rough estimate: 25% available
		}

		mem.Used = mem.Total - mem.Available
		if mem.Total > 0 {
			mem.UsedPercent = float64(mem.Used) / float64(mem.Total) * 100
		}
	}

	return mem, nil
}

// getWindowsDiskInfo gets disk information using WMI
func getWindowsDiskInfo() ([]DiskInfo, error) {
	var disks []DiskInfo

	if cmd, err := exec.LookPath("wmic"); err == nil {
		// Get logical disk information - use /value format for reliable parsing
		if data, err := exec.Command(cmd, "logicaldisk", "get", "DeviceID,Size,FreeSpace,FileSystem", "/format:csv").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				line = strings.TrimSpace(line)
				// Skip header and empty lines
				if i == 0 || line == "" || !strings.Contains(line, ",") {
					continue
				}

				// CSV format: Node,DeviceID,FileSystem,FreeSpace,Size
				fields := strings.Split(line, ",")
				if len(fields) >= 5 {
					deviceID := strings.TrimSpace(fields[1])
					filesystem := strings.TrimSpace(fields[2])
					freeSpaceStr := strings.TrimSpace(fields[3])
					sizeStr := strings.TrimSpace(fields[4])

					// Skip if no size info (e.g., CD-ROM drives)
					if sizeStr == "" || freeSpaceStr == "" {
						continue
					}

					disk := DiskInfo{
						Mountpoint: deviceID,
						Filesystem: filesystem,
					}

					// Parse sizes (wmic outputs in bytes)
					if total, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
						disk.Total = total
					}
					if available, err := strconv.ParseUint(freeSpaceStr, 10, 64); err == nil {
						disk.Available = available
					}

					// Calculate used space
					if disk.Total > 0 && disk.Available <= disk.Total {
						disk.Used = disk.Total - disk.Available
						disk.UsedPercent = float64(disk.Used) / float64(disk.Total) * 100
					}

					// Only add disks with valid size info
					if disk.Total > 0 {
						disks = append(disks, disk)
					}
				}
			}
		}
	}

	return disks, nil
}

// getWindowsProcessCount gets the number of running processes using WMI
func getWindowsProcessCount() (int, error) {
	if cmd, err := exec.LookPath("wmic"); err == nil {
		if data, err := exec.Command(cmd, "process", "get", "ProcessId").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			// Count non-empty lines that don't contain the header
			count := 0
			for _, line := range lines {
				if strings.TrimSpace(line) != "" && !strings.Contains(line, "ProcessId") {
					count++
				}
			}
			return count, nil
		}
	}

	return 0, nil
}

// getWindowsUptime gets system uptime using WMI or PowerShell
func getWindowsUptime() (string, error) {
	// Try PowerShell first for more accurate uptime
	if cmd, err := exec.LookPath("powershell"); err == nil {
		// Get uptime in seconds for precise calculation
		if data, err := exec.Command(cmd, "-Command",
			"(New-TimeSpan -Start (Get-CimInstance Win32_OperatingSystem).LastBootUpTime -End (Get-Date)).TotalSeconds").Output(); err == nil {
			secondsStr := strings.TrimSpace(string(data))
			if seconds, err := strconv.ParseFloat(secondsStr, 64); err == nil {
				return formatUptimeFromSeconds(seconds), nil
			}
		}
	}

	// Fallback to wmic with manual parsing
	if cmd, err := exec.LookPath("wmic"); err == nil {
		if data, err := exec.Command(cmd, "os", "get", "LastBootUpTime", "/value").Output(); err == nil {
			output := strings.TrimSpace(string(data))
			if strings.HasPrefix(output, "LastBootUpTime=") {
				wmiTime := strings.TrimPrefix(output, "LastBootUpTime=")
				wmiTime = strings.TrimSpace(wmiTime)
				// Parse WMI datetime format: 20251025123045.123456-300
				if len(wmiTime) >= 14 {
					// Extract date/time components: YYYYMMDDHHmmss
					year := wmiTime[0:4]
					month := wmiTime[4:6]
					day := wmiTime[6:8]
					hour := wmiTime[8:10]
					minute := wmiTime[10:12]
					second := wmiTime[12:14]

					bootTimeStr := fmt.Sprintf("%s-%s-%s %s:%s:%s", year, month, day, hour, minute, second)
					return fmt.Sprintf("Since %s", bootTimeStr), nil
				}
			}
		}
	}

	return "Unknown", nil
}

// formatUptimeFromSeconds formats uptime from seconds into human readable format
func formatUptimeFromSeconds(seconds float64) string {
	days := int(seconds / 86400)
	hours := int((seconds - float64(days*86400)) / 3600)
	minutes := int((seconds - float64(days*86400) - float64(hours*3600)) / 60)

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%d days, %d hours", days, hours)
		}
		return fmt.Sprintf("%d days", days)
	} else if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%d hours, %d minutes", hours, minutes)
		}
		return fmt.Sprintf("%d hours", hours)
	} else {
		return fmt.Sprintf("%d minutes", minutes)
	}
}

// formatUptimeFromDays formats uptime from days into human readable format
func formatUptimeFromDays(days float64) string {
	if days < 1 {
		hours := int(days * 24)
		return fmt.Sprintf("%d hours", hours)
	} else if days < 7 {
		hours := int((days - float64(int(days))) * 24)
		return fmt.Sprintf("%d days, %d hours", int(days), hours)
	} else {
		weeks := int(days / 7)
		remainingDays := int(days) % 7
		return fmt.Sprintf("%d weeks, %d days", weeks, remainingDays)
	}
}

// getWindowsIPAddress gets the primary IP address using Windows commands
func getWindowsIPAddress() (string, error) {
	// Try using ipconfig
	if cmd, err := exec.LookPath("ipconfig"); err == nil {
		if data, err := exec.Command(cmd, "/all").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "IPv4 Address") || strings.HasPrefix(line, "IP Address") {
					// Extract the IP address from the line
					parts := strings.Split(line, ":")
					if len(parts) >= 2 {
						ip := strings.TrimSpace(parts[1])
						// Prefer non-169.254.x.x (APIPA) addresses
						if !strings.HasPrefix(ip, "169.254.") {
							return ip, nil
						}
					}
				}
			}
		}
	}

	// Fallback to localhost
	return "127.0.0.1", nil
}

// Override the generic functions with Windows-specific implementations
func init() {
	// This function will be called when the package is imported on Windows
}

// getWindowsHardwareInfo gets additional hardware information
func getWindowsHardwareInfo() map[string]string {
	hardware := make(map[string]string)

	if cmd, err := exec.LookPath("wmic"); err == nil {
		// Get motherboard information
		if data, err := exec.Command(cmd, "baseboard", "get", "Manufacturer,Product,SerialNumber").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" && !strings.Contains(line, "Manufacturer") &&
				   !strings.Contains(line, "Product") && !strings.Contains(line, "SerialNumber") {
					// This is a simplified parsing - in production you'd want more robust parsing
					if strings.Contains(line, " ") {
						hardware["motherboard"] = strings.TrimSpace(line)
					}
				}
			}
		}

		// Get BIOS information
		if data, err := exec.Command(cmd, "bios", "get", "Version,SerialNumber").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" && !strings.Contains(line, "Version") &&
				   !strings.Contains(line, "SerialNumber") {
					hardware["bios"] = strings.TrimSpace(line)
				}
			}
		}

		// Get GPU information
		if data, err := exec.Command(cmd, "path", "win32_VideoController", "get", "Name").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			gpus := []string{}
			for _, line := range lines {
				if strings.TrimSpace(line) != "" && !strings.Contains(line, "Name") {
					gpu := strings.TrimSpace(line)
					if gpu != "" {
						gpus = append(gpus, gpu)
					}
				}
			}
			if len(gpus) > 0 {
				hardware["graphics"] = strings.Join(gpus, ", ")
			}
		}
	}

	return hardware
}