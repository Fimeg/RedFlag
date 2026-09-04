package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// fireRequests sends n requests through the limiter and returns how many were
// allowed (non-429).
func fireRequests(t *testing.T, rl *RateLimiter, n int) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/x", rl.RateLimit("public_access", KeyByIP), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	allowed := 0
	for i := 0; i < n; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusTooManyRequests {
			allowed++
		}
	}
	return allowed
}

// TestStartupGraceHalvesBudget locks in SEC-004: within the post-boot grace
// window the limiter runs at half the configured budget, so a forced restart
// cannot profitably reset counters.
func TestStartupGraceHalvesBudget(t *testing.T) {
	rl := NewRateLimiter() // startedAt = now → inside grace window
	full := DefaultRateLimitSettings().PublicAccess.Requests

	allowed := fireRequests(t, rl, full)
	if want := full / 2; allowed != want {
		t.Fatalf("during grace window: allowed %d of %d requests, want %d (half budget)", allowed, full, want)
	}
}

// TestFullBudgetAfterGrace verifies the limiter returns to the configured
// budget once the grace window has passed.
func TestFullBudgetAfterGrace(t *testing.T) {
	rl := NewRateLimiter()
	rl.startedAt = time.Now().Add(-2 * startupGracePeriod) // grace window elapsed
	full := DefaultRateLimitSettings().PublicAccess.Requests

	allowed := fireRequests(t, rl, full+5)
	if allowed != full {
		t.Fatalf("after grace window: allowed %d requests, want full budget %d", allowed, full)
	}
}
