package capability

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
)

// KeyIDFor returns the authority key fingerprint: hex(sha256(pubkey)[:16]).
// Matches SigningService.GetPublicKeyFingerprint and the executor's key_id_for.
// Split out of token.go so retiring Token does not strand the mutation protocol.
func KeyIDFor(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	return hex.EncodeToString(hash[:16])
}
