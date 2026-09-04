package middleware

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireAdmin is a middleware that checks if the authenticated user has admin role.
// Must be used AFTER WebAuthMiddleware which sets user_id and role in context.
// Returns 403 if the user is not an admin.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		// WebAuthMiddleware sets user_id from UserClaims
		userID, exists := c.Get("user_id")
		if !exists {
			log.Printf("[WARNING] [server] [auth] require_admin called without user_id in context")
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		// Check role from context (set by WebAuthMiddleware from UserClaims)
		role, exists := c.Get("user_role")
		if !exists {
			// Fallback: if role is not in context, deny access
			log.Printf("[WARNING] [server] [auth] non_admin_access_attempt user_id=%v role=unknown", userID)
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || roleStr != "admin" {
			log.Printf("[WARNING] [server] [auth] non_admin_access_attempt user_id=%v role=%v", userID, role)
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}
