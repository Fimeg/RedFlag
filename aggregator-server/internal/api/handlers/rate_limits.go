package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/aggregator-project/aggregator-server/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

type RateLimitHandler struct {
	rateLimiter *middleware.RateLimiter
}

func NewRateLimitHandler(rateLimiter *middleware.RateLimiter) *RateLimitHandler {
	return &RateLimitHandler{
		rateLimiter: rateLimiter,
	}
}

// GetRateLimitSettings returns current rate limit configuration
func (h *RateLimitHandler) GetRateLimitSettings(c *gin.Context) {
	settings := h.rateLimiter.GetSettings()
	c.JSON(http.StatusOK, gin.H{
		"settings": settings,
		"updated_at": time.Now(),
	})
}

// UpdateRateLimitSettings updates rate limit configuration
func (h *RateLimitHandler) UpdateRateLimitSettings(c *gin.Context) {
	var settings middleware.RateLimitSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	// Validate settings
	if err := h.validateRateLimitSettings(settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update rate limiter settings
	h.rateLimiter.UpdateSettings(settings)

	c.JSON(http.StatusOK, gin.H{
		"message": "Rate limit settings updated successfully",
		"settings": settings,
		"updated_at": time.Now(),
	})
}

// ResetRateLimitSettings resets to default values
func (h *RateLimitHandler) ResetRateLimitSettings(c *gin.Context) {
	defaultSettings := middleware.DefaultRateLimitSettings()
	h.rateLimiter.UpdateSettings(defaultSettings)

	c.JSON(http.StatusOK, gin.H{
		"message": "Rate limit settings reset to defaults",
		"settings": defaultSettings,
		"updated_at": time.Now(),
	})
}

// GetRateLimitStats returns current rate limit statistics
func (h *RateLimitHandler) GetRateLimitStats(c *gin.Context) {
	settings := h.rateLimiter.GetSettings()

	// Calculate total requests and windows
	stats := gin.H{
		"total_configured_limits": 6,
		"enabled_limits": 0,
		"total_requests_per_minute": 0,
		"settings": settings,
	}

	// Count enabled limits and total requests
	for _, config := range []middleware.RateLimitConfig{
		settings.AgentRegistration,
		settings.AgentCheckIn,
		settings.AgentReports,
		settings.AdminTokenGen,
		settings.AdminOperations,
		settings.PublicAccess,
	} {
		if config.Enabled {
			stats["enabled_limits"] = stats["enabled_limits"].(int) + 1
		}
		stats["total_requests_per_minute"] = stats["total_requests_per_minute"].(int) + config.Requests
	}

	c.JSON(http.StatusOK, stats)
}

// CleanupRateLimitEntries manually triggers cleanup of expired entries
func (h *RateLimitHandler) CleanupRateLimitEntries(c *gin.Context) {
	h.rateLimiter.CleanupExpiredEntries()

	c.JSON(http.StatusOK, gin.H{
		"message": "Rate limit entries cleanup completed",
		"timestamp": time.Now(),
	})
}

// validateRateLimitSettings validates the provided rate limit settings
func (h *RateLimitHandler) validateRateLimitSettings(settings middleware.RateLimitSettings) error {
	// Validate each configuration
	validations := []struct {
		name   string
		config middleware.RateLimitConfig
	}{
		{"agent_registration", settings.AgentRegistration},
		{"agent_checkin", settings.AgentCheckIn},
		{"agent_reports", settings.AgentReports},
		{"admin_token_generation", settings.AdminTokenGen},
		{"admin_operations", settings.AdminOperations},
		{"public_access", settings.PublicAccess},
	}

	for _, validation := range validations {
		if validation.config.Requests <= 0 {
			return fmt.Errorf("%s: requests must be greater than 0", validation.name)
		}
		if validation.config.Window <= 0 {
			return fmt.Errorf("%s: window must be greater than 0", validation.name)
		}
		if validation.config.Window > 24*time.Hour {
			return fmt.Errorf("%s: window cannot exceed 24 hours", validation.name)
		}
		if validation.config.Requests > 1000 {
			return fmt.Errorf("%s: requests cannot exceed 1000 per window", validation.name)
		}
	}

	// Specific validations for different endpoint types
	if settings.AgentRegistration.Requests > 10 {
		return fmt.Errorf("agent_registration: requests should not exceed 10 per minute for security")
	}
	if settings.PublicAccess.Requests > 50 {
		return fmt.Errorf("public_access: requests should not exceed 50 per minute for security")
	}

	return nil
}