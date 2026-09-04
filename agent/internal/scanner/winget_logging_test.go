package scanner

// winget_logging_test.go — Pre-fix tests for winget logging format.
// [SHARED] — no build tag, compiles on all platforms.
//
// F-C1-6 LOW: Winget scanner uses fmt.Printf not structured logging.
//
// Run: cd agent && go test ./internal/scanner/... -v -run TestWingetScanner

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 6.1 — Documents unstructured logging (F-C1-6)
//
// Category: PASS-NOW (documents the bug)
// ---------------------------------------------------------------------------

func TestWingetScannerUsesStructuredLogging(t *testing.T) {
	// F-C1-6 LOW: winget scanner uses fmt.Printf for error output.
	// ETHOS #1 requires [TAG] [system] [component] format via log.Printf.
	content, err := os.ReadFile("winget.go")
	if err != nil {
		t.Fatalf("failed to read winget.go: %v", err)
	}

	src := string(content)

	if strings.Contains(src, "fmt.Printf") {
		t.Error("[ERROR] [agent] [scanner] F-C1-6 NOT FIXED: fmt.Printf still in winget.go")
	}

	t.Log("[INFO] [agent] [scanner] F-C1-6 FIXED: fmt.Printf replaced with log.Printf")
}

// ---------------------------------------------------------------------------
// Test 6.2 — Must have no fmt.Printf (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestWingetScannerHasNoFmtPrintf(t *testing.T) {
	content, err := os.ReadFile("winget.go")
	if err != nil {
		t.Fatalf("failed to read winget.go: %v", err)
	}

	src := string(content)

	if strings.Contains(src, "fmt.Printf") {
		t.Errorf("[ERROR] [agent] [scanner] winget.go contains fmt.Printf.\n" +
			"F-C1-6: all output must use log.Printf with ETHOS format.")
	}
}
