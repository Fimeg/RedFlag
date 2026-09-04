// Package security provides TOTP (RFC 6238) validation for fleet-join 2FA
// (SEC-025). The seed is stored AES-256-GCM encrypted on the registration
// token; the host generates the seed and the code. The server only validates.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	// TotpPeriod is the time step in seconds (RFC 6238 default).
	TotpPeriod = 30
	// totpDigits is the number of digits in the TOTP code.
	totpDigits = 6
	// totpSkew is the number of time steps to accept in each direction.
	// ±1 step = ±30 seconds, matching standard authenticator apps.
	totpSkew = 1
)

// GenerateTOTPSeed creates a new random TOTP seed (base32-encoded, 20 bytes).
// Called by the standalone host; the server never generates seeds.
func GenerateTOTPSeed() (string, error) {
	buf := make([]byte, 20) // 160 bits, standard for HMAC-SHA1
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("totp: seed generation failed: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

// ValidTOTPSeed reports whether seed is valid base32 (the encoding TOTP uses).
// Used at token-creation time to reject garbage seeds before they're encrypted
// and stored — a seed that can't be decoded can never produce a valid code.
func ValidTOTPSeed(seed string) bool {
	_, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(seed))
	return err == nil && len(seed) > 0
}

// ValidateTOTPCode checks whether the given 6-digit code is valid for the
// seed at the current time, with ±1 step tolerance.
func ValidateTOTPCode(seed string, code string) bool {
	return ValidateTOTPCodeAt(seed, code, time.Now())
}

// ValidateTOTPCodeAt checks the code at a specific time (for testing).
func ValidateTOTPCodeAt(seed string, code string, t time.Time) bool {
	if len(code) != totpDigits {
		return false
	}

	seedBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(seed))
	if err != nil {
		return false
	}

	counter := uint64(t.Unix()) / TotpPeriod

	// Check current time step and ±skew.
	for i := -totpSkew; i <= totpSkew; i++ {
		c := counter + uint64(i)
		if i < 0 && c > counter { // underflow guard
			continue
		}
		if totpAt(seedBytes, c) == code {
			return true
		}
	}
	return false
}

// totpAt generates the TOTP code for a given counter value.
func totpAt(key []byte, counter uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 §5.4).
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	// Format as zero-padded digit string.
	divisor := uint32(1)
	for i := 0; i < totpDigits; i++ {
		divisor *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, code%divisor)
}
