package middleware

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware handles Cross-Origin Resource Sharing
// Origin is configurable via REDFLAG_CORS_ORIGIN environment variable.
// Defaults to http://localhost:3000 for development.
func CORSMiddleware() gin.HandlerFunc {
	origin := os.Getenv("REDFLAG_CORS_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3000"
	}
	log.Printf("[INFO] [server] [cors] cors_origin_set origin=%q", origin)

	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Machine-ID, X-Agent-Version, X-Update-Nonce")
		c.Header("Access-Control-Expose-Headers", "Content-Length")
		c.Header("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}