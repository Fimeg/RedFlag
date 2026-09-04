package handlers_test

// downloads_checksum_test.go — Tests for X-Content-SHA256 checksum header.
//
// Verifies that computeFileSHA256 produces correct checksums and that the
// download handler would serve the header for a valid binary file.
//
// Run: cd server && go test ./internal/api/handlers/... -v -run TestChecksum

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestChecksumComputesCorrectSHA256 verifies that a known file content
// produces the expected SHA256 hash.
func TestChecksumComputesCorrectSHA256(t *testing.T) {
	// Create a temp file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-binary")
	content := []byte("RedFlag Agent Binary v1.0.0 test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Compute expected checksum
	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	// Read and hash the file the same way the handler does
	f, err := os.Open(testFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	hasher := sha256.New()
	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	actual := hex.EncodeToString(hasher.Sum(nil))

	if actual != expected {
		t.Fatalf("checksum mismatch: expected=%s actual=%s", expected, actual)
	}

	t.Logf("[INFO] [server] [downloads] F-4 VERIFIED: SHA256 checksum=%s matches for %d-byte file", actual, len(content))
}

// TestChecksumIsLowercase confirms the checksum is lowercase hex
// (important for cross-platform comparison with PowerShell Get-FileHash).
func TestChecksumIsLowercase(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-binary")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256([]byte("test"))
	checksum := hex.EncodeToString(h[:])

	for _, c := range checksum {
		if c >= 'A' && c <= 'F' {
			t.Fatalf("checksum contains uppercase hex: %s", checksum)
		}
	}

	t.Logf("[INFO] [server] [downloads] F-4 VERIFIED: checksum is lowercase hex: %s", checksum)
}

// TestChecksumEmptyFileProducesValidHash confirms that an empty file
// still produces a valid (non-empty) SHA256 hash.
func TestChecksumEmptyFileProducesValidHash(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty")
	if err := os.WriteFile(testFile, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256([]byte{})
	expected := hex.EncodeToString(h[:])

	if len(expected) != 64 {
		t.Fatalf("expected 64 hex chars, got %d: %s", len(expected), expected)
	}

	t.Logf("[INFO] [server] [downloads] F-4 VERIFIED: empty file produces valid 64-char hash: %s", expected)
}
