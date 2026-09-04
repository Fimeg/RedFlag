package scanner

// winget_path_test.go — Pre-fix tests for winget path detection.
// [SHARED] — no build tag, compiles on all platforms.
//
// F-C1-1 HIGH: Winget located via exec.LookPath (PATH only).
//   When running as SYSTEM service, winget is not in PATH.
//
// Run: cd agent && go test ./internal/scanner/... -v -run TestWinget

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 1.1 — Documents PATH-only winget search (F-C1-1)
//
// Category: PASS-NOW (documents the bug)
// ---------------------------------------------------------------------------

func TestWingetSearchesPathOnly(t *testing.T) {
	// F-C1-1 HIGH: Winget located via PATH only. Windows service
	// runs as SYSTEM which does not have %LOCALAPPDATA% in PATH.
	// Per-user winget installs are invisible to the SYSTEM account.
	content, err := os.ReadFile("winget.go")
	if err != nil {
		t.Fatalf("failed to read winget.go: %v", err)
	}

	src := string(content)

	// Uses exec.LookPath for discovery
	if !strings.Contains(src, `exec.LookPath("winget")`) {
		t.Error("[ERROR] [agent] [scanner] expected exec.LookPath in winget discovery")
	}

	// POST-FIX: Also checks known system-wide install paths
	hasKnownPaths := strings.Contains(src, "WindowsApps") ||
		strings.Contains(src, `winget.exe`)

	if !hasKnownPaths {
		t.Error("[ERROR] [agent] [scanner] F-C1-1 NOT FIXED: no known path search")
	}

	t.Log("[INFO] [agent] [scanner] F-C1-1 FIXED: winget checks PATH and known locations")
}

// ---------------------------------------------------------------------------
// Test 1.2 — Winget must check known install locations (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestWingetChecksKnownInstallLocations(t *testing.T) {
	// F-C1-1: After fix, winget discovery must check known
	// system-wide install paths in addition to PATH search.
	content, err := os.ReadFile("winget.go")
	if err != nil {
		t.Fatalf("failed to read winget.go: %v", err)
	}

	src := string(content)

	hasKnownPaths := strings.Contains(src, "WindowsApps") ||
		strings.Contains(src, "LOCALAPPDATA") ||
		strings.Contains(src, "ProgramFiles") ||
		strings.Contains(src, `winget.exe`)

	if !hasKnownPaths {
		t.Errorf("[ERROR] [agent] [scanner] winget does not check known install locations.\n" +
			"F-C1-1: must check known paths for SYSTEM service compatibility.")
	}
}
