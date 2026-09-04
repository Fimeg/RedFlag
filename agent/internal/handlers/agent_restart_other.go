//go:build !windows

package handlers

import "fmt"

// dispatchWindowsRestart is a non-Windows stub so the package compiles on every
// platform. restartAgentService only invokes it under runtime.GOOS == "windows";
// on Linux the privileged helper owns the restart, and macOS has no self-restart
// path yet (its binaries aren't signed).
func dispatchWindowsRestart(service string) error {
	return fmt.Errorf("windows restart path invoked on non-windows platform")
}
