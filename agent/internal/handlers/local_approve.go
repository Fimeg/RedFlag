package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/capability"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/installer"
	"github.com/Fimeg/RedFlag/agent/internal/supplychain"
	"github.com/gofrs/uuid/v5"
)

// Standalone local approval (FEAT-003). The full gate pipeline runs locally:
// dry-run closure resolve → hash pin → OSV best-effort → mint request → the
// privileged helper signs with the root-owned authority → the normal execute
// path verifies and installs. Fleet mode refuses: the server is the sole
// authority there, and this path never weakens that.

// Typed failures the local API maps to honest HTTP codes.
var (
	// ErrApprovalFleetMode: this host is registered to a fleet server.
	ErrApprovalFleetMode = errors.New("local approval refused: fleet mode — approve via the server")
	// ErrApprovalBlocked: a gate refused and no override reason was supplied.
	ErrApprovalBlocked = errors.New("approval blocked by supply-chain gate")
	// ErrApprovalNoStandaloneIdentity: local authority was not provisioned.
	ErrApprovalNoStandaloneIdentity = errors.New("local approval refused: standalone identity is not provisioned")
)

// LocalApproveRequest is RedFlag Desktop's approval submission.
type LocalApproveRequest struct {
	PackageType      string `json:"package_type"`
	PackageName      string `json:"package_name"`
	AvailableVersion string `json:"available_version"`
	Operator         string `json:"operator"`
	OverrideReason   string `json:"override_reason"`
}

// LocalApproveResult carries the gate verdicts plus the execution outcome so
// Desktop can render exactly what was checked and what happened.
type LocalApproveResult struct {
	RequestID    string                      `json:"request_id"`
	OSVStatus    string                      `json:"osv_status"`
	OSVVulnCount int                         `json:"osv_vuln_count"`
	ClosureSize  int                         `json:"closure_size"`
	Policy       *supplychain.PolicyResult   `json:"policy,omitempty"`
	Receipt      *capability.MutationReceipt `json:"receipt,omitempty"`
}

// gatedLocalApproval limits local approval to the ecosystems whose closures
// the agent can resolve and pin from signed repo metadata. Mirrors the
// capability-gate set, not the full installer set.
func gatedLocalApproval(packageType string) bool {
	return packageType == "dnf" || packageType == "apt" || packageType == "pacman"
}

type pacmanExecutionLocation struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type pacmanActionPayload struct {
	ArtifactSHA256    string                  `json:"artifact_sha256"`
	ExecutionLocation pacmanExecutionLocation `json:"execution_location"`
	Repository        string                  `json:"repository"`
	Requested         bool                    `json:"requested"`
	SignatureSHA256   string                  `json:"signature_sha256"`
	SignatureLocation pacmanExecutionLocation `json:"signature_location"`
}

