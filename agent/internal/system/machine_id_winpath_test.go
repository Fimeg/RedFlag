package system

// machine_id_windows_test.go — Pre-fix tests for Windows machine ID redundancy.
// [SHARED] — no build tag, source inspection only.
//
// F-D1-4 LOW: getWindowsMachineID() redundantly retries machineid.ID().
//
// Run: cd agent && go test ./internal/system/... -v -run TestWindows

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 5.1 — Documents redundant retry (F-D1-4)
//
// Category: PASS-NOW (documents the redundancy)
// ---------------------------------------------------------------------------

func TestWindowsFallbackHasRedundantRetry(t *testing.T) {
	// POST-FIX (F-D1-4): Redundant retry removed.
	content, err := os.ReadFile("machine_id.go")
	if err != nil {
		t.Fatalf("failed to read machine_id.go: %v", err)
	}

	src := string(content)

	winIdx := strings.Index(src, "func getWindowsMachineID()")
	if winIdx == -1 {
		t.Fatal("[ERROR] [agent] [system] getWindowsMachineID not found")
	}

	fnBody := src[winIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	count := strings.Count(fnBody, "machineid.ID()")
	if count > 0 {
		t.Errorf("[ERROR] [agent] [system] F-D1-4 NOT FIXED: machineid.ID() still in Windows fallback (%d calls)", count)
	}

	t.Log("[INFO] [agent] [system] F-D1-4 FIXED: redundant retry removed from Windows fallback")
}

// ---------------------------------------------------------------------------
// Test 5.2 — Windows fallback should use alternative sources (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestWindowsFallbackUsesAlternativeSources(t *testing.T) {
	// F-D1-4: After fix, either remove the redundant retry or
	// check an alternative Windows source.
	content, err := os.ReadFile("machine_id.go")
	if err != nil {
		t.Fatalf("failed to read machine_id.go: %v", err)
	}

	src := string(content)

	winIdx := strings.Index(src, "func getWindowsMachineID()")
	if winIdx == -1 {
		t.Fatal("[ERROR] [agent] [system] getWindowsMachineID not found")
	}

	fnBody := src[winIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	// Should NOT have machineid.ID() (redundant retry removed)
	// OR should have an alternative source
	hasRedundantRetry := strings.Count(fnBody, "machineid.ID()") > 0
	hasAlternative := strings.Contains(fnBody, "Registry") ||
		strings.Contains(fnBody, "WMI") ||
		strings.Contains(fnBody, "InstallDate")

	if hasRedundantRetry && !hasAlternative {
		t.Errorf("[ERROR] [agent] [system] Windows fallback has redundant machineid.ID() retry.\n" +
			"F-D1-4: remove the retry or add an alternative source.")
	}
}
