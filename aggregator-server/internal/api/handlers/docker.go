package handlers

import (
	"net/http"
	"strconv"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/database/queries"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DockerHandler struct {
	updateQueries  *queries.UpdateQueries
	agentQueries   *queries.AgentQueries
	commandQueries *queries.CommandQueries
}

func NewDockerHandler(uq *queries.UpdateQueries, aq *queries.AgentQueries, cq *queries.CommandQueries) *DockerHandler {
	return &DockerHandler{
		updateQueries:  uq,
		agentQueries:   aq,
		commandQueries: cq,
	}
}

// GetContainers returns Docker containers and images across all agents
func (h *DockerHandler) GetContainers(c *gin.Context) {
	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	agentID := c.Query("agent")
	status := c.Query("status")

	filters := &models.UpdateFilters{
		PackageType: "docker_image",
		Page:        page,
		PageSize:    pageSize,
		Status:      status,
	}

	// Parse agent_id if provided
	if agentID != "" {
		if parsedID, err := uuid.Parse(agentID); err == nil {
			filters.AgentID = parsedID
		}
	}

	// Get Docker updates (which represent container images)
	updates, total, err := h.updateQueries.ListUpdatesFromState(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Docker containers"})
		return
	}

	// Get agent information for better display
	agentMap := make(map[uuid.UUID]models.Agent)
	for _, update := range updates {
		if _, exists := agentMap[update.AgentID]; !exists {
			if agent, err := h.agentQueries.GetAgentByID(update.AgentID); err == nil {
				agentMap[update.AgentID] = *agent
			}
		}
	}

	// Transform updates into Docker container format
	containers := make([]models.DockerContainer, 0, len(updates))
	uniqueImages := make(map[string]bool)

	for _, update := range updates {
		// Extract container info from update metadata
		containerName := update.PackageName
		var ports []models.DockerPort

		if update.Metadata != nil {
			if name, exists := update.Metadata["container_name"]; exists {
				if nameStr, ok := name.(string); ok {
					containerName = nameStr
				}
			}

			// Extract port information from metadata
			if portsData, exists := update.Metadata["ports"]; exists {
				if portsArray, ok := portsData.([]interface{}); ok {
					for _, portData := range portsArray {
						if portMap, ok := portData.(map[string]interface{}); ok {
							port := models.DockerPort{}
							if cp, ok := portMap["container_port"].(float64); ok {
								port.ContainerPort = int(cp)
							}
							if hp, ok := portMap["host_port"].(float64); ok {
								hostPort := int(hp)
								port.HostPort = &hostPort
							}
							if proto, ok := portMap["protocol"].(string); ok {
								port.Protocol = proto
							}
							if ip, ok := portMap["host_ip"].(string); ok {
								port.HostIP = ip
							} else {
								port.HostIP = "0.0.0.0"
							}
							ports = append(ports, port)
						}
					}
				}
			}
		}

		// Get agent information
		agentInfo := agentMap[update.AgentID]

		// Create container representation
		container := models.DockerContainer{
			ID:               update.ID.String(),
			ContainerID:      containerName,
			Image:            update.PackageName,
			Tag:              update.AvailableVersion, // Available version becomes the tag
			AgentID:          update.AgentID.String(),
			AgentName:        agentInfo.Hostname,
			AgentHostname:    agentInfo.Hostname,
			Status:           update.Status,
			State:            "", // Could be extracted from metadata if available
			Ports:            ports,
			CreatedAt:        update.LastDiscoveredAt,
			UpdatedAt:        update.LastUpdatedAt,
			UpdateAvailable:  update.Status != "installed",
			CurrentVersion:   update.CurrentVersion,
			AvailableVersion: update.AvailableVersion,
		}

		// Add image to unique set
		imageKey := update.PackageName + ":" + update.AvailableVersion
		uniqueImages[imageKey] = true

		containers = append(containers, container)
	}

	response := models.DockerContainerListResponse{
		Containers:   containers,
		Images:       containers, // Alias for containers to match frontend expectation
		TotalImages:  len(uniqueImages),
		Total:        len(containers),
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   (total + pageSize - 1) / pageSize,
	}

	c.JSON(http.StatusOK, response)
}

