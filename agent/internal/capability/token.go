// Package capability defines the supply-chain capability token: an Ed25519-signed
// authorization for exactly one package operation over a fully-resolved dependency
// closure. The server (authority) mints and signs tokens; the agent passes them to
// the privileged Rust executor (helper/) which independently verifies them.
//
// The canonical signed message and closure hash MUST stay byte-identical across
// this package, the server's mirror of it, and helper/src/main.rs. See
// RAF/security/05-supply-chain-gate.md for the contract.
package capability

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// Version is the only token format this code understands. Forward-only doctrine:
// new versions add fields, never reinterpret existing ones.
const Version = 1

// ClosureEntry is one resolved artifact in the dependency closure.
type ClosureEntry struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	SHA256       string `json:"sha256"`
	Source       string `json:"source"`                  // "mirror" | "registry"
	ArtifactPath string `json:"artifact_path,omitempty"` // local path or url, optional
}

// Token is the full capability token exchanged between server, agent, and executor.
type Token struct {
	Version     int            `json:"version"`
	TokenID     string         `json:"token_id"`
	AgentID     string         `json:"agent_id"`
	KeyID       string         `json:"key_id"`
	PackageType string         `json:"package_type"` // apt|dnf|npm|bun|pip|docker|winget
	Operation   string         `json:"operation"`    // install|upgrade (forward-only)
	Closure     []ClosureEntry `json:"closure"`
	IssuedAt    int64          `json:"issued_at"`
	NotBefore   int64          `json:"not_before"`
	ExpiresAt   int64          `json:"expires_at"`
	Signature   string         `json:"signature"` // hex ed25519
}

// ClosureHash computes hex(sha256( "\n".join( sorted("{name}@{version}#{sha256}") ) )).
// Lines are sorted and de-duplicated so neither array order nor exact duplicates
// can change the digest. This mirrors the Rust BTreeSet construction exactly.
func (t *Token) ClosureHash() string {
	set := make(map[string]struct{}, len(t.Closure))
	for _, e := range t.Closure {
		set[fmt.Sprintf("%s@%s#%s", e.Name, e.Version, e.SHA256)] = struct{}{}
	}
	lines := make([]string, 0, len(set))
	for line := range set {
		lines = append(lines, line)
	}
	sort.Strings(lines)

	h := sha256.New()
	for i, line := range lines {
		if i > 0 {
			h.Write([]byte("\n"))
		}
		h.Write([]byte(line))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// CanonicalMessage builds the deterministic message that is signed/verified:
// "{agent_id}:{token_id}:{operation}:{package_type}:{closure_hash}:{expires_at}".
func (t *Token) CanonicalMessage() string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%d",
		t.AgentID, t.TokenID, t.Operation, t.PackageType, t.ClosureHash(), t.ExpiresAt)
}

// Sign signs the canonical message with the authority private key, sets the
// token's KeyID and Signature, and returns the hex signature.
func (t *Token) Sign(priv ed25519.PrivateKey) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("capability: invalid private key size %d", len(priv))
	}
	pub := priv.Public().(ed25519.PublicKey)
	t.KeyID = KeyIDFor(pub)
	sig := ed25519.Sign(priv, []byte(t.CanonicalMessage()))
	t.Signature = hex.EncodeToString(sig)
	return t.Signature, nil
}

// Verify checks the token's signature against the given public key. It does not
// check the validity window, agent binding, or artifact hashes — those are the
// executor's responsibility (and the agent's bind-check). It verifies only that
// this key signed this canonical message.
func (t *Token) Verify(pub ed25519.PublicKey) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("capability: invalid public key size %d", len(pub))
	}
	sig, err := hex.DecodeString(t.Signature)
	if err != nil {
		return fmt.Errorf("capability: signature not hex: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("capability: invalid signature size %d", len(sig))
	}
	if !ed25519.Verify(pub, []byte(t.CanonicalMessage()), sig) {
		return fmt.Errorf("capability: signature verification failed")
	}
	return nil
}