func handleLocalPacmanApproval(
	ctx context.Context,
	cfg *config.Config,
	req LocalApproveRequest,
) (*LocalApproveResult, error) {
	// OSV has no Arch ecosystem mapping in RedFlag. Preserve that as an
	// unsupported gate, which requires the same recorded operator override as
	// an outage. Refuse before network and artifact work when intent is absent.
	osvStatus := supplychain.OSVStatusUnsupported
	if req.OverrideReason == "" {
		return nil, fmt.Errorf("%w: osv_status=%s — override requires an explicit reason",
			ErrApprovalBlocked, osvStatus)
	}
	resolution, err := installer.ResolvePacmanClosure(req.PackageName, req.AvailableVersion)
	if err != nil {
		return nil, fmt.Errorf("resolve pacman closure: %w", err)
	}
	defer resolution.Cleanup()
	resolvedAt := time.Now().UTC()

	actions := make([]capability.ResolvedAction, 0, len(resolution.Artifacts))
	evidence := make([]capability.Evidence, 0, len(resolution.Artifacts)*2)
	for _, artifact := range resolution.Artifacts {
		payload, err := json.Marshal(pacmanActionPayload{
			ArtifactSHA256:    artifact.ArchiveSHA256,
			ExecutionLocation: pacmanExecutionLocation{Kind: "cache", Value: artifact.ArchivePath},
			Repository:        artifact.Repository,
			Requested:         artifact.Name == req.PackageName && (req.AvailableVersion == "" || artifact.Version == req.AvailableVersion),
			SignatureSHA256:   artifact.SignatureSHA256,
			SignatureLocation: pacmanExecutionLocation{Kind: "cache", Value: artifact.SignaturePath},
		})
		if err != nil {
			return nil, fmt.Errorf("encode pacman action %s: %w", artifact.Name, err)
		}
		actions = append(actions, capability.ResolvedAction{
			Kind: "package", Identity: artifact.Name + "@" + artifact.Version, Payload: string(payload),
		})
		evidence = append(evidence,
			capability.Evidence{Kind: "pacman-package-archive", Digest: artifact.ArchiveSHA256},
			capability.Evidence{Kind: "pacman-package-signature", Digest: artifact.SignatureSHA256},
		)
	}

	operationID, err := uuid.NewV4()
	if err != nil {
		return nil, fmt.Errorf("generate pacman operation id: %w", err)
	}
	manifest := capability.MutationManifest{
		ProtocolVersion: capability.MutationProtocolVersion,
		OperationID:     operationID.String(),
		TargetID:        cfg.AgentID.String(),
		Backend:         "pacman",
		Operation:       "upgrade",
		ResolvedActions: actions,
		Evidence:        evidence,
	}
	gateEvidence := supplychain.GateEvidence{
		ResolvedAt:     resolvedAt.Unix(),
		OSVCheckedAt:   0,
		OSVStatus:      osvStatus,
		OSVVulnCount:   0,
		AgeGate:        "not_applicable",
		SoakGate:       "not_applicable",
		Operator:       req.Operator,
		OverrideReason: req.OverrideReason,
	}
	executor := supplychain.NewExecutor("")
	envelope, requestID, err := executor.MintEnvelope(ctx, manifest, gateEvidence)
	if err != nil {
		return nil, err
	}
	receipt, err := executor.ExecuteEnvelope(ctx, envelope)
	if err != nil {
		return nil, fmt.Errorf("execute pacman envelope authorization_id=%s: %w",
			envelope.Authorization.AuthorizationID, err)
	}
	log.Printf("[INFO] [agent] [localapprove] approval_completed pkg=%s decision=%s exit=%d authorization_id=%s",
		req.PackageName, receipt.Decision, receipt.ExitCode, receipt.AuthorizationID)
	return &LocalApproveResult{
		RequestID:    requestID,
		OSVStatus:    osvStatus,
		OSVVulnCount: 0,
		ClosureSize:  len(resolution.Artifacts),
		Receipt:      receipt,
	}, nil
}

