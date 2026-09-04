// Package supplychain is the unprivileged agent-side consumer of capability
// tokens. It fetches tokens minted by the server authority, confirms each is
// bound to this host, and hands it to the privileged executor (helper/) which
// independently verifies the signature and artifact hashes before performing the
// one authorized operation. The consumer holds no signing key and makes no
// allow/deny decision of its own — that authority lives in the token and the
// executor.
//
// This retires the rs-helper socket-decision daemon: its package-manager
// allowlist is salvaged here as defense-in-depth, but its runtime allow/deny RPC
// role is gone — authorization is the signed token.
package supplychain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/capability"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/gofrs/uuid/v5"
)

// tokenDir and resultDir are subdirectories under DataDir where the agent writes
// the token file for the helper and reads back the result. The directories are
// agent-writable and readable; the token file is 0600 agent-owned, the result
// file is written 0644 by the root helper so the agent can read it.
const (
	tokenDir  = "tokens"
	resultDir = "results"
)

// DefaultExecutorPath is where the privileged executor binary is expected. The
// agent process need not be privileged; the executor is the privileged boundary.
const DefaultExecutorPath = "/usr/local/bin/redflag-helper"

// executorTimeout bounds a single executor invocation. Package operations can be
// slow; this is a guard against a hung child, not a tight latency target.
const executorTimeout = 30 * time.Minute

// AllowedPackageManagers is salvaged from rs-helper. Defense-in-depth only: the
// signed token is the authorization. A token for a package_type outside this set
// and outside the self-update set is refused before any executor or local binary
// swap is attempted.
var AllowedPackageManagers = map[string]bool{
	"apt":    true,
	"dnf":    true,
	"npm":    true,
	"bun":    true,
	"pip":    true,
	"docker": true,
	"winget": true,
}

// AllowedSelfUpdatePackageTypes are capability-token package types for RedFlag's
// own binaries. They are not package managers and therefore stay out of
// AllowedPackageManagers, but the consumer accepts them as first-class capability
// operations.
var AllowedSelfUpdatePackageTypes = map[string]bool{
	AgentSelfPackageType:   true,
	HelperSelfPackageType:  true,
	DesktopSelfPackageType: true,
}

func allowedCapabilityPackageType(packageType string) bool {
	return AllowedPackageManagers[packageType] || AllowedSelfUpdatePackageTypes[packageType]
}

// PolicyResult mirrors the executor's stdout contract (helper/src/main.rs). Field
// names must stay aligned with the Rust PolicyResult serialization.
type PolicyResult struct {
	TokenID           string `json:"token_id"`
	AgentID           string `json:"agent_id"`
	PackageType       string `json:"package_type"`
	Operation         string `json:"operation"`
	Decision          string `json:"decision"` // executed | denied | failed
	Reason            string `json:"reason"`
	Executed          bool   `json:"executed"`
	VerifiedArtifacts int    `json:"verified_artifacts"`
	ExitCode          int    `json:"exit_code"`
	Error             string `json:"error,omitempty"`
	Timestamp         int64  `json:"timestamp"`
}

// TokenProcessSummary is count-only processing state safe for local status
// surfaces. It intentionally excludes token IDs and token payloads.
type TokenProcessSummary struct {
	Total     int
	Processed int
	Failed    int
}

// safeTokenFilename validates that a token ID is safe to use as a filename.
// It rejects any ID containing path separators or traversal components, which
// prevents a malicious server from writing files outside the intended directory.
func safeTokenFilename(id string) (string, error) {
	// filepath.Base strips directory components; reject if it changed the value.
	base := filepath.Base(id)
	if base != id || base == "." || base == ".." || strings.ContainsAny(id, "/\\") {
		return "", fmt.Errorf("unsafe token_id: %q", id)
	}
	if len(base) > 128 {
		return "", fmt.Errorf("token_id too long: %d chars", len(base))
	}
	return base + ".json", nil
}

// Executor invokes the privileged helper binary, piping a token to its stdin and
// parsing the structured result from stdout.
type Executor struct {
	BinaryPath string
}

// NewExecutor returns an Executor for the given binary path, defaulting when empty.
func NewExecutor(binaryPath string) *Executor {
	if binaryPath == "" {
		binaryPath = DefaultExecutorPath
	}
	return &Executor{BinaryPath: binaryPath}
}

