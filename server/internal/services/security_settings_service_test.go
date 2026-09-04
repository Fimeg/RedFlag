package services

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// newTestSettingsService builds a service with a real random AES-256 key and no
// DB (serializeForStorage / encrypt / decrypt don't touch the store).
func newTestSettingsService(t *testing.T) *SecuritySettingsService {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand key: %v", err)
	}
	svc, err := NewSecuritySettingsService(nil, nil, base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

// SEC-024: a non-sensitive setting stores plain JSON exactly as before.
func TestSerializeForStorageNonSensitivePlaintext(t *testing.T) {
	svc := newTestSettingsService(t)
	stored, encrypted, err := svc.serializeForStorage("nonce_validation", "timeout_seconds", 600)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if encrypted {
		t.Fatalf("non-sensitive setting must not be marked encrypted")
	}
	if stored != "600" {
		t.Fatalf("non-sensitive setting should store plain JSON, got %q", stored)
	}
}

// SEC-024: a sensitive setting must persist as ciphertext (never plaintext) and
// must round-trip through the same decrypt-then-unmarshal path GetSetting uses.
func TestSerializeForStorageSensitiveEncryptsAndRoundTrips(t *testing.T) {
	svc := newTestSettingsService(t)
	secret := "super-secret-ed25519-private-key"

	stored, encrypted, err := svc.serializeForStorage("command_signing", "private_key", secret)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if !encrypted {
		t.Fatalf("sensitive setting must be marked encrypted")
	}
	if strings.Contains(stored, secret) {
		t.Fatalf("sensitive value persisted in plaintext: %q", stored)
	}

	// GetSetting read path: decrypt then json-unmarshal must recover the original.
	decrypted, err := svc.decrypt(stored)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	var got string
	if err := json.Unmarshal([]byte(decrypted), &got); err != nil {
		t.Fatalf("unmarshal decrypted value: %v", err)
	}
	if got != secret {
		t.Fatalf("round-trip mismatch: got %q want %q", got, secret)
	}
}

func TestInertCommandSigningServerSettingsAreRetired(t *testing.T) {
	svc := newTestSettingsService(t)

	for _, key := range []string{"enabled", "enforcement_mode", "algorithm"} {
		t.Run(key, func(t *testing.T) {
			if _, exists := svc.getDefaultSettings()["command_signing"][key]; exists {
				t.Fatalf("retired server setting %s must not be present in defaults", key)
			}

			if _, err := svc.GetSetting("command_signing", key); err == nil || !strings.Contains(err.Error(), "was removed") {
				t.Fatalf("retired setting read should fail explicitly, got %v", err)
			}

			if err := svc.ValidateSetting("command_signing", key, "disabled"); err == nil || !strings.Contains(err.Error(), "agent-local") {
				t.Fatalf("retired setting write should fail explicitly, got %v", err)
			}
		})
	}
}
