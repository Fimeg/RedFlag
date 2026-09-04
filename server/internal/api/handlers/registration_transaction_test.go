package handlers_test

// registration_transaction_test.go — Tests for registration transaction safety.
//
// F-B2-1/F-B2-8 FIXED: Registration now wraps all DB operations in a
//   single transaction. No manual rollback needed.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistrationFlowIsNotTransactional(t *testing.T) {
	// POST-FIX: Registration IS now transactional.
	agentsPath := filepath.Join(".", "agents.go")
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	src := string(content)
	regIdx := strings.Index(src, "func (h *AgentHandler) RegisterAgent")
	if regIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] RegisterAgent function not found")
	}

	fnBody := src[regIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if !strings.Contains(fnBody, ".Beginx()") && !strings.Contains(fnBody, ".Begin()") {
		t.Error("[ERROR] [server] [handlers] F-B2-1 NOT FIXED: RegisterAgent still non-transactional")
	}

	t.Log("[INFO] [server] [handlers] F-B2-1 FIXED: RegisterAgent uses transaction")
}

func TestRegistrationFlowMustBeTransactional(t *testing.T) {
	agentsPath := filepath.Join(".", "agents.go")
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	src := string(content)
	regIdx := strings.Index(src, "func (h *AgentHandler) RegisterAgent")
	if regIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] RegisterAgent function not found")
	}

	fnBody := src[regIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if !strings.Contains(fnBody, ".Beginx()") && !strings.Contains(fnBody, ".Begin()") {
		t.Errorf("[ERROR] [server] [handlers] RegisterAgent must use a transaction")
	}

	if !strings.Contains(fnBody, "tx.Commit()") {
		t.Errorf("[ERROR] [server] [handlers] RegisterAgent transaction must commit")
	}

	t.Log("[INFO] [server] [handlers] F-B2-1 FIXED: registration is transactional")
}

func TestRegistrationManualRollbackExists(t *testing.T) {
	// POST-FIX: Manual rollback (DeleteAgent) should be GONE,
	// replaced by transaction rollback.
	agentsPath := filepath.Join(".", "agents.go")
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	src := string(content)
	regIdx := strings.Index(src, "func (h *AgentHandler) RegisterAgent")
	if regIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] RegisterAgent function not found")
	}

	fnBody := src[regIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if strings.Contains(fnBody, "DeleteAgent") {
		t.Error("[ERROR] [server] [handlers] manual DeleteAgent rollback still present; should be replaced by transaction")
	}

	if !strings.Contains(fnBody, "defer tx.Rollback()") {
		t.Error("[ERROR] [server] [handlers] expected defer tx.Rollback() for transaction safety")
	}

	t.Log("[INFO] [server] [handlers] F-B2-1 FIXED: manual rollback replaced by transaction")
}
