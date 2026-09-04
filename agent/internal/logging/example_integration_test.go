package logging

// example_integration_test.go — Pre-fix tests for dead example code.
//
// F-D1-3 LOW: example_integration.go is dead code calling machineid.ID()
//   directly, bypassing the canonical hashMachineID().
//
// Run: cd agent && go test ./internal/logging/... -v -run TestExample

import (
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 4.1 — Documents dead code exists (F-D1-3)
//
// Category: PASS-NOW (documents dead code)
// ---------------------------------------------------------------------------

func TestExampleIntegrationFileIsDeadCode(t *testing.T) {
	// POST-FIX (F-D1-3): File deleted.
	_, err := os.Stat("example_integration.go")
	if err == nil {
		t.Error("[ERROR] [agent] [logging] F-D1-3 NOT FIXED: dead code file still exists")
	}
	t.Log("[INFO] [agent] [logging] F-D1-3 FIXED: dead example code deleted")
}

// ---------------------------------------------------------------------------
// Test 4.2 — File should not exist (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestExampleIntegrationFileDoesNotExist(t *testing.T) {
	// F-D1-3: After fix, delete the file.
	_, err := os.Stat("example_integration.go")
	if err == nil {
		t.Errorf("[ERROR] [agent] [logging] example_integration.go still exists.\n" +
			"F-D1-3: delete dead example code with incorrect usage patterns.")
	}
}
