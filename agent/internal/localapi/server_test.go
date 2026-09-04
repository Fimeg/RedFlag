package localapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/cache"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/system"
	"github.com/gofrs/uuid/v5"
)

func TestIdentityRedactsSecrets(t *testing.T) {
	cfg := testConfig(t)
	handler := newHandler(Options{Config: cfg, LoadCache: func() (*cache.LocalCache, error) {
		return &cache.LocalCache{}, nil
	}})

	req := httptest.NewRequest(http.MethodGet, "/v1/identity", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, forbidden := range []string{"secret-access-token", "secret-refresh-token", "registration-token", "refresh_token", "token"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("identity response leaked %q: %s", forbidden, body)
		}
	}

	var resp IdentityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AgentID != cfg.AgentID.String() {
		t.Fatalf("agent_id = %q, want %q", resp.AgentID, cfg.AgentID.String())
	}
	if resp.Registered != true {
		t.Fatalf("registered = false, want true")
	}
	if got := strings.Join(resp.Tags, ","); got != "alpha,zeta" {
		t.Fatalf("tags = %q, want sorted alpha,zeta", got)
	}
}

func TestStatusAndPackagesReadLocalCache(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	localCache := &cache.LocalCache{
		LastScanTime: now.Add(-2 * time.Minute),
		LastCheckIn:  now.Add(-1 * time.Minute),
		LastUpdated:  now,
		UpdateCount:  1,
		AgentStatus:  "online",
		Summary: cache.UpdateSummary{
			Total:       1,
			ByEcosystem: map[string]int{"windows": 1},
			BySeverity:  map[string]int{"critical": 1},
		},
		Scanners: map[string]cache.ScannerState{
			"windows": {
				Name:        "windows",
				Status:      "success",
				UpdateCount: 1,
			},
		},
		Updates: []client.UpdateReportItem{
			{
				PackageType:      "windows",
				PackageName:      "KB5000001",
				CurrentVersion:   "1",
				AvailableVersion: "2",
				Severity:         "critical",
			},
		},
		Capabilities: cache.CapabilityTokenState{
			PendingCount:       2,
			LastFetchedCount:   3,
			LastProcessedCount: 1,
		},
	}
	handler := newHandler(Options{Config: testConfig(t), LoadCache: func() (*cache.LocalCache, error) {
		return localCache, nil
	}})

	var status StatusResponse
	requestJSON(t, handler, http.MethodGet, "/v1/status", &status)
	if status.AgentStatus != "online" {
		t.Fatalf("agent_status = %q, want online", status.AgentStatus)
	}
	if status.Summary.Total != 1 {
		t.Fatalf("summary.total = %d, want 1", status.Summary.Total)
	}

	var packages ScanResponse
	requestJSON(t, handler, http.MethodGet, "/v1/packages", &packages)
	if len(packages.Updates) != 1 {
		t.Fatalf("updates len = %d, want 1", len(packages.Updates))
	}
	if packages.Updates[0].PackageName != "KB5000001" {
		t.Fatalf("package name = %q, want KB5000001", packages.Updates[0].PackageName)
	}

	var tokens TokenResponse
	requestJSON(t, handler, http.MethodGet, "/v1/tokens/active", &tokens)
	if tokens.Capabilities.PendingCount != 2 {
		t.Fatalf("pending tokens = %d, want 2", tokens.Capabilities.PendingCount)
	}
}

func TestSystemHealthUsesAgentCollectors(t *testing.T) {
	handler := newHandler(Options{
		Config: testConfig(t),
		LoadCache: func() (*cache.LocalCache, error) {
			return &cache.LocalCache{}, nil
		},
		SystemProvider: func() (*system.SystemInfo, error) {
			return &system.SystemInfo{
				Hostname:         "workstation-01",
				OSVersion:        "Arch Linux",
				OSArchitecture:   "amd64",
				RunningProcesses: 173,
				Uptime:           "4 days",
				MemoryInfo: system.MemoryInfo{
					Total:       32 << 30,
					Used:        12 << 30,
					Available:   20 << 30,
					UsedPercent: 37.5,
				},
			}, nil
		},
		ProcessProvider: func(limit int) ([]system.TopProcess, error) {
			if limit != 8 {
				t.Fatalf("process limit = %d, want 8", limit)
			}
			return []system.TopProcess{{Name: "redflag-agent", PID: 42, CPU: 1.5, Mem: 0.8}}, nil
		},
	})

	var response SystemResponse
	requestJSON(t, handler, http.MethodGet, "/v1/system", &response)
	if response.System.Hostname != "workstation-01" {
		t.Fatalf("hostname = %q, want workstation-01", response.System.Hostname)
	}
	if response.System.MemoryInfo.UsedPercent != 37.5 {
		t.Fatalf("memory used = %.1f, want 37.5", response.System.MemoryInfo.UsedPercent)
	}
	if len(response.TopProcesses) != 1 || response.TopProcesses[0].Name != "redflag-agent" {
		t.Fatalf("top processes = %#v", response.TopProcesses)
	}
}

