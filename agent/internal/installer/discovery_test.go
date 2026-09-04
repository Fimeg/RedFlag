package installer

import (
	"strings"
	"testing"
)

func TestConfigForKnownEcosystems(t *testing.T) {
	for _, eco := range []string{"dnf", "apt", "docker", "winget", "windows_update"} {
		cfg, err := ConfigFor(eco)
		if err != nil {
			t.Errorf("ConfigFor(%q): unexpected error: %v", eco, err)
		}
		if cfg.Binary == "" {
			t.Errorf("ConfigFor(%q): empty Binary", eco)
		}
	}
}

func TestConfigForUnknownEcosystem(t *testing.T) {
	_, err := ConfigFor("nonexistent")
	if err == nil {
		t.Error("ConfigFor(nonexistent): expected error, got nil")
	}
}

func TestDNFConfigSandboxOpts(t *testing.T) {
	cfg, _ := ConfigFor("dnf")
	if cfg.SandboxOpts == nil {
		t.Fatal("dnf must have SandboxOpts")
	}
	opts := cfg.SandboxOpts("/tmp/test")
	if len(opts) != 2 {
		t.Fatalf("expected 2 opts, got %d: %v", len(opts), opts)
	}
	if !strings.HasPrefix(opts[0], "--setopt=logdir=") {
		t.Errorf("expected --setopt=logdir=..., got %s", opts[0])
	}
	if !strings.HasPrefix(opts[1], "--setopt=cachedir=") {
		t.Errorf("expected --setopt=cachedir=..., got %s", opts[1])
	}
}

func TestAPTConfigSandboxOpts(t *testing.T) {
	cfg, _ := ConfigFor("apt")
	if cfg.SandboxOpts == nil {
		t.Fatal("apt must have SandboxOpts")
	}
	opts := cfg.SandboxOpts("/tmp/test")
	// APT sandbox opts: -o Dir::State::Lists=... -o Dir::Cache=... etc.
	if len(opts) < 2 {
		t.Fatalf("expected at least 2 opts, got %d: %v", len(opts), opts)
	}
	if opts[0] != "-o" {
		t.Errorf("expected first opt to be -o, got %s", opts[0])
	}
	if !strings.HasPrefix(opts[1], "Dir::") {
		t.Errorf("expected Dir:: prefix, got %s", opts[1])
	}
}

func TestNewDiscoveryRunnerUnknown(t *testing.T) {
	_, err := NewDiscoveryRunner("auraura")
	if err == nil {
		t.Error("expected error for unknown ecosystem")
	}
}
