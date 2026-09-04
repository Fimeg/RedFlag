package middleware

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gofrs/uuid/v5"
)

// AgentClaims represents JWT claims for agent authentication
type AgentClaims struct {
	AgentID uuid.UUID `json:"agent_id"`
	jwt.RegisteredClaims
}

// JWTSecret is set by the server at initialization
var JWTSecret string

// JWT issuer constants for token type differentiation (F-A3-12 fix)
const (
	JWTIssuerAgent = "redflag-agent"
	JWTIssuerWeb   = "redflag-web"
)

// GenerateAgentToken creates a new JWT token for an agent
func GenerateAgentToken(agentID uuid.UUID) (string, error) {
	claims := AgentClaims{
		AgentID: agentID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    JWTIssuerAgent,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JWTSecret))
}

// AuthMiddleware validates JWT tokens from agents
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		token, err := jwt.ParseWithClaims(tokenString, &AgentClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(*AgentClaims); ok {
			// F-A3-12: Validate issuer to prevent cross-type token confusion
			if claims.Issuer != JWTIssuerAgent {
				log.Printf("[WARNING] [server] [auth] wrong_token_issuer expected=%s got=%s", JWTIssuerAgent, claims.Issuer)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token type"})
				c.Abort()
				return
			}
			c.Set("agent_id", claims.AgentID)
			c.Next()
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
		}
	}
}
