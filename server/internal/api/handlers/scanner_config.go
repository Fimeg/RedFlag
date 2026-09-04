package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// ScannerConfigHandler manages scanner timeout configuration
type ScannerConfigHandler struct {
	queries *queries.ScannerConfigQueries
}

// NewScannerConfigHandler creates a new scanner config handler
func NewScannerConfigHandler(db *sqlx.DB) *ScannerConfigHandler {
	return &ScannerConfigHandler{
		queries: queries.NewScannerConfigQueries(db),
	}
}

// GetScannerTimeouts returns current scanner timeout configuration
// GET /api/v1/admin/scanner-timeouts
// Security: Requires admin authentication (WebAuthMiddleware)
func (h *ScannerConfigHandler) GetScannerTimeouts(c *gin.Context) {
	configs, err := h.queries.GetAllScannerConfigs()
	if err != nil {
		log.Printf("[ERROR] Failed to fetch scanner configs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to fetch scanner configuration",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"scanner_timeouts": configs,
		"default_timeout_ms": 1800000, // 30 minutes default
	})
}

// UpdateScannerTimeout updates scanner timeout configuration
// PUT /api/v1/admin/scanner-timeouts/:scanner_name
// Security: Requires admin authentication + audit logging
func (h *ScannerConfigHandler) UpdateScannerTimeout(c *gin.Context) {
	scannerName := c.Param("scanner_name")
	if scannerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "scanner_name is required",
		})
		return
	}

	var req struct {
		TimeoutMs int `json:"timeout_ms" binding:"required,min=1000,max=7200000"` // 1s to 2 hours
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond

	// Update config
	if err := h.queries.UpsertScannerConfig(scannerName, timeout); err != nil {
		log.Printf("[ERROR] Failed to update scanner config for %s: %v", scannerName, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update scanner configuration",
		})
		return
	}

	// Create audit event in History table (ETHOS compliance)
	// user_id is stored as a string by WebAuthMiddleware
	userID := c.GetString("user_id")
	/*
	event := &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		EventType:    "scanner_config_change",
		EventSubtype: "timeout_updated",
		Severity:     "info",
		Component:    "admin_api",
		Message:      fmt.Sprintf("Scanner timeout updated: %s = %v", scannerName, timeout),
		Metadata: map[string]interface{}{
			"scanner_name": scannerName,
			"timeout_ms":   req.TimeoutMs,
			"user_id":      userID.String(),
			"source_ip":    c.ClientIP(),
		},
		CreatedAt: time.Now().UTC(),
	}
	// TODO: Integrate with event logging system when available
	*/
	log.Printf("[AUDIT] User %s updated scanner timeout: %s = %v", userID, scannerName, timeout)

	c.JSON(http.StatusOK, gin.H{
		"message":      "scanner timeout updated successfully",
		"scanner_name": scannerName,
		"timeout_ms":   req.TimeoutMs,
		"timeout_human": timeout.String(),
	})
}

// ResetScannerTimeout resets scanner timeout to default (30 minutes)
// POST /api/v1/admin/scanner-timeouts/:scanner_name/reset
// Security: Requires admin authentication + audit logging
func (h *ScannerConfigHandler) ResetScannerTimeout(c *gin.Context) {
	scannerName := c.Param("scanner_name")
	if scannerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "scanner_name is required",
		})
		return
	}

	defaultTimeout := 30 * time.Minute

	if err := h.queries.UpsertScannerConfig(scannerName, defaultTimeout); err != nil {
		log.Printf("[ERROR] Failed to reset scanner config for %s: %v", scannerName, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to reset scanner configuration",
		})
		return
	}

	// Audit log
	userID := c.GetString("user_id")
	log.Printf("[AUDIT] User %s reset scanner timeout: %s to default %v", userID, scannerName, defaultTimeout)

	c.JSON(http.StatusOK, gin.H{
		"message":        "scanner timeout reset to default",
		"scanner_name":   scannerName,
		"timeout_ms":     int(defaultTimeout.Milliseconds()),
		"timeout_human":  defaultTimeout.String(),
	})
}

// GetScannerConfigQueries provides access to the queries for config_builder.go
func (h *ScannerConfigHandler) GetScannerConfigQueries() *queries.ScannerConfigQueries {
	return h.queries
}
