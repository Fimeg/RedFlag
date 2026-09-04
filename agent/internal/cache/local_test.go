package cache

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
)

func TestSaveLoadFromPathRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last_scan.json")
	localCache := &LocalCache{
		AgentStatus: "online",
		Updates: []client.UpdateReportItem{
			{PackageType: "apt", PackageName: "openssl", Severity: "important"},
		},
	}
	localCache.UpdateScanResults(localCache.Updates)

	if err := localCache.SaveToPath(path); err != nil {
		t.Fatalf("SaveToPath() error = %v", err)
	}

	loaded, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath() error = %v", err)
	}
	if loaded.AgentStatus != "online" {
		t.Fatalf("AgentStatus = %q, want online", loaded.AgentStatus)
	}
	if loaded.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", loaded.Summary.Total)
	}
	if loaded.Summary.ByEcosystem["apt"] != 1 {
		t.Fatalf("Summary.ByEcosystem[apt] = %d, want 1", loaded.Summary.ByEcosystem["apt"])
	}
	if loaded.Summary.BySeverity["important"] != 1 {
		t.Fatalf("Summary.BySeverity[important] = %d, want 1", loaded.Summary.BySeverity["important"])
	}
}

func TestRecordScannerResultReplacesOnlyThatScanner(t *testing.T) {
	localCache := &LocalCache{}
	localCache.UpdateScanResults([]client.UpdateReportItem{
		{PackageType: "apt", PackageName: "openssl", Severity: "important"},
		{PackageType: "dnf", PackageName: "kernel", Severity: "critical"},
	})

	localCache.RecordScannerResult("apt", "success", []client.UpdateReportItem{
		{PackageType: "apt", PackageName: "curl", Severity: "moderate"},
	}, nil, 250*time.Millisecond, true)

	if len(localCache.Updates) != 2 {
		t.Fatalf("len(Updates) = %d, want 2", len(localCache.Updates))
	}
	if got := packageNames(localCache.Updates); got["openssl"] {
		t.Fatalf("stale apt package remained in updates: %#v", localCache.Updates)
	}
	if got := packageNames(localCache.Updates); !got["curl"] || !got["kernel"] {
		t.Fatalf("updates = %#v, want curl and kernel", localCache.Updates)
	}
	if localCache.Summary.ByEcosystem["apt"] != 1 || localCache.Summary.ByEcosystem["dnf"] != 1 {
		t.Fatalf("summary by ecosystem = %#v, want apt=1 dnf=1", localCache.Summary.ByEcosystem)
	}
	if localCache.Scanners["apt"].Status != "success" {
		t.Fatalf("scanner status = %q, want success", localCache.Scanners["apt"].Status)
	}
}

func TestRecordScannerResultSuccessfulEmptyScanClearsScannerUpdates(t *testing.T) {
	localCache := &LocalCache{}
	localCache.UpdateScanResults([]client.UpdateReportItem{
		{PackageType: "windows_update", PackageName: "KB123", Severity: "important"},
		{PackageType: "winget", PackageName: "Git.Git", Severity: "moderate"},
	})

	localCache.RecordScannerResult("windows", "success", nil, nil, 100*time.Millisecond, true)

	if len(localCache.Updates) != 1 {
		t.Fatalf("len(Updates) = %d, want 1", len(localCache.Updates))
	}
	if localCache.Updates[0].PackageType != "winget" {
		t.Fatalf("remaining PackageType = %q, want winget", localCache.Updates[0].PackageType)
	}
	if localCache.Summary.ByEcosystem["windows_update"] != 0 {
		t.Fatalf("windows_update summary = %d, want 0", localCache.Summary.ByEcosystem["windows_update"])
	}
}

func TestRecordScannerResultFailureDoesNotClearStaleUpdates(t *testing.T) {
	localCache := &LocalCache{}
	localCache.UpdateScanResults([]client.UpdateReportItem{
		{PackageType: "apt", PackageName: "openssl", Severity: "important"},
	})

	localCache.RecordScannerResult("apt", "failed", nil, errors.New("scanner failed"), time.Second, true)

	if len(localCache.Updates) != 1 {
		t.Fatalf("len(Updates) = %d, want stale update retained", len(localCache.Updates))
	}
	if localCache.Scanners["apt"].LastError != "scanner failed" {
		t.Fatalf("LastError = %q, want scanner failed", localCache.Scanners["apt"].LastError)
	}
}

func TestRecordCapabilityTokenCounts(t *testing.T) {
	localCache := &LocalCache{}

	localCache.RecordCapabilityTokenFetch(3, nil)
	localCache.RecordCapabilityTokenProcess(2, 1)

	if localCache.Capabilities.LastFetchedCount != 3 {
		t.Fatalf("LastFetchedCount = %d, want 3", localCache.Capabilities.LastFetchedCount)
	}
	if localCache.Capabilities.LastProcessedCount != 2 {
		t.Fatalf("LastProcessedCount = %d, want 2", localCache.Capabilities.LastProcessedCount)
	}
	if localCache.Capabilities.LastFailedCount != 1 {
		t.Fatalf("LastFailedCount = %d, want 1", localCache.Capabilities.LastFailedCount)
	}
	if localCache.Capabilities.PendingCount != 0 {
		t.Fatalf("PendingCount = %d, want 0", localCache.Capabilities.PendingCount)
	}

	localCache.RecordCapabilityTokenFetch(0, errors.New("server unavailable"))
	if localCache.Capabilities.LastError != "server unavailable" {
		t.Fatalf("LastError = %q, want server unavailable", localCache.Capabilities.LastError)
	}
}

func packageNames(updates []client.UpdateReportItem) map[string]bool {
	names := make(map[string]bool, len(updates))
	for _, update := range updates {
		names[update.PackageName] = true
	}
	return names
}