// HandleLocalApprove runs the standalone approval flow end to end. Synchronous:
// the caller holds the local socket connection until the helper's verdict (and,
// on success, the install) completes.
func HandleLocalApprove(ctx context.Context, cfg *config.Config, req LocalApproveRequest) (*LocalApproveResult, error) {
	if cfg.IsRegistered() {
		return nil, ErrApprovalFleetMode
	}
	if !cfg.IsStandalone() {
		return nil, ErrApprovalNoStandaloneIdentity
	}
	if req.PackageType == "" || req.PackageName == "" {
		return nil, fmt.Errorf("package_type and package_name are required")
	}
	if !gatedLocalApproval(req.PackageType) {
		return nil, fmt.Errorf("local approval not supported for package_type=%s (dnf|apt|pacman only)", req.PackageType)
	}
	if req.Operator == "" {
		return nil, fmt.Errorf("operator is required")
	}

	log.Printf("[INFO] [agent] [localapprove] approval_started pkg=%s type=%s version=%s operator=%s",
		req.PackageName, req.PackageType, req.AvailableVersion, req.Operator)
	if req.PackageType == "pacman" {
		return handleLocalPacmanApproval(ctx, cfg, req)
	}

	// Resolve the closure exactly as the fleet dry-run path does.
	inst, err := installer.InstallerFactory(req.PackageType, cfg.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] factory_failed type=%s error=%w", req.PackageType, err)
	}
	if !inst.IsAvailable() {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] not_available type=%s", req.PackageType)
	}
	dryRun, err := inst.DryRun(req.PackageName, req.AvailableVersion)
	if err != nil {
		return nil, fmt.Errorf("dry run failed: %w", err)
	}
	resolvedAt := time.Now().UTC()

	closureItems := resolveClosureHashes(req.PackageType, req.PackageName, req.AvailableVersion, dryRun.Dependencies)
	if len(closureItems) == 0 {
		// No pin, no mint. The execute path could not verify anything.
		return nil, fmt.Errorf("closure hash resolution failed for %s — refusing unpinned approval", req.PackageName)
	}

	closure := make([]capability.ClosureEntry, len(closureItems))
	pkgs := make([]supplychain.PkgVersion, len(closureItems))
	for i, c := range closureItems {
		closure[i] = capability.ClosureEntry{
			Name:    c.Name,
			Version: c.Version,
			SHA256:  c.SHA256,
			Source:  c.Source,
		}
		pkgs[i] = supplychain.PkgVersion{Name: c.Name, Version: c.Version}
	}

	// OSV best-effort with honest verdict: vulnerable or unreachable proceeds
	// only over an explicit operator reason, refused here before the helper is
	// ever invoked (the helper re-enforces — defense in depth).
	osvStatus, vulnCount := supplychain.CheckClosureOSV(ctx, req.PackageType, pkgs)
	osvCheckedAt := time.Now().UTC()
	if osvStatus != supplychain.OSVStatusClear && req.OverrideReason == "" {
		log.Printf("[SECURITY] [agent] [localapprove] approval_blocked pkg=%s osv_status=%s vulns=%d",
			req.PackageName, osvStatus, vulnCount)
		return nil, fmt.Errorf("%w: osv_status=%s vulns=%d — override requires an explicit reason",
			ErrApprovalBlocked, osvStatus, vulnCount)
	}

	mintReq := &supplychain.MintRequest{
		AgentID:     cfg.AgentID.String(),
		PackageType: req.PackageType,
		Operation:   "install",
		Closure:     closure,
		GateEvidence: supplychain.GateEvidence{
			ResolvedAt:   resolvedAt.Unix(),
			OSVCheckedAt: osvCheckedAt.Unix(),
			OSVStatus:    osvStatus,
			OSVVulnCount: vulnCount,
			// Standalone has no registry age data for dnf/apt (matches the
			// fleet age gate's ecosystem scope) and no local version
			// first-seen tracking yet — journaled honestly as not_applicable.
			AgeGate:        "not_applicable",
			SoakGate:       "not_applicable",
			Operator:       req.Operator,
			OverrideReason: req.OverrideReason,
		},
	}

	executor := supplychain.NewExecutor("")
	token, err := executor.Mint(ctx, mintReq)
	if err != nil {
		return nil, err
	}

	policy, err := executor.Execute(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("execute after mint failed (token_id=%s): %w", token.TokenID, err)
	}

	log.Printf("[INFO] [agent] [localapprove] approval_completed pkg=%s decision=%s exit=%d",
		req.PackageName, policy.Decision, policy.ExitCode)
	return &LocalApproveResult{
		RequestID:    mintReq.RequestID,
		OSVStatus:    osvStatus,
		OSVVulnCount: vulnCount,
		ClosureSize:  len(closure),
		Policy:       policy,
	}, nil
}
