package system

import (
	"runtime"
	"testing"
)

func TestGetTopProcesses(t *testing.T) {
	procs, err := GetTopProcesses(5)
	if err != nil {
		t.Fatalf("GetTopProcesses(5) error: %v", err)
	}
	if len(procs) == 0 {
		t.Fatal("expected at least one process")
	}

	t.Logf("Top %d processes (on %s):", len(procs), runtime.GOOS)
	for i, p := range procs {
		t.Logf("  %d. %s (pid=%d) cpu=%.1f%% mem=%.1f%%", i+1, p.Name, p.PID, p.CPU, p.Mem)
		if p.Name == "" {
			t.Errorf("process %d has empty name", p.PID)
		}
		if p.PID == 0 {
			t.Error("process has zero PID")
		}
	}
}

func TestGetTopProcessesLimit(t *testing.T) {
	for _, limit := range []int{1, 3, 10} {
		procs, err := GetTopProcesses(limit)
		if err != nil {
			t.Fatalf("GetTopProcesses(%d) error: %v", limit, err)
		}
		if len(procs) > limit {
			t.Errorf("GetTopProcesses(%d) returned %d processes", limit, len(procs))
		}
	}
}
