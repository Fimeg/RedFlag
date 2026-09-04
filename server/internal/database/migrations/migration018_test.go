package migrations_test

// migration018_test.go — Tests for scanner_config migration fixes.
//
// F-B1-3 FIXED: Renamed from 018_create_scanner_config_table.sql (no .up.sql suffix)
//   to 027_create_scanner_config_table.up.sql (correct suffix, unique number).
//
// F-B1-4 FIXED: GRANT to non-existent role `redflag_user` removed.

import (
	"os"
	"strings"
	"testing"
)

func TestMigration018ScannerConfigHasWrongSuffix(t *testing.T) {
	// POST-FIX: old .sql file must be gone, new .up.sql must exist
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	for _, f := range files {
		if f.Name() == "018_create_scanner_config_table.sql" {
			t.Error("[ERROR] [server] [database] F-B1-3 NOT FIXED: old .sql file still exists")
			return
		}
	}
	t.Log("[INFO] [server] [database] F-B1-3 FIXED: old 018_create_scanner_config_table.sql removed")
}

func TestMigration018ScannerConfigHasCorrectSuffix(t *testing.T) {
	// POST-FIX: scanner_config migration must exist with .up.sql suffix
	// (renumbered to 027)
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	found := false
	for _, f := range files {
		if f.Name() == "027_create_scanner_config_table.up.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("[ERROR] [server] [database] 027_create_scanner_config_table.up.sql not found.\n" +
			"F-B1-3: scanner_config migration must have .up.sql suffix.")
	}
	t.Log("[INFO] [server] [database] F-B1-3 FIXED: scanner_config migration has correct suffix (027)")
}

func TestMigration018ScannerConfigHasNoGrantToWrongRole(t *testing.T) {
	// POST-FIX: no GRANT to redflag_user in the scanner_config migration
	content, err := os.ReadFile("027_create_scanner_config_table.up.sql")
	if err != nil {
		t.Fatalf("failed to read scanner_config migration: %v", err)
	}

	if strings.Contains(string(content), "redflag_user") {
		t.Errorf("[ERROR] [server] [database] scanner_config migration GRANTs to non-existent role.\n" +
			"F-B1-4: GRANT to redflag_user must be removed.")
	}
	t.Log("[INFO] [server] [database] F-B1-4 FIXED: no GRANT to wrong role")
}
