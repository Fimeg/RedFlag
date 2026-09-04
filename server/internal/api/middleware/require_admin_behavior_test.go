package middleware_test

// require_admin_behavior_test.go — Behavioral test for RequireAdmin middleware.
//
// POST-FIX (F-A3-13): RequireAdmin() is now implemented.
// Build tag removed — test compiles and runs.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

func TestRequireAdminBlocksNonAdminUsers(t *testing.T) {
	router := gin.New()

	// Simulate WebAuthMiddleware having set user_id and user_role
	router.Use(func(c *gin.Context) {
		role := c.GetHeader("X-Test-Role")
		c.Set("user_id", "test-user-1")
		c.Set("user_role", role)
		c.Next()
	})
	router.Use(middleware.RequireAdmin())
	router.GET("/admin-only", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"admin": true})
	})

	// Test A: non-admin user should be blocked
	req := httptest.NewRequest("GET", "/admin-only", nil)
	req.Header.Set("X-Test-Role", "viewer")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("[ERROR] [server] [middleware] non-admin user got %d, expected 403", rec.Code)
	}

	// Test B: admin user should pass
	req2 := httptest.NewRequest("GET", "/admin-only", nil)
	req2.Header.Set("X-Test-Role", "admin")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("[ERROR] [server] [middleware] admin user got %d, expected 200", rec2.Code)
	}

	t.Log("[INFO] [server] [middleware] F-A3-13 FIXED: RequireAdmin correctly blocks non-admin users")
}
