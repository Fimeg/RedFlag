package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
)

// HandleCaptureScreenshot captures the current display output and reports it
// back as a base64-encoded PNG in the command result's stdout field.
// Observe-only: reads the screen, does not interact with it.
//
// Linux: uses scrot (X11) or import (ImageMagick) as fallback.
// Windows: uses PowerShell with .NET System.Drawing.
// The temp file is cleaned up after encoding.
func HandleCaptureScreenshot(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, commandID string) error {
	start := time.Now()

	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("redflag_screenshot_%d.png", time.Now().UnixNano()))
	defer os.Remove(tmpPath)

	diag, err := captureScreen(tmpPath)
	if err != nil {
		// Report detailed diagnostics to the server instead of a generic message.
		diagJSON, _ := json.Marshal(diag)
		logReport := client.LogReport{
			CommandID: commandID,
			Action:    "capture_screenshot",
			Result:    "failed",
			Stderr:    fmt.Sprintf("screen capture failed: %v", err),
			ExitCode:  1,
			Metadata: map[string]string{
				"diagnostics": string(diagJSON),
			},
			DurationSeconds: int(time.Since(start).Seconds()),
		}
		_ = ReportLogWithAck(apiClient, cfg, ackTracker, logReport)
		return fmt.Errorf("screen capture failed: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to read screenshot: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "capture_screenshot",
		Result:          "success",
		Stdout:          encoded,
		ExitCode:        0,
		DurationSeconds: int(time.Since(start).Seconds()),
	}
	if err := ReportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		return fmt.Errorf("failed to report screenshot: %w", err)
	}

	log.Printf("[INFO] [agent] [screenshot] captured size_bytes=%d duration=%s",
		len(data), time.Since(start).Round(time.Millisecond))
	return nil
}

// captureScreen writes a PNG screenshot to outputPath. Platform-specific.
// Returns diagnostics for health reporting on failure.
func captureScreen(outputPath string) (*screenshotDiagnostics, error) {
	switch runtime.GOOS {
	case "linux":
		return captureScreenLinux(outputPath)
	case "windows":
		if err := captureScreenWindows(outputPath); err != nil {
			return &screenshotDiagnostics{SessionType: "windows"}, err
		}
		return &screenshotDiagnostics{SessionType: "windows"}, nil
	default:
		return &screenshotDiagnostics{}, fmt.Errorf("screenshot not supported on %s", runtime.GOOS)
	}
}

// sessionDisplayInfo holds discovered display environment variables and the
// detected session type (x11, wayland, or unknown).
type sessionDisplayInfo struct {
	env         []string
	sessionType string // "x11", "wayland", or ""
	sourcePID   int    // pid whose environ provided the vars (0 = none)
}

// toolAttempt records one screenshot tool invocation for diagnostic reporting.
type toolAttempt struct {
	Name     string `json:"name"`
	Found    bool   `json:"found"`              // in PATH
	Tried    bool   `json:"tried"`              // actually executed
	ExitCode int    `json:"exit_code,omitempty"` // 0 = success, -1 = not tried
	Stderr   string `json:"stderr,omitempty"`    // first 200 chars of output
}

// screenshotDiagnostics captures the full picture for the server when a
// screenshot fails. The generic "no tool found" message hid the real cause;
// this exposes it as structured health data.
type screenshotDiagnostics struct {
	SessionType    string        `json:"session_type"`
	SessionFound   bool          `json:"session_found"`
	DisplayVars    int           `json:"display_vars"`
	SessionPID     int           `json:"session_pid,omitempty"`
	Tools          []toolAttempt `json:"tools"`
	CapSysPtrace   bool          `json:"cap_sys_ptrace"`
	ProcReadable   bool          `json:"proc_readable"`
}

