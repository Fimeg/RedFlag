package services

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type UpdateNonce struct {
	AgentID       string `json:"agent_id"`
	TargetVersion string `json:"target_version"`
	Timestamp     int64  `json:"timestamp"`
	Signature     string `json:"signature"`
}

type UpdateNonceService struct {
	privateKey ed25519.PrivateKey
	maxAge     time.Duration
}

func NewUpdateNonceService(privateKey ed25519.PrivateKey) *UpdateNonceService {
	return &UpdateNonceService{
		privateKey: privateKey,
		maxAge:     10 * time.Minute,
	}
}

// Generate creates a signed nonce authorizing an agent to update
func (s *UpdateNonceService) Generate(agentID, targetVersion string) (string, error) {
	nonce := UpdateNonce{
		AgentID:       agentID,
		TargetVersion: targetVersion,
		Timestamp:     time.Now().UTC().Unix(),
	}

	data, err := json.Marshal(nonce)
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	signature := ed25519.Sign(s.privateKey, data)
	nonce.Signature = base64.StdEncoding.EncodeToString(signature)

	encoded, err := json.Marshal(nonce)
	if err != nil {
		return "", fmt.Errorf("encode failed: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encoded), nil
}

// Validate verifies the nonce signature and freshness
func (s *UpdateNonceService) Validate(encodedNonce string) (*UpdateNonce, error) {
	data, err := base64.StdEncoding.DecodeString(encodedNonce)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}

	var nonce UpdateNonce
	if err := json.Unmarshal(data, &nonce); err != nil {
		return nil, fmt.Errorf("invalid format: %w", err)
	}

	// Check freshness
	if time.Now().UTC().Unix()-nonce.Timestamp > int64(s.maxAge.Seconds()) {
		return nil, fmt.Errorf("nonce expired")
	}

	// Verify signature
	signature, err := base64.StdEncoding.DecodeString(nonce.Signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}

	// Remove signature for verification
	nonce.Signature = ""
	verifyData, err := json.Marshal(nonce)
	if err != nil {
		return nil, fmt.Errorf("marshal verify data: %w", err)
	}

	if !ed25519.Verify(s.privateKey.Public().(ed25519.PublicKey), verifyData, signature) {
		return nil, fmt.Errorf("signature verification failed")
	}

	// Return validated nonce
	return &nonce, nil
}
