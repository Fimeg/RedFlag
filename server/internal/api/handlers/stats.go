package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/gin-gonic/gin"
)

// StatsHandler handles statistics for the dashboard
type StatsHandler struct {
	agentQueries    *queries.AgentQueries
	updateQueries   *queries.UpdateQueries
	checkInInterval int // seconds; used for online/offline threshold
}

// NewStatsHandler creates a new stats handler
func NewStatsHandler(agentQueries *queries.AgentQueries, updateQueries *queries.UpdateQueries, checkInInterval int) *StatsHandler {
	if checkInInterval <= 0 {
		checkInInterval = 300 // default 5 minutes
	}
	return &StatsHandler{
		agentQueries:    agentQueries,
		updateQueries:   updateQueries,
		checkInInterval: checkInInterval,
	}
}

// DashboardStats represents dashboard statistics
type DashboardStats struct {
	TotalAgents       int                 `json:"total_agents"`
	OnlineAgents      int                 `json:"online_agents"`
	OfflineAgents     int                 `json:"offline_agents"`
	TotalUpdates      int                 `json:"total_updates"`
	PendingUpdates    int                 `json:"pending_updates"`
	FailedUpdates     int                 `json:"failed_updates"`
	AvailableFixCount int                 `json:"available_fix_count"`   // distinct advisories on available version (remediation)
	OpenThreatCount   int                 `json:"open_threat_count"`     // distinct advisories on installed version (threat)
	TopThreats        []queries.TopThreat `json:"top_threats,omitempty"` // worst open threats, KEV then CVSS order
	CriticalUpdates   int                 `json:"critical_updates"`
	ImportantUpdates  int                 `json:"high_updates"`
	ModerateUpdates   int                 `json:"medium_updates"`
	LowUpdates        int                 `json:"low_updates"`
	UpdatesByType     map[string]int      `json:"updates_by_type"`
}

// GetDashboardStats returns dashboard statistics using aggregate queries (F-B1-6 fix)
func (h *StatsHandler) GetDashboardStats(c *gin.Context) {
	// Get all agents for online/offline count
	agents, err := h.agentQueries.ListAgents("", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get agents"})
		return
	}

	stats := DashboardStats{
		TotalAgents:   len(agents),
		UpdatesByType: make(map[string]int),
	}

	// Count online/offline agents using check-in interval (2x for threshold).
	threshold := time.Duration(h.checkInInterval*2) * time.Second
	for _, agent := range agents {
		if time.Since(agent.LastSeen) <= threshold {
			stats.OnlineAgents++
		} else {
			stats.OfflineAgents++
		}
	}

	// Single aggregate query for all update stats (replaces N+1 per-agent loop)
	updateStats, err := h.updateQueries.GetAllUpdateStats()
	if err == nil {
		stats.TotalUpdates = updateStats.TotalUpdates
		stats.PendingUpdates = updateStats.PendingUpdates
		stats.FailedUpdates = updateStats.FailedUpdates
		stats.CriticalUpdates = updateStats.CriticalUpdates
		stats.ImportantUpdates = updateStats.ImportantUpdates
		stats.ModerateUpdates = updateStats.ModerateUpdates
		stats.LowUpdates = updateStats.LowUpdates
	} else {
		log.Printf("[ERROR] [server] [stats] all_update_stats_failed: %v", err)
	}

	// Update counts grouped by package type — drives the "Updates by Type" card.
	if byType, err := h.updateQueries.GetUpdatesByType(); err == nil {
		stats.UpdatesByType = byType
	} else {
		log.Printf("[ERROR] [server] [stats] updates_by_type_failed: %v", err)
	}

	// Remediation count — distinct advisories on available version.
	if n, err := h.updateQueries.GetAvailableFixCount(); err == nil {
		stats.AvailableFixCount = n
	} else {
		log.Printf("[ERROR] [server] [stats] available_fix_count_failed: %v", err)
	}
	// Threat count — distinct advisories on installed version.
	if n, err := h.updateQueries.GetOpenThreatCount(); err == nil {
		stats.OpenThreatCount = n
	} else {
		log.Printf("[ERROR] [server] [stats] open_threat_count_failed: %v", err)
	}
	if stats.OpenThreatCount > 0 {
		if top, err := h.updateQueries.GetTopOpenThreats(3); err == nil {
			stats.TopThreats = top
		} else {
			log.Printf("[ERROR] [server] [stats] top_open_threats_failed: %v", err)
		}
	}

	c.JSON(http.StatusOK, stats)
}
