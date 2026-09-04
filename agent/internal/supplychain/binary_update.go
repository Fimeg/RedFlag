package supplychain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Fimeg/RedFlag/agent/internal/capability"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
)

// Staging paths are derived from constants.GetAgentStagingPath so they are
// platform-aware. The cross-language contract with the Rust helper
// (helper/src/main.rs DEFAULT_* constants) uses the same suffix values.
var (
	agentSelfStagingPath   = constants.GetAgentStagingPath("pending-upgrade.bin")
	helperSelfStagingPath  = constants.GetAgentStagingPath("pending-helper.bin")
	desktopSelfStagingPath = constants.GetAgentStagingPath("pending-desktop.bin")
)

const (
	AgentSelfPackageType   = "agent-self"
	HelperSelfPackageType  = "helper-self"
	DesktopSelfPackageType = "desktop-self"

	selfUpdateOperation = "upgrade"

	maxSelfUpdateBinarySize = 500 * 1024 * 1024
)

func (c *Consumer) processAgentSelfToken(ctx context.Context, token *capability.Token) (*PolicyResult, error) {
	if token.Operation != selfUpdateOperation {
		return nil, fmt.Errorf("agent-self operation %q not supported", token.Operation)
	}

	agentEntry, err := findClosureEntry(token, "redflag-agent")
	if err != nil {
		return nil, err
	}
	stagedAgent, err := c.stageClosureArtifact(agentEntry, agentSelfStagingPath)
	if err != nil {
		return nil, fmt.Errorf("stage agent-self artifact: %w", err)
	}
	defer os.Remove(stagedAgent)

	var helperArgs []string
	if helperEntry, err := findClosureEntry(token, "redflag-helper"); err == nil {
		stagedHelper, err := c.stageClosureArtifact(helperEntry, helperSelfStagingPath)
		if err != nil {
			return nil, fmt.Errorf("stage agent-self helper artifact: %w", err)
		}
		defer os.Remove(stagedHelper)
		helperArgs = []string{"--helper-file", stagedHelper}
	}

	return c.executor.Execute(ctx, token, helperArgs...)
}

func (c *Consumer) processHelperSelfToken(ctx context.Context, token *capability.Token) (*PolicyResult, error) {
	if token.Operation != selfUpdateOperation {
		return nil, fmt.Errorf("helper-self operation %q not supported", token.Operation)
	}

	entry, err := findClosureEntry(token, "redflag-helper")
	if err != nil {
		return nil, err
	}
	staged, err := c.stageClosureArtifact(entry, helperSelfStagingPath)
	if err != nil {
		return nil, fmt.Errorf("stage helper-self artifact: %w", err)
	}
	defer os.Remove(staged)

	return c.executor.Execute(ctx, token)
}

// processDesktopSelfToken stages the new desktop binary where the helper reads
// it, then hands the token to the privileged executor — the same path agent-self
// and helper-self take. The helper verifies the signature and hash, consumes the
// replay slot (record-before-install), and atomically installs the binary. The
// agent no longer mutates the binary or keeps its own replay file (GATE-004): the
// desktop app stays a pure status surface, and the one hardened replay guard
// lives in the helper. The token, closure, and signed message are unchanged.
func (c *Consumer) processDesktopSelfToken(ctx context.Context, token *capability.Token) (*PolicyResult, error) {
	if token.Operation != selfUpdateOperation {
		return nil, fmt.Errorf("desktop-self operation %q not supported", token.Operation)
	}

	entry, err := findClosureEntry(token, "redflag-desktop")
	if err != nil {
		return nil, err
	}
	staged, err := c.stageClosureArtifact(entry, desktopSelfStagingPath)
	if err != nil {
		return nil, fmt.Errorf("stage desktop-self artifact: %w", err)
	}
	defer os.Remove(staged)

	result, err := c.executor.Execute(ctx, token)
	if err != nil {
		return result, err
	}

	// Best-effort: nudge the running desktop app to relaunch on the new binary.
	// This is not a privileged or trust-sensitive step — just a signal — so it
	// stays agent-side. A failure here does not undo a successful install.
	if result != nil && result.Executed {
		if target, perr := desktopBinaryPath(); perr == nil {
			if sigErr := signalDesktopRestart(target); sigErr != nil {
				log.Printf("[WARNING] [agent] [supplychain] desktop_restart_signal_failed token_id=%s error=%v", token.TokenID, sigErr)
			}
		}
	}
	return result, nil
}

