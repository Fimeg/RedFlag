//go:build !windows

package localapi

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/cache"
)

func TestFetchSnapshotOverUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "redflag-agent.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skipf("unix sockets are not permitted in this sandbox: %v", err)
		}
		t.Fatalf("listen unix socket: %v", err)
	}

	cfg := testConfig(t)
	server, err := Start(Options{
		Config: cfg,
		LoadCache: func() (*cache.LocalCache, error) {
			return &cache.LocalCache{
				AgentStatus: "online",
				UpdateCount: 7,
				Summary: cache.UpdateSummary{
					Total: 7,
				},
			}, nil
		},
		RequestLog:       t.Logf,
		ListenerOverride: ln,
	})
	if err != nil {
		t.Fatalf("start local API: %v", err)
	}
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	snapshot, err := FetchSnapshot(ctx, ClientOptions{UnixSocketPath: socketPath})
	if err != nil {
		t.Fatalf("fetch snapshot: %v", err)
	}

	if snapshot.Identity.AgentID != cfg.AgentID.String() {
		t.Fatalf("agent_id = %q, want %q", snapshot.Identity.AgentID, cfg.AgentID.String())
	}
	if snapshot.Status.AgentStatus != "online" {
		t.Fatalf("agent_status = %q, want online", snapshot.Status.AgentStatus)
	}
	if snapshot.Status.UpdateCount != 7 {
		t.Fatalf("update_count = %d, want 7", snapshot.Status.UpdateCount)
	}
}
