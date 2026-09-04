package migrations_test

// migration024_test.go — Tests for migration 024 fixes.
//
// F-B1-1 FIXED: Self-insert into schema_migrations removed.
// F-B1-2 FIXED: Non-existent `deprecated` column reference removed.
//   Migration now uses existing enabled/auto_run columns.

import (
	"os"
	"strings"
	"testing"
)

func TestMigration024HasSelfInsert(t *testing.T) {
	// POST-FIX: migration 024 must NOT contain self-insert
	content, err := os.ReadFile("024_disable_updates_subsystem.up.sql")
	if err != nil {
		t.Fatalf("failed to read migration 024: %v", err)
	}
	if strings.Contains(string(content), "INSERT INTO schema_migrations") {
		t.Error("[ERROR] [server] [database] F-B1-1: migration 024 still contains self-insert")
	}
	t.Log("[INFO] [server] [database] F-B1-1 FIXED: no self-insert in migration 024")
}

func TestMigration024ShouldNotHaveSelfInsert(t *testing.T) {
	content, err := os.ReadFile("024_disable_updates_subsystem.up.sql")
	if err != nil {
		t.Fatalf("failed to read migration 024: %v", err)
	}
	if strings.Contains(string(content), "INSERT INTO schema_migrations") {
		t.Errorf("[ERROR] [server] [database] migration 024 contains self-insert into schema_migrations.\n" +
			"F-B1-1: the migration runner handles schema_migrations tracking.")
	}
}

func TestMigration024ReferencesDeprecatedColumn(t *testing.T) {
	// POST-FIX: migration 024 must NOT reference `deprecated` column
	content, err := os.ReadFile("024_disable_updates_subsystem.up.sql")
	if err != nil {
		t.Fatalf("failed to read migration 024: %v", err)
	}
	// Check for "deprecated" as a column SET, not in comments
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), "deprecated") {
			t.Error("[ERROR] [server] [database] F-B1-2: migration 024 still references deprecated column")
			return
		}
	}
	t.Log("[INFO] [server] [database] F-B1-2 FIXED: no deprecated column reference in migration 024")
}

func TestMigration024ColumnExistsInSchema(t *testing.T) {
	// POST-FIX: migration 024 only uses columns that exist on agent_subsystems
	// (enabled, auto_run, updated_at — all defined in migration 015)
	content024, err := os.ReadFile("024_disable_updates_subsystem.up.sql")
	if err != nil {
		t.Fatalf("failed to read migration 024: %v", err)
	}
	content015, err := os.ReadFile("015_agent_subsystems.up.sql")
	if err != nil {
		t.Fatalf("failed to read migration 015: %v", err)
	}

	// Verify the columns 024 uses are in 015's CREATE TABLE
	src015 := string(content015)
	src024 := string(content024)

	// 024 sets enabled, auto_run, updated_at — all must be in 015
	for _, col := range []string{"enabled", "auto_run", "updated_at"} {
		if strings.Contains(src024, col) && !strings.Contains(src015, col) {
			t.Errorf("[ERROR] [server] [database] migration 024 uses column %q not defined in 015", col)
		}
	}
	t.Log("[INFO] [server] [database] F-B1-2 FIXED: all columns used by 024 exist in schema")
}
