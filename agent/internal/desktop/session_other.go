//go:build !linux && !windows

package desktop

import "os"

// hasDesktopSession checks if a desktop session is available on other platforms.
// Default: check for DISPLAY environment variable.
func hasDesktopSession() bool {
	return os.Getenv("DISPLAY") != ""
}
