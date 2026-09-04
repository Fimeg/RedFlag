package handlers

import (
	"fmt"
	"net/http"

	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// SecuritySettingsHandler handles security settings API endpoints
type SecuritySettingsHandler struct {
	securitySettingsService *services.SecuritySettingsService
}

// NewSecuritySettingsHandler creates a new security settings handler
func NewSecuritySettingsHandler(securitySettingsService *services.SecuritySettingsService) *SecuritySettingsHandler {
	return &SecuritySettingsHandler{
		securitySettingsService: securitySettingsService,
	}
}

// GetAllSecuritySettings returns all security settings
func (h *SecuritySettingsHandler) GetAllSecuritySettings(c *gin.Context) {
	settings, err := h.securitySettingsService.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"settings":           settings,
		"user_has_permission": true,
	})
}

// GetSecuritySettingsByCategory returns settings for a specific category
func (h *SecuritySettingsHandler) GetSecuritySettingsByCategory(c *gin.Context) {
	category := c.Param("category")

	settings, err := h.securitySettingsService.GetSettingsByCategory(category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateSecuritySetting updates a specific security setting
func (h *SecuritySettingsHandler) UpdateSecuritySetting(c *gin.Context) {
	var req struct {
		Value  interface{} `json:"value" binding:"required"`
		Reason string      `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category := c.Param("category")
	key := c.Param("key")
	userIDStr := c.GetString("user_id")

	// Parse user_id to UUID (the service expects uuid.UUID)
	userID, err := uuid.FromString(userIDStr)
	if err != nil {
		// Fallback for numeric user IDs — use a deterministic UUID
		userID = uuid.NewV5(uuid.NamespaceURL, "user:"+userIDStr)
	}

	if err := h.securitySettingsService.ValidateSetting(category, key, req.Value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.securitySettingsService.SetSetting(category, key, req.Value, userID, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	setting, err := h.securitySettingsService.GetSetting(category, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Setting updated",
		"setting": map[string]interface{}{
			"category": category,
			"key":      key,
			"value":    setting,
		},
	})
}

// ValidateSecuritySettings validates settings without applying them
func (h *SecuritySettingsHandler) ValidateSecuritySettings(c *gin.Context) {
	var req struct {
		Category string      `json:"category" binding:"required"`
		Key      string      `json:"key" binding:"required"`
		Value    interface{} `json:"value" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.securitySettingsService.ValidateSetting(req.Category, req.Key, req.Value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"valid": true, "message": "Setting is valid"})
}

// GetSecurityAuditTrail returns audit trail of security setting changes (F-E1-7 fix)
func (h *SecuritySettingsHandler) GetSecurityAuditTrail(c *gin.Context) {
	entries, err := h.securitySettingsService.GetAuditTrail(100)
	if err != nil || entries == nil {
		c.JSON(http.StatusOK, gin.H{
			"audit_entries": []interface{}{},
			"pagination":    gin.H{"page": 1, "page_size": 50, "total": 0, "total_pages": 0},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"audit_entries": entries,
		"pagination":    gin.H{"page": 1, "page_size": 50, "total": len(entries), "total_pages": 1},
	})
}

// GetSecurityOverview returns security settings overview for the settings management page.
// Returns all settings organized by category. The dashboard security overview is served
// by SecurityHandler.SecurityOverview at /security/overview (separate endpoint).
func (h *SecuritySettingsHandler) GetSecurityOverview(c *gin.Context) {
	settings, err := h.securitySettingsService.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"overview": settings,
	})
}

// ApplySecuritySettings applies batch of setting changes
func (h *SecuritySettingsHandler) ApplySecuritySettings(c *gin.Context) {
	var req struct {
		Settings map[string]map[string]interface{} `json:"settings" binding:"required"`
		Reason   string                            `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userIDStr := c.GetString("user_id")
	userID, err := uuid.FromString(userIDStr)
	if err != nil {
		userID = uuid.NewV5(uuid.NamespaceURL, "user:"+userIDStr)
	}

	// Validate all settings first
	for category, settings := range req.Settings {
		for key, value := range settings {
			if err := h.securitySettingsService.ValidateSetting(category, key, value); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Validation failed for %s.%s: %v", category, key, err),
				})
				return
			}
		}
	}

	// Apply settings one by one (batch method not available in service)
	applied := 0
	for category, settings := range req.Settings {
		for key, value := range settings {
			if err := h.securitySettingsService.SetSetting(category, key, value, userID, req.Reason); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": fmt.Sprintf("Failed to apply %s.%s: %v", category, key, err),
				})
				return
			}
			applied++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "All settings applied",
		"applied_count": applied,
	})
}
