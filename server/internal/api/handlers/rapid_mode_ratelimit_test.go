package handlers_test

// rapid_mode_ratelimit_test.go — Tests for rapid mode rate limiting.
//
// F-B2-4 FIXED: GET /agents/:id/commands now has a rate limiter.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetCommandsEndpointHasNoRateLimit(t *testing.T) {
	// POST-FIX: GetCommands now HAS a rate limit.
	mainPath := filepath.Join("..", "..", "..", "cmd", "server", "main.go")
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	src := string(content)

	cmdRouteIdx := strings.Index(src, `/:id/commands"`)
	if cmdRouteIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] GetCommands route not found")
	}

	lineStart := strings.LastIndex(src[:cmdRouteIdx], "\n") + 1
	lineEnd := strings.Index(src[cmdRouteIdx:], "\n") + cmdRouteIdx
	routeLine := src[lineStart:lineEnd]

	if !strings.Contains(routeLine, "rateLimiter.RateLimit") {
		t.Error("[ERROR] [server] [handlers] F-B2-4 NOT FIXED: GetCommands still has no rate limit")
	}

	t.Log("[INFO] [server] [handlers] F-B2-4 FIXED: GetCommands has rate limit")
}

func TestGetCommandsEndpointShouldHaveRateLimit(t *testing.T) {
	mainPath := filepath.Join("..", "..", "..", "cmd", "server", "main.go")
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	src := string(content)

	cmdRouteIdx := strings.Index(src, `/:id/commands"`)
	if cmdRouteIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] GetCommands route not found")
	}

	lineStart := strings.LastIndex(src[:cmdRouteIdx], "\n") + 1
	lineEnd := strings.Index(src[cmdRouteIdx:], "\n") + cmdRouteIdx
	routeLine := src[lineStart:lineEnd]

	if !strings.Contains(routeLine, "rateLimiter.RateLimit") {
		t.Errorf("[ERROR] [server] [handlers] GetCommands has no rate limit")
	}
	t.Log("[INFO] [server] [handlers] F-B2-4 FIXED: rate limit applied to GetCommands")
}

func TestRapidModeHasServerSideMaxDuration(t *testing.T) {
	agentsPath := filepath.Join(".", "agents.go")
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read agents.go: %v", err)
	}

	src := string(content)
	rapidIdx := strings.Index(src, "func (h *AgentHandler) SetRapidPollingMode")
	if rapidIdx == -1 {
		t.Fatal("[ERROR] [server] [handlers] SetRapidPollingMode not found")
	}

	fnBody := src[rapidIdx:]
	if len(fnBody) > 1500 {
		fnBody = fnBody[:1500]
	}

	if !strings.Contains(fnBody, "max=60") {
		t.Error("[ERROR] [server] [handlers] no max duration validation in SetRapidPollingMode")
	}

	t.Log("[INFO] [server] [handlers] F-B2-4: rapid mode max duration cap exists (60 minutes)")
}
