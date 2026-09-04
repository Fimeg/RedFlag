package supplychain

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fimeg/RedFlag/agent/internal/capability"
	"github.com/gofrs/uuid/v5"
)

type recordingReporter struct {
	agentID  uuid.UUID
	tokenID  string
	decision string
	reason   string
	exitCode int
	calls    int
}

func (r *recordingReporter) ReportCapabilityResult(agentID uuid.UUID, tokenID string, decision, reason string, exitCode int) error {
	r.agentID = agentID
	r.tokenID = tokenID
	r.decision = decision
	r.reason = reason
	r.exitCode = exitCode
	r.calls++
	return nil
}

func TestSafeTokenFilename(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid uuid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"simple alphanumeric", "tok-123", false},
		{"path traversal dot-dot", "../etc/passwd", true},
		{"path traversal slash", "tok/../../secret", true},
		{"path traversal backslash", "tok\\..\\secret", true},
		{"single dot", ".", true},
		{"double dot", "..", true},
		{"empty string", "", true},
		{"too long", string(make([]byte, 200)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeTokenFilename(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("safeTokenFilename(%q) = %q, want error", tt.id, got)
				}
				return
			}
			if err != nil {
				t.Errorf("safeTokenFilename(%q) error: %v", tt.id, err)
				return
			}
			if got != tt.id+".json" {
				t.Errorf("safeTokenFilename(%q) = %q, want %q", tt.id, got, tt.id+".json")
			}
		})
	}
}

func TestAllowedPackageManagers(t *testing.T) {
	allowed := []string{"apt", "dnf", "npm", "bun", "pip", "docker", "winget"}
	for _, pm := range allowed {
		if !AllowedPackageManagers[pm] {
			t.Errorf("expected %q to be in AllowedPackageManagers", pm)
		}
	}

	blocked := []string{"yum", "pacman", "brew", "cargo", ""}
	for _, pm := range blocked {
		if AllowedPackageManagers[pm] {
			t.Errorf("expected %q to NOT be in AllowedPackageManagers", pm)
		}
	}
}

func TestAllowedSelfUpdatePackageTypes(t *testing.T) {
	allowed := []string{AgentSelfPackageType, HelperSelfPackageType, DesktopSelfPackageType}
	for _, packageType := range allowed {
		if !AllowedSelfUpdatePackageTypes[packageType] {
			t.Errorf("expected %q to be in AllowedSelfUpdatePackageTypes", packageType)
		}
		if AllowedPackageManagers[packageType] {
			t.Errorf("expected %q to stay out of AllowedPackageManagers", packageType)
		}
		if !allowedCapabilityPackageType(packageType) {
			t.Errorf("expected %q to be accepted by allowedCapabilityPackageType", packageType)
		}
	}

	if allowedCapabilityPackageType("cargo") {
		t.Fatal("expected cargo to remain disallowed")
	}
}

func TestStageClosureArtifactFromLocalFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "redflag-desktop.src")
	dst := filepath.Join(dir, "redflag-desktop.staged")
	body := []byte("desktop-binary")
	if err := os.WriteFile(src, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)

	consumer := NewConsumer(uuid.Must(uuid.NewV4()), NewExecutor("/nonexistent/helper"), nil)
	entry := &capability.ClosureEntry{
		Name:         "redflag-desktop",
		Version:      "test",
		SHA256:       hex.EncodeToString(sum[:]),
		Source:       "desktop-self",
		ArtifactPath: src,
	}

	staged, err := consumer.stageClosureArtifact(entry, dst)
	if err != nil {
		t.Fatalf("stageClosureArtifact returned error: %v", err)
	}
	if staged != dst {
		t.Fatalf("staged path = %q, want %q", staged, dst)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("staged body = %q, want %q", string(got), string(body))
	}
}

func TestConsumerProcessTokenRejectsWrongAgentID(t *testing.T) {
	agentID := uuid.Must(uuid.NewV4())
	executor := NewExecutor("/nonexistent/helper")
	reporter := &recordingReporter{}
	consumer := NewConsumer(agentID, executor, reporter)

	wrongID := uuid.Must(uuid.NewV4())
	tok := &capability.Token{
		TokenID:     "tok-1",
		AgentID:     wrongID.String(),
		PackageType: "npm",
		Operation:   "install",
	}

	_, err := consumer.ProcessToken(t.Context(), tok)
	if err == nil {
		t.Fatal("expected error for wrong agent ID, got nil")
	}
	if got := err.Error(); got != "token not bound to this host" {
		t.Errorf("unexpected error: %v", got)
	}
	if reporter.calls != 1 {
		t.Fatalf("reporter calls = %d, want 1", reporter.calls)
	}
	if reporter.agentID != agentID || reporter.tokenID != "tok-1" || reporter.decision != "failed" || reporter.exitCode != 1 {
		t.Fatalf("reported result = %#v", reporter)
	}
	if reporter.reason != "token not bound to this host" {
		t.Fatalf("reported reason = %q", reporter.reason)
	}
}

func TestConsumerProcessTokenRejectsDisallowedPackageType(t *testing.T) {
	agentID := uuid.Must(uuid.NewV4())
	executor := NewExecutor("/nonexistent/helper")
	reporter := &recordingReporter{}
	consumer := NewConsumer(agentID, executor, reporter)

	tok := &capability.Token{
		TokenID:     "tok-2",
		AgentID:     agentID.String(),
		PackageType: "cargo", // not in allowlist
		Operation:   "install",
	}

	_, err := consumer.ProcessToken(t.Context(), tok)
	if err == nil {
		t.Fatal("expected error for disallowed package type, got nil")
	}
	if reporter.calls != 1 {
		t.Fatalf("reporter calls = %d, want 1", reporter.calls)
	}
	if reporter.tokenID != "tok-2" || reporter.decision != "failed" || reporter.reason != "package_type \"cargo\" not in allowlist" {
		t.Fatalf("reported result = %#v", reporter)
	}
}

func TestConsumerProcessTokensSummary(t *testing.T) {
	agentID := uuid.Must(uuid.NewV4())
	executor := NewExecutor("/nonexistent/helper")
	consumer := NewConsumer(agentID, executor, nil)

	// All three tokens should fail (wrong agent ID), so summary counts failures.
	tokens := []*capability.Token{
		{TokenID: "t1", AgentID: "wrong", PackageType: "npm"},
		{TokenID: "t2", AgentID: "wrong", PackageType: "dnf"},
		{TokenID: "t3", AgentID: "wrong", PackageType: "apt"},
	}

	summary := consumer.ProcessTokensWithSummary(t.Context(), tokens)
	if summary.Total != 3 {
		t.Errorf("Total = %d, want 3", summary.Total)
	}
	if summary.Failed != 3 {
		t.Errorf("Failed = %d, want 3", summary.Failed)
	}
	if summary.Processed != 0 {
		t.Errorf("Processed = %d, want 0", summary.Processed)
	}
}
