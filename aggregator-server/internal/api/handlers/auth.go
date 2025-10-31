package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/database/queries"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AuthHandler handles authentication for the web dashboard
type AuthHandler struct {
	jwtSecret   string
	userQueries *queries.UserQueries
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(jwtSecret string, userQueries *queries.UserQueries) *AuthHandler {
	return &AuthHandler{
		jwtSecret:   jwtSecret,
		userQueries: userQueries,
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string     `json:"token"`
	User  *models.User `json:"user"`
}

// UserClaims represents JWT claims for web dashboard users
type UserClaims struct {
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	jwt.RegisteredClaims
}

// Login handles web dashboard login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	// Validate credentials against database
	user, err := h.userQueries.VerifyCredentials(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	// Create JWT token for web dashboard
	claims := UserClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
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
		User:  user,
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
			// Debug: Log the JWT validation error (remove in production)
			fmt.Printf("🔓 JWT validation failed: %v (secret: %s)\n", err, h.jwtSecret)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(*UserClaims); ok {
			c.Set("user_id", claims.UserID)
			c.Next()
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
		}
	}
}