func findClosureEntry(token *capability.Token, name string) (*capability.ClosureEntry, error) {
	for i := range token.Closure {
		if token.Closure[i].Name == name {
			return &token.Closure[i], nil
		}
	}
	return nil, fmt.Errorf("closure missing %s entry", name)
}

func (c *Consumer) stageClosureArtifact(entry *capability.ClosureEntry, dstPath string) (string, error) {
	if entry.ArtifactPath == "" {
		return "", fmt.Errorf("closure entry %s missing artifact_path", entry.Name)
	}
	if strings.TrimSpace(entry.SHA256) == "" {
		return "", fmt.Errorf("closure entry %s missing sha256", entry.Name)
	}

	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}

	tmp, err := os.CreateTemp(dstDir, filepath.Base(dstPath)+".")
	if err != nil {
		return "", fmt.Errorf("create staging temp: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("close staging temp: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpPath)
		}
	}()

	if err := c.fetchArtifactToFile(entry.ArtifactPath, tmpPath); err != nil {
		return "", err
	}
	actual, err := computeFileSHA256Hex(tmpPath)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(actual, entry.SHA256) {
		return "", fmt.Errorf("hash mismatch for %s: expected=%s actual=%s", entry.Name, strings.ToLower(entry.SHA256), actual)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return "", fmt.Errorf("chmod staged artifact: %w", err)
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return "", fmt.Errorf("commit staged artifact: %w", err)
	}
	cleanup = false
	return dstPath, nil
}

func (c *Consumer) fetchArtifactToFile(ref, dstPath string) error {
	// Download refs (full URLs and server-relative /api/ paths) never touch the
	// local-path branch — "/api/v1/..." would otherwise be mistaken for a
	// missing on-disk staging path.
	if isDownloadRef(ref) {
		if c.downloader == nil {
			return fmt.Errorf("artifact downloader unavailable for %s", ref)
		}
		if _, err := c.downloader.DownloadAuthenticatedToFile(ref, dstPath, maxSelfUpdateBinarySize); err != nil {
			return fmt.Errorf("download artifact %s: %w", ref, err)
		}
		return nil
	}

	if info, err := os.Stat(ref); err == nil {
		if info.IsDir() {
			return fmt.Errorf("artifact path is a directory: %s", ref)
		}
		return copyRegularFile(ref, dstPath)
	} else if !os.IsNotExist(err) {
		// Stat failed for a reason other than "not found" (e.g. permission denied).
		return fmt.Errorf("stat artifact %s: %w", ref, err)
	}

	// Local absolute path that doesn't exist — don't fall through to the
	// downloader; the file was expected on disk and is missing.
	if strings.HasPrefix(ref, "/") {
		return fmt.Errorf("artifact not staged at %s", ref)
	}

	return copyRegularFile(ref, dstPath)
}

// isDownloadRef reports whether ref is fetched over the network: a full URL or
// a server-relative API path (resolved against the configured server with the
// agent's credentials by DownloadAuthenticatedToFile).
func isDownloadRef(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "/api/")
}

func desktopBinaryPath() (string, error) {
	if p := os.Getenv("REDFLAG_DESKTOP_BINARY"); p != "" {
		return p, nil
	}
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("determine agent executable path: %w", err)
	}
	name := "redflag-desktop"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(execPath), name), nil
}

func signalDesktopRestart(target string) error {
	name := filepath.Base(target)
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "linux":
		cmd = exec.Command("pkill", "-x", name)
	case runtime.GOOS == "windows":
		cmd = exec.Command("taskkill", "/F", "/IM", name)
	default:
		return nil
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		log.Printf("[INFO] [agent] [supplychain] desktop_restart_signal_sent binary=%s", name)
		return nil
	}
	// pkill exit 1 = no process matched; taskkill exit 128 = process not found.
	// Both are non-errors — Desktop wasn't running.
	if exitErr, ok := err.(*exec.ExitError); ok {
		if (runtime.GOOS == "linux" && exitErr.ExitCode() == 1) ||
			(runtime.GOOS == "windows" && exitErr.ExitCode() == 128) {
			log.Printf("[INFO] [agent] [supplychain] desktop_restart_signal_skipped binary=%s reason=not_running", name)
			return nil
		}
	}
	return fmt.Errorf("signal restart %s failed: %w output=%s", name, err, strings.TrimSpace(string(out)))
}

func copyRegularFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	if info, err := in.Stat(); err != nil {
		return fmt.Errorf("stat source: %w", err)
	} else if info.IsDir() {
		return fmt.Errorf("source is a directory: %s", src)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy file: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination: %w", err)
	}
	return nil
}

func computeFileSHA256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open for hash: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
