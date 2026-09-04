package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/security"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// maxRegistrationTokenDuration is the hard ceiling on how long a registration
// token (a bearer credential for enrolling new agents) may live. Raised from
// the previous 168h (7d) cap 2026-06-30 — see
// docs/tasks/UI-REGISTRATION-ENROLLMENT-UNIFY.md. 90 days matches the
// existing precedent for a long-lived bound credential elsewhere in this
// system (refresh tokens — RAF/security/03-refresh-tokens.md) — same
// yardstick already in use, not a new risk category. True "never expires"
// was deliberately not added here: expires_at is NOT NULL and load-bearing
// in the active-token query (`expires_at > NOW()`), so making it optional is
// a schema change, not a UX pass. Blast radius still bounded by max_seats
// and by revocation being one click away in the UI.
const maxRegistrationTokenDuration = 2160 * time.Hour // 90 days

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
		Label      string                 `json:"label" binding:"required"`
		ExpiresIn  string                 `json:"expires_in"` // e.g., "24h", "7d", "168h"
		MaxSeats   int                    `json:"max_seats"`  // Number of agents that can use this token
		Metadata   map[string]interface{} `json:"metadata"`
		// FleetJoin + TOTPSeed: SEC-025 fleet-join 2FA. When the operator
		// creates a fleet-join token, they enter the host's TOTP seed
		// (displayed on the standalone host). This is the one admin-
		// authenticated transfer the seed ever makes: the server stores it
		// encrypted, and the join request later carries only the 6-digit code.
		FleetJoin bool   `json:"fleet_join"`
		TOTPSeed  string `json:"totp_seed"`
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
			"error":   "Maximum agent seats reached",
			"limit":   h.config.AgentRegistration.MaxSeats,
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
	if duration > maxRegistrationTokenDuration {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token expiration cannot exceed 90 days"})
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
	metadata["server_url"] = resolveServerURL(c, h.config, "registration-tokens")
	metadata["expires_in"] = expiresIn

	// Default max_seats to 1 if not provided or invalid
	maxSeats := request.MaxSeats
	if maxSeats < 1 {
		maxSeats = 1
	}

	// Store token in database. For fleet-join tokens (SEC-025), the operator
	// provides the TOTP seed from the standalone host; it must be valid
	// base32 or neither an authenticator nor join validation can use it.
	var totpSeed string
	if request.FleetJoin {
		if request.TOTPSeed == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "fleet-join tokens require a TOTP seed"})
			return
		}
		if !security.ValidTOTPSeed(request.TOTPSeed) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "TOTP seed must be base32"})
			return
		}
		totpSeed = request.TOTPSeed
	}

	err = h.tokenQueries.CreateRegistrationToken(token, request.Label, expiresAt, maxSeats, metadata, totpSeed)
	if err != nil {
		log.Printf("[ERROR] [server] [registration-tokens] create_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create token"})
		return
	}

	// Build install command
	serverURL := resolveServerURL(c, h.config, "registration-tokens")
	// SEC-002: token travels in a header, never the URL — query strings land in
	// shell history, process lists, and access logs.
	installCommand := fmt.Sprintf("curl -sfL -H \"X-Registration-Token: %s\" \"%s/api/v1/install/linux\" | sudo bash", token, serverURL)

	response := gin.H{
		"token":           token,
		"label":           request.Label,
		"expires_at":      expiresAt,
		"install_command": installCommand,
		"metadata":        metadata,
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
			"page":   page,
			"limit":  limit,
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

// GetAgentsBoundToToken returns the agents that have consumed seats on a
// given registration token, for the token-detail expansion in Settings →
// Token Management. Spec: docs/AGENT_LIFECYCLE.md "Operator surfaces".
//
// Behavior: the audit ledger (registration_token_usage) is the source of
// truth. This endpoint does NOT cross-reference current agent state beyond
// the join — a deleted agent (CASCADE on agents.id) simply won't appear.
func (h *RegistrationTokenHandler) GetAgentsBoundToToken(c *gin.Context) {
	// :token in the route here is the registration_tokens.id UUID, not the
	// secret token string. Mirrors the /registration-tokens/delete/:id pattern.
	tokenID, err := uuid.FromString(c.Param("token"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token id (expected UUID)"})
		return
	}
	agents, err := h.tokenQueries.GetAgentsBoundToToken(tokenID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch bound agents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agents": agents, "count": len(agents)})
}

// RevokeRegistrationToken revokes a registration token by its UUID.
// The route param (:token) carries the row UUID from the UI — the secret
// plaintext never travels on the wire for this operation.
func (h *RegistrationTokenHandler) RevokeRegistrationToken(c *gin.Context) {
	tokenID, err := uuid.FromString(c.Param("token"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token id (expected UUID)"})
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

	if err := h.tokenQueries.RevokeRegistrationTokenByID(tokenID, reason); err != nil {
		if err.Error() == "token not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Token not found"})
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
	id, err := uuid.FromString(tokenID)
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
			"available":     h.config.AgentRegistration.MaxSeats - activeAgentCount,
		},
		"security_limits": gin.H{
			"max_tokens_per_request": h.config.AgentRegistration.MaxTokens,
			"max_token_duration":     "90 days",
			"token_expiry_default":   h.config.AgentRegistration.TokenExpiry,
		},
	}

	c.JSON(http.StatusOK, response)
}
