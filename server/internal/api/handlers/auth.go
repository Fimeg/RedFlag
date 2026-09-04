package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthHandler handles authentication for the web dashboard
type AuthHandler struct {
	jwtSecret    string
	adminQueries *queries.AdminQueries
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(jwtSecret string, adminQueries *queries.AdminQueries) *AuthHandler {
	return &AuthHandler{
		jwtSecret:    jwtSecret,
		adminQueries: adminQueries,
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string       `json:"token"`
	User  *queries.Admin `json:"user"`
}

// UserClaims represents JWT claims for web dashboard users
type UserClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Login handles web dashboard login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	// Validate credentials against database hash
	admin, err := h.adminQueries.VerifyAdminCredentials(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	// Create JWT token for web dashboard
	claims := UserClaims{
		UserID:   fmt.Sprintf("%d", admin.ID),
		Username: admin.Username,
		Role:     "admin", // Always admin for single-admin system
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "redflag-web",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token: tokenString,
		User:  admin,
	})
}

// VerifyToken handles token verification
func (h *AuthHandler) VerifyToken(c *gin.Context) {
	// This is handled by middleware, but we can add additional verification here
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"valid": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"user_id": userID,
	})
}

// Logout handles logout (client-side token removal)
func (h *AuthHandler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

// WebAuthMiddleware validates JWT tokens from web dashboard
func (h *AuthHandler) WebAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		tokenString := authHeader
		// Remove "Bearer " prefix if present
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		}

		token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(h.jwtSecret), nil
		})

		if err != nil || !token.Valid {
			log.Printf("[WARNING] [server] [auth] jwt_validation_failed error=%q", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(*UserClaims); ok {
			// F-A3-12: Validate issuer to prevent cross-type token confusion
			if claims.Issuer != "redflag-web" {
				log.Printf("[WARNING] [server] [auth] wrong_token_issuer expected=redflag-web got=%s", claims.Issuer)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token type"})
				c.Abort()
				return
			}
			c.Set("user_id", claims.UserID)
			c.Set("user_role", claims.Role)
			c.Next()
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
		}
	}
}