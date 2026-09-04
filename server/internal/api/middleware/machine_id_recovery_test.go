package middleware_test

// machine_id_recovery_test.go — Tests for machine ID recovery path.
//
// F-D1-2 FIXED: POST /admin/agents/:id/rebind-machine-id endpoint added.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMachineBindingHasNoUpdatePath(t *testing.T) {
	// POST-FIX: Rebind endpoint exists in main.go admin routes.
	mainPath := filepath.Join("..", "..", "..", "cmd", "server", "main.go")
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	src := strings.ToLower(string(content))

	if !strings.Contains(src, "rebind-machine-id") {
		t.Error("[ERROR] [server] [middleware] F-D1-2 NOT FIXED: no rebind endpoint")
	}

	t.Log("[INFO] [server] [middleware] F-D1-2 FIXED: rebind-machine-id endpoint exists")
}

func TestMachineBindingShouldHaveUpdatePath(t *testing.T) {
	mainPath := filepath.Join("..", "..", "..", "cmd", "server", "main.go")
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	src := strings.ToLower(string(content))

	if !strings.Contains(src, "rebind-machine-id") {
		t.Errorf("[ERROR] [server] [middleware] no rebind endpoint found.\n" +
			"F-D1-2: admin endpoint for machine ID rebind required.")
	}

	t.Log("[INFO] [server] [middleware] F-D1-2 FIXED: admin rebind endpoint registered")
}
