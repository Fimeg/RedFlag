package handlers

import (
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/database/queries"
	"github.com/gin-gonic/gin"
)

// StatsHandler handles statistics for the dashboard
type StatsHandler struct {
	agentQueries  *queries.AgentQueries
	updateQueries *queries.UpdateQueries
}

// NewStatsHandler creates a new stats handler
func NewStatsHandler(agentQueries *queries.AgentQueries, updateQueries *queries.UpdateQueries) *StatsHandler {
	return &StatsHandler{
		agentQueries:  agentQueries,
		updateQueries: updateQueries,
	}
}

// DashboardStats represents dashboard statistics
type DashboardStats struct {
	TotalAgents      int            `json:"total_agents"`
	OnlineAgents     int            `json:"online_agents"`
	OfflineAgents    int            `json:"offline_agents"`
	PendingUpdates   int            `json:"pending_updates"`
	FailedUpdates    int            `json:"failed_updates"`
	CriticalUpdates  int            `json:"critical_updates"`
	ImportantUpdates int            `json:"important_updates"`
	ModerateUpdates  int            `json:"moderate_updates"`
	LowUpdates       int            `json:"low_updates"`
	UpdatesByType    map[string]int `json:"updates_by_type"`
}

// GetDashboardStats returns dashboard statistics using the new state table
func (h *StatsHandler) GetDashboardStats(c *gin.Context) {
	// Get all agents
	agents, err := h.agentQueries.ListAgents("", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get agents"})
		return
	}

	// Calculate stats
	stats := DashboardStats{
		TotalAgents:    len(agents),
		UpdatesByType:  make(map[string]int),
	}

	// Count online/offline agents based on last_seen timestamp
	for _, agent := range agents {
		// Consider agent online if it has checked in within the last 10 minutes
		if time.Since(agent.LastSeen) <= 10*time.Minute {
			stats.OnlineAgents++
		} else {
			stats.OfflineAgents++
		}

		// Get update stats for each agent using the new state table
		agentStats, err := h.updateQueries.GetUpdateStatsFromState(agent.ID)
		if err != nil {
			// Log error but continue with other agents
			continue
		}

		// Aggregate stats across all agents
		stats.PendingUpdates += agentStats.PendingUpdates
		stats.FailedUpdates += agentStats.FailedUpdates
		stats.CriticalUpdates += agentStats.CriticalUpdates
		stats.ImportantUpdates += agentStats.ImportantUpdates
		stats.ModerateUpdates += agentStats.ModerateUpdates
		stats.LowUpdates += agentStats.LowUpdates
	}

	c.JSON(http.StatusOK, stats)
}