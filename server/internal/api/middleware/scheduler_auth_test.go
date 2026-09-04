package middleware_test

// scheduler_auth_test.go — Pre-fix tests for scheduler stats wrong middleware.
//
// BUG F-A3-10 HIGH: GET /api/v1/scheduler/stats uses AuthMiddleware (agent JWT)
// instead of WebAuthMiddleware (admin JWT). Any registered agent can view
// scheduler internals (queue stats, subsystem counts, timing data).
//
// ETHOS #2: All admin dashboard routes must use WebAuthMiddleware.
//
// Run: cd server && go test ./internal/api/middleware/... -v -run TestScheduler

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

// makeAgentJWT creates a valid agent JWT for testing
func makeAgentJWT(t *testing.T, secret string) string {
	t.Helper()
	claims := middleware.AgentClaims{
		AgentID: uuid.Must(uuid.NewV4()),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    middleware.JWTIssuerAgent,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign agent JWT: %v", err)
	}
	return signed
}

// ---------------------------------------------------------------------------
// Test 3.1 — Scheduler stats should reject agent JWTs (require admin)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// BUG F-A3-10: /scheduler/stats uses AuthMiddleware (agent JWT).
// An agent JWT is currently accepted. After fix, agent JWT must be
// rejected (route should use WebAuthMiddleware instead).
// ETHOS #2: All admin dashboard routes must use WebAuthMiddleware.
// ---------------------------------------------------------------------------

func TestSchedulerStatsRequiresAdminAuth(t *testing.T) {
	// POST-FIX (F-A3-10): Route now uses WebAuthMiddleware (admin JWT required).
	// Agent JWTs are rejected because WebAuthMiddleware expects UserClaims.
	testSecret := "scheduler-test-secret"
	middleware.JWTSecret = testSecret

	authHandler := handlers.NewAuthHandler(testSecret, nil)

	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/api/v1/scheduler/stats", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"scheduler": "stats"})
	})

	// Agent JWT should be rejected
	agentToken := makeAgentJWT(t, testSecret)
	req := httptest.NewRequest("GET", "/api/v1/scheduler/stats", nil)
	req.Header.Set("Authorization", "Bearer "+agentToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Errorf("[ERROR] [server] [scheduler] agent JWT accepted on scheduler stats (got %d, expected 401/403)", rec.Code)
	}
	t.Logf("[INFO] [server] [scheduler] F-A3-10 FIXED: agent JWT rejected on scheduler stats (%d)", rec.Code)
}

// ---------------------------------------------------------------------------
// Test 3.2 — Documents that agent JWT currently grants scheduler access
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// This test PASSES because the bug exists (agent JWT accepted).
// When the fix changes the middleware to WebAuthMiddleware, agent JWTs
// will be rejected and this test will FAIL.
// ---------------------------------------------------------------------------

func TestSchedulerStatsCurrentlyAcceptsAgentJWT(t *testing.T) {
	// POST-FIX (F-A3-10): Agent JWT is now rejected on scheduler stats.
	testSecret := "scheduler-test-secret-2"
	middleware.JWTSecret = testSecret

	authHandler := handlers.NewAuthHandler(testSecret, nil)

	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/api/v1/scheduler/stats", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"scheduler": "stats"})
	})

	agentToken := makeAgentJWT(t, testSecret)

	req := httptest.NewRequest("GET", "/api/v1/scheduler/stats", nil)
	req.Header.Set("Authorization", "Bearer "+agentToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// POST-FIX: agent JWT must be rejected
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Errorf("[ERROR] [server] [scheduler] agent JWT still accepted (%d), expected 401/403", rec.Code)
	}
	t.Log("[INFO] [server] [scheduler] F-A3-10 FIXED: agent JWT rejected on scheduler stats")
}
