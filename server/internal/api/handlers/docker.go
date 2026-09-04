package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/Fimeg/RedFlag/server/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

type DockerHandler struct {
	dockerQueries  *queries.DockerQueries
	agentQueries   *queries.AgentQueries
	commandQueries *queries.CommandQueries
	updateQueries  *queries.UpdateQueries // kept for ApproveUpdate/RejectUpdate which read from current_package_state
	signingService *services.SigningService
	securityLogger *logging.SecurityLogger
}

func NewDockerHandler(dq *queries.DockerQueries, uq *queries.UpdateQueries, aq *queries.AgentQueries, cq *queries.CommandQueries, signingService *services.SigningService, securityLogger *logging.SecurityLogger) *DockerHandler {
	return &DockerHandler{
		dockerQueries:  dq,
		updateQueries:  uq,
		agentQueries:   aq,
		commandQueries: cq,
		signingService: signingService,
		securityLogger: securityLogger,
	}
}

// signAndCreateCommand signs a command before storing.
// STRICT MODE: Commands without signatures are rejected (ETHOS #2 Security is Non-Negotiable)
func (h *DockerHandler) signAndCreateCommand(cmd *models.AgentCommand) error {
	// STRICT MODE: If signing service disabled, FAIL FAST
	if h.signingService == nil || !h.signingService.IsEnabled() {
		err := fmt.Errorf("signing service not available - command rejected")
		log.Printf("[ERROR] [server] [signing] command_rejected reason=%q type=%s",
			err, cmd.CommandType)
		if h.securityLogger != nil {
			h.securityLogger.LogUnsignedCommandRejected(cmd)
		}
		return err // DO NOT store unsigned command
	}

	// Sign the command
	signature, err := h.signingService.SignCommand(cmd)
	if err != nil {
		log.Printf("[ERROR] [server] [signing] command_sign_failed error=%q type=%s",
			err, cmd.CommandType)
		return fmt.Errorf("failed to sign command: %w", err)
	}
	cmd.Signature = signature

	// Log successful signing
	if h.securityLogger != nil {
		h.securityLogger.LogCommandSigned(cmd)
	}

	// Store in database
	err = h.commandQueries.CreateCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to create command: %w", err)
	}

	log.Printf("[INFO] [server] [command] created_signed_command id=%s type=%s",
		cmd.ID, cmd.CommandType)
	return nil
}

// GetContainers returns Docker containers and images across all agents
func (h *DockerHandler) GetContainers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	agentID := c.Query("agent")
	search := c.Query("search")
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	filter := &models.DockerFilter{
		Limit:  &pageSize,
		Offset: &offset,
	}

	if agentID != "" {
		if parsedID, err := uuid.FromString(agentID); err == nil {
			filter.AgentID = &parsedID
		}
	}
	if search != "" {
		filter.ImageName = &search
	}
	if status != "" {
		filter.Severity = &status // reused; "status" in docker page means severity or update-state
	}

	result, err := h.dockerQueries.GetDockerImages(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Docker containers"})
		return
	}

	agentMap := make(map[uuid.UUID]models.Agent)
	containers := make([]models.DockerContainer, 0, len(result.Images))
	images := make([]models.DockerImage, 0, len(result.Images))

	for _, img := range result.Images {
		if _, exists := agentMap[img.AgentID]; !exists {
			if agent, err := h.agentQueries.GetAgentByID(img.AgentID); err == nil {
				agentMap[img.AgentID] = *agent
			}
		}
		agentInfo := agentMap[img.AgentID]

		imageName, imageTag := splitImageName(img.PackageName)
		repo := img.RepositorySource
		if repo == "" {
			repo = imageName
		}

		containerName := imageName
		var ports []models.DockerPort
		if img.Metadata != nil {
			if cn, ok := img.Metadata["container_name"].(string); ok && cn != "" {
				containerName = cn
			}
			if cNames, ok := img.Metadata["container_names"].([]interface{}); ok && len(cNames) > 0 {
				if first, ok := cNames[0].(string); ok {
					containerName = strings.TrimPrefix(first, "/")
				}
			}
			if portsData, ok := img.Metadata["ports"].([]interface{}); ok {
				for _, pd := range portsData {
					if pm, ok := pd.(map[string]interface{}); ok {
						p := models.DockerPort{HostIP: "0.0.0.0"}
						if cp, ok := pm["container_port"].(float64); ok {
							p.ContainerPort = int(cp)
						}
						if hp, ok := pm["host_port"].(float64); ok {
							v := int(hp)
							p.HostPort = &v
						}
						if proto, ok := pm["protocol"].(string); ok {
							p.Protocol = proto
						}
						if ip, ok := pm["host_ip"].(string); ok {
							p.HostIP = ip
						}
						ports = append(ports, p)
					}
				}
			}
		}

		hasUpdate := img.CurrentVersion != img.AvailableVersion && img.AvailableVersion != ""
		container := models.DockerContainer{
			ID:               img.ID.String(),
			ContainerID:      containerName,
			Image:            imageName,
			Tag:              imageTag,
			AgentID:          img.AgentID.String(),
			AgentName:        agentInfo.Hostname,
			AgentHostname:    agentInfo.Hostname,
			Status:           img.EventType,
			Severity:         img.Severity,
			State:            "",
			Ports:            ports,
			CreatedAt:        img.CreatedAt,
			UpdatedAt:        img.CreatedAt,
			UpdateAvailable:  hasUpdate,
			CurrentVersion:   img.CurrentVersion,
			AvailableVersion: img.AvailableVersion,
		}
		containers = append(containers, container)

		sizeBytes := int64(0)
		if sb, ok := img.Metadata["size_bytes"].(float64); ok {
			sizeBytes = int64(sb)
		}

		image := models.DockerImage{
			ID:               img.ID.String(),
			Repository:       repo,
			Tag:              imageTag,
			Size:             sizeBytes,
			CreatedAt:        img.CreatedAt,
			UpdatedAt:        img.CreatedAt,
			AgentID:          img.AgentID.String(),
			AgentName:        agentInfo.Hostname,
			UpdateAvailable:  hasUpdate,
			CurrentVersion:   img.CurrentVersion,
			AvailableVersion: img.AvailableVersion,
		}
		images = append(images, image)
	}

	response := models.DockerContainerListResponse{
		Containers:  containers,
		Images:      images,
		TotalImages: len(images),
		Total:       result.Total,
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  (result.Total + pageSize - 1) / pageSize,
	}

	c.JSON(http.StatusOK, response)
}

