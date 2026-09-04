package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

type AgentEventsHandler struct {
	agentQueries *queries.AgentQueries
}

func NewAgentEventsHandler(aq *queries.AgentQueries) *AgentEventsHandler {
	return &AgentEventsHandler{agentQueries: aq}
}

// GetAgentEvents returns system events for an agent with optional filtering
// GET /api/v1/agents/:id/events?severity=error,critical,warning&limit=50
func (h *AgentEventsHandler) GetAgentEvents(c *gin.Context) {
	agentIDStr := c.Param("id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Optional query parameters
	severity := c.Query("severity") // comma-separated filter: error,critical,warning,info
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000 // Cap at 1000 to prevent excessive queries
	}

	// Get events using the agent queries
	events, err := h.agentQueries.GetAgentEvents(agentID, severity, limit)
	if err != nil {
		log.Printf("ERROR: Failed to fetch agent events: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch events"})
		return
	}

	// Populate narrative for each event before responding — UI consumes the
	// rendered text and never composes its own from raw enum names.
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