// Execute runs the executor for one token. It returns the parsed PolicyResult.
// A non-zero executor exit is reflected in result.ExitCode/Decision, not as a Go
// error; a Go error is returned only when the executor could not be run or its
// output could not be parsed.
//
// The token is delivered to the helper via a file under the agent's data dir
// rather than --pipe fd-passing over dbus. dbus-broker 37 on Fedora 43 drops
// SCM_RIGHTS for new-connection handshake messages (MSG_CTRUNC), which causes
// --pipe to fail with "Connection reset by peer" at StartTransientUnit. Writing
// the token to a temp file and passing --token-file avoids the fd-transfer
// entirely while preserving the same mount-namespace escape.
func (e *Executor) Execute(ctx context.Context, token *capability.Token, extraArgs ...string) (*PolicyResult, error) {
	payload, err := json.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("marshal token: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, executorTimeout)
	defer cancel()

	agentDir := filepath.Join(constants.GetBaseDir(), constants.AgentDir)
	tokDir := filepath.Join(agentDir, tokenDir)
	resDir := filepath.Join(agentDir, resultDir)
	if err := os.MkdirAll(tokDir, 0o700); err != nil {
		return nil, fmt.Errorf("create token dir: %w", err)
	}
	if err := os.MkdirAll(resDir, 0o700); err != nil {
		return nil, fmt.Errorf("create result dir: %w", err)
	}

	fname, err := safeTokenFilename(token.TokenID)
	if err != nil {
		return nil, err
	}
	tokPath := filepath.Join(tokDir, fname)
	resPath := filepath.Join(resDir, fname)
	if err := os.Remove(resPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("clear stale result file: %w", err)
	}

	if err := os.WriteFile(tokPath, payload, 0o600); err != nil {
		return nil, fmt.Errorf("write token file: %w", err)
	}
	defer os.Remove(tokPath)
	defer os.Remove(resPath) // best-effort removal of stale results

	// Invoke the helper. On Linux this goes through sudo systemd-run --wait
	// so the helper escapes the agent's ProtectSystem=strict sandbox. On
	// Windows the helper runs as a direct child process (both run as SYSTEM).
	// Without --pipe, no SCM_RIGHTS fd-passing crosses dbus, working around
	// dbus-broker 37's MSG_CTRUNC bug.
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" {
		args := []string{"systemd-run", "--wait",
			"--property=ProtectSystem=no",
			"--", e.BinaryPath, "--token-file", tokPath, "--result-file", resPath}
		args = append(args, extraArgs...)
		cmd = exec.CommandContext(runCtx, "sudo", args...)
	} else {
		args := []string{"--token-file", tokPath, "--result-file", resPath}
		args = append(args, extraArgs...)
		cmd = exec.CommandContext(runCtx, e.BinaryPath, args...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// Forward the executor's structured stderr logs to the agent log stream.
	// Without --pipe, stderr from systemd-run itself (unit summary, errors) is
	// captured here; the helper's own stderr goes to journald via the transient
	// unit and is not blocked.
	if stderr.Len() > 0 {
		for _, line := range bytes.Split(bytes.TrimRight(stderr.Bytes(), "\n"), []byte("\n")) {
			log.Printf("[INFO] [agent] [supplychain] executor_log %s", string(line))
		}
	}

	resultBytes, err := os.ReadFile(resPath)
	if err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("helper failed and result file not found: %w (exec error: %v)", err, runErr)
		}
		return nil, fmt.Errorf("helper succeeded but result file not found: %w", err)
	}

	var result PolicyResult
	if err := json.Unmarshal(bytes.TrimSpace(resultBytes), &result); err != nil {
		return nil, fmt.Errorf("parse executor result: %w", err)
	}

	// TOCTOU sanity check: the result's token_id must match what we sent.
	// Catches accidental mismatches; a deliberate forgery would need write
	// access to the results directory (requires agent-user compromise).
	if result.TokenID != "" && result.TokenID != token.TokenID {
		return nil, fmt.Errorf("result token_id mismatch: expected %s, got %s", token.TokenID, result.TokenID)
	}

	return &result, nil
}

// Reporter delivers a token result back to the server for audit (optional).
type Reporter interface {
	ReportCapabilityResult(agentID uuid.UUID, tokenID string, decision, reason string, exitCode int) error
}

// ArtifactDownloader is implemented by the authenticated API client. Self-update
// tokens may point closure entries at relative or absolute binary URLs.
type ArtifactDownloader interface {
	DownloadAuthenticatedToFile(rawURL, dstPath string, maxBytes int64) (int64, error)
}

