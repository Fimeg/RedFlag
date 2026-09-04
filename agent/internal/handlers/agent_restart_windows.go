//go:build windows

package handlers

import (
	"fmt"
	"log"
	"os/exec"
	"syscall"
)

// dispatchWindowsRestart cycles the agent's own Windows service after the binary
// has already been swapped on disk.
//
// A service cannot stop itself and then start again from the same process: the
// stop terminates this process before the start runs, so nothing issues the start.
// The work is handed to a detached child that outlives us — it waits a few seconds
// for this process to exit, then stops and starts the service. This is the standard
// self-update restart pattern for a Windows service, the same shape any tray/agent
// app (Avira, AGV, etc.) uses to relaunch itself after replacing its own binary.
func dispatchWindowsRestart(service string) error {
	// `ping -n 4 localhost` is a dependency-free ~3s sleep; then stop and start.
	script := fmt.Sprintf("ping -n 4 127.0.0.1 >nul & sc stop %s & sc start %s", service, service)
	cmd := exec.Command("cmd", "/C", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// DETACHED_PROCESS (0x8) | CREATE_NEW_PROCESS_GROUP (0x200): fully decouple
		// the child so it survives this process being killed by the service stop.
		CreationFlags: 0x00000008 | 0x00000200,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch detached restart helper: %w", err)
	}
	log.Printf("[INFO] [agent] [service] windows_restart_dispatched service=%s", service)
	return nil
}