// GetAgentContainers returns Docker containers for a specific agent
func (h *DockerHandler) GetAgentContainers(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	images, err := h.dockerQueries.GetDockerImagesByAgentID(agentID, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Docker containers for agent"})
		return
	}

	agentInfo, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	containers := make([]models.DockerContainer, 0, len(images))
	dockerImages := make([]models.DockerImage, 0, len(images))

	for _, img := range images {
		imageName, imageTag := splitImageName(img.PackageName)
		hasUpdate := img.CurrentVersion != img.AvailableVersion && img.AvailableVersion != ""

		container := models.DockerContainer{
			ID:               img.ID.String(),
			Image:            imageName,
			Tag:              imageTag,
			AgentID:          img.AgentID.String(),
			AgentName:        agentInfo.Hostname,
			AgentHostname:    agentInfo.Hostname,
			Status:           img.EventType,
			Severity:         img.Severity,
			CreatedAt:        img.CreatedAt,
			UpdatedAt:        img.CreatedAt,
			UpdateAvailable:  hasUpdate,
			CurrentVersion:   img.CurrentVersion,
			AvailableVersion: img.AvailableVersion,
		}
		containers = append(containers, container)

		sizeBytes := int64(0)
		if sb, ok := img.Metadata["size_bytes"].(float64); ok {
			sizeBytes = int64(sb)
		}
		di := models.DockerImage{
			ID:               img.ID.String(),
			Repository:       img.RepositorySource,
			Tag:              imageTag,
			Size:             sizeBytes,
			CreatedAt:        img.CreatedAt,
			UpdatedAt:        img.CreatedAt,
			AgentID:          img.AgentID.String(),
			AgentName:        agentInfo.Hostname,
			UpdateAvailable:  hasUpdate,
			CurrentVersion:   img.CurrentVersion,
			AvailableVersion: img.AvailableVersion,
		}
		dockerImages = append(dockerImages, di)
	}

	response := models.DockerContainerListResponse{
		Containers:  containers,
		Images:      dockerImages,
		TotalImages: len(dockerImages),
		Total:       len(containers),
		Page:        1,
		PageSize:    100,
		TotalPages:  1,
	}

	c.JSON(http.StatusOK, response)
}

// GetStats returns Docker statistics across all agents
func (h *DockerHandler) GetStats(c *gin.Context) {
	stats, err := h.dockerQueries.GetDockerStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Docker stats"})
		return
	}

	response := models.DockerStats{
		TotalContainers:     0, // not tracked in docker_images
		TotalImages:         stats.TotalImages,
		UpdatesAvailable:    stats.UpdatesAvailable,
		PendingApproval:     0, // not applicable for Docker flow
		CriticalUpdates:     stats.CriticalUpdates,
		AgentsWithContainers: stats.AgentsWithContainers,
	}

	c.JSON(http.StatusOK, response)
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
	updateID, err := uuid.FromString(containerID)
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
	updateID, err := uuid.FromString(containerID)
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

	if err := h.updateQueries.UpdatePackageStatus(update.AgentID, "docker", update.PackageName, models.StatusIgnored, nil, nil); err != nil {
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
	updateID, err := uuid.FromString(containerID)
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
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     update.AgentID,
		CommandType: models.CommandTypeInstallUpdate, // Install Docker image update
		Params: models.JSONB{
			"package_type": "docker",
			"package_name": update.PackageName,
			"target_version": update.AvailableVersion,
			"container_id": containerID,
		},
		Status: models.CommandStatusPending,
		Source: models.CommandSourceManual, // User-initiated Docker update
	}

	if err := h.signAndCreateCommand(command); err != nil {
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

// splitImageName splits "image:tag" into name and tag parts.
// If no colon, the whole string is the name and tag defaults to "latest".
func splitImageName(full string) (name, tag string) {
	idx := strings.LastIndex(full, ":")
	if idx == -1 {
		return full, "latest"
	}
	return full[:idx], full[idx+1:]
}