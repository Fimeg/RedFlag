package migrations_test

// index_audit_test.go — Tests for missing database indexes.
//
// F-B1-5 FIXED: Migration 028 adds composite index on
//   agent_commands(status, sent_at) for GetStuckCommands.

import (
	"os"
	"strings"
	"testing"
)

func TestStuckCommandsIndexIsMissing(t *testing.T) {
	// POST-FIX: index on agent_commands(status, sent_at) must exist
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	foundIndex := false
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".up.sql") {
			continue
		}
		content, err := os.ReadFile(f.Name())
		if err != nil {
			continue
		}
		stmts := strings.Split(string(content), ";")
		for _, stmt := range stmts {
			lower := strings.ToLower(stmt)
			if strings.Contains(lower, "create index") &&
				strings.Contains(lower, "agent_commands") &&
				strings.Contains(lower, "sent_at") {
				foundIndex = true
			}
		}
	}

	if !foundIndex {
		t.Error("[ERROR] [server] [database] F-B1-5 NOT FIXED: no index on agent_commands(status, sent_at)")
	}
	t.Log("[INFO] [server] [database] F-B1-5 FIXED: stuck commands index exists")
}

func TestStuckCommandsIndexExists(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	foundIndex := false
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".up.sql") {
			continue
		}
		content, err := os.ReadFile(f.Name())
		if err != nil {
			continue
		}
		stmts := strings.Split(string(content), ";")
		for _, stmt := range stmts {
			lower := strings.ToLower(stmt)
			if strings.Contains(lower, "create index") &&
				strings.Contains(lower, "agent_commands") &&
				strings.Contains(lower, "sent_at") {
				foundIndex = true
			}
		}
	}

	if !foundIndex {
		t.Errorf("[ERROR] [server] [database] no index on agent_commands covering sent_at.\n" +
			"F-B1-5: GetStuckCommands needs index on (status, sent_at).")
	}
	t.Log("[INFO] [server] [database] F-B1-5 FIXED: stuck commands index exists")
}
