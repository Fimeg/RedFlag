package services

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/capability"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// SigningService handles Ed25519 cryptographic operations
type SigningService struct {
	privateKey        ed25519.PrivateKey
	publicKey         ed25519.PublicKey
	enabled           bool
	signingKeyQueries *queries.SigningKeyQueries
}

// NewSigningService creates a new signing service with the provided private key
func NewSigningService(privateKeyHex string) (*SigningService, error) {
	// Check if private key is provided
	if privateKeyHex == "" {
		return &SigningService{
			enabled: false,
		}, nil
	}

	// Decode private key from hex
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key format: %w", err)
	}

	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size: expected %d bytes, got %d", ed25519.PrivateKeySize, len(privateKeyBytes))
	}

	// Ed25519 private key format: first 32 bytes are seed, next 32 bytes are public key
	privateKey := ed25519.PrivateKey(privateKeyBytes)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	return &SigningService{
		privateKey: privateKey,
		publicKey:  publicKey,
		enabled:    true,
	}, nil
}

// SetSigningKeyQueries sets the database queries handle for key registration
func (s *SigningService) SetSigningKeyQueries(q *queries.SigningKeyQueries) {
	s.signingKeyQueries = q
}

// IsEnabled returns true if the signing service is enabled
func (s *SigningService) IsEnabled() bool {
	return s.enabled
}

// GetPublicKey returns the public key in hex format
func (s *SigningService) GetPublicKey() string {
	if !s.enabled {
		return ""
	}
	return hex.EncodeToString(s.publicKey)
}

// GetPublicKeyHex returns the full hex-encoded public key (alias for GetPublicKey, clearer name)
func (s *SigningService) GetPublicKeyHex() string {
	return s.GetPublicKey()
}

// GetPublicKeyFingerprint returns a fingerprint of the public key.
// Uses SHA-256 of the full public key, truncated to 16 bytes (32 hex characters).
func (s *SigningService) GetPublicKeyFingerprint() string {
	if !s.enabled {
		return ""
	}
	hash := sha256.Sum256(s.publicKey)
	return hex.EncodeToString(hash[:16]) // 16 bytes = 32 hex chars
}

// GetCurrentKeyID returns the fingerprint of the current signing key.
// Equivalent to GetPublicKeyFingerprint() but named for clarity in key-rotation contexts.
func (s *SigningService) GetCurrentKeyID() string {
	return s.GetPublicKeyFingerprint()
}

// InitializePrimaryKey registers the current signing key in the database and marks it as primary.
// If signingKeyQueries is not set, this is a no-op (returns nil).
// The version number is determined by querying MAX(version) + 1 from the signing_keys table,
// so each new unique key gets a monotonically increasing version.
// Note: This query is not wrapped in a transaction; for a single-instance server this is safe.
// A future improvement could use SELECT ... FOR UPDATE within a transaction.
func (s *SigningService) InitializePrimaryKey(ctx context.Context) error {
	if s.signingKeyQueries == nil {
		return nil
	}
	if !s.enabled {
		return fmt.Errorf("signing service is not enabled")
	}

	keyID := s.GetCurrentKeyID()
	publicKeyHex := s.GetPublicKeyHex()

	// Query the next version number from the database
	nextVersion, err := s.signingKeyQueries.GetNextVersion(ctx)
	if err != nil {
		// Non-fatal: fall back to version 1 if the query fails
		nextVersion = 1
	}

	// Insert the key (ON CONFLICT DO NOTHING — safe to call on every startup)
	if err := s.signingKeyQueries.InsertSigningKey(ctx, keyID, publicKeyHex, nextVersion); err != nil {
		return fmt.Errorf("failed to insert signing key: %w", err)
	}

	// Set this key as primary
	if err := s.signingKeyQueries.SetPrimaryKey(ctx, keyID); err != nil {
		return fmt.Errorf("failed to set primary key: %w", err)
	}

	return nil
}

