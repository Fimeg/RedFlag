package main

// ethos_emoji_test.go — Tests for emoji in agent main.go log statements.
// D-2 FIXED: emoji removed from token renewal and install result log paths.
// EXCLUDES: registration CLI output and startup banner (exempt).

import (
	"os"
	"strings"
	"testing"
)

func hasEmojiRune(s string) bool {
	for _, r := range s {
		if r >= 0x1F300 || (r >= 0x2600 && r <= 0x27BF) {
			return true
		}
	}
	return false
}

// isExemptLine checks if a line number falls in an exempt range.
// Exempt ranges are user-facing CLI output (registration, startup banner).
func isExemptLine(lineNum int) bool {
	// Registration CLI output: ~lines 294-322
	if lineNum >= 290 && lineNum <= 330 {
		return true
	}
	// Startup banner: ~lines 691-700
	if lineNum >= 685 && lineNum <= 705 {
		return true
	}
	return false
}

func TestMainGoHasEmojiInLogStatements(t *testing.T) {
	// POST-FIX: No emoji in non-exempt log statements.
	content, err := os.ReadFile("agent/main.go")
	if err != nil {
		t.Fatalf("failed to read agent/main.go: %v", err)
	}

	lines := strings.Split(string(content), "\n")

	emojiLogCount := 0
	for i, line := range lines {
		if isExemptLine(i + 1) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		isLog := strings.Contains(trimmed, "log.Printf") || strings.Contains(trimmed, "log.Println")
		if isLog && hasEmojiRune(trimmed) {
			emojiLogCount++
		}
	}

	if emojiLogCount > 0 {
		t.Errorf("[ERROR] [agent] [main] D-2 NOT FIXED: %d non-exempt log statements with emoji", emojiLogCount)
	}

	t.Log("[INFO] [agent] [main] D-2 FIXED: no emoji in non-exempt log statements")
}

func TestMainGoLogStatementsHaveNoEmoji(t *testing.T) {
	content, err := os.ReadFile("agent/main.go")
	if err != nil {
		t.Fatalf("failed to read agent/main.go: %v", err)
	}

	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		if isExemptLine(i + 1) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		isLog := strings.Contains(trimmed, "log.Printf") || strings.Contains(trimmed, "log.Println")
		if isLog && hasEmojiRune(trimmed) {
			t.Errorf("[ERROR] [agent] [main] emoji in non-exempt log at line %d", i+1)
		}
	}
}
