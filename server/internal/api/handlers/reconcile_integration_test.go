//go:build integration

package handlers

// reconcile_integration_test.go — DB-backed tests for RECONCILE-001 scan-set closure.
//
// These require a live PostgreSQL. Set TEST_DATABASE_URL to a *disposable* database
// (the suite runs all migrations and writes rows); the tests skip when it is unset:
//
//	TEST_DATABASE_URL='postgres://user:pass@localhost:5432/redflag_test?sslmode=disable' \
//	  go test -tags=integration ./internal/api/handlers/ -run Integration -v
//
// Coverage (the positive paths the no-DB unit tests cannot reach):
//   - closeScanAbsentRows: a waiting (pending) row absent from a successful scan closes
//     to installed with provenance out_of_band.
//   - Regression: a *stale* consumed capability token linked to the same row ID (from a
//     prior lifecycle, preserved across the UPSERT key) does NOT flip provenance to
//     redflag_receipt — closures are unconditionally out_of_band.
//   - A row still present in the reported set is left untouched.
//   - ReopenFailedUpdate is scoped to failed-only: it reopens failed rows and rejects
//     installed / pending / ignored rows (the installed→pending edge must not leak in).

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/Fimeg/RedFlag/server/internal/database"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
)

var (
	testDBOnce sync.Once
	testDB     *database.DB
	testDBErr  error
)

// getTestDB connects once per package run and applies all migrations. It skips the
// calling test when TEST_DATABASE_URL is unset so the suite is a no-op in CI lanes
// without a database.
func getTestDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB-backed integration test")
	}
	testDBOnce.Do(func() {
		testDB, testDBErr = database.Connect(dsn)
		if testDBErr != nil {
			return
		}
		// migrations live at server/internal/database/migrations; tests run from
		// server/internal/api/handlers.
		testDBErr = testDB.Migrate("../../database/migrations")
	})
	if testDBErr != nil {
		t.Fatalf("test DB setup: %v", testDBErr)
	}
	return testDB
}

// seedAgent inserts a throwaway agent and registers cleanup (ON DELETE CASCADE
// removes its package rows, history, and tokens).
func seedAgent(t *testing.T, db *database.DB) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV4())
	if _, err := db.Exec(
		`INSERT INTO agents (id, hostname, os_type, agent_version) VALUES ($1, $2, 'linux', 'test')`,
		id, "reconcile-it-"+id.String()); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM agents WHERE id = $1`, id) })
	return id
}

// seedPackage inserts one current_package_state row in the given status and returns its id.
func seedPackage(t *testing.T, db *database.DB, agentID uuid.UUID, name string, status models.PackageStatus, metadata models.JSONB) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV4())
	meta := models.JSONB{}
	if metadata != nil {
		meta = metadata
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	// repository_source is non-null: the row is loaded via SELECT * into
	// models.UpdateState, whose RepositorySource is a plain string (NULL fails to scan).
	if _, err := db.Exec(`
		INSERT INTO current_package_state
			(id, agent_id, package_type, package_name, current_version, available_version, severity, repository_source, status, metadata)
		VALUES ($1, $2, 'dnf', $3, '1.0.0', '2.0.0', 'low', 'test-repo', $4, $5::jsonb)`,
		id, agentID, name, status, string(metaJSON)); err != nil {
		t.Fatalf("seed package %s (%s): %v", name, status, err)
	}
	return id
}

// seedConsumedToken links a CONSUMED capability token to updateID, simulating a token
// minted-and-consumed in a *prior* lifecycle that survived the UPSERT on the same row id.
func seedConsumedToken(t *testing.T, db *database.DB, agentID, updateID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO capability_tokens
			(token_id, update_id, agent_id, key_id, package_type, operation, closure,
			 issued_at, not_before, expires_at, signature, consumed_at)
		VALUES ($1, $2, $3, 'testkey', 'dnf', 'install', '{}'::jsonb,
			 $4, $4, $5, 'sig', $6)`,
		uuid.Must(uuid.NewV4()), updateID, agentID,
		now.Add(-2*time.Hour).Unix(), now.Add(2*time.Hour).Unix(), now.Add(-time.Hour)); err != nil {
		t.Fatalf("seed consumed token: %v", err)
	}
}

