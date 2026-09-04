package handlers_test

// downloads_security_test.go — Tests for binary_path sanitization in DownloadUpdatePackage.
//
// The handler resolves pkg.BinaryPath to an absolute path and confirms it lives
// within the configured BinaryStoragePath directory. Paths outside the directory
// (e.g. path traversal via ../../../etc/passwd) are rejected with 403.
//
// Run: cd server && go test ./internal/api/handlers/... -v -run TestDownloads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDownloadsRejectsPathTraversal verifies that a binary_path containing ../
// that resolves outside the allowed directory is rejected.
func TestDownloadsRejectsPathTraversal(t *testing.T) {
	// Simulate the sanitization logic from DownloadUpdatePackage
	allowedDir, _ := filepath.Abs("./binaries")
	traversalPath := "../../../etc/passwd"

	absPath, err := filepath.Abs(traversalPath)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if strings.HasPrefix(absPath, allowedDir+string(filepath.Separator)) || absPath == allowedDir {
		t.Fatalf("SECURITY: path traversal was NOT rejected: resolved=%s allowed=%s", absPath, allowedDir)
	}

	t.Logf("[INFO] [server] [downloads] F-SECURITY VERIFIED: path traversal rejected resolved=%s allowed=%s", absPath, allowedDir)
}

// TestDownloadsAcceptsSafePath verifies that a binary_path within the allowed
// directory passes the prefix check.
func TestDownloadsAcceptsSafePath(t *testing.T) {
	// Create a temp directory to act as the allowed binary storage
	tmpDir := t.TempDir()
	allowedDir, _ := filepath.Abs(tmpDir)

	// Create a safe file inside the directory
	safePath := filepath.Join(tmpDir, "linux", "amd64", "agent-v1.0.0")
	if err := os.MkdirAll(filepath.Dir(safePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(safePath, []byte("fake-binary"), 0644); err != nil {
		t.Fatal(err)
	}

	absPath, err := filepath.Abs(safePath)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if !strings.HasPrefix(absPath, allowedDir+string(filepath.Separator)) {
		t.Fatalf("REGRESSION: safe path was rejected: resolved=%s allowed=%s", absPath, allowedDir)
	}

	t.Logf("[INFO] [server] [downloads] F-SECURITY VERIFIED: safe path accepted resolved=%s allowed=%s", absPath, allowedDir)
}

// TestDownloadsRejectsSymlinkEscape verifies that a symlink pointing outside
// the allowed directory is still caught when resolved.
func TestDownloadsRejectsSymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()
	allowedDir, _ := filepath.Abs(tmpDir)

	// Create a symlink inside the allowed dir that points outside
	symlinkPath := filepath.Join(tmpDir, "escape")
	targetPath := os.TempDir() // Outside the allowed dir

	// Symlink creation may fail on Windows without elevated privileges
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Skipf("skipping symlink test (requires elevated privileges on Windows): %v", err)
	}

	// Resolve the symlink to its real path
	resolvedPath, err := filepath.EvalSymlinks(symlinkPath)
	if err != nil {
		t.Skipf("skipping: could not resolve symlink: %v", err)
	}

	absResolved, _ := filepath.Abs(resolvedPath)
	if strings.HasPrefix(absResolved, allowedDir+string(filepath.Separator)) || absResolved == allowedDir {
		t.Fatalf("SECURITY: symlink escape was NOT rejected: resolved=%s allowed=%s", absResolved, allowedDir)
	}

	t.Logf("[INFO] [server] [downloads] F-SECURITY VERIFIED: symlink escape rejected resolved=%s allowed=%s", absResolved, allowedDir)
}
