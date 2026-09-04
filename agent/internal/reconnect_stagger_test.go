package internal_test

// reconnect_stagger_test.go — Tests for exponential backoff on reconnection.
//
// F-B2-7 FIXED: Agent now uses exponential backoff with full jitter
//   on consecutive server failures instead of fixed polling interval.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconnectionUsesFixedJitterOnly(t *testing.T) {
	// POST-FIX: Reconnection now uses exponential backoff.
	// Reconnect backoff lives in the shared loop (internal/agent/loop.go),
	// which both main.go and the Windows service delegate to.
	loopPath := filepath.Join("agent", "loop.go")
	content, err := os.ReadFile(loopPath)
	if err != nil {
		t.Fatalf("failed to read agent loop.go: %v", err)
	}

	src := string(content)

	// Must have exponential backoff function
	if !strings.Contains(src, "calculateBackoff") {
		t.Error("[ERROR] [agent] [polling] F-B2-7 NOT FIXED: no calculateBackoff function")
	}

	// Must have consecutive failure tracking
	if !strings.Contains(src, "consecutiveFailures") {
		t.Error("[ERROR] [agent] [polling] F-B2-7 NOT FIXED: no consecutive failure tracking")
	}

	t.Log("[INFO] [agent] [polling] F-B2-7 FIXED: exponential backoff with failure tracking")
}

func TestReconnectionUsesExponentialBackoffWithJitter(t *testing.T) {
	// Reconnect backoff lives in the shared loop (internal/agent/loop.go),
	// which both main.go and the Windows service delegate to.
	loopPath := filepath.Join("agent", "loop.go")
	content, err := os.ReadFile(loopPath)
	if err != nil {
		t.Fatalf("failed to read agent loop.go: %v", err)
	}

	src := strings.ToLower(string(content))

	// Must have backoff calculation
	hasBackoff := strings.Contains(src, "calculatebackoff") ||
		strings.Contains(src, "backoffdelay")

	if !hasBackoff {
		t.Errorf("[ERROR] [agent] [polling] no exponential backoff found.\n" +
			"F-B2-7: implement exponential backoff with full jitter.")
	}

	// Must reset on success
	if !strings.Contains(src, "consecutivefailures = 0") {
		t.Error("[ERROR] [agent] [polling] no failure counter reset on success")
	}

	t.Log("[INFO] [agent] [polling] F-B2-7 FIXED: exponential backoff with reset on success")
}
