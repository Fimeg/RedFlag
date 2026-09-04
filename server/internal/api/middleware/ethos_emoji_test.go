package middleware_test

// ethos_emoji_test.go — Pre-fix tests for emoji in middleware log statements.
// D-2: machine_binding.go uses emoji in security log messages.

import (
	"os"
	"strings"
	"testing"
)

func hasEmoji(s string) bool {
	for _, r := range s {
		if r >= 0x1F300 || (r >= 0x2600 && r <= 0x27BF) {
			return true
		}
	}
	return false
}

func TestMachineBindingMiddlewareHasEmojiInLogs(t *testing.T) {
	// D-2: machine_binding.go uses emoji in security alert and
	// validation log messages. ETHOS #1 prohibits emoji in log output.
	content, err := os.ReadFile("machine_binding.go")
	if err != nil {
		t.Fatalf("failed to read machine_binding.go: %v", err)
	}

	src := string(content)

	// Find log lines with emoji
	lines := strings.Split(src, "\n")
	emojiLogCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if (strings.Contains(trimmed, "log.Printf") || strings.Contains(trimmed, "fmt.Printf")) && hasEmoji(trimmed) {
			emojiLogCount++
		}
	}

	if emojiLogCount > 0 {
		t.Errorf("[ERROR] [server] [middleware] D-2 NOT FIXED: %d emoji log lines remain", emojiLogCount)
	}

	t.Log("[INFO] [server] [middleware] D-2 FIXED: no emoji in machine_binding.go logs")
}

func TestMachineBindingMiddlewareHasNoEmojiInLogs(t *testing.T) {
	content, err := os.ReadFile("machine_binding.go")
	if err != nil {
		t.Fatalf("failed to read machine_binding.go: %v", err)
	}

	src := string(content)
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if (strings.Contains(trimmed, "log.Printf") || strings.Contains(trimmed, "fmt.Printf")) && hasEmoji(trimmed) {
			t.Errorf("[ERROR] [server] [middleware] emoji in log at line %d: %s", i+1, trimmed[:80])
		}
	}
}
