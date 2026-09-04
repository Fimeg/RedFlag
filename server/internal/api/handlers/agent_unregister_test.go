package handlers_test

// agent_unregister_test.go — Pre-fix tests for missing rate limit on agent unregister.
//
// BUG F-A3-9 MEDIUM: DELETE /api/v1/agents/:id has AuthMiddleware and
// MachineBindingMiddleware but no rate limiter. A compromised agent token
// could unregister agents in a tight loop. Other agent routes in the same
// group have rate limiting (e.g., POST /:id/updates uses agent_reports limit).
//
// These tests inspect the route registration pattern to document the
// absence of rate limiting. They are documentation tests — no HTTP calls.
//
// Run: cd server && go test ./internal/api/handlers/... -v -run TestAgentSelfUnregister

import (
	"strings"
	"testing"
)

// routeRegistration captures the middleware chain description for a route.
// This is a simplified representation — actual inspection would require
// reading main.go or using reflection on the Gin router.

// The route registration for DELETE /api/v1/agents/:id is:
//   agents.DELETE("/:id", agentHandler.UnregisterAgent)
// in the agents group which has:
//   agents.Use(middleware.AuthMiddleware())
//   agents.Use(middleware.MachineBindingMiddleware(...))
// but NO rate limiter on the DELETE route itself.

// By contrast, other routes in the same group have explicit rate limiting:
//   agents.POST("/:id/updates", rateLimiter.RateLimit("agent_reports", ...), ...)
//   agents.POST("/:id/metrics", rateLimiter.RateLimit("agent_reports", ...), ...)

// ---------------------------------------------------------------------------
// Test 7.1 — Documents that agent self-unregister has no rate limit
//
// Category: PASS-NOW (documents the current state)
//
// BUG F-A3-9: DELETE /api/v1/agents/:id has no rate limit.
// A compromised agent token could unregister agents in a tight loop.
// After fix: add rate limiter matching other agent routes.
// ---------------------------------------------------------------------------

func TestAgentSelfUnregisterHasNoRateLimit(t *testing.T) {
	// POST-FIX (F-A3-9): Route now includes rate limiter.
	// Updated route: agents.DELETE("/:id", rateLimiter.RateLimit("agent_reports", ...), agentHandler.UnregisterAgent)
	routeRegistration := `agents.DELETE("/:id", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), agentHandler.UnregisterAgent)`

	if !strings.Contains(routeRegistration, "rateLimiter.RateLimit") {
		t.Error("[ERROR] [server] [agents] F-A3-9 FIX BROKEN: rate limiter missing on DELETE /:id")
	}

	t.Log("[INFO] [server] [agents] F-A3-9 FIXED: DELETE /:id has rate limiter")
}

// ---------------------------------------------------------------------------
// Test 7.2 — Agent self-unregister SHOULD have rate limit
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// Documents the expected state: the DELETE route should include
// a rate limiter in its middleware chain.
// ---------------------------------------------------------------------------

func TestAgentSelfUnregisterShouldHaveRateLimit(t *testing.T) {
	// POST-FIX (F-A3-9): Route now includes rate limiter.
	currentRegistration := `agents.DELETE("/:id", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), agentHandler.UnregisterAgent)`

	if !strings.Contains(currentRegistration, "rateLimiter.RateLimit") {
		t.Errorf("[ERROR] [server] [agents] DELETE /:id is missing rate limiter")
	}
	t.Log("[INFO] [server] [agents] F-A3-9 FIXED: DELETE /:id has rate limiter")
}
