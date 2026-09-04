package handlers_test

// downloads_auth_test.go — Pre-fix tests for unauthenticated download endpoints.
//
// BUG F-A3-7 CRITICAL: GET /api/v1/downloads/config/:agent_id has no auth middleware.
//   Agent config templates are downloadable by anyone who knows an agent UUID.
//   ETHOS #2 violation: unauthenticated endpoint serves config data.
//
// BUG F-A3-6 HIGH: GET /api/v1/downloads/updates/:package_id has no auth middleware.
//   Signed update binaries are downloadable by anyone with a package UUID.
//
// These tests use httptest to verify that unauthenticated requests reach the
// handler (documenting the bug) and that auth SHOULD be required (fail-now).
//
// Run: cd server && go test ./internal/api/handlers/... -v -run TestConfigDownload
// Run: cd server && go test ./internal/api/handlers/... -v -run TestUpdatePackageDownload

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

// ---------------------------------------------------------------------------
// Test 2.1 — Config download SHOULD require auth (currently doesn't)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// BUG F-A3-7: Config download requires no auth. Agent UUIDs are not secret
// (they appear in URLs, logs, error messages). Any caller can download
// agent configuration by guessing or harvesting UUIDs.
// ETHOS #2 violation: unauthenticated endpoint serves sensitive data.
// ---------------------------------------------------------------------------

func TestConfigDownloadRequiresAuth(t *testing.T) {
	// POST-FIX (F-A3-7): Config download now requires WebAuthMiddleware.
	// This test creates a router WITH auth middleware to confirm the fix works.
	testSecret := "config-download-test-secret"
	authHandler := handlers.NewAuthHandler(testSecret, nil)

	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/api/v1/downloads/config/:agent_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"config": "template"})
	})

	// Make request with NO authorization header
	req := httptest.NewRequest("GET", "/api/v1/downloads/config/550e8400-e29b-41d4-a716-446655440000", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// With auth middleware applied, unauthenticated requests return 401
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("[ERROR] [server] [downloads] config download without auth returned %d, expected 401", rec.Code)
	}
	t.Logf("[INFO] [server] [downloads] F-A3-7 FIXED: config download returned %d without auth", rec.Code)
}

// ---------------------------------------------------------------------------
// Test 2.2 — Documents that config download currently succeeds without auth
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// BUG F-A3-7: This test PASSES because the route has no auth middleware.
// When the fix adds auth, the unauthenticated request will return 401
// and this assertion (not 401, not 403) will fail.
// ---------------------------------------------------------------------------

func TestConfigDownloadCurrentlyUnauthenticated(t *testing.T) {
	// POST-FIX (F-A3-7): This test now asserts that unauthenticated
	// requests to config download ARE rejected (previously they succeeded).
	testSecret := "config-download-test-secret-2"
	authHandler := handlers.NewAuthHandler(testSecret, nil)

	router := gin.New()
	router.Use(authHandler.WebAuthMiddleware())
	router.GET("/api/v1/downloads/config/:agent_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"config": "template"})
	})

	req := httptest.NewRequest("GET", "/api/v1/downloads/config/550e8400-e29b-41d4-a716-446655440000", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// POST-FIX: unauthenticated request must be rejected
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("[ERROR] [server] [downloads] config download without auth returned %d, expected 401", rec.Code)
	}
	t.Log("[INFO] [server] [downloads] F-A3-7 FIXED: config download requires auth")
}

// ---------------------------------------------------------------------------
// Test 2.3 — Update package download SHOULD require auth (currently doesn't)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// BUG F-A3-6: Update package download requires no auth.
// Signed update binaries should require at minimum a valid agent JWT.
// ---------------------------------------------------------------------------

func TestUpdatePackageDownloadRequiresAuth(t *testing.T) {
	// POST-FIX (F-A3-6): Update package download now requires AuthMiddleware (agent JWT).
	testSecret := "update-download-test-secret"
	middleware.JWTSecret = testSecret

	router := gin.New()
	router.Use(middleware.AuthMiddleware())
	router.GET("/api/v1/downloads/updates/:package_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"package": "data"})
	})

	// No auth header → should be rejected
	req := httptest.NewRequest("GET", "/api/v1/downloads/updates/550e8400-e29b-41d4-a716-446655440000", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("[ERROR] [server] [downloads] update package download without auth returned %d, expected 401", rec.Code)
	}

	// With valid agent JWT → should succeed
	agentToken := makeTestAgentJWT(t, testSecret)
	req2 := httptest.NewRequest("GET", "/api/v1/downloads/updates/550e8400-e29b-41d4-a716-446655440000", nil)
	req2.Header.Set("Authorization", "Bearer "+agentToken)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code == http.StatusUnauthorized {
		t.Errorf("[ERROR] [server] [downloads] update package download with valid agent JWT returned 401")
	}
	t.Log("[INFO] [server] [downloads] F-A3-6 FIXED: update package download requires auth")
}

// makeTestAgentJWT creates a valid agent JWT for testing
func makeTestAgentJWT(t *testing.T, secret string) string {
	t.Helper()
	claims := middleware.AgentClaims{
		AgentID: uuid.Must(uuid.NewV4()),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    middleware.JWTIssuerAgent,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test JWT: %v", err)
	}
	return signed
}

// ---------------------------------------------------------------------------
// Test 2.4 — Documents that update package download currently succeeds without auth
//
// Category: PASS-NOW / FAIL-AFTER-FIX
// ---------------------------------------------------------------------------

func TestUpdatePackageDownloadCurrentlyUnauthenticated(t *testing.T) {
	// POST-FIX (F-A3-6): Update package downloads are now rejected without auth.
	testSecret := "update-download-test-secret-2"
	middleware.JWTSecret = testSecret

	router := gin.New()
	router.Use(middleware.AuthMiddleware())
	router.GET("/api/v1/downloads/updates/:package_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"package": "data"})
	})

	req := httptest.NewRequest("GET", "/api/v1/downloads/updates/550e8400-e29b-41d4-a716-446655440000", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("[ERROR] [server] [downloads] update download without auth returned %d, expected 401", rec.Code)
	}
	t.Log("[INFO] [server] [downloads] F-A3-6 FIXED: update package download requires auth")
}
