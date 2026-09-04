package supplychain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/capability"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/gofrs/uuid/v5"
)

// Standalone local authority — mint invocation (FEAT-003, design of record
// RAF/security/06-standalone-authority.md). The unprivileged agent assembles a
// mint request (closure + gate evidence) and asks the privileged helper to
// sign a capability token with the root-owned local authority key. The helper
// re-validates everything; this file is transport, not judgment.

const (
	mintRequestDir = "mint"
	mintTimeout    = 60 * time.Second
)

// Helper mint-mode exit codes (helper/src/main.rs deny taxonomy).
const (
	mintExitGate      = 22
	mintExitStale     = 23
	mintExitKey       = 24
	mintExitDuplicate = 25
)

// Typed mint failures so the local API can map them to honest HTTP codes.
var (
	ErrMintGateRefused = errors.New("mint refused by gate verdict")
	ErrMintStale       = errors.New("mint refused: gate evidence stale")
	ErrMintNoAuthority = errors.New("mint refused: no local authority key (fleet mode or unprovisioned)")
	ErrMintDuplicate   = errors.New("mint refused: request already minted")
)

// GateEvidence is the agent's gate verdict bundle. JSON contract with the
// helper's MintRequest parsing — keep aligned with helper/src/main.rs.
type GateEvidence struct {
	ResolvedAt     int64  `json:"resolved_at"`
	OSVCheckedAt   int64  `json:"osv_checked_at"`
	OSVStatus      string `json:"osv_status"`
	OSVVulnCount   int    `json:"osv_vuln_count"`
	AgeGate        string `json:"age_gate"`
	SoakGate       string `json:"soak_gate"`
	Operator       string `json:"operator"`
	OverrideReason string `json:"override_reason"`
}

// MintRequest is the file handed to `redflag-helper mint --request-file`.
type MintRequest struct {
	Version      int                       `json:"version"`
	RequestID    string                    `json:"request_id"`
	AgentID      string                    `json:"agent_id"`
	PackageType  string                    `json:"package_type"`
	Operation    string                    `json:"operation"`
	Closure      []capability.ClosureEntry `json:"closure"`
	GateEvidence GateEvidence              `json:"gate_evidence"`
}

// Mint invokes the privileged helper's mint mode and returns the signed token.
// The request and token travel as files under the agent data dir, mirroring the
// execute path's --token-file pattern (and the same sudoers pinning style).
func (e *Executor) Mint(ctx context.Context, req *MintRequest) (*capability.Token, error) {
	if req.RequestID == "" {
		id, err := uuid.NewV4()
		if err != nil {
			return nil, fmt.Errorf("generate request id: %w", err)
		}
		req.RequestID = id.String()
	}
	req.Version = capability.Version

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal mint request: %w", err)
	}

	agentDir := filepath.Join(constants.GetBaseDir(), constants.AgentDir)
	reqDir := filepath.Join(agentDir, mintRequestDir)
	tokDir := filepath.Join(agentDir, tokenDir)
	if err := os.MkdirAll(reqDir, 0o750); err != nil {
		return nil, fmt.Errorf("create mint request dir: %w", err)
	}
	if err := os.MkdirAll(tokDir, 0o700); err != nil {
		return nil, fmt.Errorf("create token dir: %w", err)
	}

	fname, err := safeTokenFilename(req.RequestID)
	if err != nil {
		return nil, err
	}
	reqPath := filepath.Join(reqDir, fname)
	tokPath := filepath.Join(tokDir, fname)
	if err := os.Remove(tokPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("clear stale minted token: %w", err)
	}
	if err := os.WriteFile(reqPath, payload, 0o640); err != nil {
		return nil, fmt.Errorf("write mint request: %w", err)
	}
	defer os.Remove(reqPath)
	defer os.Remove(tokPath)

	runCtx, cancel := context.WithTimeout(ctx, mintTimeout)
	defer cancel()

	args := []string{"systemd-run", "--wait",
		"--property=ProtectSystem=no",
		"--", e.BinaryPath, "mint", "--request-file", reqPath, "--token-out", tokPath}
	cmd := exec.CommandContext(runCtx, "sudo", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		log.Printf("[SECURITY] [agent] [supplychain] mint_denied request_id=%s exit=%d stderr=%s",
			req.RequestID, exitCode, stderr.String())
		switch exitCode {
		case mintExitGate:
			return nil, fmt.Errorf("%w (request_id=%s)", ErrMintGateRefused, req.RequestID)
		case mintExitStale:
			return nil, fmt.Errorf("%w (request_id=%s)", ErrMintStale, req.RequestID)
		case mintExitKey:
			return nil, fmt.Errorf("%w (request_id=%s)", ErrMintNoAuthority, req.RequestID)
		case mintExitDuplicate:
			return nil, fmt.Errorf("%w (request_id=%s)", ErrMintDuplicate, req.RequestID)
		default:
			return nil, fmt.Errorf("mint failed exit=%d request_id=%s: %w", exitCode, req.RequestID, runErr)
		}
	}

	raw, err := os.ReadFile(tokPath)
	if err != nil {
		return nil, fmt.Errorf("read minted token: %w", err)
	}
	var token capability.Token
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, fmt.Errorf("parse minted token: %w", err)
	}
	log.Printf("[SECURITY] [agent] [supplychain] mint_succeeded request_id=%s token_id=%s package_type=%s closure_size=%d",
		req.RequestID, token.TokenID, token.PackageType, len(token.Closure))
	return &token, nil
}
