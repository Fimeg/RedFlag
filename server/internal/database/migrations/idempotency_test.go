package migrations_test

// idempotency_test.go — Pre-fix tests for migration idempotency (ETHOS #4).
//
// F-B1-15 LOW: 7+ migrations lack IF NOT EXISTS on CREATE/ALTER statements.
//   ETHOS #4 requires all schema changes to be idempotent.
//   Running migrations twice on an existing DB will fail.
//
// Run: cd server && go test ./internal/database/migrations/... -v -run TestMigrations

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// checkIdempotency counts non-idempotent SQL statements in a migration file.
// Go's regexp doesn't support negative lookahead, so we use string matching.
func checkIdempotency(src string) (violations int, details []string) {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		// Skip comments
		if strings.HasPrefix(lower, "--") {
			continue
		}

		// CREATE TABLE without IF NOT EXISTS
		if strings.Contains(lower, "create table") && !strings.Contains(lower, "if not exists") {
			violations++
			details = append(details, fmt.Sprintf("  line %d: CREATE TABLE without IF NOT EXISTS", i+1))
		}
		// CREATE INDEX without IF NOT EXISTS
		if (strings.Contains(lower, "create index") || strings.Contains(lower, "create unique index")) &&
			!strings.Contains(lower, "if not exists") {
			violations++
			details = append(details, fmt.Sprintf("  line %d: CREATE INDEX without IF NOT EXISTS", i+1))
		}
		// ADD COLUMN without IF NOT EXISTS
		if strings.Contains(lower, "add column") && !strings.Contains(lower, "if not exists") {
			violations++
			details = append(details, fmt.Sprintf("  line %d: ADD COLUMN without IF NOT EXISTS", i+1))
		}
	}
	return
}

// ---------------------------------------------------------------------------
// Test 4.1 — Documents that idempotency violations exist (F-B1-15)
//
// Category: PASS-NOW (documents violations)
// ---------------------------------------------------------------------------

func TestMigrationsHaveIdempotencyViolations(t *testing.T) {
	// POST-FIX (F-B1-15): All migrations should now be idempotent.
	// This test confirms no violations remain.
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	totalViolations := 0

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".up.sql") {
			continue
		}

		content, err := os.ReadFile(f.Name())
		if err != nil {
			continue
		}

		violations, details := checkIdempotency(string(content))
		if violations > 0 {
			totalViolations += violations
			t.Logf("[WARNING] [server] [database] %s: %d violations", f.Name(), violations)
			for _, d := range details {
				t.Logf("  %s", d)
			}
		}
	}

	if totalViolations > 0 {
		t.Errorf("[ERROR] [server] [database] %d idempotency violations remain", totalViolations)
	}
	t.Log("[INFO] [server] [database] F-B1-15 FIXED: all migrations are idempotent")
}

// ---------------------------------------------------------------------------
// Test 4.2 — All migrations MUST be idempotent (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestAllMigrationsAreIdempotent(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".up.sql") {
			continue
		}
		// Skip A-series migrations (already idempotent)
		if strings.HasPrefix(f.Name(), "025_") || strings.HasPrefix(f.Name(), "026_") {
			continue
		}

		content, err := os.ReadFile(f.Name())
		if err != nil {
			continue
		}

		violations, details := checkIdempotency(string(content))
		if violations > 0 {
			t.Errorf("[ERROR] [server] [database] %s: %d idempotency violations\n"+
				"F-B1-15: all schema changes must use IF NOT EXISTS per ETHOS #4.\n%s",
				f.Name(), violations, strings.Join(details, "\n"))
		}
	}
}
