package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func TestResolveSetupSigningKeysGeneratesWhenMissing(t *testing.T) {
	keys, generated, err := resolveSetupSigningKeys(serverSetupRequest{})
	if err != nil {
		t.Fatalf("resolveSetupSigningKeys returned error: %v", err)
	}
	if !generated {
		t.Fatal("resolveSetupSigningKeys should report generated keys")
	}
	if got := len(keys["public_key"]); got != ed25519.PublicKeySize*2 {
		t.Fatalf("public key hex length = %d", got)
	}
	if got := len(keys["private_key"]); got != ed25519.PrivateKeySize*2 {
		t.Fatalf("private key hex length = %d", got)
	}
}

func TestResolveSetupSigningKeysUsesProvidedPair(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	keys, generated, err := resolveSetupSigningKeys(serverSetupRequest{
		SigningPrivateKey: hex.EncodeToString(privateKey),
		SigningPublicKey:  hex.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatalf("resolveSetupSigningKeys returned error: %v", err)
	}
	if generated {
		t.Fatal("resolveSetupSigningKeys should not report generated keys")
	}
	if keys["private_key"] != hex.EncodeToString(privateKey) {
		t.Fatal("private key was not preserved")
	}
	if keys["public_key"] != hex.EncodeToString(publicKey) {
		t.Fatal("public key was not preserved")
	}
}

func TestResolveSetupSigningKeysRejectsMismatchedPair(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey public: %v", err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey private: %v", err)
	}

	if _, _, err := resolveSetupSigningKeys(serverSetupRequest{
		SigningPrivateKey: hex.EncodeToString(privateKey),
		SigningPublicKey:  hex.EncodeToString(publicKey),
	}); err == nil {
		t.Fatal("resolveSetupSigningKeys accepted mismatched keys")
	}
}