func TestMonitorAndProcessInventoryUseAgentCollectors(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	handler := newHandler(Options{
		Config: testConfig(t),
		LoadCache: func() (*cache.LocalCache, error) {
			return &cache.LocalCache{}, nil
		},
		MonitorProvider: func() (*system.ResourceSnapshot, error) {
			return &system.ResourceSnapshot{
				CollectedAt: now,
				CPU:         system.CPUMetrics{UsagePercent: 27.5, Load1: 1.2},
				Memory:      system.MemoryMetrics{UsedPercent: 42},
				History:     []system.ResourcePoint{{Timestamp: now, CPUPercent: 27.5}},
			}, nil
		},
		ProcessesProvider: func() (*system.FullProcessSnapshot, error) {
			return &system.FullProcessSnapshot{
				Processes:    []system.FullProcess{{PID: 42, Name: "redflag-agent", User: "root"}},
				ProcessCount: 1,
				ScannedAt:    now,
			}, nil
		},
		ProcessDetailProvider: func(pid int, caps system.ProcessCaps) (*system.FullProcess, error) {
			if pid != 42 {
				t.Fatalf("pid = %d, want 42", pid)
			}
			if caps.MaxSockets != 500 || caps.MaxOpenFiles != 2000 {
				t.Fatalf("default process caps = %#v", caps)
			}
			return &system.FullProcess{PID: pid, Name: "redflag-agent", Path: "/usr/bin/redflag-agent"}, nil
		},
		SoftwareProvider: func() (*system.SoftwareSnapshot, error) {
			return &system.SoftwareSnapshot{
				Supported: true,
				Packages:  []system.InstalledPackage{{PackageType: "pacman", Identity: "redflag-agent", Name: "redflag-agent", Version: "0.3.0", InstallReason: "explicit", Origin: "repository"}},
				Count:     1,
			}, nil
		},
		PackageDetailProvider: func(packageType, identity string) (*system.PackageDetail, error) {
			if packageType != "pacman" || identity != "redflag-agent" {
				t.Fatalf("package detail request = %s/%s", packageType, identity)
			}
			return &system.PackageDetail{InstalledPackage: system.InstalledPackage{PackageType: packageType, Identity: identity, Name: identity, Version: "0.3.0"}, DependsOn: []string{"glibc"}}, nil
		},
		ConnectionsProvider: func() (*system.ConnectionSnapshot, error) {
			return &system.ConnectionSnapshot{Connections: []system.Connection{{PID: 42, Process: "redflag-agent", Protocol: "TCP", LocalPort: 443, State: "LISTEN"}}, Count: 1, CollectedAt: now}, nil
		},
		ServicesProvider: func() (*system.ServiceSnapshot, error) {
			return &system.ServiceSnapshot{Manager: "systemd", Services: []system.Service{{Name: "redflag-agent", ActiveState: "active", SubState: "running"}}, Count: 1, Running: 1, CollectedAt: now}, nil
		},
	})

	var monitor system.ResourceSnapshot
	requestJSON(t, handler, http.MethodGet, "/v1/monitor", &monitor)
	if monitor.CPU.UsagePercent != 27.5 || len(monitor.History) != 1 {
		t.Fatalf("monitor = %#v", monitor)
	}

	var processes system.FullProcessSnapshot
	requestJSON(t, handler, http.MethodGet, "/v1/processes", &processes)
	if processes.ProcessCount != 1 || processes.Processes[0].Name != "redflag-agent" {
		t.Fatalf("processes = %#v", processes)
	}

	var process system.FullProcess
	requestJSON(t, handler, http.MethodGet, "/v1/processes/42", &process)
	if process.Path != "/usr/bin/redflag-agent" {
		t.Fatalf("process detail = %#v", process)
	}

	var software system.SoftwareSnapshot
	requestJSON(t, handler, http.MethodGet, "/v1/software", &software)
	if software.Count != 1 || software.Packages[0].Version != "0.3.0" {
		t.Fatalf("software = %#v", software)
	}

	var packageDetail system.PackageDetail
	requestJSON(t, handler, http.MethodGet, "/v1/software/detail?manager=pacman&identity=redflag-agent", &packageDetail)
	if packageDetail.Name != "redflag-agent" || len(packageDetail.DependsOn) != 1 {
		t.Fatalf("package detail = %#v", packageDetail)
	}

	var connections system.ConnectionSnapshot
	requestJSON(t, handler, http.MethodGet, "/v1/connections", &connections)
	if connections.Count != 1 || connections.Connections[0].State != "LISTEN" {
		t.Fatalf("connections = %#v", connections)
	}

	var services system.ServiceSnapshot
	requestJSON(t, handler, http.MethodGet, "/v1/services", &services)
	if services.Running != 1 || services.Services[0].Name != "redflag-agent" {
		t.Fatalf("services = %#v", services)
	}
}

func TestProcessDetailRejectsInvalidPID(t *testing.T) {
	handler := newHandler(Options{Config: testConfig(t), LoadCache: func() (*cache.LocalCache, error) {
		return &cache.LocalCache{}, nil
	}})
	req := httptest.NewRequest(http.MethodGet, "/v1/processes/not-a-pid", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCacheLoadFailureReturnsServiceUnavailable(t *testing.T) {
	handler := newHandler(Options{Config: testConfig(t), LoadCache: func() (*cache.LocalCache, error) {
		return nil, errors.New("cannot read cache")
	}})

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestOnlyGetMethodsAllowed(t *testing.T) {
	handler := newHandler(Options{Config: testConfig(t), LoadCache: func() (*cache.LocalCache, error) {
		return &cache.LocalCache{}, nil
	}})

	req := httptest.NewRequest(http.MethodPost, "/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", rec.Header().Get("Allow"))
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, target interface{}) {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()

	agentID, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("new uuid: %v", err)
	}
	return &config.Config{
		Version:           "5",
		ServerURL:         "https://redflag.example",
		RegistrationToken: "registration-token",
		AgentID:           agentID,
		Token:             "secret-access-token",
		RefreshToken:      "secret-refresh-token",
		CheckInInterval:   300,
		Tags:              []string{"zeta", "alpha"},
		DisplayName:       "workstation-01",
		Organization:      "lab",
		OSType:            "windows",
	}
}
