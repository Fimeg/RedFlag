package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// MetricsHandler handles system and storage metrics
type MetricsHandler struct {
	metricsQueries *queries.MetricsQueries
	agentQueries   *queries.AgentQueries
	commandQueries *queries.CommandQueries
}

func NewMetricsHandler(mq *queries.MetricsQueries, aq *queries.AgentQueries, cq *queries.CommandQueries) *MetricsHandler {
	return &MetricsHandler{
		metricsQueries: mq,
		agentQueries:   aq,
		commandQueries: cq,
	}
}

// ReportMetrics handles metrics reports from agents using event sourcing
func (h *MetricsHandler) ReportMetrics(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	var req models.MetricsReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate command exists and belongs to agent
	commandID, err := uuid.FromString(req.CommandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command ID format"})
		return
	}

	command, err := h.commandQueries.GetCommandByID(commandID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "command not found"})
		return
	}

	if command.AgentID != agentID {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized command"})
		return
	}

	// Convert metrics to events
	events := make([]models.StoredMetric, 0, len(req.Metrics))
	for _, item := range req.Metrics {
		event := models.StoredMetric{
			ID:               uuid.Must(uuid.NewV4()),
			AgentID:          agentID,
			PackageType:      item.PackageType,
			PackageName:      item.PackageName,
			CurrentVersion:   item.CurrentVersion,
			AvailableVersion: item.AvailableVersion,
			Severity:         item.Severity,
			RepositorySource: item.RepositorySource,
			Metadata:         convertStringMapToJSONB(item.Metadata),
			EventType:        "discovered",
			CreatedAt:        req.Timestamp,
		}
		events = append(events, event)
	}

	// Store events in batch with error isolation
	if err := h.metricsQueries.CreateMetricsEventsBatch(events); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record metrics events"})
		return
	}

	// Command finalization is owned by ReportLog — the single point that closes
	// the command AND writes the [HISTORY] row. Completing it here too would mark
	// the command terminal, then 409 the agent's history-bearing ReportLog and drop
	// system-scan events from the History page. Storage and dnf already leave the
	// command open for ReportLog; this keeps system consistent with them.
	c.JSON(http.StatusOK, gin.H{
		"message":    "metrics events recorded",
		"count":      len(events),
		"command_id": req.CommandID,
	})
}

// GetAgentMetrics retrieves metrics for a specific agent
func (h *MetricsHandler) GetAgentMetrics(c *gin.Context) {
	agentIDStr := c.Param("agentId")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize

	packageType := c.Query("package_type")
	severity := c.Query("severity")

	// Build filter
	filter := &models.MetricFilter{
		AgentID:     &agentID,
		PackageType: nil,
		Severity:    nil,
		Limit:       &pageSize,
		Offset:      &offset,
	}

	if packageType != "" {
		filter.PackageType = &packageType
	}
	if severity != "" {
		filter.Severity = &severity
	}

	// Fetch metrics
	result, err := h.metricsQueries.GetMetrics(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch metrics"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAgentStorageMetrics retrieves storage metrics for a specific agent
func (h *MetricsHandler) GetAgentStorageMetrics(c *gin.Context) {
	agentIDStr := c.Param("agentId")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Filter for storage metrics only
	packageType := "storage"
	pageSize := 100 // Get all storage metrics
	offset := 0

	filter := &models.MetricFilter{
		AgentID:     &agentID,
		PackageType: &packageType,
		Limit:       &pageSize,
		Offset:      &offset,
	}

	result, err := h.metricsQueries.GetMetrics(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch storage metrics"})
		return
	}

	// Convert to storage-specific format
	storageMetrics := make([]models.StorageMetrics, 0, len(result.Metrics))
	for _, metric := range result.Metrics {
		storageMetric := models.StorageMetrics{
			MountPoint:    metric.PackageName,
			TotalBytes:    parseBytes(metric.AvailableVersion), // Available version stores total
			UsedBytes:     parseBytes(metric.CurrentVersion),   // Current version stores used
			UsedPercent:   calculateUsagePercent(parseBytes(metric.CurrentVersion), parseBytes(metric.AvailableVersion)),
			Status:        metric.Severity,
			LastUpdated:   metric.CreatedAt,
		}
		storageMetrics = append(storageMetrics, storageMetric)
	}

	c.JSON(http.StatusOK, gin.H{
		"storage_metrics": storageMetrics,
		"is_live":        isRecentlyUpdated(result.Metrics),
	})
}

// GetAgentSystemMetrics retrieves system metrics for a specific agent
func (h *MetricsHandler) GetAgentSystemMetrics(c *gin.Context) {
	agentIDStr := c.Param("agentId")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Filter for system metrics only
	packageType := "system"
	pageSize := 100
	offset := 0

	filter := &models.MetricFilter{
		AgentID:     &agentID,
		PackageType: &packageType,
		Limit:       &pageSize,
		Offset:      &offset,
	}

	result, err := h.metricsQueries.GetMetrics(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch system metrics"})
		return
	}

	// Aggregate system metrics
	systemMetrics := aggregateSystemMetrics(result.Metrics)

	c.JSON(http.StatusOK, gin.H{
		"system_metrics": systemMetrics,
		"is_live":        isRecentlyUpdated(result.Metrics),
	})
}

// Helper function to parse bytes from string
func parseBytes(s string) int64 {
	// Simple implementation - in real code, parse "10GB", "500MB", etc.
	// For now, return 0 if parsing fails
	return 0
}

// Helper function to calculate usage percentage
func calculateUsagePercent(used, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

// Helper function to check if metrics are recently updated
func isRecentlyUpdated(metrics []models.StoredMetric) bool {
	if len(metrics) == 0 {
		return false
	}

	// Check if any metric was updated in the last 5 minutes
	now := time.Now().UTC()
	for _, metric := range metrics {
		if now.Sub(metric.CreatedAt) < 5*time.Minute {
			return true
		}
	}
	return false
}

// Helper function to aggregate system metrics
func aggregateSystemMetrics(metrics []models.StoredMetric) *models.SystemMetrics {
	if len(metrics) == 0 {
		return nil
	}

	// Aggregate the most recent metrics
	// This is a simplified implementation - real code would need proper aggregation
	return &models.SystemMetrics{
		CPUModel:      "Unknown",
		CPUCores:      0,
		CPUThreads:    0,
		MemoryTotal:   0,
		MemoryUsed:    0,
		MemoryPercent: 0,
		Processes:     0,
		Uptime:        "Unknown",
		LoadAverage:   []float64{0, 0, 0},
		LastUpdated:   metrics[0].CreatedAt,
	}
}

// Helper function to convert map[string]string to models.JSONB
func convertStringMapToJSONB(data map[string]string) models.JSONB {
	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = v
	}
	return models.JSONB(result)
}