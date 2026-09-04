//go:build ignore
// +build ignore

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

func main() {
	// Generate Ed25519 keypair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Printf("Failed to generate keypair: %v\n", err)
		os.Exit(1)
	}

	// Output keys in hex format
	fmt.Printf("Ed25519 Keypair Generated:\n\n")
	fmt.Printf("Private Key (keep secret, add to server env):\n")
	fmt.Printf("REDFLAG_SIGNING_PRIVATE_KEY=%s\n", hex.EncodeToString(privateKey))

	fmt.Printf("\nPublic Key (embed in agent binaries):\n")
	fmt.Printf("REDFLAG_PUBLIC_KEY=%s\n", hex.EncodeToString(publicKey))

	fmt.Printf("\nPublic Key Fingerprint (for database):\n")
	fmt.Printf("Fingerprint: %s\n", hex.EncodeToString(publicKey[:8])) // First 8 bytes as fingerprint

	fmt.Printf("\nAdd the private key to your server environment and embed the public key in agent builds.\n")
}