// GetAllActivePublicKeys returns all currently active signing keys.
// If signingKeyQueries is set, fetches from database.
// Otherwise, returns a single-entry slice containing only the current key.
func (s *SigningService) GetAllActivePublicKeys(ctx context.Context) ([]models.SigningKey, error) {
	if !s.enabled {
		return nil, fmt.Errorf("signing service is not enabled")
	}

	if s.signingKeyQueries != nil {
		keys, err := s.signingKeyQueries.GetActiveSigningKeys(ctx)
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			return keys, nil
		}
		// Fall through to return current key if DB is empty
	}

	// No DB or no results: return current key as single-entry slice
	return []models.SigningKey{
		{
			ID:        uuid.Nil,
			KeyID:     s.GetCurrentKeyID(),
			PublicKey: s.GetPublicKeyHex(),
			Algorithm: "ed25519",
			IsActive:  true,
			IsPrimary: true,
			CreatedAt: time.Now().UTC(),
			Version:   1,
		},
	}, nil
}

// ComputeFileChecksum returns the SHA-256 hex digest of the file at the given
// path without signing it. Used by the build orchestrator to decide whether
// an already-stored signed package is still valid for the on-disk binary
// (so we can skip a redundant re-sign on every server boot).
func (s *SigningService) ComputeFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

// SignFile signs a file and returns the signature and checksum
func (s *SigningService) SignFile(filePath string) (*models.AgentUpdatePackage, error) {
	// Check if signing is enabled
	if !s.enabled {
		return nil, fmt.Errorf("signing service is disabled")
	}
	// Read the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Calculate checksum and sign content
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Calculate SHA-256 checksum
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	// Sign the content
	signature := ed25519.Sign(s.privateKey, content)

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Determine platform and architecture from file path or use runtime defaults
	platform, architecture := s.detectPlatformArchitecture(filePath)

	pkg := &models.AgentUpdatePackage{
		ID:           uuid.Must(uuid.NewV4()),
		BinaryPath:   filePath,
		Signature:    hex.EncodeToString(signature),
		Checksum:     checksum,
		FileSize:     fileInfo.Size(),
		Platform:     platform,
		Architecture: architecture,
		CreatedBy:    "signing-service",
		IsActive:     true,
	}

	return pkg, nil
}

// SignBytes signs arbitrary content with the server's Ed25519 key and returns a
// hex signature. Used for the release manifest (the cold-start trust root the
// installer verifies before executing a freshly-downloaded binary). The
// counterpart verifier is VerifySignature.
func (s *SigningService) SignBytes(content []byte) (string, error) {
	if !s.enabled || s.privateKey == nil {
		return "", fmt.Errorf("signing service is disabled")
	}
	return hex.EncodeToString(ed25519.Sign(s.privateKey, content)), nil
}

// VerifySignature verifies a file signature using the embedded public key
func (s *SigningService) VerifySignature(content []byte, signatureHex string) (bool, error) {
	// Decode signature
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("invalid signature format: %w", err)
	}

	if len(signature) != ed25519.SignatureSize {
		return false, fmt.Errorf("invalid signature size: expected %d bytes, got %d", ed25519.SignatureSize, len(signature))
	}

	// Verify signature
	valid := ed25519.Verify(s.publicKey, content, signature)
	return valid, nil
}

// VerifyFileIntegrity verifies a file's checksum
func (s *SigningService) VerifyFileIntegrity(filePath, expectedChecksum string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	hash := sha256.Sum256(content)
	actualChecksum := hex.EncodeToString(hash[:])

	return actualChecksum == expectedChecksum, nil
}

// detectPlatformArchitecture attempts to detect platform and architecture from file path
func (s *SigningService) detectPlatformArchitecture(filePath string) (string, string) {
	// Default to current runtime
	platform := runtime.GOOS
	arch := runtime.GOARCH

	// Map architectures
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
		"386":   "386",
	}

	// Try to detect from filename patterns
	if contains(filePath, "windows") || contains(filePath, ".exe") {
		platform = "windows"
	} else if contains(filePath, "linux") {
		platform = "linux"
	} else if contains(filePath, "darwin") || contains(filePath, "macos") {
		platform = "darwin"
	}

	for archName, archValue := range archMap {
		if contains(filePath, archName) {
			arch = archValue
			break
		}
	}

	// Normalize architecture names
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "arm64" {
		arch = "arm64"
	}

	return platform, arch
}

