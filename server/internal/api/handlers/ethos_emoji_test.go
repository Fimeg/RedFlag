package handlers_test

// ethos_emoji_test.go — Pre-fix tests for emoji in handler log statements.
// D-2: agents.go, agent_updates.go, update_handler.go, updates.go have emoji in logs.

import (
	"os"
	"strings"
	"testing"
)

func hasEmojiChar(s string) bool {
	for _, r := range s {
		if r >= 0x1F300 || (r >= 0x2600 && r <= 0x27BF) {
			return true
		}
	}
	return false
}

func countEmojiLogLines(filepath string) (int, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if (strings.Contains(trimmed, "log.Printf") || strings.Contains(trimmed, "log.Println")) && hasEmojiChar(trimmed) {
			count++
		}
	}
	return count, nil
}

func TestAgentsHandlerHasEmojiInLogs(t *testing.T) {
	// D-2: agents.go has ~9 emoji in log statements including
	// heartbeat, rapid mode, and status indicators.
	count, err := countEmojiLogLines("agents.go")
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	if count > 0 {
		t.Errorf("[ERROR] [server] [handlers] D-2 NOT FIXED: %d emoji log lines in agents.go", count)
	}

	t.Log("[INFO] [server] [handlers] D-2 FIXED: no emoji in agents.go logs")
}

func TestAgentsHandlerHasNoEmojiInLogs(t *testing.T) {
	count, err := countEmojiLogLines("agents.go")
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	if count > 0 {
		t.Errorf("[ERROR] [server] [handlers] %d emoji-containing log statements in agents.go.\n"+
			"D-2: replace emoji with ETHOS [TAG] format text.", count)
	}
}

func TestUpdateHandlersHaveEmojiInLogs(t *testing.T) {
	// D-2: agent_updates.go, update_handler.go, updates.go have emoji in logs.
	files := []string{"agent_updates.go", "update_handler.go", "updates.go"}
	total := 0
	for _, f := range files {
		count, err := countEmojiLogLines(f)
		if err != nil {
			t.Logf("[WARNING] [server] [handlers] could not read %s: %v", f, err)
			continue
		}
		total += count
	}

	if total > 0 {
		t.Errorf("[ERROR] [server] [handlers] D-2 NOT FIXED: %d emoji log lines in update handlers", total)
	}

	t.Log("[INFO] [server] [handlers] D-2 FIXED: no emoji in update handler logs")
}

func TestUpdateHandlersHaveNoEmojiInLogs(t *testing.T) {
	files := []string{"agent_updates.go", "update_handler.go", "updates.go"}
	for _, f := range files {
		count, err := countEmojiLogLines(f)
		if err != nil {
			continue
		}
		if count > 0 {
			t.Errorf("[ERROR] [server] [handlers] %d emoji log lines in %s", count, f)
		}
	}
}
