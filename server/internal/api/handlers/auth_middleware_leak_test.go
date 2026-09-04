package handlers_test

// auth_middleware_leak_test.go — Pre-fix tests for JWT secret leak in WebAuthMiddleware.
//
// BUG F-A3-11 CRITICAL: WebAuthMiddleware (auth.go:128) prints the JWT signing
// secret to stdout on every validation failure:
//   fmt.Printf("🔓 JWT validation failed: %v (secret: %s)\n", err, h.jwtSecret)
//
// Violations:
//   1. Secret in log output — any log collector captures the signing key
//   2. Emoji in log output — violates ETHOS #1 log format requirement
//   3. Log format wrong — must be [TAG] [system] [component] per ETHOS #1
//
// Tests 1.1-1.3 currently FAIL (bug present). They will PASS after the fix.
//
// Run: cd server && go test ./internal/api/handlers/... -v -run TestWebAuthMiddleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/api/handlers"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Test 1.1 — WebAuthMiddleware does NOT log the JWT secret
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// BUG F-A3-11: auth.go:128 prints h.jwtSecret directly to stdout.
// Any log collector (Docker logs, journald, CloudWatch) captures this secret.
// An attacker with log access can forge arbitrary admin tokens.
// ---------------------------------------------------------------------------

func TestWebAuthMiddlewareDoesNotLogSecret(t *testing.T) {
	testSecret := "test-secret-12345"
	authHandler := handlers.NewAuthHandler(testSecret, nil)

	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Capture stdout during middleware execution
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	// Send request with an invalid JWT (valid format, wrong signature)
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiMSJ9.invalidsig")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	captured := buf.String()

	// The secret MUST NOT appear in stdout
	if strings.Contains(captured, testSecret) {
		t.Errorf("[ERROR] [server] [auth] WebAuthMiddleware leaked JWT secret to stdout.\n"+
			"BUG F-A3-11: auth.go:128 prints h.jwtSecret on validation failure.\n"+
			"Captured output contains: %q\n"+
			"After fix: remove secret from log output entirely.", testSecret)
	}

	// Confirm the request was still rejected
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("[ERROR] [server] [auth] expected 401 for invalid token, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Test 1.2 — WebAuthMiddleware log output has no emoji characters
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// BUG F-A3-11: auth.go:128 uses "🔓" (U+1F513) in log output.
// ETHOS #1 requires [TAG] [system] [component] format, no emojis.
// ---------------------------------------------------------------------------

func TestWebAuthMiddlewareLogFormatHasNoEmoji(t *testing.T) {
	authHandler := handlers.NewAuthHandler("emoji-test-secret", nil)

	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	captured := buf.String()

	// Check for emoji characters (Unicode symbols above U+1F300)
	hasEmoji := false
	for _, ch := range captured {
		if ch >= 0x1F300 || (ch >= 0x2600 && ch <= 0x27BF) {
			hasEmoji = true
			t.Errorf("[ERROR] [server] [auth] emoji character found in log output: U+%04X\n"+
				"BUG F-A3-11: auth.go:128 uses emoji in log. ETHOS #1 violation.\n"+
				"After fix: use [WARNING] [server] [auth] format.", ch)
			break
		}
	}

	// Also check for the specific emoji used
	if strings.Contains(captured, "\U0001F513") {
		if !hasEmoji {
			t.Errorf("[ERROR] [server] [auth] lock emoji U+1F513 found in log output")
		}
	}

	// Check that the word "secret" does not appear
	if strings.Contains(strings.ToLower(captured), "secret") {
		t.Errorf("[ERROR] [server] [auth] word 'secret' found in log output")
	}

	_ = rec // ensure request completed
}

// ---------------------------------------------------------------------------
// Test 1.3 — WebAuthMiddleware log format is ETHOS-compliant
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// BUG F-A3-11: Log format must be [TAG] [system] [component] message.
// Current output: "🔓 JWT validation failed: ... (secret: ...)"
// Expected: "[WARNING] [server] [auth] jwt_validation_failed error=..."
// ---------------------------------------------------------------------------

func TestWebAuthMiddlewareLogFormatCompliant(t *testing.T) {
	authHandler := handlers.NewAuthHandler("format-test-secret", nil)

	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-jwt-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	captured := buf.String()

	if captured == "" {
		// No output is acceptable (middleware can log to logger instead of stdout)
		t.Log("[INFO] [server] [auth] no stdout output produced (acceptable)")
		return
	}

	// If output exists, it must follow ETHOS format
	lines := strings.Split(strings.TrimSpace(captured), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Must start with [TAG] pattern
		if !strings.HasPrefix(line, "[") {
			t.Errorf("[ERROR] [server] [auth] log line does not follow [TAG] format: %q\n"+
				"BUG F-A3-11: expected [WARNING] [server] [auth] or [ERROR] [server] [auth]", line)
		}
		// Must not contain the secret
		if strings.Contains(line, "format-test-secret") {
			t.Errorf("[ERROR] [server] [auth] log line contains JWT secret")
		}
	}

	_ = rec
}
