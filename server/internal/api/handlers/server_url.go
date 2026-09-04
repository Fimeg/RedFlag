package handlers

import (
	"fmt"
	"log"

	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/gin-gonic/gin"
)

// resolveServerURL returns the canonical external base URL for this server.
//
// Resolution order:
//  1. REDFLAG_PUBLIC_URL / cfg.Server.PublicURL — operator-configured; always wins.
//  2. c.Request.Host — the host the client reached; baked into agent configs and
//     install scripts, so it must be reachable from the agent. Logs a structured
//     warning because trusting an HTTP header is a fallback, not a guarantee.
//  3. Bind address — used when c is nil (non-HTTP callers, template pre-render).
//
// component is the [component] field for ETHOS-format log lines.
func resolveServerURL(c *gin.Context, cfg *config.Config, component string) string {
	// Priority 1: explicit operator config — not attacker-controllable.
	if cfg != nil && cfg.Server.PublicURL != "" {
		return cfg.Server.PublicURL
	}

	// Priority 2: Host header from the live request.
	// The Host header is attacker-controllable on unauthenticated endpoints; log
	// a warning so operators know the public URL is unconfigured.
	if c != nil && c.Request != nil && c.Request.Host != "" {
		scheme := "http"
		if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else if c.Request.TLS != nil {
			scheme = "https"
		}
		log.Printf("[WARN] [server] [%s] public_url_unconfigured: trusting request Host header; set REDFLAG_PUBLIC_URL to fix", component)
		return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
	}

	// Priority 3: bind address (non-HTTP callers).
	if cfg == nil {
		return "http://localhost:8080"
	}
	scheme := "http"
	host := cfg.Server.Host
	port := cfg.Server.Port
	if cfg.Server.TLS.Enabled {
		scheme = "https"
	}
	if host == "0.0.0.0" {
		host = "localhost"
	}
	if (scheme == "http" && port != 80) || (scheme == "https" && port != 443) {
		return fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}
