package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// AgentTrackedSoftwareHandler exposes the per-agent binding CRUD.
// Bindings link an agent to a tracked_software entry so upstream drift
// becomes per-host actionable. All routes mount under the existing admin
// group; auth is enforced by WebAuthMiddleware on the parent group.
type AgentTrackedSoftwareHandler struct {
	bindings *queries.AgentTrackedSoftwareQueries
	upstream *queries.UpstreamQueries
}

func NewAgentTrackedSoftwareHandler(b *queries.AgentTrackedSoftwareQueries, u *queries.UpstreamQueries) *AgentTrackedSoftwareHandler {
	return &AgentTrackedSoftwareHandler{
		bindings: b,
		upstream: u,
	}
}

// ListByAgent returns the bindings + joined tracked_software metadata for one agent.
// GET /admin/agents/:id/tracked-software
func (h *AgentTrackedSoftwareHandler) ListByAgent(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent id"})
		return
	}
	rows, err := h.bindings.ListByAgent(agentID)
	if err != nil {
		log.Printf("[ERROR] [server] [agent_tracked_software] list_by_agent agent=%s err=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list bindings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"bindings": rows})
}

// ListInstallations returns the agents that have the given tracked_software bound.
// GET /admin/upstream/:id/installations
func (h *AgentTrackedSoftwareHandler) ListInstallations(c *gin.Context) {
	softwareID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tracked_software id"})
		return
	}
	rows, err := h.bindings.ListInstallations(softwareID)
	if err != nil {
		log.Printf("[ERROR] [server] [agent_tracked_software] list_installations software=%s err=%v", softwareID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list installations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"installations": rows})
}

// Upsert creates or updates the binding for (agent, tracked_software).
// POST /admin/agents/:id/tracked-software
func (h *AgentTrackedSoftwareHandler) Upsert(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent id"})
		return
	}
	var in models.AgentTrackedSoftwareInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	// Confirm the tracked_software exists before we let an operator bind to a
	// dangling UUID. FK would catch this but the error message is friendlier here.
	if _, err := h.upstream.GetByID(in.TrackedSoftwareID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tracked_software not found"})
		return
	}
	row, err := h.bindings.Upsert(agentID, in)
	if err != nil {
		log.Printf("[ERROR] [server] [agent_tracked_software] upsert agent=%s software=%s err=%v",
			agentID, in.TrackedSoftwareID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Roll the binding's installed_version up into tracked_software.current_version
	// so the global drift panel reflects what's actually deployed.
	if err := h.bindings.RecomputeCurrentVersion(in.TrackedSoftwareID); err != nil {
		log.Printf("[WARN] [server] [agent_tracked_software] recompute_current_version software=%s err=%v",
			in.TrackedSoftwareID, err)
	}
	c.JSON(http.StatusOK, row)
}

// Delete removes a binding by id, scoped to the agent.
// DELETE /admin/agents/:id/tracked-software/:bindingID
func (h *AgentTrackedSoftwareHandler) Delete(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent id"})
		return
	}
	bindingID, err := uuid.FromString(c.Param("bindingID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding id"})
		return
	}
	// Look up the parent software_id before deleting so we can recompute after.
	prior, err := h.bindings.GetBinding(agentID, bindingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "binding not found"})
			return
		}
		log.Printf("[ERROR] [server] [agent_tracked_software] get_binding agent=%s binding=%s err=%v",
			agentID, bindingID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up binding"})
		return
	}
	if err := h.bindings.Delete(agentID, bindingID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "binding not found"})
			return
		}
		log.Printf("[ERROR] [server] [agent_tracked_software] delete agent=%s binding=%s err=%v",
			agentID, bindingID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete binding"})
		return
	}
	if err := h.bindings.RecomputeCurrentVersion(prior.TrackedSoftwareID); err != nil {
		log.Printf("[WARN] [server] [agent_tracked_software] recompute_current_version software=%s err=%v",
			prior.TrackedSoftwareID, err)
	}
	c.Status(http.StatusNoContent)
}
