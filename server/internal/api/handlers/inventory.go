package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// InventoryHandler handles agent inventory reporting and queries.
type InventoryHandler struct {
	inventoryQueries *queries.InventoryQueries
	agentQueries     *queries.AgentQueries
}

// NewInventoryHandler creates a new inventory handler.
func NewInventoryHandler(iq *queries.InventoryQueries, aq *queries.AgentQueries) *InventoryHandler {
	return &InventoryHandler{
		inventoryQueries: iq,
		agentQueries:     aq,
	}
}

// ReportInventory handles POST /api/v1/agents/:id/inventory
// Accepts inventory items from an agent and upserts them into agent_inventory.
func (h *InventoryHandler) ReportInventory(c *gin.Context) {
	agentIDStr := c.Param("id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req models.InventoryReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(req.Items) == 0 {
		c.JSON(http.StatusOK, gin.H{"accepted": 0, "message": "no items to report"})
		return
	}

	// Convert wire-format items to database model
	items := make([]models.AgentInventoryItem, 0, len(req.Items))
	for _, itemReq := range req.Items {
		item := models.AgentInventoryItem{
			ID:                 uuid.Must(uuid.NewV4()),
			AgentID:            agentID,
			InventoryEcosystem: req.Ecosystem,
			ItemName:           itemReq.ItemName,
			ItemVersion:        itemReq.ItemVersion,
			Description:        itemReq.Description,
			Arch:               itemReq.Arch,
			SizeBytes:          itemReq.SizeBytes,
			Vendor:             itemReq.Vendor,
			Metadata:           models.JSONB(itemReq.Metadata),
		}

		// Parse install_time if provided
		if itemReq.InstallTime != "" {
			if t, err := time.Parse(time.RFC3339, itemReq.InstallTime); err == nil {
				item.InstallTime = &t
			}
		}

		items = append(items, item)
	}

	// Upsert all items
	if err := h.inventoryQueries.UpsertBatch(agentID, items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to upsert inventory: %s", err.Error())})
		return
	}

	// Update agent last_seen
	_ = h.agentQueries.UpdateAgentLastSeen(agentID)

	c.JSON(http.StatusOK, gin.H{
		"accepted":       len(items),
		"ecosystem":      req.Ecosystem,
		"scan_succeeded": req.ScanSucceeded,
	})
}

// GetAgentInventory handles GET /api/v1/agents/:id/inventory
// Returns inventory items for an agent with optional ecosystem filter.
func (h *InventoryHandler) GetAgentInventory(c *gin.Context) {
	agentIDStr := c.Param("id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	ecosystem := c.Query("ecosystem")

	limit := 100
	offset := 0
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := c.Query("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	items, total, err := h.inventoryQueries.GetByAgent(agentID, ecosystem, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query inventory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetFleetInventory handles GET /api/v1/inventory
// Returns a fleet-wide inventory summary.
func (h *InventoryHandler) GetFleetInventory(c *gin.Context) {
	summaries, err := h.inventoryQueries.GetFleetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query fleet inventory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"summary": summaries})
}
