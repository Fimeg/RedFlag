package database_test

// refresh_token_cleanup_test.go — Tests for background token cleanup.
//
// F-B1-10 FIXED: Background goroutine added to main.go that calls
//   CleanupExpiredTokens every 24 hours.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoBackgroundRefreshTokenCleanup(t *testing.T) {
	// POST-FIX: background cleanup now exists in main.go
	mainPath := filepath.Join("..", "..", "cmd", "server", "main.go")
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	src := strings.ToLower(string(content))

	if !strings.Contains(src, "cleanupexpiredtokens") {
		t.Error("[ERROR] [server] [database] F-B1-10 NOT FIXED: no CleanupExpiredTokens call in main.go")
		return
	}

	// Check it's in a background context: raw goroutine/ticker, or a
	// managed taskrunner registration (bgRunner.Every).
	idx := strings.Index(src, "cleanupexpiredtokens")
	start := idx - 300
	if start < 0 {
		start = 0
	}
	context := src[start:idx]
	if !strings.Contains(context, "go func") && !strings.Contains(context, "ticker") &&
		!strings.Contains(context, ".every(") {
		t.Error("[ERROR] [server] [database] CleanupExpiredTokens exists but not in background context")
		return
	}

	t.Log("[INFO] [server] [database] F-B1-10 FIXED: background refresh token cleanup exists")
}

func TestBackgroundRefreshTokenCleanupExists(t *testing.T) {
	mainPath := filepath.Join("..", "..", "cmd", "server", "main.go")
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	src := strings.ToLower(string(content))

	if !strings.Contains(src, "refresh_token_cleanup") {
		t.Errorf("[ERROR] [server] [database] no background refresh token cleanup found.\n" +
			"F-B1-10: must periodically call CleanupExpiredTokens.")
		return
	}

	t.Log("[INFO] [server] [database] F-B1-10 FIXED: background cleanup goroutine found")
}
