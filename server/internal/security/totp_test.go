package security

import (
	"encoding/base32"
	"testing"
	"time"
)

func TestGenerateTOTPSeed(t *testing.T) {
	seed, err := GenerateTOTPSeed()
	if err != nil {
		t.Fatalf("GenerateTOTPSeed failed: %v", err)
	}
	if len(seed) == 0 {
		t.Fatal("seed is empty")
	}
	// Two seeds should differ.
	seed2, err := GenerateTOTPSeed()
	if err != nil {
		t.Fatalf("GenerateTOTPSeed (2) failed: %v", err)
	}
	if seed == seed2 {
		t.Fatal("two generated seeds are identical")
	}
}

func TestTOTPValidation(t *testing.T) {
	seed := "JBSWY3DPEHPK3PXP"

	// Generate a valid code for a fixed time and verify it passes.
	fixed := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	code := CodeAt(seed, fixed)
	if len(code) != 6 {
		t.Fatalf("expected 6-digit code, got %q", code)
	}

	if !ValidateTOTPCodeAt(seed, code, fixed) {
		t.Fatalf("code %q rejected at exact time", code)
	}

	// ±30 seconds should also pass.
	if !ValidateTOTPCodeAt(seed, code, fixed.Add(29*time.Second)) {
		t.Fatal("code rejected at +29s")
	}
	if !ValidateTOTPCodeAt(seed, code, fixed.Add(-29*time.Second)) {
		t.Fatal("code rejected at -29s")
	}

	// ±61 seconds should fail (outside ±1 step window).
	if ValidateTOTPCodeAt(seed, code, fixed.Add(61*time.Second)) {
		t.Fatal("code accepted at +61s — should be outside tolerance")
	}
	if ValidateTOTPCodeAt(seed, code, fixed.Add(-61*time.Second)) {
		t.Fatal("code accepted at -61s — should be outside tolerance")
	}
}

func TestTOTPInvalidCode(t *testing.T) {
	seed := "JBSWY3DPEHPK3PXP"
	fixed := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

	if ValidateTOTPCodeAt(seed, "000000", fixed) {
		t.Fatal("all-zero code accepted")
	}
	if ValidateTOTPCodeAt(seed, "12345", fixed) {
		t.Fatal("5-digit code accepted")
	}
	if ValidateTOTPCodeAt(seed, "1234567", fixed) {
		t.Fatal("7-digit code accepted")
	}
}

// CodeAt generates the TOTP code at a specific time (exposed for testing).
func CodeAt(seed string, t time.Time) string {
	seedBytes, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(seed)
	counter := uint64(t.Unix()) / TotpPeriod
	return totpAt(seedBytes, counter)
}
