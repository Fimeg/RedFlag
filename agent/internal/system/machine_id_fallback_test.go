package system

// machine_id_fallback_test.go — Tests for machine ID registration fallback.
//
// F-D1-1 FIXED: Registration no longer uses unhashed "unknown-" fallback.
//   GetMachineID() is trusted as canonical; registration aborts if it fails.

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistrationFallbackIsNotHashed(t *testing.T) {
	// POST-FIX: "unknown-" fallback is gone from main.go.
	mainPath := filepath.Join("..", "..", "cmd", "agent", "main.go")
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	src := string(content)

	if strings.Contains(src, `"unknown-"`) {
		t.Error("[ERROR] [agent] [system] F-D1-1 NOT FIXED: 'unknown-' fallback still present")
	}

	t.Log("[INFO] [agent] [system] F-D1-1 FIXED: unhashed fallback removed from registration")
}

func TestRegistrationFallbackUsesCanonicalFunction(t *testing.T) {
	// POST-FIX: Registration uses system.GetMachineID() with no inline fallback.
	// The registration flow lives in internal/registration/service.go, which
	// calls the canonical function and aborts if it fails (no "unknown-" path).
	regPath := filepath.Join("..", "registration", "service.go")
	content, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatalf("failed to read registration/service.go: %v", err)
	}

	src := string(content)

	if strings.Contains(src, `"unknown-"`) {
		t.Errorf("[ERROR] [agent] [system] registration still uses 'unknown-' fallback.\n" +
			"F-D1-1: must use canonical GetMachineID() or abort.")
	}

	if !strings.Contains(src, "system.GetMachineID()") {
		t.Error("[ERROR] [agent] [system] registration doesn't call system.GetMachineID()")
	}

	t.Log("[INFO] [agent] [system] F-D1-1 FIXED: registration uses canonical function only")
}

func TestMachineIDIsAlways64HexChars(t *testing.T) {
	id, err := GetMachineID()
	if err != nil {
		t.Fatalf("GetMachineID failed: %v", err)
	}

	if len(id) != 64 {
		t.Errorf("[ERROR] [agent] [system] machine ID is %d chars, expected 64", len(id))
	}

	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("[ERROR] [agent] [system] machine ID is not valid hex: %v", err)
	}

	// Idempotency check
	id2, _ := GetMachineID()
	if id != id2 {
		t.Error("[ERROR] [agent] [system] GetMachineID not idempotent")
	}

	t.Logf("[INFO] [agent] [system] canonical machine ID: %s... (64 hex chars)", id[:16])
}

func TestRegistrationAndRuntimeUseSameCodePath(t *testing.T) {
	// POST-FIX: registration and runtime both resolve the machine ID through the
	// canonical system.GetMachineID(), with no divergent "unknown-" fallback.
	// Registration: internal/registration/service.go. Runtime: internal/client/client.go.
	regPath := filepath.Join("..", "registration", "service.go")
	regContent, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatalf("failed to read registration/service.go: %v", err)
	}

	clientPath := filepath.Join("..", "client", "client.go")
	clientContent, err := os.ReadFile(clientPath)
	if err != nil {
		t.Fatalf("failed to read client.go: %v", err)
	}

	regSrc := string(regContent)
	clientSrc := string(clientContent)

	if !strings.Contains(regSrc, "system.GetMachineID()") {
		t.Error("[ERROR] [agent] [system] registration doesn't call system.GetMachineID()")
	}
	if !strings.Contains(clientSrc, "system.GetMachineID()") {
		t.Error("[ERROR] [agent] [system] client.go doesn't call system.GetMachineID()")
	}

	if strings.Contains(regSrc, `"unknown-"`) {
		t.Errorf("[ERROR] [agent] [system] registration has divergent 'unknown-' fallback")
	}

	t.Log("[INFO] [agent] [system] F-D1-1 FIXED: both paths use canonical GetMachineID()")
}
