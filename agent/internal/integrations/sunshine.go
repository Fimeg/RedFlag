package integrations

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// sunshineDefaultWebPort is Sunshine's default HTTPS management UI port. If the
// operator moved it, the constructed URL will be wrong — that's a known limit of
// observe-only detection until we parse sunshine.conf (a later slice).
const sunshineDefaultWebPort = 47990

// sunshineReport mirrors the ReportedIntegration contract the web UI reads
// (web/src/types/integrations.ts). JSON keys must stay in sync.
type sunshineReport struct {
	State         string `json:"state"`             // detected | running | not_detected
	Version       string `json:"version,omitempty"` // observed via `sunshine --version`
	WebUI         string `json:"web_ui,omitempty"`  // https://<ip>:47990 (default)
	SessionActive bool   `json:"session_active"`    // not yet observable (slice 1)
	LastObserved  string `json:"last_observed"`     // RFC3339, when detection ran
}

// detectSunshine inspects the host for Sunshine. Returns nil only if Sunshine is
// neither installed nor running (so the integration block stays empty rather than
// carrying a noisy "not_detected" for every host that never had it). Observe-only:
// the only subprocess it ever spawns is `sunshine --version`, which is read-only.
func detectSunshine(primaryIP string) *sunshineReport {
	installed := false
	if _, err := exec.LookPath("sunshine"); err == nil {
		installed = true
	}
	running := sunshineRunning()

	if !installed && !running {
		return nil
	}

	rep := &sunshineReport{
		LastObserved: time.Now().UTC().Format(time.RFC3339),
	}
	switch {
	case running:
		rep.State = "running"
	default:
		rep.State = "detected"
	}

	if installed {
		if v := sunshineVersion(); v != "" {
			rep.Version = v
		}
	}
	if primaryIP != "" {
		rep.WebUI = fmt.Sprintf("https://%s:%d", primaryIP, sunshineDefaultWebPort)
	}

	log.Printf("[INFO] [agent] [integrations] sunshine_detected state=%s version=%q", rep.State, rep.Version)
	return rep
}

// sunshineRunning reports whether a sunshine process is currently live. Uses the
// same plain-exec, GOOS-switched idiom as the rest of the system package — no new
// dependency. Best-effort: a failure to enumerate returns false, never an error.
func sunshineRunning() bool {
	const name = "sunshine"
	switch runtime.GOOS {
	case "linux", "darwin":
		// pgrep is the cheapest exact-match; exit 0 means a match exists.
		if exec.Command("pgrep", "-x", name).Run() == nil {
			return true
		}
		// Fallback for hosts without pgrep: scan the process table.
		if out, err := exec.Command("ps", "-e", "-o", "comm").Output(); err == nil {
			return processListContains(string(out), name)
		}
	case "windows":
		if out, err := exec.Command("tasklist", "/fo", "csv", "/nh").Output(); err == nil {
			return processListContains(strings.ToLower(string(out)), name)
		}
	}
	return false
}

func processListContains(output, name string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(strings.TrimSpace(line), name) {
			return true
		}
	}
	return false
}

// sunshineVersion reads the reported version. `sunshine --version` is read-only and
// does not start the streaming service. Guarded by a short timeout so a wedged
// binary can never stall the system-info report (ETHOS #3 — assume failure).
func sunshineVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sunshine", "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	// Output is typically a single line like "Sunshine version: v0.23.1".
	line := strings.TrimSpace(string(out))
	if line == "" {
		return ""
	}
	if idx := strings.LastIndex(line, ":"); idx >= 0 && idx+1 < len(line) {
		return strings.TrimSpace(line[idx+1:])
	}
	// Fall back to the last whitespace-delimited token.
	fields := strings.Fields(line)
	if len(fields) > 0 {
		return fields[len(fields)-1]
	}
	return line
}
