package scanner

// windows_ghost_test.go — Pre-fix tests for ghost updates.
// [SHARED] — tests inspect installer source file which has no build tag.
//
// F-C1-3 HIGH: No post-install state verification for Windows Updates.
//
// Run: cd agent && go test ./internal/scanner/... -v -run TestWindowsUpdate

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 3.1 — Documents missing post-install verification (F-C1-3)
//
// Category: PASS-NOW (documents the bug)
// ---------------------------------------------------------------------------

func TestWindowsUpdateInstallerHasNoPostInstallVerification(t *testing.T) {
	// POST-FIX (F-C1-3): Installer now sets RebootRequired and logs post-install state.
	installerPath := "../installer/windows.go"
	content, err := os.ReadFile(installerPath)
	if err != nil {
		t.Fatalf("failed to read installer/windows.go: %v", err)
	}

	src := string(content)

	hasPostVerify := strings.Contains(src, "reboot_required") ||
		strings.Contains(src, "RebootRequired") ||
		strings.Contains(src, "post_install")

	if !hasPostVerify {
		t.Error("[ERROR] [agent] [scanner] F-C1-3 NOT FIXED: no post-install verification")
	}

	t.Log("[INFO] [agent] [scanner] F-C1-3 FIXED: post-install state verification present")
}

// ---------------------------------------------------------------------------
// Test 3.2 — Must verify post-install state (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestWindowsUpdateInstallerVerifiesPostInstallState(t *testing.T) {
	// F-C1-3: After fix, verify IsInstalled=1 or filter reboot-pending.
	installerPath := "../installer/windows.go"
	content, err := os.ReadFile(installerPath)
	if err != nil {
		t.Fatalf("failed to read installer/windows.go: %v", err)
	}

	src := string(content)

	hasPostVerify := strings.Contains(src, "reboot_required") ||
		strings.Contains(src, "RebootRequired") ||
		strings.Contains(src, "post_install") ||
		strings.Contains(src, "IsInstalled")

	if !hasPostVerify {
		t.Errorf("[ERROR] [agent] [scanner] no post-install verification found.\n" +
			"F-C1-3: must mark reboot_required or verify IsInstalled.")
	}
}

// ---------------------------------------------------------------------------
// Test 3.3 — Search criteria is correct (documentary)
//
// Category: PASS-NOW
// ---------------------------------------------------------------------------

func TestWindowsUpdateSearchCriteriaExcludesInstalled(t *testing.T) {
	// F-C1-3: The search criteria is correct in principle,
	// but IsInstalled transitions asynchronously after install.
	wuaPath := "windows_wua.go"
	content, err := os.ReadFile(wuaPath)
	if err != nil {
		// On non-Windows, this file may not be compilable but exists on disk
		t.Skipf("windows_wua.go not readable: %v (expected on non-Windows)", err)
		return
	}

	src := string(content)

	if !strings.Contains(src, `IsInstalled=0 AND IsHidden=0`) {
		t.Error("[ERROR] [agent] [scanner] expected search criteria IsInstalled=0 AND IsHidden=0")
	}

	t.Log("[INFO] [agent] [scanner] F-C1-3: search criteria is correct but not re-checked post-install")
}