// readPackage returns the current status and decoded live metadata for a row.
func readPackage(t *testing.T, db *database.DB, id uuid.UUID) (string, map[string]interface{}) {
	t.Helper()
	var status string
	var metaRaw []byte
	if err := db.QueryRow(`SELECT status, metadata FROM current_package_state WHERE id = $1`, id).
		Scan(&status, &metaRaw); err != nil {
		t.Fatalf("read package %s: %v", id, err)
	}
	meta := map[string]interface{}{}
	if len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			t.Fatalf("decode metadata: %v", err)
		}
	}
	return status, meta
}

// latestHistoryMeta returns the decoded metadata of the most recent terminal-history
// row for a package, and whether any history row exists. Closure provenance is recorded
// here (update_version_history), not on the live current_package_state row — a terminal
// transition into installed merges meta into history, not back onto the live row.
func latestHistoryMeta(t *testing.T, db *database.DB, agentID uuid.UUID, packageName string) (map[string]interface{}, bool) {
	t.Helper()
	var metaRaw []byte
	err := db.QueryRow(`
		SELECT metadata FROM update_version_history
		WHERE agent_id = $1 AND package_name = $2
		ORDER BY update_completed_at DESC LIMIT 1`, agentID, packageName).Scan(&metaRaw)
	if err != nil {
		return nil, false // sql.ErrNoRows → no history row written
	}
	meta := map[string]interface{}{}
	if len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			t.Fatalf("decode history metadata: %v", err)
		}
	}
	return meta, true
}

func newReconcileHandler(db *database.DB) *UpdateHandler {
	h := NewUpdateHandler(
		queries.NewUpdateQueries(db.DB),
		queries.NewAgentQueries(db.DB),
		nil, nil, nil, nil,
	)
	// Wire the token store directly (SetCapabilityMinter also requires a minter, which
	// closeScanAbsentRows does not use). The whole point is to prove the closure ignores it.
	h.tokenQueries = queries.NewCapabilityTokenQueries(db.DB)
	return h
}

// TestCloseScanAbsentRows_StaleTokenStillOutOfBand_Integration is the regression for the
// provenance false-positive: a pending row that carries a stale consumed token (prior
// lifecycle) must still close as out_of_band, never redflag_receipt.
func TestCloseScanAbsentRows_StaleTokenStillOutOfBand_Integration(t *testing.T) {
	db := getTestDB(t)
	agentID := seedAgent(t, db)
	h := newReconcileHandler(db)

	rowID := seedPackage(t, db, agentID, "vim", models.StatusPending, nil)
	seedConsumedToken(t, db, agentID, rowID) // stale token from a previous cycle

	// Empty reported set → the pending row is absent → must close.
	h.closeScanAbsentRows(agentID, "dnf", map[string]struct{}{})

	if status, _ := readPackage(t, db, rowID); status != string(models.StatusInstalled) {
		t.Fatalf("row should have closed to installed, got %q", status)
	}
	meta, ok := latestHistoryMeta(t, db, agentID, "vim")
	if !ok {
		t.Fatalf("expected a terminal-history row for the closure")
	}
	if got := meta["resolution_provenance"]; got != "out_of_band" {
		t.Errorf("provenance = %v, want out_of_band (stale consumed token must NOT flip it to redflag_receipt)", got)
	}
	if got := meta["closed_by"]; got != "scan_set_reconciler" {
		t.Errorf("closed_by = %v, want scan_set_reconciler", got)
	}
}

