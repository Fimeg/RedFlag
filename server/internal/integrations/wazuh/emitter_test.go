package wazuh

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

func listen(t *testing.T) (string, *net.UnixConn) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "queue")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: sock, Net: "unixgram"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return sock, conn
}

func TestEmitFrameAndECSShape(t *testing.T) {
	sock, conn := listen(t)
	e := New(sock)

	agentID := uuid.Must(uuid.NewV4())
	ev := models.NewSecurityEvent("CRITICAL", models.SecurityEventTypes.MachineIDMismatch, agentID, "machine binding violation")
	e.Emit(ev)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64*1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	frame := string(buf[:n])

	if !strings.HasPrefix(frame, "1:redflag:") {
		t.Fatalf("frame prefix wrong: %q", frame[:20])
	}

	var got ecsEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "1:redflag:")), &got); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if got.Event.Module != "redflag" || got.Event.Action != "MACHINE_ID_MISMATCH" {
		t.Errorf("event block wrong: %+v", got.Event)
	}
	if got.Rule.ID != "999005" || got.Rule.Level != 12 {
		t.Errorf("rule mapping wrong: %+v", got.Rule)
	}
	if got.Agent == nil || got.Agent.ID != agentID.String() {
		t.Errorf("agent id missing: %+v", got.Agent)
	}
	if got.Wazuh.Integration.Name != "redflag" {
		t.Errorf("integration block wrong: %+v", got.Wazuh)
	}
	if e.Dropped() != 0 {
		t.Errorf("dropped %d events on healthy socket", e.Dropped())
	}
}

func TestEmitAbsentSocketDropsWithoutBlocking(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "no-such-socket"))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			e.Emit(models.NewSecurityEvent("WARNING", models.SecurityEventTypes.UpdateNonceInvalid, uuid.Nil, "replay"))
		}
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Emit blocked on absent socket")
	}
	if e.Dropped() != 5 {
		t.Errorf("expected 5 drops, got %d", e.Dropped())
	}
}

func TestUnknownEventTypeGetsGenericRule(t *testing.T) {
	sock, conn := listen(t)
	e := New(sock)
	e.Emit(models.NewSecurityEvent("INFO", "SOME_FUTURE_EVENT", uuid.Nil, "x"))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64*1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got ecsEvent
	if err := json.Unmarshal(buf[len("1:redflag:"):n], &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Rule.ID != "999001" {
		t.Errorf("expected generic rule 999001, got %s", got.Rule.ID)
	}
	if got.Agent != nil {
		t.Errorf("nil agent UUID should omit agent block, got %+v", got.Agent)
	}
}
