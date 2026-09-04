// Package crypto is the single AES-256-GCM at-rest primitive for the RedFlag
// server. It exists to collapse the duplicated, divergent encrypt/decrypt copies
// that previously lived in services (one of which prepended the nonce twice and
// could not round-trip). There is one encrypt path and one decrypt path here;
// callers supply a raw 32-byte key and own where that key comes from.
//
// Wire format: nonce || ciphertext-with-tag (the nonce is prepended once). This
// matches the format already written by the security settings service, so values
// it produced remain decryptable through this helper.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// KeySize is the required key length for AES-256.
const KeySize = 32

// Encrypt seals plaintext with AES-256-GCM under key (which must be 32 bytes).
// The returned slice is nonce || ciphertext; store or transmit it as-is.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// Seal appends the ciphertext to nonce, yielding nonce||ciphertext in one
	// allocation. The nonce is prefixed exactly once.
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt reverses Encrypt. data must be nonce || ciphertext as produced by
// Encrypt. A failed open (wrong key, truncation, tampering) returns an error;
// it never returns a partial or guessed plaintext.
func Decrypt(key, data []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short: %d bytes", len(data))
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid key size: expected %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