// Consumer ties together fetching, bind-checking, executing, and reporting.
type Consumer struct {
	agentID    uuid.UUID
	executor   *Executor
	reporter   Reporter           // optional
	downloader ArtifactDownloader // optional, inferred from reporter when available

	// mu serializes ProcessToken. The helper's replay-state guard is check-then-act
	// (read-scan-rewrite) and not safe under concurrent processing of the same token;
	// GATE-004 B removed the agent-side desktop-self replay file, so the helper guard
	// is now the only one. The instance lock is process-level only; today's single
	// caller (loop.go) never overlaps, but a future second caller — local-API trigger,
	// retry worker — would race without this. One token at a time is the design
	// invariant; this enforces it.
	mu sync.Mutex
}

// NewConsumer builds a consumer bound to this host's agent identity.
func NewConsumer(agentID uuid.UUID, executor *Executor, reporter Reporter) *Consumer {
	var downloader ArtifactDownloader
	if d, ok := reporter.(ArtifactDownloader); ok {
		downloader = d
	}
	return &Consumer{agentID: agentID, executor: executor, reporter: reporter, downloader: downloader}
}

// ProcessToken runs the full path for one token. It refuses, before invoking the
// executor, any token not bound to this host or for an unsupported package type;
// the executor enforces the same checks again as the privileged authority.
func (c *Consumer) ProcessToken(ctx context.Context, token *capability.Token) (*PolicyResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if token.AgentID != c.agentID.String() {
		log.Printf("[SECURITY] [agent] [supplychain] bind_check_failed token_id=%s token_agent_id=%s host_agent_id=%s",
			token.TokenID, token.AgentID, c.agentID)
		err := fmt.Errorf("token not bound to this host")
		c.reportTokenProcessFailure(token, err)
		return nil, err
	}
	if !allowedCapabilityPackageType(token.PackageType) {
		log.Printf("[SECURITY] [agent] [supplychain] package_type_refused token_id=%s package_type=%s",
			token.TokenID, token.PackageType)
		err := fmt.Errorf("package_type %q not in allowlist", token.PackageType)
		c.reportTokenProcessFailure(token, err)
		return nil, err
	}

	var (
		result *PolicyResult
		err    error
	)
	switch token.PackageType {
	case AgentSelfPackageType:
		result, err = c.processAgentSelfToken(ctx, token)
	case HelperSelfPackageType:
		result, err = c.processHelperSelfToken(ctx, token)
	case DesktopSelfPackageType:
		result, err = c.processDesktopSelfToken(ctx, token)
	default:
		result, err = c.executor.Execute(ctx, token)
	}
	if err != nil {
		log.Printf("[ERROR] [agent] [supplychain] token_process_failed token_id=%s package_type=%s error=%v",
			token.TokenID, token.PackageType, err)
		c.reportTokenProcessFailure(token, err)
		return nil, err
	}

	log.Printf("[SECURITY] [agent] [supplychain] token_processed token_id=%s decision=%s reason=%s exit=%d verified_artifacts=%d",
		token.TokenID, result.Decision, result.Reason, result.ExitCode, result.VerifiedArtifacts)

	if c.reporter != nil {
		if err := c.reporter.ReportCapabilityResult(c.agentID, token.TokenID, result.Decision, result.Reason, result.ExitCode); err != nil {
			log.Printf("[WARNING] [agent] [supplychain] result_report_failed token_id=%s error=%v", token.TokenID, err)
		}
	}

	return result, nil
}

func (c *Consumer) reportTokenProcessFailure(token *capability.Token, cause error) {
	if c.reporter == nil || token == nil {
		return
	}
	reason := "token_process_failed"
	if cause != nil {
		reason = cause.Error()
	}
	if err := c.reporter.ReportCapabilityResult(c.agentID, token.TokenID, "failed", reason, 1); err != nil {
		log.Printf("[WARNING] [agent] [supplychain] failure_result_report_failed token_id=%s error=%v", token.TokenID, err)
	}
}

// ProcessTokens runs ProcessToken for each token, continuing past individual
// failures so one bad token does not strand the rest.
func (c *Consumer) ProcessTokens(ctx context.Context, tokens []*capability.Token) {
	_ = c.ProcessTokensWithSummary(ctx, tokens)
}

// ProcessTokensWithSummary runs ProcessToken for each token and returns count-only
// outcome state for local observability.
func (c *Consumer) ProcessTokensWithSummary(ctx context.Context, tokens []*capability.Token) TokenProcessSummary {
	summary := TokenProcessSummary{Total: len(tokens)}
	for _, token := range tokens {
		if _, err := c.ProcessToken(ctx, token); err != nil {
			summary.Failed++
			log.Printf("[WARNING] [agent] [supplychain] token_skipped token_id=%s error=%v", token.TokenID, err)
			continue
		}
		summary.Processed++
	}
	return summary
}
