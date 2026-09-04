package client

// machine_id_logging_test.go — Pre-fix tests for machine ID logging format.
//
// F-D1-5 LOW: client.go:39 uses fmt.Printf instead of log.Printf.
//
// Run: cd agent && go test ./internal/client/... -v -run TestClientMachineID

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 6.1 — Documents fmt.Printf usage (F-D1-5)
//
// Category: PASS-NOW (documents ETHOS violation)
// ---------------------------------------------------------------------------

func TestClientMachineIDErrorUsesFmtPrintf(t *testing.T) {
	// POST-FIX (F-D1-5): fmt.Printf replaced with log.Printf.
	content, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatalf("failed to read client.go: %v", err)
	}

	src := string(content)

	newClientIdx := strings.Index(src, "func NewClient(")
	if newClientIdx == -1 {
		t.Fatal("[ERROR] [agent] [client] NewClient function not found")
	}

	fnBody := src[newClientIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if strings.Contains(fnBody, "fmt.Printf") {
		t.Error("[ERROR] [agent] [client] F-D1-5 NOT FIXED: fmt.Printf still in NewClient")
	}

	t.Log("[INFO] [agent] [client] F-D1-5 FIXED: structured logging in NewClient")
}

// ---------------------------------------------------------------------------
// Test 6.2 — Must use structured logging (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestClientMachineIDErrorUsesStructuredLogging(t *testing.T) {
	content, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatalf("failed to read client.go: %v", err)
	}

	src := string(content)

	newClientIdx := strings.Index(src, "func NewClient(")
	if newClientIdx == -1 {
		t.Fatal("[ERROR] [agent] [client] NewClient function not found")
	}

	fnBody := src[newClientIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if strings.Contains(fnBody, "fmt.Printf") {
		t.Errorf("[ERROR] [agent] [client] NewClient uses fmt.Printf for machine ID error.\n" +
			"F-D1-5: use log.Printf with [WARNING] [agent] [client] format.")
	}
}
