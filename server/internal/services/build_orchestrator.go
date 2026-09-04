package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
)

// BuildOrchestratorService handles building and signing agent binaries
type BuildOrchestratorService struct {
	signingService *SigningService
	packageQueries *queries.PackageQueries
	agentDir       string // Directory containing pre-built binaries
}

// NewBuildOrchestratorService creates a new build orchestrator service
func NewBuildOrchestratorService(signingService *SigningService, packageQueries *queries.PackageQueries, agentDir string) *BuildOrchestratorService {
	return &BuildOrchestratorService{
		signingService: signingService,
		packageQueries: packageQueries,
		agentDir:       agentDir,
	}
}

// BuildAndSignAgent builds (or retrieves) and signs an agent binary.
//
// Idempotent across server restarts: if a signed package already exists for
// (version, platform, architecture) and the on-disk binary's checksum matches,
// the existing row is returned unchanged. Previously the server would re-sign
// and insert a fresh row every boot, accumulating duplicate packages and
// confusing the dashboard's "update available" list.
//
// Signing is required. If the signing service is disabled, returns an error.
// The pre-v0.2.1 unsigned fallback path has been removed — every binary served
// must carry an Ed25519 signature.
func (s *BuildOrchestratorService) BuildAndSignAgent(version, platform, architecture string) (*models.AgentUpdatePackage, error) {
	binaryName := "redflag-agent"
	if strings.HasPrefix(platform, "windows") {
		binaryName += ".exe"
	}

	binaryPath := filepath.Join(s.agentDir, "binaries", platform+"-"+architecture, binaryName)

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("binary not found for platform %s: %w", platform, err)
	}

	if !s.signingService.IsEnabled() {
		return nil, fmt.Errorf("cannot build agent: signing is disabled")
	}

	// Compute the on-disk checksum once so we can compare against any
	// existing row before paying the cost of a fresh sign.
	diskChecksum, checksumErr := s.signingService.ComputeFileChecksum(binaryPath)
	if checksumErr != nil {
		log.Printf("[WARNING] [server] [build_orchestrator] checksum_failed path=%s error=%v — falling through to fresh sign", binaryPath, checksumErr)
	} else if existing, getErr := s.packageQueries.GetSignedPackage(version, platform, architecture); getErr == nil && existing != nil && existing.Checksum == diskChecksum && existing.Signature != "" {
		log.Printf("[INFO] [server] [build_orchestrator] package_reused version=%s platform=%s arch=%s id=%s", version, platform, architecture, existing.ID)
		return existing, nil
	}

	signedPackage, err := s.signingService.SignFile(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to sign agent binary: %w", err)
	}

	signedPackage.Version = version
	signedPackage.Platform = platform
	signedPackage.Architecture = architecture

	err = s.packageQueries.StoreSignedPackage(signedPackage)
	if err != nil {
		return nil, fmt.Errorf("failed to store signed package: %w", err)
	}

	log.Printf("[INFO] [server] [build_orchestrator] package_signed id=%s version=%s platform=%s arch=%s", signedPackage.ID, version, platform, architecture)
	return signedPackage, nil
}

// SignExistingBinary signs an existing binary file
func (s *BuildOrchestratorService) SignExistingBinary(binaryPath, version, platform, architecture string) (*models.AgentUpdatePackage, error) {
	// Check if file exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("binary not found: %s", binaryPath)
	}

	// Sign the binary if signing is enabled
	if !s.signingService.IsEnabled() {
		return nil, fmt.Errorf("signing service is disabled")
	}

	signedPackage, err := s.signingService.SignFile(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to sign agent binary: %w", err)
	}

	// Set additional fields
	signedPackage.Version = version
	signedPackage.Platform = platform
	signedPackage.Architecture = architecture

	// Store signed package in database
	err = s.packageQueries.StoreSignedPackage(signedPackage)
	if err != nil {
		return nil, fmt.Errorf("failed to store signed package: %w", err)
	}

	log.Printf("Successfully signed and stored agent binary: %s (%s/%s)", signedPackage.ID, platform, architecture)
	return signedPackage, nil
}

// GetSignedPackage retrieves a signed package by version and platform
func (s *BuildOrchestratorService) GetSignedPackage(version, platform, architecture string) (*models.AgentUpdatePackage, error) {
	return s.packageQueries.GetSignedPackage(version, platform, architecture)
}

// ListSignedPackages lists all signed packages (with optional filters)
func (s *BuildOrchestratorService) ListSignedPackages(version, platform string, limit, offset int) ([]models.AgentUpdatePackage, error) {
	return s.packageQueries.ListUpdatePackages(version, platform, limit, offset)
}
