package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MetricsAuthConfig is resolved per request so settings-based token rotation
// takes effect without restarting the server.
type MetricsAuthConfig struct {
	Enabled        bool
	TokenSHA256Hex string
}

// HashMetricsToken returns the hex SHA-256 hash used for metrics token storage.
// The plaintext token is accepted from env for bootstrap, but persisted settings
// should store only this hash.
func HashMetricsToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// MetricsBearerAuth protects the Prometheus scrape endpoint with a dedicated
// bearer token. It is intentionally separate from WebAuthMiddleware so scraping
// does not depend on dashboard JWT rotation.
func MetricsBearerAuth(resolve func() MetricsAuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := resolve()
		if !cfg.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "metrics disabled"})
			c.Abort()
			return
		}

		expected, err := hex.DecodeString(strings.TrimSpace(cfg.TokenSHA256Hex))
		if err != nil || len(expected) != sha256.Size {
			log.Printf("[ERROR] [server] [metrics] metrics_token_not_configured")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "metrics token not configured"})
			c.Abort()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader || token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		actualHash := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(actualHash[:], expected) != 1 {
			log.Printf("[WARNING] [server] [metrics] invalid_metrics_token client_ip=%s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Next()
	}
}
