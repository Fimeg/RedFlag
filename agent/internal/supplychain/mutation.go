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

const (
	mutationRequestDir  = "mutation-requests"
	mutationEnvelopeDir = "mutation-envelopes"
	mutationReceiptDir  = "mutation-receipts"
	mutationMintTimeout = 2 * time.Minute
)

// EnvelopeMintRequest is the unprivileged request handed to the standalone
// root authority. The helper re-resolves trust from the manifest bytes and
// gate evidence before it signs anything.
type EnvelopeMintRequest struct {
	Version      int                         `json:"version"`
	RequestID    string                      `json:"request_id"`
	Manifest     capability.MutationManifest `json:"manifest"`
	GateEvidence GateEvidence                `json:"gate_evidence"`
}

func runMutationHelper(
	ctx context.Context,
	timeout time.Duration,
	binaryPath string,
	args ...string,
) (int, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	unitArgs := []string{
		"systemd-run", "--wait", "--property=ProtectSystem=no", "--", binaryPath,
	}
	unitArgs = append(unitArgs, args...)
	cmd := exec.CommandContext(runCtx, "sudo", unitArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return exitCode, stderr.String(), err
}

// MintEnvelope asks the root-owned standalone authority to sign one immutable
// mutation manifest. It returns the mint request ID for the durable audit join.
func (e *Executor) MintEnvelope(
	ctx context.Context,
	manifest capability.MutationManifest,
	gateEvidence GateEvidence,
) (*capability.MutationEnvelope, string, error) {
	requestID, err := uuid.NewV4()
	if err != nil {
		return nil, "", fmt.Errorf("generate envelope request id: %w", err)
	}
	request := EnvelopeMintRequest{
		Version: capability.MutationProtocolVersion, RequestID: requestID.String(),
		Manifest: manifest, GateEvidence: gateEvidence,
	}
	if err := request.Manifest.Validate(); err != nil {
		return nil, request.RequestID, err
	}
	payload, err := json.Marshal(&request)
	if err != nil {
		return nil, request.RequestID, fmt.Errorf("marshal envelope request: %w", err)
	}

	agentDir := filepath.Join(constants.GetBaseDir(), constants.AgentDir)
	requestDir := filepath.Join(agentDir, mutationRequestDir)
	envelopeDir := filepath.Join(agentDir, mutationEnvelopeDir)
	if err := os.MkdirAll(requestDir, 0o700); err != nil {
		return nil, request.RequestID, fmt.Errorf("create mutation request dir: %w", err)
	}
	if err := os.MkdirAll(envelopeDir, 0o700); err != nil {
		return nil, request.RequestID, fmt.Errorf("create mutation envelope dir: %w", err)
	}
	filename, err := safeTokenFilename(request.RequestID)
	if err != nil {
		return nil, request.RequestID, err
	}
	requestPath := filepath.Join(requestDir, filename)
	envelopePath := filepath.Join(envelopeDir, filename)
	if err := os.Remove(envelopePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, request.RequestID, fmt.Errorf("clear stale mutation envelope: %w", err)
	}
	if err := os.WriteFile(requestPath, payload, 0o600); err != nil {
		return nil, request.RequestID, fmt.Errorf("write mutation request: %w", err)
	}
	defer os.Remove(requestPath)
	defer os.Remove(envelopePath)

	exitCode, stderr, runErr := runMutationHelper(
		ctx, mutationMintTimeout, e.BinaryPath,
		"mint-envelope", "--request-file", requestPath, "--envelope-out", envelopePath,
	)
	if runErr != nil {
		log.Printf("[SECURITY] [agent] [supplychain] envelope_mint_denied request_id=%s exit=%d stderr=%s",
			request.RequestID, exitCode, stderr)
		switch exitCode {
		case mintExitGate:
			return nil, request.RequestID, fmt.Errorf("%w (request_id=%s)", ErrMintGateRefused, request.RequestID)
		case mintExitStale:
			return nil, request.RequestID, fmt.Errorf("%w (request_id=%s)", ErrMintStale, request.RequestID)
		case mintExitKey:
			return nil, request.RequestID, fmt.Errorf("%w (request_id=%s)", ErrMintNoAuthority, request.RequestID)
		case mintExitDuplicate:
			return nil, request.RequestID, fmt.Errorf("%w (request_id=%s)", ErrMintDuplicate, request.RequestID)
		default:
			return nil, request.RequestID, fmt.Errorf("mint envelope failed exit=%d request_id=%s: %w", exitCode, request.RequestID, runErr)
		}
	}
	raw, err := os.ReadFile(envelopePath)
	if err != nil {
		return nil, request.RequestID, fmt.Errorf("read minted envelope: %w", err)
	}
	var envelope capability.MutationEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, request.RequestID, fmt.Errorf("parse minted envelope: %w", err)
	}
	if envelope.Manifest.Hash() != manifest.Hash() {
		return nil, request.RequestID, fmt.Errorf("minted envelope manifest mismatch")
	}
	log.Printf("[SECURITY] [agent] [supplychain] envelope_mint_succeeded request_id=%s authorization_id=%s actions=%d",
		request.RequestID, envelope.Authorization.AuthorizationID, len(envelope.Manifest.ResolvedActions))
	return &envelope, request.RequestID, nil
}

