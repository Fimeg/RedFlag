package event

import (
	"testing"

	"github.com/Fimeg/RedFlag/agent/internal/models"
	"github.com/gofrs/uuid/v5"
)

func TestTeeLoggerNilBuffer(t *testing.T) {
	// TeeLogger with nil buffer should not panic
	logger := NewTeeLogger(nil, uuid.Nil)

	// Should not panic — log-only mode
	logger.Info("agent", "test", "test_component", "test message", nil)
	logger.Warning("agent", "test", "test_component", "test warning", nil)
	logger.Error("agent", "test", "test_component", "test error", nil)
	logger.Critical("agent", "test", "test_component", "test critical", nil)
}

func TestTeeLoggerNilAgentID(t *testing.T) {
	// TeeLogger with Nil agent ID should produce events with nil AgentID
	buffer := NewBuffer(t.TempDir() + "/events.json")
	logger := NewTeeLogger(buffer, uuid.Nil)

	logger.Info("agent", "test", "test_component", "test message", nil)

	events, err := buffer.GetBufferedEvents()
	if err != nil {
		t.Fatalf("GetBufferedEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].AgentID != nil {
		t.Errorf("expected nil AgentID, got %v", events[0].AgentID)
	}
}

func TestTeeLoggerEventFields(t *testing.T) {
	buffer := NewBuffer(t.TempDir() + "/events.json")
	agentID := uuid.Must(uuid.NewV4())
	logger := NewTeeLogger(buffer, agentID)

	metadata := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}
	logger.Warning("agent", "crypto", "pubkey", "key fetch failed", metadata)

	events, err := buffer.GetBufferedEvents()
	if err != nil {
		t.Fatalf("GetBufferedEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.AgentID == nil || *evt.AgentID != agentID {
		t.Errorf("expected AgentID %v, got %v", agentID, evt.AgentID)
	}
	if evt.EventType != models.EventTypeAgentCrypto {
		t.Errorf("expected EventType %q, got %q", models.EventTypeAgentCrypto, evt.EventType)
	}
	if evt.EventSubtype != models.SubtypeWarning {
		t.Errorf("expected EventSubtype %q, got %q", models.SubtypeWarning, evt.EventSubtype)
	}
	if evt.Severity != models.SeverityWarning {
		t.Errorf("expected Severity %q, got %q", models.SeverityWarning, evt.Severity)
	}
	if evt.Component != "pubkey" {
		t.Errorf("expected Component %q, got %q", "pubkey", evt.Component)
	}
	if evt.Message != "key fetch failed" {
		t.Errorf("expected Message %q, got %q", "key fetch failed", evt.Message)
	}
	if evt.Metadata["key1"] != "value1" {
		t.Errorf("expected metadata key1=value1, got %v", evt.Metadata["key1"])
	}
	// JSON round-trip converts int to float64
	if int(evt.Metadata["key2"].(float64)) != 42 {
		t.Errorf("expected metadata key2=42, got %v", evt.Metadata["key2"])
	}
}

func TestTeeLoggerConvenienceMethods(t *testing.T) {
	buffer := NewBuffer(t.TempDir() + "/events.json")
	logger := NewTeeLogger(buffer, uuid.Must(uuid.NewV4()))

	tests := []struct {
		name           string
		call           func()
		expectedLevel  string
		expectedSubtype string
		expectedSeverity string
	}{
		{"Info", func() { logger.Info("agent", "config", "config", "loaded", nil) }, "INFO", models.SubtypeInfo, models.SeverityInfo},
		{"Warning", func() { logger.Warning("agent", "config", "config", "stale", nil) }, "WARNING", models.SubtypeWarning, models.SeverityWarning},
		{"Error", func() { logger.Error("agent", "config", "config", "failed", nil) }, "ERROR", models.SubtypeFailed, models.SeverityError},
		{"Critical", func() { logger.Critical("agent", "config", "config", "corrupt", nil) }, "CRITICAL", models.SubtypeCritical, models.SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.call()
		})
	}

	events, err := buffer.GetBufferedEvents()
	if err != nil {
		t.Fatalf("GetBufferedEvents failed: %v", err)
	}
	if len(events) != len(tests) {
		t.Fatalf("expected %d events, got %d", len(tests), len(events))
	}

	for i, tt := range tests {
		evt := events[i]
		if evt.EventSubtype != tt.expectedSubtype {
			t.Errorf("%s: expected EventSubtype %q, got %q", tt.name, tt.expectedSubtype, evt.EventSubtype)
		}
		if evt.Severity != tt.expectedSeverity {
			t.Errorf("%s: expected Severity %q, got %q", tt.name, tt.expectedSeverity, evt.Severity)
		}
	}
}

func TestDeriveEventType(t *testing.T) {
	tests := []struct {
		component string
		expected  string
	}{
		{"migration", models.EventTypeAgentMigration},
		{"crypto", models.EventTypeAgentCrypto},
		{"config", models.EventTypeAgentConfig},
		{"docker", models.EventTypeAgentDocker},
		{"installer", models.EventTypeAgentInstall},
		{"unknown", models.EventTypeError},
	}

	for _, tt := range tests {
		t.Run(tt.component, func(t *testing.T) {
			got := deriveEventType(tt.component)
			if got != tt.expected {
				t.Errorf("deriveEventType(%q) = %q, want %q", tt.component, got, tt.expected)
			}
		})
	}
}

func TestTeeLoggerMetadataNil(t *testing.T) {
	buffer := NewBuffer(t.TempDir() + "/events.json")
	logger := NewTeeLogger(buffer, uuid.Must(uuid.NewV4()))

	// Should not panic with nil metadata
	logger.Info("agent", "test", "test_component", "test message", nil)

	events, err := buffer.GetBufferedEvents()
	if err != nil {
		t.Fatalf("GetBufferedEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Metadata != nil {
		t.Errorf("expected nil metadata, got %v", events[0].Metadata)
	}
}