// TestCloseScanAbsentRows_PresentRowUntouched_Integration verifies the diff: a row still
// in the reported set is kept; only the absent one closes.
func TestCloseScanAbsentRows_PresentRowUntouched_Integration(t *testing.T) {
	db := getTestDB(t)
	agentID := seedAgent(t, db)
	h := newReconcileHandler(db)

	keepID := seedPackage(t, db, agentID, "nano", models.StatusPending, nil)
	closeID := seedPackage(t, db, agentID, "curl", models.StatusApproved, nil)

	// nano is still reported; curl is absent.
	h.closeScanAbsentRows(agentID, "dnf", map[string]struct{}{"nano": {}})

	if status, _ := readPackage(t, db, keepID); status != string(models.StatusPending) {
		t.Errorf("present row should stay pending, got %q", status)
	}
	if _, ok := latestHistoryMeta(t, db, agentID, "nano"); ok {
		t.Errorf("present row must not produce a closure history row")
	}
	if status, _ := readPackage(t, db, closeID); status != string(models.StatusInstalled) {
		t.Errorf("absent row should close to installed, got %q", status)
	}
	if meta, ok := latestHistoryMeta(t, db, agentID, "curl"); !ok {
		t.Errorf("absent row should produce a closure history row")
	} else if meta["resolution_provenance"] != "out_of_band" {
		t.Errorf("absent row provenance = %v, want out_of_band", meta["resolution_provenance"])
	}
}

// TestCloseScanAbsentRows_Idempotent_Integration verifies running closure 3× is safe
// (ETHOS §4): the row is closed once and re-runs are no-ops, not errors.
func TestCloseScanAbsentRows_Idempotent_Integration(t *testing.T) {
	db := getTestDB(t)
	agentID := seedAgent(t, db)
	h := newReconcileHandler(db)

	rowID := seedPackage(t, db, agentID, "wget", models.StatusPending, nil)

	for i := 0; i < 3; i++ {
		h.closeScanAbsentRows(agentID, "dnf", map[string]struct{}{})
	}

	if status, _ := readPackage(t, db, rowID); status != string(models.StatusInstalled) {
		t.Errorf("after 3x close, status = %q, want installed", status)
	}
}

// TestReopenFailedUpdate_ScopedToFailed_Integration verifies the requireFrom guard: a
// failed row reopens (and its failure markers clear), while installed / pending / ignored
// rows are rejected — the installed→pending edge (RECONCILE-001) must not leak into reopen.
func TestReopenFailedUpdate_ScopedToFailed_Integration(t *testing.T) {
	db := getTestDB(t)
	agentID := seedAgent(t, db)
	uq := queries.NewUpdateQueries(db.DB)

	t.Run("failed reopens and clears markers", func(t *testing.T) {
		id := seedPackage(t, db, agentID, "pkg-failed", models.StatusFailed,
			models.JSONB{"failure_reason": "boom", "failed_by": "operator", "dry_run_attempts": 2})
		if err := uq.ReopenFailedUpdate(id); err != nil {
			t.Fatalf("reopen of failed row should succeed: %v", err)
		}
		status, meta := readPackage(t, db, id)
		if status != string(models.StatusPending) {
			t.Errorf("status = %q, want pending", status)
		}
		for _, k := range []string{"failure_reason", "failed_by", "dry_run_attempts"} {
			if _, ok := meta[k]; ok {
				t.Errorf("metadata key %q should have been cleared, still present", k)
			}
		}
	})

	for _, tc := range []struct {
		name   string
		status models.PackageStatus
	}{
		{"installed rejected", models.StatusInstalled},
		{"pending rejected", models.StatusPending},
		{"ignored rejected", models.StatusIgnored},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := seedPackage(t, db, agentID, "pkg-"+string(tc.status), tc.status, nil)
			if err := uq.ReopenFailedUpdate(id); err == nil {
				t.Errorf("reopen of %s row must be rejected, got nil error", tc.status)
			}
			if status, _ := readPackage(t, db, id); status != string(tc.status) {
				t.Errorf("status changed to %q, want unchanged %q", status, tc.status)
			}
		})
	}
}