// ExecuteEnvelope hands one signed envelope to the privileged verifier and
// returns its joined receipt, including denials and failed package execution.
func (e *Executor) ExecuteEnvelope(
	ctx context.Context,
	envelope *capability.MutationEnvelope,
) (*capability.MutationReceipt, error) {
	if envelope == nil {
		return nil, fmt.Errorf("mutation envelope is nil")
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal mutation envelope: %w", err)
	}
	agentDir := filepath.Join(constants.GetBaseDir(), constants.AgentDir)
	envelopeDir := filepath.Join(agentDir, mutationEnvelopeDir)
	receiptDir := filepath.Join(agentDir, mutationReceiptDir)
	if err := os.MkdirAll(envelopeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create mutation envelope dir: %w", err)
	}
	if err := os.MkdirAll(receiptDir, 0o700); err != nil {
		return nil, fmt.Errorf("create mutation receipt dir: %w", err)
	}
	filename, err := safeTokenFilename(envelope.Authorization.AuthorizationID)
	if err != nil {
		return nil, err
	}
	envelopePath := filepath.Join(envelopeDir, filename)
	receiptPath := filepath.Join(receiptDir, filename)
	if err := os.Remove(receiptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("clear stale mutation receipt: %w", err)
	}
	if err := os.WriteFile(envelopePath, payload, 0o600); err != nil {
		return nil, fmt.Errorf("write mutation envelope: %w", err)
	}
	defer os.Remove(envelopePath)
	defer os.Remove(receiptPath)

	exitCode, stderr, runErr := runMutationHelper(
		ctx, executorTimeout, e.BinaryPath,
		"execute-envelope", "--envelope-file", envelopePath, "--receipt-file", receiptPath,
	)
	if stderr != "" {
		log.Printf("[INFO] [agent] [supplychain] envelope_executor_log %s", stderr)
	}
	raw, err := os.ReadFile(receiptPath)
	if err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("helper failed without mutation receipt exit=%d: %w", exitCode, runErr)
		}
		return nil, fmt.Errorf("helper returned without mutation receipt: %w", err)
	}
	var receipt capability.MutationReceipt
	if err := json.Unmarshal(bytes.TrimSpace(raw), &receipt); err != nil {
		return nil, fmt.Errorf("parse mutation receipt: %w", err)
	}
	if receipt.AuthorizationID != envelope.Authorization.AuthorizationID ||
		receipt.OperationID != envelope.Manifest.OperationID ||
		receipt.ManifestHash != envelope.Manifest.Hash() ||
		receipt.TargetID != envelope.Manifest.TargetID ||
		receipt.ProtocolVersion != capability.MutationProtocolVersion ||
		receipt.Backend != envelope.Manifest.Backend ||
		receipt.Operation != envelope.Manifest.Operation {
		return nil, fmt.Errorf("mutation receipt audit join mismatch")
	}
	if receipt.VerifiedActions < 0 || receipt.VerifiedActions > len(envelope.Manifest.ResolvedActions) {
		return nil, fmt.Errorf("mutation receipt verified action count is outside the signed manifest")
	}
	if receipt.Executed != (receipt.Decision == "executed" && receipt.ExitCode == 0) {
		return nil, fmt.Errorf("mutation receipt outcome is internally inconsistent")
	}
	return &receipt, nil
}
