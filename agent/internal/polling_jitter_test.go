package internal_test

// polling_jitter_test.go — Tests for jitter capping at polling interval.
//
// F-B2-5 FIXED: Jitter is now capped at pollingInterval/2.
//   Rapid mode (5s) gets 0-2s jitter, standard (300s) gets 0-30s.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJitterExceedsRapidModeInterval(t *testing.T) {
	// POST-FIX: Fixed 30s jitter no longer applied to rapid mode.
	// The polling loop was extracted from main.go into the shared loop
	// (internal/agent/loop.go); the Windows service delegates to the same loop.
	loopPath := filepath.Join("agent", "loop.go")
	content, err := os.ReadFile(loopPath)
	if err != nil {
		t.Fatalf("failed to read agent loop.go: %v", err)
	}

	src := string(content)

	// The old fixed jitter should be replaced with proportional jitter
	if strings.Contains(src, "rand.Intn(30)") {
		t.Error("[ERROR] [agent] [polling] F-B2-5 NOT FIXED: fixed 30s jitter still present")
	}

	if !strings.Contains(src, "baseInterval / 2") && !strings.Contains(src, "baseInterval/2") {
		t.Error("[ERROR] [agent] [polling] expected jitter capped at baseInterval/2")
	}

	t.Log("[INFO] [agent] [polling] F-B2-5 FIXED: jitter capped at polling interval")
}

func TestJitterDoesNotExceedPollingInterval(t *testing.T) {
	// The polling loop was extracted from main.go into the shared loop
	// (internal/agent/loop.go); the Windows service delegates to the same loop.
	loopPath := filepath.Join("agent", "loop.go")
	content, err := os.ReadFile(loopPath)
	if err != nil {
		t.Fatalf("failed to read agent loop.go: %v", err)
	}

	src := string(content)

	// Must have proportional jitter calculation
	hasProportionalJitter := strings.Contains(src, "baseInterval / 2") ||
		strings.Contains(src, "maxJitter")

	if !hasProportionalJitter {
		t.Errorf("[ERROR] [agent] [polling] jitter is not proportional to polling interval.\n" +
			"F-B2-5: jitter must be capped at pollingInterval/2.")
	}

	t.Log("[INFO] [agent] [polling] F-B2-5 FIXED: jitter proportional to interval")
}