// captureScreenLinux captures the display. The agent service does not inherit
// DISPLAY/WAYLAND_DISPLAY from systemd, so we discover them from the running
// user session before invoking any capture tool.
//
// Tool priority depends on session type:
//
//	X11:       scrot → magick import → import (ImageMagick v6)
//	Wayland:   grim (wlroots) → gnome-screenshot (GNOME) → spectacle (KDE)
//	             → magick import (fallback, needs root on some compositors)
//	Unknown:   try all tools in order
func captureScreenLinux(outputPath string) (*screenshotDiagnostics, error) {
	info := discoverSessionDisplay()
	env := append(os.Environ(), info.env...)

	diag := &screenshotDiagnostics{
		SessionType:  info.sessionType,
		SessionFound: info.sessionType != "",
		DisplayVars:  len(info.env),
		SessionPID:   info.sourcePID,
		CapSysPtrace: hasCapSysPtrace(),
		ProcReadable: info.sourcePID != 0,
	}

	log.Printf("[INFO] [agent] [screenshot] session_type=%s display_vars=%d session_pid=%d cap_sys_ptrace=%v",
		orDefault(info.sessionType, "unknown"), len(info.env), info.sourcePID, diag.CapSysPtrace)

	type cmdFunc func(string, ...string) bool
	var tryCmd cmdFunc
	tryCmd = func(name string, args ...string) bool {
		_, lookErr := exec.LookPath(name)
		attempt := toolAttempt{Name: name, Found: lookErr == nil, ExitCode: -1}
		if lookErr != nil {
			diag.Tools = append(diag.Tools, attempt)
			return false
		}
		attempt.Tried = true
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			attempt.Stderr = truncate(string(out), 200)
			if exitErr, ok := err.(*exec.ExitError); ok {
				attempt.ExitCode = exitErr.ExitCode()
			}
			log.Printf("[WARN] [agent] [screenshot] %s_failed exit=%d output=%q error=%v", name, attempt.ExitCode, string(out), err)
			diag.Tools = append(diag.Tools, attempt)
			return false
		}
		attempt.ExitCode = 0
		diag.Tools = append(diag.Tools, attempt)
		return true
	}

	var err error
	switch info.sessionType {
	case "wayland":
		err = captureWayland(outputPath, env, tryCmd)
	case "x11":
		err = captureX11(outputPath, tryCmd)
	default:
		err = captureFallback(outputPath, tryCmd)
	}
	if err != nil {
		return diag, err
	}
	return diag, nil
}

// captureX11 tries X11 screenshot tools in priority order.
func captureX11(outputPath string, tryCmd func(string, ...string) bool) error {
	if tryCmd("scrot", "-o", outputPath) {
		return nil
	}
	// ImageMagick v7: standalone `import` is gone, use `magick import`.
	if tryCmd("magick", "import", "-window", "root", outputPath) {
		return nil
	}
	// ImageMagick v6 compat.
	if tryCmd("import", "-window", "root", outputPath) {
		return nil
	}
	return fmt.Errorf("no X11 screenshot tool found (tried scrot, magick import)")
}

// captureWayland tries Wayland screenshot tools. Tool availability depends on
// the compositor:
//   - wlroots (Sway, Hyprland, River): grim
//   - GNOME (Mutter): gnome-screenshot or D-Bus org.gnome.Shell.Screenshot
//   - KDE (KWin): spectacle --background --output
//   - Fallback: magick import (may work under XWayland or as root)
func captureWayland(outputPath string, env []string, tryCmd func(string, ...string) bool) error {
	// wlroots compositors — grim speaks wlr-screencopy / ext-image-capture-source.
	if tryCmd("grim", outputPath) {
		return nil
	}

	// GNOME Wayland — gnome-screenshot uses the GNOME Shell D-Bus API.
	if tryCmd("gnome-screenshot", "-f", outputPath) {
		return nil
	}

	// KDE Wayland — spectacle's --background flag captures without opening the GUI.
	if tryCmd("spectacle", "--background", "--output", outputPath) {
		return nil
	}

	// ImageMagick — may work via XWayland or with compositor-specific backends.
	if tryCmd("magick", "import", "-window", "root", outputPath) {
		return nil
	}
	if tryCmd("import", "-window", "root", outputPath) {
		return nil
	}

	return fmt.Errorf("no Wayland screenshot tool found (tried grim, gnome-screenshot, spectacle, magick import)")
}

