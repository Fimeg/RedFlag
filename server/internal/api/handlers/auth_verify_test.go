package handlers_test

// auth_verify_test.go — Pre-fix tests for the dead /auth/verify endpoint.
//
// BUG F-A3-2 MEDIUM: GET /api/v1/auth/verify has no middleware applied.
// The handler calls c.Get("user_id") which is never set because no
// middleware runs to parse the JWT. The endpoint always returns 401
// {"valid": false} regardless of the token provided.
//
// Run: cd server && go test ./internal/api/handlers/... -v -run TestAuthVerify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/api/handlers"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ---------------------------------------------------------------------------
// Test 5.1 — /auth/verify always returns 401 without middleware
//
// Category: PASS-NOW (documents the broken state)
//
// BUG F-A3-2: The handler calls c.Get("user_id") but no middleware
// sets this context value. The endpoint always returns 401 {"valid": false}
// regardless of the token provided. This is a dead endpoint.
// ---------------------------------------------------------------------------

func TestAuthVerifyAlwaysReturns401WithoutMiddleware(t *testing.T) {
	testSecret := "verify-test-secret"
	authHandler := handlers.NewAuthHandler(testSecret, nil)

	// Mirror current production state: NO middleware on this route
	router := gin.New()
	router.GET("/api/v1/auth/verify", authHandler.VerifyToken)

	// Create a valid web JWT
	claims := handlers.UserClaims{
		UserID:   "1",
		Username: "admin",
		Role:     "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "redflag-web",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}

	// Send with valid JWT — should still fail because no middleware parses it
	req := httptest.NewRequest("GET", "/api/v1/auth/verify", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Documents the bug: endpoint always returns 401
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("[ERROR] [server] [auth] expected 401 from dead verify endpoint, got %d", rec.Code)
	}

	// Check the response body says valid: false
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil {
		if valid, ok := body["valid"].(bool); ok && valid {
			t.Error("[ERROR] [server] [auth] verify endpoint returned valid=true without middleware (unexpected)")
		}
	}

	t.Log("[INFO] [server] [auth] BUG F-A3-2 confirmed: /auth/verify always returns 401 without middleware")
}

// ---------------------------------------------------------------------------
// Test 5.2 — /auth/verify works correctly WITH middleware
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// BUG F-A3-2: With WebAuthMiddleware applied, the verify endpoint should
// return 200 for a valid token. Fix: add WebAuthMiddleware to the route
// in main.go.
// ---------------------------------------------------------------------------

func TestAuthVerifyWorksWithMiddleware(t *testing.T) {
	testSecret := "verify-test-secret-2"
	authHandler := handlers.NewAuthHandler(testSecret, nil)

	// Correct configuration: WITH WebAuthMiddleware
	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/api/v1/auth/verify", authHandler.VerifyToken)

	// Create a valid web JWT
	claims := handlers.UserClaims{
		UserID:   "1",
		Username: "admin",
		Role:     "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "redflag-web",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/auth/verify", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// With middleware: should return 200 with valid=true
	if rec.Code != http.StatusOK {
		t.Errorf("[ERROR] [server] [auth] verify with middleware returned %d, expected 200.\n"+
			"BUG F-A3-2: /auth/verify is dead because main.go doesn't apply WebAuthMiddleware.\n"+
			"This test shows the endpoint WORKS when middleware is correctly applied.\n"+
			"After fix: add WebAuthMiddleware to the route in main.go.", rec.Code)
	}

	// Verify response contains valid: true
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil {
		if valid, ok := body["valid"].(bool); !ok || !valid {
			t.Errorf("[ERROR] [server] [auth] verify response missing valid=true: %v", body)
		}
	}
}
