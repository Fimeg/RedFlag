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
		if data, err := exec.Command(cmd, "os", "get", "Caption,Version,BuildNumber,SKU").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.Contains(line, "Microsoft Windows") {
					// Clean up the output
					line = strings.TrimSpace(line)
					// Remove extra spaces
					for strings.Contains(line, "  ") {
						line = strings.ReplaceAll(line, "  ", " ")
					}
					return line
				}
			}
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
		// Get logical disk information
		if data, err := exec.Command(cmd, "logicaldisk", "get", "DeviceID,Size,FreeSpace,FileSystem").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" && !strings.Contains(line, "DeviceID") {
					fields := strings.Fields(line)
					if len(fields) >= 4 {
						disk := DiskInfo{
							Mountpoint: strings.TrimSpace(fields[0]),
							Filesystem: strings.TrimSpace(fields[3]),
						}

						// Parse sizes (wmic outputs in bytes)
						if total, err := strconv.ParseUint(strings.TrimSpace(fields[1]), 10, 64); err == nil {
							disk.Total = total
						}
						if available, err := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 64); err == nil {
							disk.Available = available
						}

						disk.Used = disk.Total - disk.Available
						if disk.Total > 0 {
							disk.UsedPercent = float64(disk.Used) / float64(disk.Total) * 100
						}

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
		if data, err := exec.Command(cmd, "-Command",
			"(Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime | Select-Object TotalDays").Output(); err == nil {
			// Parse the output to get days
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.Contains(line, "TotalDays") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if days, err := strconv.ParseFloat(fields[len(fields)-1], 64); err == nil {
							return formatUptimeFromDays(days), nil
						}
					}
				}
			}
		}
	}

	// Fallback to wmic
	if cmd, err := exec.LookPath("wmic"); err == nil {
		if data, err := exec.Command(cmd, "os", "get", "LastBootUpTime").Output(); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" && !strings.Contains(line, "LastBootUpTime") {
					// Parse WMI datetime format: 20231201123045.123456-300
					wmiTime := strings.TrimSpace(line)
					if len(wmiTime) >= 14 {
						// Extract just the date part for basic calculation
						// This is a simplified approach - in production you'd want proper datetime parsing
						return fmt.Sprintf("Since %s", wmiTime[:8]), nil
					}
				}
			}
		}
	}

	return "Unknown", nil
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