package handlers_test

// stats_n1_test.go — Tests for N+1 query fix in GetDashboardStats.
//
// F-B1-6 FIXED: GetDashboardStats now uses GetAllUpdateStats() (single
//   aggregate query) instead of GetUpdateStatsFromState() per agent.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetDashboardStatsHasNPlusOneLoop(t *testing.T) {
	// POST-FIX: GetUpdateStatsFromState should NOT be called inside a loop
	statsPath := filepath.Join(".", "stats.go")
	content, err := os.ReadFile(statsPath)
	if err != nil {
		t.Fatalf("failed to read stats.go: %v", err)
	}

	src := string(content)

	// The old pattern: GetUpdateStatsFromState inside a range loop
	forIdx := strings.Index(src, "for _, agent := range")
	if forIdx == -1 {
		// No agent loop at all — that's fine if GetAllUpdateStats is used instead
		if strings.Contains(src, "GetAllUpdateStats") {
			t.Log("[INFO] [server] [handlers] F-B1-6 FIXED: using aggregate query instead of per-agent loop")
			return
		}
		t.Error("[ERROR] [server] [handlers] no agent loop AND no GetAllUpdateStats found")
		return
	}

	// If there IS a loop, check that GetUpdateStatsFromState is NOT inside it
	loopBody := src[forIdx:]
	if len(loopBody) > 1000 {
		loopBody = loopBody[:1000]
	}
	if strings.Contains(loopBody, "GetUpdateStatsFromState") {
		t.Error("[ERROR] [server] [handlers] F-B1-6 NOT FIXED: GetUpdateStatsFromState still inside agent loop")
	} else {
		t.Log("[INFO] [server] [handlers] F-B1-6 FIXED: no per-agent query in loop")
	}
}

func TestGetDashboardStatsUsesJoin(t *testing.T) {
	statsPath := filepath.Join(".", "stats.go")
	content, err := os.ReadFile(statsPath)
	if err != nil {
		t.Fatalf("failed to read stats.go: %v", err)
	}

	src := string(content)

	// Must use GetAllUpdateStats (single aggregate) not GetUpdateStatsFromState (per-agent)
	if !strings.Contains(src, "GetAllUpdateStats") {
		t.Errorf("[ERROR] [server] [handlers] GetAllUpdateStats not found in stats.go.\n" +
			"F-B1-6: dashboard stats must use a single aggregate query.")
	}

	// Must NOT have GetUpdateStatsFromState in a loop
	forIdx := strings.Index(src, "for _, agent := range")
	if forIdx != -1 {
		loopBody := src[forIdx:]
		if len(loopBody) > 1000 {
			loopBody = loopBody[:1000]
		}
		if strings.Contains(loopBody, "GetUpdateStatsFromState") {
			t.Errorf("[ERROR] [server] [handlers] per-agent query still in loop")
		}
	}

	t.Log("[INFO] [server] [handlers] F-B1-6 FIXED: uses aggregate query")
}
