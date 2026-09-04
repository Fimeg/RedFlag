package desktop

import (
	"os"
	"strings"
)

// hasDesktopSession checks if a desktop session is available on Linux.
// Looks for DISPLAY or WAYLAND_DISPLAY in the environment, which indicates
// an X11 or Wayland session is active.
func hasDesktopSession() bool {
	// Check standard environment variables.
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}

	// Fallback: scan /proc for a running desktop session (the agent service
	// may not inherit DISPLAY from systemd).
	return findDesktopSessionInProc()
}

// findDesktopSessionInProc scans /proc/*/environ for DISPLAY or WAYLAND_DISPLAY.
// Returns true if any user process has a desktop session active.
func findDesktopSessionInProc() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Only check numeric (PID) directories.
		if entry.Name()[0] < '0' || entry.Name()[0] > '9' {
			continue
		}

		data, err := os.ReadFile("/proc/" + entry.Name() + "/environ")
		if err != nil {
			continue
		}

		for _, v := range strings.Split(string(data), "\x00") {
			if strings.HasPrefix(v, "DISPLAY=") && len(v) > 8 {
				return true
			}
			if strings.HasPrefix(v, "WAYLAND_DISPLAY=") && len(v) > 17 {
				return true
			}
		}
	}

	return false
}
