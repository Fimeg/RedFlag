package version

import (
	"testing"
)

func TestCompareVersionsCorrect(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
		desc     string
	}{
		{"0.1.22", "0.1.9", 1, "multi-digit minor beats single-digit"},
		{"0.2.0", "0.1.99", 1, "major bump beats high patch"},
		{"dev", "0.1.0", -1, "dev is always older"},
		{"0.1.0", "dev", 1, "any release beats dev"},
		{"0.1.26.0", "0.1.26.0", 0, "equal versions"},
		{"0.1.26.1", "0.1.26.0", 1, "config version differs"},
		{"1.0.0", "0.99.99", 1, "major version wins"},
		{"dev", "dev", 0, "dev equals dev"},
		{"", "0.1.0", -1, "empty is older"},
		{"v0.1.22", "0.1.22", 0, "v prefix stripped"},
		{"0.1.22", "v0.1.22", 0, "v prefix on second arg"},
	}

	for _, tt := range tests {
		result := CompareVersions(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("CompareVersions(%q, %q): got %d, want %d (%s)", tt.a, tt.b, result, tt.expected, tt.desc)
		}
	}
	t.Logf("[INFO] [server] [version] U-2 VERIFIED: all %d semver comparison cases pass", len(tests))
}

func TestExtractConfigVersionFromAgent(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"0.1.23.6", "6"},
		{"0.1.23", "23"},
		{"v0.1.30", "30"},
		{"0.1.26.10", "10"},
		{"dev", "dev"},
	}

	for _, tt := range tests {
		result := ExtractConfigVersionFromAgent(tt.version)
		if result != tt.expected {
			t.Errorf("ExtractConfigVersionFromAgent(%q): got %q, want %q", tt.version, result, tt.expected)
		}
	}
	t.Logf("[INFO] [server] [version] U-10 VERIFIED: config version extraction works for multi-digit versions")
}

func TestValidateAgentVersionSemverAware(t *testing.T) {
	// Save and restore original MinAgentVersion
	orig := MinAgentVersion
	defer func() { MinAgentVersion = orig }()

	MinAgentVersion = "0.1.10"

	// 0.1.9 < 0.1.10 — should fail
	if err := ValidateAgentVersion("0.1.9"); err == nil {
		t.Error("expected 0.1.9 to fail validation against min 0.1.10, but it passed")
	}

	// 0.1.22 > 0.1.10 — should pass
	if err := ValidateAgentVersion("0.1.22"); err != nil {
		t.Errorf("expected 0.1.22 to pass validation against min 0.1.10, got: %v", err)
	}

	t.Logf("[INFO] [server] [version] U-2 VERIFIED: ValidateAgentVersion uses octet comparison, not lexicographic")
}

func TestInfoEndpointReturnsCurrentVersion(t *testing.T) {
	// Test that GetCurrentVersions returns values (not the old hardcoded v0.1.21)
	versions := GetCurrentVersions()
	if versions.AgentVersion == "v0.1.21" {
		t.Error("AgentVersion is still the old hardcoded v0.1.21 — should be dynamic")
	}
	if versions.AgentVersion == "" {
		t.Error("AgentVersion is empty")
	}
	t.Logf("[INFO] [server] [version] U-1 VERIFIED: GetCurrentVersions returns %q (not hardcoded v0.1.21)", versions.AgentVersion)
}
