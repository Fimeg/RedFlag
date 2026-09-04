package database_test

// stuck_command_retry_test.go — Tests for stuck command retry limit.
//
// F-B2-10 FIXED: retry_count column added (migration 029).
//   GetStuckCommands filters retry_count < 5.
//   RedeliverStuckCommandTx increments retry_count on re-delivery (DEV-029 fix).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStuckCommandHasNoMaxRetryCount(t *testing.T) {
	// POST-FIX: retry_count column exists, filter in query, and increment wired.
	migrationsDir := filepath.Join("migrations")
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	hasRetryCount := false
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".up.sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(migrationsDir, f.Name()))
		if err != nil {
			continue
		}
		src := strings.ToLower(string(content))
		if strings.Contains(src, "agent_commands") && strings.Contains(src, "retry_count") {
			hasRetryCount = true
		}
	}

	if !hasRetryCount {
		t.Error("[ERROR] [server] [database] F-B2-10 NOT FIXED: no retry_count column")
	}

	// Check GetStuckCommands for retry limit
	cmdPath := filepath.Join("queries", "commands.go")
	content, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("failed to read commands.go: %v", err)
	}

	src := string(content)

	// Verify filter exists in GetStuckCommands
	stuckIdx := strings.Index(src, "func (q *CommandQueries) GetStuckCommands")
	if stuckIdx == -1 {
		t.Fatal("[ERROR] [server] [database] GetStuckCommands function not found")
	}
	stuckBody := src[stuckIdx:]
	if len(stuckBody) > 500 {
		stuckBody = stuckBody[:500]
	}
	if !strings.Contains(strings.ToLower(stuckBody), "retry_count") {
		t.Error("[ERROR] [server] [database] F-B2-10 NOT FIXED: GetStuckCommands has no retry filter")
	}

	// DEV-029: Verify RedeliverStuckCommandTx increments retry_count
	if !strings.Contains(src, "RedeliverStuckCommandTx") {
		t.Error("[ERROR] [server] [database] DEV-029 NOT FIXED: no RedeliverStuckCommandTx function")
	}
	redeliverIdx := strings.Index(src, "func (q *CommandQueries) RedeliverStuckCommandTx")
	if redeliverIdx != -1 {
		redeliverBody := src[redeliverIdx:]
		if len(redeliverBody) > 300 {
			redeliverBody = redeliverBody[:300]
		}
		if !strings.Contains(redeliverBody, "retry_count = retry_count + 1") {
			t.Error("[ERROR] [server] [database] DEV-029 NOT FIXED: RedeliverStuckCommandTx does not increment retry_count")
		}
	}

	t.Log("[INFO] [server] [database] F-B2-10 + DEV-029 FIXED: retry count wired end-to-end")
}

func TestStuckCommandHasMaxRetryCount(t *testing.T) {
	// Verify the cap is wired end-to-end: increment on re-delivery, a retry_count
	// filter in the stuck-command queries, and a default cap of 5.
	//
	// NOTE: the cap value was parameterized in accab6de — the queries now filter
	// `retry_count < $3` (maxRetries arg) instead of a hardcoded `retry_count < 5`.
	// The literal 5 moved to the handler's default fallback, which is the real
	// source of the cap. This test asserts that invariant, not the old literal.
	cmdPath := filepath.Join("queries", "commands.go")
	content, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("failed to read commands.go: %v", err)
	}

	src := string(content)

	// Must have RedeliverStuckCommandTx with retry_count increment
	if !strings.Contains(src, "retry_count = retry_count + 1") {
		t.Errorf("[ERROR] [server] [database] retry_count is never incremented.\n" +
			"DEV-029: RedeliverStuckCommandTx must increment retry_count on re-delivery.")
	}

	// Must filter on retry_count (parameterized maxRetries) in the stuck queries
	if !strings.Contains(src, "retry_count < $") {
		t.Errorf("[ERROR] [server] [database] no retry_count filter in stuck command queries")
	}

	// The cap value must default to 5 in the handler fallback (agents.go).
	agentsPath := filepath.Join("..", "api", "handlers", "agents.go")
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}
	agentsSrc := string(agentsContent)
	if !strings.Contains(agentsSrc, "maxCommandRetries = 5") {
		t.Errorf("[ERROR] [server] [database] no default cap of 5 (maxCommandRetries = 5) in agents.go")
	}

	t.Log("[INFO] [server] [database] F-B2-10 + DEV-029 FIXED: retry count capped, default 5")
}
