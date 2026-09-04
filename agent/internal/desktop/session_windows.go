package desktop

import (
	"os/exec"
	"strings"
)

// hasDesktopSession checks if a desktop session is available on Windows.
// Looks for an active console session via query session.
func hasDesktopSession() bool {
	// query session lists all sessions. An active console session with
	// a logged-in user means a desktop is available.
	out, err := exec.Command("query", "session").Output()
	if err != nil {
		return false
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		// Active console sessions have "Active" in the state column.
		if strings.Contains(line, "Active") && strings.Contains(line, "console") {
			return true
		}
	}

	return false
}
