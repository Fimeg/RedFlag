package handlers_test

// token_renewal_transaction_test.go — Tests for token renewal transaction safety.
//
// F-B2-9 FIXED: Token renewal now wraps validate + update in a transaction.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenRenewalIsNotTransactional(t *testing.T) {
	// POST-FIX: Token renewal IS now transactional.
	agentsPath := filepath.Join(".", "agents.go")
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	src := string(content)
	renewIdx := strings.Index(src, "func (h *AgentHandler) RenewToken")
	if renewIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] RenewToken function not found")
	}

	fnBody := src[renewIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if !strings.Contains(fnBody, ".Beginx()") {
		t.Error("[ERROR] [server] [handlers] F-B2-9 NOT FIXED: RenewToken not transactional")
	}
	t.Log("[INFO] [server] [handlers] F-B2-9 FIXED: RenewToken is transactional")
}

func TestTokenRenewalShouldBeTransactional(t *testing.T) {
	agentsPath := filepath.Join(".", "agents.go")
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	src := string(content)
	renewIdx := strings.Index(src, "func (h *AgentHandler) RenewToken")
	if renewIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] RenewToken function not found")
	}

	fnBody := src[renewIdx:]
	nextFn := strings.Index(fnBody[1:], "\nfunc ")
	if nextFn > 0 {
		fnBody = fnBody[:nextFn+1]
	}

	if !strings.Contains(fnBody, ".Beginx()") {
		t.Errorf("[ERROR] [server] [handlers] RenewToken must use a transaction")
	}
	t.Log("[INFO] [server] [handlers] F-B2-9 FIXED: renewal is transactional")
}
