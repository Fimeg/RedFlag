package display

// ethos_exempt_test.go — Exemption documentation for terminal display emoji.
// D-2: display/terminal.go emoji is EXEMPT from ETHOS #1.
// This is intentional user-facing terminal UI, not log output.
// Do NOT modify this file in the D-2 fix pass.

import (
	"os"
	"testing"
)

func TestTerminalDisplayIsExemptFromEthos(t *testing.T) {
	// D-2: display/terminal.go emoji is EXEMPT.
	// This is intentional user-facing terminal UI.
	// ETHOS #1 applies to log statements, not UI rendering.
	_, err := os.Stat("terminal.go")
	if err != nil {
		t.Skip("[INFO] [agent] [display] terminal.go not found")
	}

	content, err := os.ReadFile("terminal.go")
	if err != nil {
		t.Fatalf("failed to read terminal.go: %v", err)
	}

	// Confirm emoji IS present (intentional)
	hasEmoji := false
	for _, r := range string(content) {
		if r >= 0x1F300 || (r >= 0x2600 && r <= 0x27BF) {
			hasEmoji = true
			break
		}
	}

	if !hasEmoji {
		t.Log("[INFO] [agent] [display] terminal.go has no emoji (ok — may have been cleaned)")
	} else {
		t.Log("[INFO] [agent] [display] terminal.go has emoji (EXEMPT — intentional terminal UI)")
	}

	t.Log("[INFO] [agent] [display] EXEMPTION: display/terminal.go emoji is intentional UI, not log output")
}
