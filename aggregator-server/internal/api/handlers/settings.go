package handlers

import (
	"net/http"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/services"
	"github.com/gin-gonic/gin"
)

type SettingsHandler struct {
	timezoneService *services.TimezoneService
}

func NewSettingsHandler(timezoneService *services.TimezoneService) *SettingsHandler {
	return &SettingsHandler{
		timezoneService: timezoneService,
	}
}

// GetTimezones returns available timezone options
func (h *SettingsHandler) GetTimezones(c *gin.Context) {
	timezones := h.timezoneService.GetAvailableTimezones()
	c.JSON(http.StatusOK, gin.H{"timezones": timezones})
}

// GetTimezone returns the current timezone configuration
func (h *SettingsHandler) GetTimezone(c *gin.Context) {
	// TODO: Get from user settings when implemented
	// For now, return the server timezone
	c.JSON(http.StatusOK, gin.H{
		"timezone": "UTC",
		"label":    "UTC (Coordinated Universal Time)",
	})
}

// UpdateTimezone updates the timezone configuration
func (h *SettingsHandler) UpdateTimezone(c *gin.Context) {
	var req struct {
		Timezone string `json:"timezone" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Save to user settings when implemented
	// For now, just validate it's a valid timezone
	timezones := h.timezoneService.GetAvailableTimezones()
	valid := false
	for _, tz := range timezones {
		if tz.Value == req.Timezone {
			valid = true
			break
		}
	}

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid timezone"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "timezone updated",
		"timezone": req.Timezone,
	})
}