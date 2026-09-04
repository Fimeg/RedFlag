package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gin-gonic/gin"
)

// GlobalEventsHandler serves the fleet-wide event feed for the notification bell.
type GlobalEventsHandler struct {
	agentQueries *queries.AgentQueries
}

// NewGlobalEventsHandler creates a GlobalEventsHandler.
func NewGlobalEventsHandler(aq *queries.AgentQueries) *GlobalEventsHandler {
	return &GlobalEventsHandler{agentQueries: aq}
}

// GetGlobalEvents returns recent system events across all agents.
// GET /api/v1/events?severity=error,critical,warning&limit=50
func (h *GlobalEventsHandler) GetGlobalEvents(c *gin.Context) {
	severity := c.Query("severity")
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	events, err := h.agentQueries.GetGlobalEvents(severity, limit)
	if err != nil {
		log.Printf("ERROR: Failed to fetch global events: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch events"})
		return
	}

	// Populate narrative for each event — UI consumes rendered text.
	for i := range events {
		events[i].Narrative = services.RenderSystemEvent(
			events[i].EventType,
			events[i].EventSubtype,
			events[i].Message,
			events[i].Metadata,
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"total":  len(events),
	})
}
