package middleware_test

// auth_secret_leak_test.go — Pre-fix tests for JWT secret leak in WebAuthMiddleware.
//
// BUG F-A3-11: WebAuthMiddleware prints the JWT secret to stdout on validation
// failure (auth.go:128). Two violations:
//   1. Secret value in log output — any log collector captures the signing key
//   2. Emoji in log output — violates ETHOS #1
//
// These tests verify that the middleware does NOT leak secrets or use emojis.
// They currently FAIL because the bug exists.
//
// Run: cd server && go test ./internal/api/middleware/... -v -run TestWebAuth

// NOTE: WebAuthMiddleware is defined in handlers/auth.go (on AuthHandler),
// not in the middleware package. These tests exercise the middleware indirectly
// via the handler's exported method. Since we need to test the actual handler
// behavior without importing the handlers package from a middleware_test package,
// we test the agent AuthMiddleware here (which is in this package) and create
// a parallel test file in handlers_test for WebAuthMiddleware.
//
// See: server/internal/api/handlers/auth_middleware_leak_test.go

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/Fimeg/RedFlag/server/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Test 1.1 — Agent AuthMiddleware does not leak middleware.JWTSecret
//
// Category: PASS-NOW (agent middleware does not have the leak bug)
//
// This test confirms that the agent-side AuthMiddleware does NOT print
// secrets on failure. It serves as a contrast to the WebAuthMiddleware
// which DOES print secrets (see handlers/auth_middleware_leak_test.go).
// ---------------------------------------------------------------------------

func TestAgentAuthMiddlewareDoesNotLogSecret(t *testing.T) {
	testSecret := "agent-test-secret-67890"
	middleware.JWTSecret = testSecret

	router := gin.New()
	router.Use(middleware.AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-abc123")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	captured := buf.String()

	// Agent middleware should NOT print the secret
	if strings.Contains(captured, testSecret) {
		t.Errorf("[ERROR] [server] [auth] agent AuthMiddleware leaked JWT secret to stdout")
	}

	// Confirm the request was rejected
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("[ERROR] [server] [auth] expected 401, got %d", rec.Code)
	}

	t.Log("[INFO] [server] [auth] agent AuthMiddleware does not leak secrets (correct)")
}

// ---------------------------------------------------------------------------
// Test 1.2 — Agent AuthMiddleware stdout has no emoji characters
//
// Category: PASS-NOW (agent middleware does not use emojis)
// ---------------------------------------------------------------------------

func TestAgentAuthMiddlewareLogHasNoEmoji(t *testing.T) {
	middleware.JWTSecret = "test-secret-no-emoji"

	router := gin.New()
	router.Use(middleware.AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	captured := buf.String()

	for _, r := range captured {
		if r > 0x1F300 || (r >= 0x1F600 && r <= 0x1F64F) || unicode.Is(unicode.So, r) {
			t.Errorf("[ERROR] [server] [auth] emoji character found in agent middleware output: U+%04X", r)
		}
	}

	t.Log("[INFO] [server] [auth] agent AuthMiddleware output has no emoji (correct)")
}
