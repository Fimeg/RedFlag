package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// StorageMetricsHandler handles storage metrics endpoints
type StorageMetricsHandler struct {
	queries *queries.StorageMetricsQueries
}

// NewStorageMetricsHandler creates a new storage metrics handler
func NewStorageMetricsHandler(queries *queries.StorageMetricsQueries) *StorageMetricsHandler {
	return &StorageMetricsHandler{
		queries: queries,
	}
}

// ReportStorageMetrics handles POST /api/v1/agents/:id/storage-metrics
func (h *StorageMetricsHandler) ReportStorageMetrics(c *gin.Context) {
	// Get agent ID from context (set by middleware)
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Parse request body
	var req models.StorageMetricRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate agent ID matches
	if req.AgentID != agentID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Agent ID mismatch"})
		return
	}

	// Insert storage metrics with error isolation
	for _, metric := range req.Metrics {
		dbMetric := models.StorageMetric{
			ID:             uuid.Must(uuid.NewV4()),
			AgentID:        req.AgentID,
			Mountpoint:     metric.Mountpoint,
			Device:         metric.Device,
			DiskType:       metric.DiskType,
			Filesystem:     metric.Filesystem,
			TotalBytes:     metric.TotalBytes,
			UsedBytes:      metric.UsedBytes,
			AvailableBytes: metric.AvailableBytes,
			UsedPercent:    metric.UsedPercent,
			Severity:       metric.Severity,
			Metadata:       metric.Metadata,
			CreatedAt:      time.Now().UTC(),
		}

		if err := h.queries.InsertStorageMetric(c.Request.Context(), dbMetric); err != nil {
			log.Printf("[ERROR] Failed to insert storage metric for agent %s: %v\n", agentID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert storage metric"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Storage metrics reported successfully",
	})
}

// StorageMetricResponse represents the response format for storage metrics
 type StorageMetricResponse struct {
	ID             uuid.UUID              `json:"id"`
	AgentID        uuid.UUID              `json:"agent_id"`
	Mountpoint     string                 `json:"mountpoint"`
	Device         string                 `json:"device"`
	DiskType       string                 `json:"disk_type"`
	Filesystem     string                 `json:"filesystem"`
	Total          int64                  `json:"total"`           // Changed from total_bytes
	Used           int64                  `json:"used"`            // Changed from used_bytes
	Available      int64                  `json:"available"`       // Changed from available_bytes
	UsedPercent    float64                `json:"used_percent"`
	Severity       string                 `json:"severity"`
	IsRoot         bool                   `json:"is_root"`
	IsLargest      bool                   `json:"is_largest"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// GetStorageMetrics handles GET /api/v1/agents/:id/storage-metrics
func (h *StorageMetricsHandler) GetStorageMetrics(c *gin.Context) {
	// Get agent ID from URL parameter (this is a dashboard endpoint, not agent endpoint)
	agentIDStr := c.Param("id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		log.Printf("[ERROR] Invalid agent ID %s: %v\n", agentIDStr, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	// Get the latest storage metrics (one per mountpoint)
	latestMetrics, err := h.queries.GetLatestStorageMetrics(c.Request.Context(), agentID)
	if err != nil {
		log.Printf("[ERROR] Failed to retrieve storage metrics for agent %s: %v\n", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve storage metrics"})
		return
	}

	// Transform to response format
	var responseMetrics []StorageMetricResponse
	for _, metric := range latestMetrics {
		// Check if this is the root mountpoint
		isRoot := metric.Mountpoint == "/"

		// Create response with fields matching frontend expectations
		responseMetric := StorageMetricResponse{
			ID:          metric.ID,
			AgentID:     metric.AgentID,
			Mountpoint:  metric.Mountpoint,
			Device:      metric.Device,
			DiskType:    metric.DiskType,
			Filesystem:  metric.Filesystem,
			Total:       metric.TotalBytes,     // Map total_bytes -> total
			Used:        metric.UsedBytes,      // Map used_bytes -> used
			Available:   metric.AvailableBytes, // Map available_bytes -> available
			UsedPercent: metric.UsedPercent,
			Severity:    metric.Severity,
			IsRoot:      isRoot,
			IsLargest:   false, // Will be determined below
			Metadata:    metric.Metadata,
			CreatedAt:   metric.CreatedAt,
		}
		responseMetrics = append(responseMetrics, responseMetric)
	}

	// Determine which disk is the largest
	if len(responseMetrics) > 0 {
		var maxSize int64
		var maxIndex int
		for i, metric := range responseMetrics {
			if metric.Total > maxSize {
				maxSize = metric.Total
				maxIndex = i
			}
		}
		// Mark the largest disk
		responseMetrics[maxIndex].IsLargest = true
	}

	c.JSON(http.StatusOK, gin.H{
		"metrics": responseMetrics,
		"total":   len(responseMetrics),
	})
}