// GetAgentContainers returns Docker containers for a specific agent
func (h *DockerHandler) GetAgentContainers(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	status := c.Query("status")

	filters := &models.UpdateFilters{
		AgentID:     agentID,
		PackageType: "docker_image",
		Page:        page,
		PageSize:    pageSize,
		Status:      status,
	}

	// Get Docker updates for specific agent
	updates, total, err := h.updateQueries.ListUpdatesFromState(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Docker containers for agent"})
		return
	}

	// Get agent information
	agentInfo, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Transform updates into Docker container format
	containers := make([]models.DockerContainer, 0, len(updates))
	uniqueImages := make(map[string]bool)

	for _, update := range updates {
		// Extract container info from update metadata
		containerName := update.PackageName
		var ports []models.DockerPort

		if update.Metadata != nil {
			if name, exists := update.Metadata["container_name"]; exists {
				if nameStr, ok := name.(string); ok {
					containerName = nameStr
				}
			}

			// Extract port information from metadata
			if portsData, exists := update.Metadata["ports"]; exists {
				if portsArray, ok := portsData.([]interface{}); ok {
					for _, portData := range portsArray {
						if portMap, ok := portData.(map[string]interface{}); ok {
							port := models.DockerPort{}
							if cp, ok := portMap["container_port"].(float64); ok {
								port.ContainerPort = int(cp)
							}
							if hp, ok := portMap["host_port"].(float64); ok {
								hostPort := int(hp)
								port.HostPort = &hostPort
							}
							if proto, ok := portMap["protocol"].(string); ok {
								port.Protocol = proto
							}
							if ip, ok := portMap["host_ip"].(string); ok {
								port.HostIP = ip
							} else {
								port.HostIP = "0.0.0.0"
							}
							ports = append(ports, port)
						}
					}
				}
			}
		}

		container := models.DockerContainer{
			ID:               update.ID.String(),
			ContainerID:      containerName,
			Image:            update.PackageName,
			Tag:              update.AvailableVersion,
			AgentID:          update.AgentID.String(),
			AgentName:        agentInfo.Hostname,
			AgentHostname:    agentInfo.Hostname,
			Status:           update.Status,
			State:            "", // Could be extracted from metadata if available
			Ports:            ports,
			CreatedAt:        update.LastDiscoveredAt,
			UpdatedAt:        update.LastUpdatedAt,
			UpdateAvailable:  update.Status != "installed",
			CurrentVersion:   update.CurrentVersion,
			AvailableVersion: update.AvailableVersion,
		}

		imageKey := update.PackageName + ":" + update.AvailableVersion
		uniqueImages[imageKey] = true

		containers = append(containers, container)
	}

	response := models.DockerContainerListResponse{
		Containers:   containers,
		Images:       containers, // Alias for containers to match frontend expectation
		TotalImages:  len(uniqueImages),
		Total:        len(containers),
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   (total + pageSize - 1) / pageSize,
	}

	c.JSON(http.StatusOK, response)
}

// GetStats returns Docker statistics across all agents
func (h *DockerHandler) GetStats(c *gin.Context) {
	// Get all Docker updates
	filters := &models.UpdateFilters{
		PackageType: "docker_image",
		Page:        1,
		PageSize:    10000, // Get all for stats
	}

	updates, _, err := h.updateQueries.ListUpdatesFromState(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Docker stats"})
		return
	}

	stats := models.DockerStats{
		TotalContainers:    len(updates),
		TotalImages:        0,
		UpdatesAvailable:   0,
		PendingApproval:    0,
		CriticalUpdates:    0,
	}

	// Calculate stats
	uniqueImages := make(map[string]bool)
	agentsWithContainers := make(map[uuid.UUID]bool)

	for _, update := range updates {
		// Count unique images
		imageKey := update.PackageName + ":" + update.AvailableVersion
		uniqueImages[imageKey] = true

		// Count agents with containers
		agentsWithContainers[update.AgentID] = true

		// Count updates available
		if update.Status != "installed" {
			stats.UpdatesAvailable++
		}

		// Count pending approval
		if update.Status == "pending_approval" {
			stats.PendingApproval++
		}

		// Count critical updates
		if update.Severity == "critical" {
			stats.CriticalUpdates++
		}
	}

	stats.TotalImages = len(uniqueImages)
	stats.AgentsWithContainers = len(agentsWithContainers)

	c.JSON(http.StatusOK, stats)
}

// ApproveUpdate approves a Docker image update
func (h *DockerHandler) ApproveUpdate(c *gin.Context) {
	containerID := c.Param("container_id")
	imageID := c.Param("image_id")

	if containerID == "" || imageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container_id and image_id are required"})
		return
	}

	// Parse the update ID from container_id (they're the same in our implementation)
	updateID, err := uuid.Parse(containerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container ID"})
		return
	}

	// Approve the update
	if err := h.updateQueries.ApproveUpdate(updateID, "admin"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve Docker update"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Docker update approved",
		"container_id": containerID,
		"image_id": imageID,
	})
}

// RejectUpdate rejects a Docker image update
func (h *DockerHandler) RejectUpdate(c *gin.Context) {
	containerID := c.Param("container_id")
	imageID := c.Param("image_id")

	if containerID == "" || imageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container_id and image_id are required"})
		return
	}

	// Parse the update ID
	updateID, err := uuid.Parse(containerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container ID"})
		return
	}

	// Get the update details to find the agent ID and package name
	update, err := h.updateQueries.GetUpdateByID(updateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	// For now, we'll mark as rejected (this would need a proper reject method in queries)
	if err := h.updateQueries.UpdatePackageStatus(update.AgentID, "docker", update.PackageName, "rejected", nil, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject Docker update"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Docker update rejected",
		"container_id": containerID,
		"image_id": imageID,
	})
}

// InstallUpdate installs a Docker image update immediately
func (h *DockerHandler) InstallUpdate(c *gin.Context) {
	containerID := c.Param("container_id")
	imageID := c.Param("image_id")

	if containerID == "" || imageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container_id and image_id are required"})
		return
	}

	// Parse the update ID
	updateID, err := uuid.Parse(containerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container ID"})
		return
	}

	// Get the update details to find the agent ID
	update, err := h.updateQueries.GetUpdateByID(updateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	// Create a command for the agent to install the update
	// This would trigger the agent to pull the new image
	command := &models.AgentCommand{
		ID:          uuid.New(),
		AgentID:     update.AgentID,
		CommandType: models.CommandTypeScanUpdates, // Reuse scan for Docker updates
		Params: models.JSONB{
			"package_type": "docker",
			"package_name": update.PackageName,
			"target_version": update.AvailableVersion,
			"container_id": containerID,
		},
		Status: models.CommandStatusPending,
	}

	if err := h.commandQueries.CreateCommand(command); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create Docker update command"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Docker update command sent",
		"container_id": containerID,
		"image_id": imageID,
		"command_id": command.ID,
	})
}