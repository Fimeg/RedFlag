// discovery.go — single chokepoint for all read-only package-manager operations.
// Every scan, dry-run, and hash-resolve goes through here. Discovery runs
// in the agent's sandbox; the runner owns sandbox compatibility (temp logdir
// for dnf, etc.) per ecosystem.
package installer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// EcosystemConfig describes one package manager's discovery behaviour.
type EcosystemConfig struct {
	Binary           string
	SudoForDiscovery bool
	// SandboxOpts returns extra args to insert before the subcommand
	// so the package manager can write incidental logs/cache inside
	// a ProtectSystem=strict systemd unit. tmpDir is a private temp
	// dir created by the runner; it is cleaned up after the command.
	SandboxOpts func(tmpDir string) []string
}

// ecosystemRegistry is the canonical set of known ecosystems.
// Adding a new ecosystem (AUR, Snap, Flatpak, Homebrew) means one entry here.
var ecosystemRegistry = map[string]EcosystemConfig{
	"dnf": {
		Binary: "dnf",
		// Discovery is unprivileged. SandboxOpts redirect dnf's log and cache
		// into an agent-writable temp dir, so check-update / makecache /
		// install --downloadonly / download all run without root. Mutation is
		// the helper's job (root via systemd-run); the agent holds no dnf sudo.
		SudoForDiscovery: false,
		SandboxOpts: func(tmpDir string) []string {
			return []string{
				"--setopt=logdir=" + tmpDir,
				"--setopt=cachedir=" + tmpDir,
			}
		},
	},
	"apt": {
		Binary: "apt",
		// Discovery is unprivileged, same model as dnf. SandboxOpts redirect apt's
		// lists/cache/state/log into an agent-writable temp dir so `apt update`,
		// `apt list --upgradable`, and `apt install --dry-run` run without root.
		// Mutation is the helper's job; the agent holds no apt sudo. Cost: each
		// scan re-fetches repo metadata into the ephemeral dir (perf, not
		// correctness), mirroring dnf's tradeoff.
		SudoForDiscovery: false,
		SandboxOpts: func(tmpDir string) []string {
			return []string{
				"-o", "Dir::State::Lists=" + tmpDir + "/lists",
				"-o", "Dir::Cache=" + tmpDir + "/cache",
				"-o", "Dir::State=" + tmpDir + "/state",
				"-o", "Dir::Log=" + tmpDir,
			}
		},
	},
	"docker": {
		Binary:           "docker",
		SudoForDiscovery: false,
	},
	"pacman": {
		Binary:           "checkupdates",
		SudoForDiscovery: false,
		// checkupdates handles its own sandboxing internally — it syncs a
		// private copy of the repo databases into a temp dir and runs
		// pacman -Qu against it. No root required, no mutation of the live
		// pacman database. Discovery is purely read-only.
	},
	"winget": {
		Binary:           "winget",
		SudoForDiscovery: false,
	},
	"windows_update": {
		Binary:           "powershell",
		SudoForDiscovery: false,
	},
}

// ConfigFor returns the EcosystemConfig for a package type.
func ConfigFor(packageType string) (EcosystemConfig, error) {
	cfg, ok := ecosystemRegistry[packageType]
	if !ok {
		return EcosystemConfig{}, fmt.Errorf("unknown ecosystem: %q", packageType)
	}
	return cfg, nil
}

// DiscoveryRunner executes read-only package-manager commands inside the
// agent's sandbox.
type DiscoveryRunner struct {
	cfg EcosystemConfig
}

// NewDiscoveryRunner returns a runner for the given ecosystem.
func NewDiscoveryRunner(packageType string) (*DiscoveryRunner, error) {
	cfg, err := ConfigFor(packageType)
	if err != nil {
		return nil, err
	}
	return &DiscoveryRunner{cfg: cfg}, nil
}

// RunResult is the structured output of a discovery command.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Run executes a discovery command. args is the subcommand and its arguments.
// SandboxOpts are prepended if set; sudo is used if SudoForDiscovery is true.
func (r *DiscoveryRunner) Run(ctx context.Context, args ...string) (*RunResult, error) {
	start := time.Now()
	if len(args) == 0 {
		return nil, fmt.Errorf("[ERROR] [agent] [discovery] no_args ecosystem=%s", r.cfg.Binary)
	}

	var tmpDir string
	var fullArgs []string

	if r.cfg.SandboxOpts != nil {
		var err error
		tmpDir, err = os.MkdirTemp("", "redflag-discovery-")
		if err != nil {
			return nil, fmt.Errorf("[ERROR] [agent] [discovery] tmpdir_create ecosystem=%s error=%w", r.cfg.Binary, err)
		}
		defer os.RemoveAll(tmpDir)
		fullArgs = append(fullArgs, r.cfg.SandboxOpts(tmpDir)...)
	}
	fullArgs = append(fullArgs, args...)

	var cmd *exec.Cmd
	if r.cfg.SudoForDiscovery {
		fullPath, err := exec.LookPath(r.cfg.Binary)
		if err != nil {
			return nil, fmt.Errorf("[ERROR] [agent] [discovery] binary_not_found ecosystem=%s binary=%s error=%w",
				r.cfg.Binary, r.cfg.Binary, err)
		}
		sudoArgs := append([]string{fullPath}, fullArgs...)
		cmd = exec.CommandContext(ctx, "sudo", sudoArgs...)
	} else {
		cmd = exec.CommandContext(ctx, r.cfg.Binary, fullArgs...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	dur := time.Since(start)

	result := &RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: dur,
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		return result, fmt.Errorf("[ERROR] [agent] [discovery] command_failed ecosystem=%s args=%v exit=%d stderr=%s error=%w",
			r.cfg.Binary, args, result.ExitCode, strings.TrimSpace(result.Stderr), runErr)
	}

	return result, nil
}