// contains is a simple helper for case-insensitive substring checking
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				findSubstring(s, substr))))
}

// findSubstring is a simple substring finder
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SignNonce signs a nonce (UUID + timestamp) for replay protection
func (s *SigningService) SignNonce(nonceUUID uuid.UUID, timestamp time.Time) (string, error) {
	// Create nonce data: UUID + Unix timestamp as string
	nonceData := fmt.Sprintf("%s:%d", nonceUUID.String(), timestamp.Unix())

	// Sign the nonce data
	signature := ed25519.Sign(s.privateKey, []byte(nonceData))

	// Return hex-encoded signature
	return hex.EncodeToString(signature), nil
}

// VerifyNonce verifies a nonce signature and checks freshness
func (s *SigningService) VerifyNonce(nonceUUID uuid.UUID, timestamp time.Time, signatureHex string, maxAge time.Duration) (bool, error) {
	// Check nonce freshness first
	if time.Since(timestamp) > maxAge {
		return false, fmt.Errorf("nonce is too old: %v > %v", time.Since(timestamp), maxAge)
	}

	// Recreate nonce data
	nonceData := fmt.Sprintf("%s:%d", nonceUUID.String(), timestamp.Unix())

	// Verify signature
	valid, err := s.VerifySignature([]byte(nonceData), signatureHex)
	if err != nil {
		return false, fmt.Errorf("failed to verify nonce signature: %w", err)
	}

	return valid, nil
}

// SignCommand creates an Ed25519 signature for a command.
// Signs the v3 message format: "{agent_id}:{id}:{command_type}:{sha256(params)}:{unix_timestamp}"
// Also sets cmd.SignedAt and cmd.KeyID as a side-effect.
// The agent_id binding (F-1 fix) prevents cross-agent command replay.
func (s *SigningService) SignCommand(cmd *models.AgentCommand) (string, error) {
	if s.privateKey == nil {
		return "", fmt.Errorf("signing service not initialized with private key")
	}

	// Record the signing time and key identity
	now := time.Now().UTC()
	cmd.SignedAt = &now
	cmd.KeyID = s.GetCurrentKeyID()

	// Serialize command parameters for signing
	paramsJSON, _ := json.Marshal(cmd.Params)
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])

	// v3 format: "{agent_id}:{id}:{command_type}:{params_hash}:{unix_timestamp}"
	// agent_id binding prevents cross-agent replay (F-1 fix)
	message := fmt.Sprintf("%s:%s:%s:%s:%d",
		cmd.AgentID.String(),
		cmd.ID.String(),
		cmd.CommandType,
		paramsHashHex,
		now.Unix())

	// Sign with Ed25519
	signature := ed25519.Sign(s.privateKey, []byte(message))
	return hex.EncodeToString(signature), nil
}

// SignCapabilityToken signs a supply-chain capability token in place, setting its
// KeyID and Signature over the canonical message. The private key never leaves the
// signing service. The canonical encoding lives in the capability package and is
// byte-identical to the agent mirror and the Rust executor.
func (s *SigningService) SignCapabilityToken(t *capability.Token) error {
	if !s.enabled || s.privateKey == nil {
		return fmt.Errorf("signing service not initialized with private key")
	}
	_, err := t.Sign(s.privateKey)
	return err
}

// Key rotation is now implemented via the signing_keys table (migration 020).
// Use InitializePrimaryKey() at startup to register the active key.
// Use GetAllActivePublicKeys() to enumerate all active keys for agents during rotation.
// The signing_keys table supports multiple concurrent active keys to allow
// graceful transition windows when rotating to a new key pair.
