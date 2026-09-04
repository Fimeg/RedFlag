package migration

// ethos_emoji_test.go — Pre-fix tests for emoji in migration executor output.
// D-2: migration/executor.go uses emoji in progress output.

import (
	"os"
	"strings"
	"testing"
)

func hasEmojiR(s string) bool {
	for _, r := range s {
		if r >= 0x1F300 || (r >= 0x2600 && r <= 0x27BF) {
			return true
		}
	}
	return false
}

func TestMigrationExecutorHasEmojiInOutput(t *testing.T) {
	// D-2: migration/executor.go uses emoji in progress output.
	content, err := os.ReadFile("executor.go")
	if err != nil {
		t.Fatalf("failed to read executor.go: %v", err)
	}

	src := string(content)
	lines := strings.Split(src, "\n")

	emojiCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if (strings.Contains(trimmed, "fmt.Printf") || strings.Contains(trimmed, "fmt.Println")) && hasEmojiR(trimmed) {
			emojiCount++
		}
	}

	if emojiCount > 0 {
		t.Errorf("[ERROR] [agent] [migration] D-2 NOT FIXED: %d output lines with emoji", emojiCount)
	}

	t.Log("[INFO] [agent] [migration] D-2 FIXED: no emoji in executor.go")
}

func TestMigrationExecutorHasNoEmojiInOutput(t *testing.T) {
	content, err := os.ReadFile("executor.go")
	if err != nil {
		t.Fatalf("failed to read executor.go: %v", err)
	}

	src := string(content)
	lines := strings.Split(src, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if (strings.Contains(trimmed, "fmt.Printf") || strings.Contains(trimmed, "fmt.Println")) && hasEmojiR(trimmed) {
			t.Errorf("[ERROR] [agent] [migration] emoji in output at line %d", i+1)
		}
	}
}