// captureFallback tries all tools regardless of session type.
func captureFallback(outputPath string, tryCmd func(string, ...string) bool) error {
	if tryCmd("scrot", "-o", outputPath) {
		return nil
	}
	if tryCmd("grim", outputPath) {
		return nil
	}
	if tryCmd("gnome-screenshot", "-f", outputPath) {
		return nil
	}
	if tryCmd("spectacle", "--background", "--output", outputPath) {
		return nil
	}
	if tryCmd("magick", "import", "-window", "root", outputPath) {
		return nil
	}
	if tryCmd("import", "-window", "root", outputPath) {
		return nil
	}
	return fmt.Errorf("no screenshot tool found (tried scrot, grim, gnome-screenshot, spectacle, magick import)")
}

// discoverSessionDisplay reads /proc environ entries to find DISPLAY,
// WAYLAND_DISPLAY, XDG_RUNTIME_DIR, and XDG_SESSION_TYPE from an active
// user session. The agent service runs without these inherited from systemd.
//
// XDG_SESSION_TYPE is used to select the right screenshot tool chain:
// "x11" → scrot/magick, "wayland" → grim/gnome-screenshot/spectacle.
func discoverSessionDisplay() sessionDisplayInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return sessionDisplayInfo{}
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + entry.Name() + "/environ")
		if err != nil {
			continue
		}
		var display, wayland, xdgRuntime, sessionType string
		for _, v := range bytes.Split(data, []byte{0}) {
			s := string(v)
			switch {
			case strings.HasPrefix(s, "DISPLAY=") && display == "":
				display = s
			case strings.HasPrefix(s, "WAYLAND_DISPLAY=") && wayland == "":
				wayland = s
			case strings.HasPrefix(s, "XDG_RUNTIME_DIR=") && xdgRuntime == "":
				xdgRuntime = s
			case strings.HasPrefix(s, "XDG_SESSION_TYPE=") && sessionType == "":
				sessionType = strings.TrimPrefix(s, "XDG_SESSION_TYPE=")
			}
		}
		if display != "" || wayland != "" {
			var env []string
			if display != "" {
				env = append(env, display)
			}
			if wayland != "" {
				env = append(env, wayland)
			}
			if xdgRuntime != "" {
				env = append(env, xdgRuntime)
			}
			// Infer session type from env if XDG_SESSION_TYPE was not set.
			if sessionType == "" {
				if wayland != "" {
					sessionType = "wayland"
				} else if display != "" {
					sessionType = "x11"
				}
			}
			pid, _ := strconv.Atoi(entry.Name())
			return sessionDisplayInfo{env: env, sessionType: sessionType, sourcePID: pid}
		}
	}
	return sessionDisplayInfo{}
}

// orDefault returns s if non-empty, otherwise fallback.
func orDefault(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// hasCapSysPtrace checks if the current process has CAP_SYS_PTRACE in its
// effective set. Reads /proc/self/status to avoid a cgo dependency on libcap.
func hasCapSysPtrace() bool {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			// CAP_SYS_PTRACE is bit 19 = 0x80000.
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				var cap uint64
				fmt.Sscanf(fields[1], "%x", &cap)
				return cap&0x80000 != 0
			}
		}
	}
	return false
}

// captureScreenWindows captures the display using PowerShell + .NET
// System.Drawing. No extra tools required — always available on Windows.
func captureScreenWindows(outputPath string) error {
	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bmp = New-Object System.Drawing.Bitmap($bounds.Width, $bounds.Height)
$gfx = [System.Drawing.Graphics]::FromImage($bmp)
$gfx.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
$bmp.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)
$gfx.Dispose()
$bmp.Dispose()
`, outputPath)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell screenshot failed: %s: %w", string(out), err)
	}
	return nil
}
