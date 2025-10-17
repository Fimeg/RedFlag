//go:build !windows
// +build !windows

package system

// Stub functions for non-Windows platforms
// These return empty/default values on non-Windows systems

func getWindowsCPUInfo() (*CPUInfo, error) {
	return &CPUInfo{}, nil
}

func getWindowsMemoryInfo() (*MemoryInfo, error) {
	return &MemoryInfo{}, nil
}

func getWindowsDiskInfo() ([]DiskInfo, error) {
	return []DiskInfo{}, nil
}

func getWindowsProcessCount() (int, error) {
	return 0, nil
}

func getWindowsUptime() (string, error) {
	return "Unknown", nil
}

func getWindowsIPAddress() (string, error) {
	return "127.0.0.1", nil
}

func getWindowsHardwareInfo() map[string]string {
	return make(map[string]string)
}

func getWindowsInfo() string {
	return "Windows"
}