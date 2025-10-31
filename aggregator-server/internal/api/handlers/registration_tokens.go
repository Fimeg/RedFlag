package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/config"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/database/queries"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RegistrationTokenHandler struct {
	tokenQueries *queries.RegistrationTokenQueries
	agentQueries *queries.AgentQueries
	config       *config.Config
}

func NewRegistrationTokenHandler(tokenQueries *queries.RegistrationTokenQueries, agentQueries *queries.AgentQueries, config *config.Config) *RegistrationTokenHandler {
	return &RegistrationTokenHandler{
		tokenQueries: tokenQueries,
		agentQueries: agentQueries,
		config:       config,
	}
}

// GenerateRegistrationToken creates a new registration token
func (h *RegistrationTokenHandler) GenerateRegistrationToken(c *gin.Context) {
	var request struct {
		Label     string                 `json:"label" binding:"required"`
		ExpiresIn string                 `json:"expires_in"` // e.g., "24h", "7d", "168h"
		MaxSeats  int                    `json:"max_seats"`  // Number of agents that can use this token
		Metadata  map[string]interface{} `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	// Check agent seat limit (security, not licensing)
	activeAgents, err := h.agentQueries.GetActiveAgentCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check agent count"})
		return
	}

	if activeAgents >= h.config.AgentRegistration.MaxSeats {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Maximum agent seats reached",
			"limit": h.config.AgentRegistration.MaxSeats,
			"current": activeAgents,
		})
		return
	}

	// Parse expiration duration
	expiresIn := request.ExpiresIn
	if expiresIn == "" {
		expiresIn = h.config.AgentRegistration.TokenExpiry
	}

	duration, err := time.ParseDuration(expiresIn)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiration format. Use formats like '24h', '7d', '168h'"})
		return
	}

	expiresAt := time.Now().Add(duration)
	if duration > 168*time.Hour { // Max 7 days
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token expiration cannot exceed 7 days"})
		return
	}

	// Generate secure token
	token, err := config.GenerateSecureToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Create metadata with default values
	metadata := request.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["server_url"] = c.Request.Host
	metadata["expires_in"] = expiresIn

	// Default max_seats to 1 if not provided or invalid
	maxSeats := request.MaxSeats
	if maxSeats < 1 {
		maxSeats = 1
	}

	// Store token in database
	err = h.tokenQueries.CreateRegistrationToken(token, request.Label, expiresAt, maxSeats, metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create token"})
		return
	}

	// Build install command
	serverURL := c.Request.Host
	if serverURL == "" {
		serverURL = "localhost:8080" // Fallback for development
	}
	installCommand := "curl -sfL https://" + serverURL + "/install | bash -s -- " + token

	response := gin.H{
		"token":          token,
		"label":          request.Label,
		"expires_at":     expiresAt,
		"install_command": installCommand,
		"metadata":       metadata,
	}

	c.JSON(http.StatusCreated, response)
}

// ListRegistrationTokens returns all registration tokens with pagination
func (h *RegistrationTokenHandler) ListRegistrationTokens(c *gin.Context) {
	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")
	isActive := c.Query("is_active") == "true"

	// Validate pagination
	if limit > 100 {
		limit = 100
	}
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * limit

	var tokens []queries.RegistrationToken
	var err error

	// Handle filtering by active status
	if isActive || status == "active" {
		// Get only active tokens (no pagination for active-only queries)
		tokens, err = h.tokenQueries.GetActiveRegistrationTokens()

		// Apply manual pagination to active tokens if needed
		if err == nil && len(tokens) > 0 {
			start := offset
			end := offset + limit
			if start >= len(tokens) {
				tokens = []queries.RegistrationToken{}
			} else {
				if end > len(tokens) {
					end = len(tokens)
				}
				tokens = tokens[start:end]
			}
		}
	} else {
		// Get all tokens with database-level pagination
		tokens, err = h.tokenQueries.GetAllRegistrationTokens(limit, offset)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list tokens"})
		return
	}

	// Get token usage stats
	stats, err := h.tokenQueries.GetTokenUsageStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get token stats"})
		return
	}

	response := gin.H{
		"tokens": tokens,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"offset": offset,
		},
		"stats": stats,
		"seat_usage": gin.H{
			"current": func() int {
				count, _ := h.agentQueries.GetActiveAgentCount()
				return count
			}(),
			"max": h.config.AgentRegistration.MaxSeats,
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetActiveRegistrationTokens returns only active tokens
func (h *RegistrationTokenHandler) GetActiveRegistrationTokens(c *gin.Context) {
	tokens, err := h.tokenQueries.GetActiveRegistrationTokens()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get active tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

// RevokeRegistrationToken revokes a registration token
func (h *RegistrationTokenHandler) RevokeRegistrationToken(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token is required"})
		return
	}

	var request struct {
		Reason string `json:"reason"`
	}

	c.ShouldBindJSON(&request) // Reason is optional

	reason := request.Reason
	if reason == "" {
		reason = "Revoked via API"
	}

	err := h.tokenQueries.RevokeRegistrationToken(token, reason)
	if err != nil {
		if err.Error() == "token not found or already used/revoked" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Token not found or already used/revoked"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke token"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token revoked successfully"})
}

// DeleteRegistrationToken permanently deletes a registration token
func (h *RegistrationTokenHandler) DeleteRegistrationToken(c *gin.Context) {
	tokenID := c.Param("id")
	if tokenID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token ID is required"})
		return
	}

	// Parse UUID
	id, err := uuid.Parse(tokenID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid token ID format"})
		return
	}

	err = h.tokenQueries.DeleteRegistrationToken(id)
	if err != nil {
		if err.Error() == "token not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Token not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete token"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token deleted successfully"})
}

// ValidateRegistrationToken checks if a token is valid (for testing/debugging)
func (h *RegistrationTokenHandler) ValidateRegistrationToken(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token query parameter is required"})
		return
	}

	tokenInfo, err := h.tokenQueries.ValidateRegistrationToken(token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid": true,
		"token": tokenInfo,
	})
}

// CleanupExpiredTokens performs cleanup of expired tokens
func (h *RegistrationTokenHandler) CleanupExpiredTokens(c *gin.Context) {
	count, err := h.tokenQueries.CleanupExpiredTokens()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup expired tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cleanup completed",
		"cleaned": count,
	})
}

// GetTokenStats returns comprehensive token usage statistics
func (h *RegistrationTokenHandler) GetTokenStats(c *gin.Context) {
	// Get token stats
	tokenStats, err := h.tokenQueries.GetTokenUsageStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get token stats"})
		return
	}

	// Get agent count
	activeAgentCount, err := h.agentQueries.GetActiveAgentCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get agent count"})
		return
	}

	response := gin.H{
		"token_stats": tokenStats,
		"agent_usage": gin.H{
			"active_agents": activeAgentCount,
			"max_seats":     h.config.AgentRegistration.MaxSeats,
			"available":    h.config.AgentRegistration.MaxSeats - activeAgentCount,
		},
		"security_limits": gin.H{
			"max_tokens_per_request": h.config.AgentRegistration.MaxTokens,
			"max_token_duration":    "7 days",
			"token_expiry_default":  h.config.AgentRegistration.TokenExpiry,
		},
	}

	c.JSON(http.StatusOK, response)
}