// Package desktop manages the native Qt/QML local-machine operations console.
// The agent service spawns the desktop binary as a child process when a desktop
// session is available. The binary connects back to the agent's local API socket
// and presents Agent-owned machine state and bounded operator intent.
package desktop

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Manager handles spawning and monitoring the desktop app process.
type Manager struct {
	binPath      string
	enabled      bool
	maxRestarts  int
	restartDelay time.Duration

	mu         sync.Mutex
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	stopped    bool
	lastHealth healthReport
}

// healthReport is Desktop's most recent POST /v1/desktop self-report. On
// Linux Desktop is launched by XDG autostart, not by this manager, so the
// spawned-process state is always empty there — the health report is the only
// liveness and version signal the agent has.
type healthReport struct {
	version    string
	windowOpen bool
	reportedAt time.Time
}

// healthFreshness is how long a health report counts as proof of a live Desktop.
// Desktop reports every 30s; three missed beats means it is gone.
const healthFreshness = 90 * time.Second

// HealthSnapshot is the fleet-reportable desktop component state.
type HealthSnapshot struct {
	Installed  bool   `json:"installed"`
	Running    bool   `json:"running"`
	Version    string `json:"version,omitempty"`
	WindowOpen bool   `json:"window_open,omitempty"`
	LastReport string `json:"last_report,omitempty"`
}

// NewManager creates a desktop manager. binPath is the path to the redflag-desktop
// binary (empty string = auto-detect alongside the agent binary).
func NewManager(binPath string, enabled bool, maxRestarts, restartDelaySec int) *Manager {
	if binPath == "" {
		binPath = autoDetectBinary()
	}
	if restartDelaySec <= 0 {
		restartDelaySec = 5
	}
	return &Manager{
		binPath:      binPath,
		enabled:      enabled,
		maxRestarts:  maxRestarts,
		restartDelay: time.Duration(restartDelaySec) * time.Second,
	}
}

// autoDetectBinary finds the desktop binary alongside the agent binary.
func autoDetectBinary() string {
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(execPath)
	name := "redflag-desktop"
	if runtime.GOOS == "windows" {
		name = "redflag-desktop.exe"
	}
	return filepath.Join(dir, name)
}

// IsAvailable checks if the desktop binary exists and a desktop session is detectable.
func (m *Manager) IsAvailable() bool {
	if m.binPath == "" {
		return false
	}
	info, err := os.Stat(m.binPath)
	if err != nil || info.IsDir() {
		return false
	}
	return hasDesktopSession()
}

// Start begins managing the desktop process. It blocks until Stop() is called
// or the context is cancelled. Spawns the binary, restarts on crash up to
// maxRestarts times.
func (m *Manager) Start(ctx context.Context) {
	if !m.enabled {
		log.Printf("[INFO] [agent] [desktop] disabled by config")
		return
	}

	if !m.IsAvailable() {
		log.Printf("[INFO] [agent] [desktop] not available — binary=%s session_detected=false", m.binPath)
		return
	}

	log.Printf("[INFO] [agent] [desktop] starting binary=%s", m.binPath)

	m.mu.Lock()
	ctx, m.cancel = context.WithCancel(ctx)
	m.stopped = false
	m.mu.Unlock()

	restarts := 0
	for {
		select {
		case <-ctx.Done():
			log.Printf("[INFO] [agent] [desktop] stopped (context cancelled)")
			return
		default:
		}

		if m.maxRestarts > 0 && restarts >= m.maxRestarts {
			log.Printf("[WARNING] [agent] [desktop] max restarts reached (%d), giving up", m.maxRestarts)
			return
		}

		err := m.run(ctx)
		if err == nil {
			// Clean exit — don't restart.
			log.Printf("[INFO] [agent] [desktop] exited cleanly")
			return
		}

		restarts++
		log.Printf("[WARNING] [agent] [desktop] process exited with error: %v (restart %d)", err, restarts)

		select {
		case <-ctx.Done():
			return
		case <-time.After(m.restartDelay):
		}
	}
}

// run spawns the desktop binary and waits for it to exit.
func (m *Manager) run(ctx context.Context) error {
	m.mu.Lock()
	cmd := exec.CommandContext(ctx, m.binPath)
	m.cmd = cmd
	m.mu.Unlock()

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start desktop: %w", err)
	}

	log.Printf("[INFO] [agent] [desktop] spawned pid=%d", cmd.Process.Pid)

	return cmd.Wait()
}

// Stop terminates the desktop process if running.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
	m.stopped = true

	if m.cmd != nil && m.cmd.Process != nil {
		log.Printf("[INFO] [agent] [desktop] terminating pid=%d", m.cmd.Process.Pid)
		m.cmd.Process.Kill()
	}
}

// Status returns current desktop process state.
func (m *Manager) Status() (running bool, pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil || m.cmd.Process == nil {
		return false, 0
	}

	// Check if process is still alive.
	if m.cmd.ProcessState != nil && m.cmd.ProcessState.Exited() {
		return false, 0
	}

	return true, m.cmd.Process.Pid
}

// RecordHealth stores Desktop's self-report (POST /v1/desktop via localapi).
func (m *Manager) RecordHealth(version string, windowOpen bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastHealth = healthReport{
		version:    version,
		windowOpen: windowOpen,
		reportedAt: time.Now().UTC(),
	}
}

// Health returns the desktop component state for fleet reporting. Running is
// true when this manager spawned a live process (Windows) or when a health
// report landed within the freshness window (Linux autostart path).
func (m *Manager) Health() HealthSnapshot {
	running, _ := m.Status()

	m.mu.Lock()
	last := m.lastHealth
	m.mu.Unlock()

	snap := HealthSnapshot{Running: running}
	if m.binPath != "" {
		if info, err := os.Stat(m.binPath); err == nil && !info.IsDir() {
			snap.Installed = true
		}
	}
	if !last.reportedAt.IsZero() {
		snap.Version = last.version
		snap.WindowOpen = last.windowOpen
		snap.LastReport = last.reportedAt.Format(time.RFC3339)
		if time.Since(last.reportedAt) <= healthFreshness {
			snap.Running = true
		}
	}
	return snap
}
