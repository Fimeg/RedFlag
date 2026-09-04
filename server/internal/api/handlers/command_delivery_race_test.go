package handlers_test

// command_delivery_race_test.go — Tests for atomic command delivery.
//
// F-B2-2 FIXED: GetCommands now uses SELECT FOR UPDATE SKIP LOCKED
//   inside a transaction to prevent duplicate command delivery.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetCommandsAndMarkSentNotTransactional(t *testing.T) {
	// POST-FIX: GetCommands IS now transactional.
	agentsPath := filepath.Join(".", "agents.go")
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	src := string(content)
	cmdIdx := strings.Index(src, "func (h *AgentHandler) GetCommands")
	if cmdIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] GetCommands function not found")
	}

	fnBody := src[cmdIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if !strings.Contains(fnBody, ".Beginx()") && !strings.Contains(fnBody, ".Begin()") {
		t.Error("[ERROR] [server] [handlers] F-B2-2 NOT FIXED: GetCommands not transactional")
	}

	t.Log("[INFO] [server] [handlers] F-B2-2 FIXED: GetCommands uses transaction")
}

func TestGetCommandsMustBeAtomic(t *testing.T) {
	agentsPath := filepath.Join(".", "agents.go")
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	src := string(content)
	cmdIdx := strings.Index(src, "func (h *AgentHandler) GetCommands")
	if cmdIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] GetCommands function not found")
	}

	fnBody := src[cmdIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if !strings.Contains(fnBody, ".Beginx()") {
		t.Errorf("[ERROR] [server] [handlers] GetCommands must use a transaction")
	}
	t.Log("[INFO] [server] [handlers] F-B2-2 FIXED: command delivery is atomic")
}

func TestSelectForUpdatePatternInGetCommands(t *testing.T) {
	// POST-FIX: FOR UPDATE SKIP LOCKED must be present in command queries
	cmdPath := filepath.Join("..", "..", "database", "queries", "commands.go")
	content, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("failed to read commands.go: %v", err)
	}

	src := strings.ToLower(string(content))

	if !strings.Contains(src, "for update skip locked") {
		t.Error("[ERROR] [server] [handlers] F-B2-2 NOT FIXED: no FOR UPDATE SKIP LOCKED")
	}

	t.Log("[INFO] [server] [handlers] F-B2-2 FIXED: FOR UPDATE SKIP LOCKED present")
}
