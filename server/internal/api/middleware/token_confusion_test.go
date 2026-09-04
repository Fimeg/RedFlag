package middleware_test

// token_confusion_test.go — Pre-fix tests for cross-type JWT token confusion.
//
// BUG F-A3-12 MEDIUM: Agent and web JWTs share the same signing secret with
// no issuer/audience differentiation. Cross-type token confusion is possible.
//
// The shared secret is set at main.go:166. Both AuthMiddleware (agent JWT)
// and WebAuthMiddleware (admin JWT) use the same HMAC key. Without issuer
// or audience claims, a JWT valid for one context may pass signature
// validation in the other.
//
// Run: cd server && go test ./internal/api/middleware/... -v -run TestToken

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/api/handlers"
	"github.com/Fimeg/RedFlag/server/internal/api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gofrs/uuid/v5"
)

// makeWebJWT creates a valid web/admin JWT with correct issuer for testing
func makeWebJWT(t *testing.T, secret string) string {
	t.Helper()
	claims := handlers.UserClaims{
		UserID:   "1",
		Username: "admin",
		Role:     "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "redflag-web",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign web JWT: %v", err)
	}
	return signed
}

// ---------------------------------------------------------------------------
// Test 4.1 — Web token SHOULD be rejected by agent AuthMiddleware
//
// Category: Verify actual behavior — PASS or FAIL depending on claims parsing
//
// BUG F-A3-12: Shared JWT secret allows cross-type token use. A web token
// passes signature validation on agent middleware. The question is whether
// claims parsing (AgentClaims expecting AgentID uuid.UUID) rejects the
// web token that has UserID string instead.
// ---------------------------------------------------------------------------

func TestWebTokenRejectedByAgentAuthMiddleware(t *testing.T) {
	// POST-FIX (F-A3-12): Web token with issuer=redflag-web is rejected by
	// agent AuthMiddleware which validates issuer=redflag-agent.
	sharedSecret := "shared-secret-confusion-test"
	middleware.JWTSecret = sharedSecret

	router := gin.New()
	router.Use(middleware.AuthMiddleware())
	router.GET("/agent-route", func(c *gin.Context) {
		agentID, exists := c.Get("agent_id")
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no agent_id"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"agent_id": agentID})
	})

	// Web JWT with issuer=redflag-web
	webToken := makeWebJWT(t, sharedSecret)

	req := httptest.NewRequest("GET", "/agent-route", nil)
	req.Header.Set("Authorization", "Bearer "+webToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Errorf("[ERROR] [server] [auth] web JWT accepted by agent AuthMiddleware (got %d, expected 401)", rec.Code)
	}
	t.Logf("[INFO] [server] [auth] F-A3-12 FIXED: web JWT rejected by agent middleware (%d)", rec.Code)
}

// ---------------------------------------------------------------------------
// Test 4.2 — Agent token SHOULD be rejected by WebAuthMiddleware
//
// Category: Verify actual behavior — PASS or FAIL depending on claims parsing
//
// BUG F-A3-12: An agent token (AgentClaims with AgentID uuid.UUID) may pass
// signature validation on WebAuthMiddleware. The question is whether the
// UserClaims parsing rejects it.
// ---------------------------------------------------------------------------

func TestAgentTokenRejectedByWebAuthMiddleware(t *testing.T) {
	// POST-FIX (F-A3-12): Agent token with issuer=redflag-agent is rejected by
	// WebAuthMiddleware which validates issuer=redflag-web.
	sharedSecret := "shared-secret-confusion-test-2"
	middleware.JWTSecret = sharedSecret

	authHandler := handlers.NewAuthHandler(sharedSecret, nil)

	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/admin-route", func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no user_id"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": userID})
	})

	// Agent JWT with issuer=redflag-agent
	agentClaims := middleware.AgentClaims{
		AgentID: uuid.Must(uuid.NewV4()),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    middleware.JWTIssuerAgent,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, agentClaims)
	agentToken, err := token.SignedString([]byte(sharedSecret))
	if err != nil {
		t.Fatalf("failed to sign agent JWT: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin-route", nil)
	req.Header.Set("Authorization", "Bearer "+agentToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Errorf("[ERROR] [server] [auth] agent JWT accepted by WebAuthMiddleware (got %d, expected 401)", rec.Code)
	}
	t.Logf("[INFO] [server] [auth] F-A3-12 FIXED: agent JWT rejected by web middleware (%d)", rec.Code)
}
