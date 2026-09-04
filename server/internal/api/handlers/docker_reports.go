package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// DockerReportsHandler handles Docker image reports from agents
type DockerReportsHandler struct {
	dockerQueries  *queries.DockerQueries
	agentQueries   *queries.AgentQueries
	commandQueries *queries.CommandQueries
}

func NewDockerReportsHandler(dq *queries.DockerQueries, aq *queries.AgentQueries, cq *queries.CommandQueries) *DockerReportsHandler {
	return &DockerReportsHandler{
		dockerQueries:  dq,
		agentQueries:   aq,
		commandQueries: cq,
	}
}

// ReportDockerImages handles Docker image reports from agents using event sourcing
func (h *DockerReportsHandler) ReportDockerImages(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	var req models.DockerReportRequest
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

	// Convert Docker images to events
	events := make([]models.StoredDockerImage, 0, len(req.Images))
	for _, item := range req.Images {
		// Extract image_name and image_tag from the package_name (format: "name:tag" or just "name")
		imageName := item.PackageName
		imageTag := "latest"
		if idx := strings.LastIndex(item.PackageName, ":"); idx >= 0 {
			imageName = item.PackageName[:idx]
			imageTag = item.PackageName[idx+1:]
		}
		// Metadata also carries image_name/image_tag from the agent; use those
		// as the authoritative source when present.
		if metaName, ok := item.Metadata["image_name"].(string); ok && metaName != "" {
			imageName = metaName
		}
		if metaTag, ok := item.Metadata["image_tag"].(string); ok && metaTag != "" {
			imageTag = metaTag
		}

		// Extract ports from container metadata
		if ports, ok := item.Metadata["ports"]; ok {
			if portsList, ok := ports.([]interface{}); ok {
				item.Metadata["port_count"] = len(portsList)
			}
		}
		if containerNames, ok := item.Metadata["container_names"]; ok {
			if names, ok := containerNames.([]interface{}); ok && len(names) > 0 {
				item.Metadata["container_name"] = names[0]
			}
		}

		event := models.StoredDockerImage{
			ID:               uuid.Must(uuid.NewV4()),
			AgentID:          agentID,
			PackageType:      item.PackageType,
			PackageName:      imageName + ":" + imageTag,
			CurrentVersion:   item.CurrentVersion,
			AvailableVersion: item.AvailableVersion,
			Severity:         item.Severity,
			RepositorySource: item.RepositorySource,
			Metadata:         convertToJSONB(item.Metadata),
			EventType:        "discovered",
			CreatedAt:        req.Timestamp,
		}
		events = append(events, event)
	}

	// Store events in batch with error isolation
	if err := h.dockerQueries.CreateDockerEventsBatch(events); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record docker image events"})
		return
	}

	// Store enriched data: containers, stacks, engine version
	if len(req.Containers) > 0 {
		if err := h.dockerQueries.UpsertContainers(agentID, req.Containers); err != nil {
			log.Printf("[WARNING] [server] [docker] container_upsert_failed agent=%s error=%v", agentID, err)
		}
	}
	if len(req.Stacks) > 0 {
		if err := h.dockerQueries.UpsertStacks(agentID, req.Stacks); err != nil {
			log.Printf("[WARNING] [server] [docker] stack_upsert_failed agent=%s error=%v", agentID, err)
		}
	}
	if req.EngineVersion != "" {
		if err := h.dockerQueries.UpdateDockerEngineVersion(agentID, req.EngineVersion); err != nil {
			log.Printf("[WARNING] [server] [docker] engine_version_update_failed agent=%s error=%v", agentID, err)
		}
	}

	// Command finalization is owned by ReportLog — the single point that closes
	// the command AND writes the [HISTORY] row. Completing it here too would mark
	// the command terminal, then 409 the agent's history-bearing ReportLog and drop
	// docker-scan events from the History page. Storage and dnf already leave the
	// command open for ReportLog; this keeps docker consistent with them.
	c.JSON(http.StatusOK, gin.H{
		"message":    "docker image events recorded",
		"count":      len(events),
		"command_id": req.CommandID,
	})
}

// GetAgentDockerImages retrieves Docker image updates for a specific agent
func (h *DockerReportsHandler) GetAgentDockerImages(c *gin.Context) {
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

	imageName := c.Query("image_name")
	registry := c.Query("registry")
	severity := c.Query("severity")
	hasUpdatesStr := c.Query("has_updates")

	// Build filter
	filter := &models.DockerFilter{
		AgentID:    &agentID,
		ImageName:  nil,
		Registry:   nil,
		Severity:   nil,
		HasUpdates: nil,
		Limit:      &pageSize,
		Offset:     &(offset),
	}

	if imageName != "" {
		filter.ImageName = &imageName
	}
	if registry != "" {
		filter.Registry = &registry
	}
	if severity != "" {
		filter.Severity = &severity
	}
	if hasUpdatesStr != "" {
		hasUpdates := hasUpdatesStr == "true"
		filter.HasUpdates = &hasUpdates
	}

	// Fetch Docker images
	result, err := h.dockerQueries.GetDockerImages(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch docker images"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAgentDockerInfo retrieves detailed Docker information for an agent
func (h *DockerReportsHandler) GetAgentDockerInfo(c *gin.Context) {
	agentIDStr := c.Param("agentId")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Get all Docker images for this agent
	pageSize := 100
	offset := 0

	filter := &models.DockerFilter{
		AgentID: &agentID,
		Limit:   &pageSize,
		Offset:  &offset,
	}

	result, err := h.dockerQueries.GetDockerImages(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch docker images"})
		return
	}

	// Convert to detailed format
	dockerInfo := make([]models.DockerImageInfo, 0, len(result.Images))
	for _, image := range result.Images {
		imageName := extractName(image.PackageName)
		imageTag := extractTag(image.PackageName)
		// Fallback to metadata for legacy rows where PackageName was stored as ":"
		if imageName == "" || imageName == ":" {
			if metaName, ok := image.Metadata["image_name"].(string); ok && metaName != "" {
				imageName = metaName
			}
		}
		if imageTag == "" || imageTag == "latest" {
			if metaTag, ok := image.Metadata["image_tag"].(string); ok && metaTag != "" {
				imageTag = metaTag
			}
		}

		// Extract ports from metadata
		ports := extractPorts(image.Metadata)

		info := models.DockerImageInfo{
			ID:               image.ID.String(),
			AgentID:          image.AgentID.String(),
			ImageName:        imageName,
			ImageTag:         imageTag,
			ImageID:          image.CurrentVersion,
			RepositorySource: image.RepositorySource,
			SizeBytes:        parseImageSize(image.Metadata),
			CreatedAt:        image.CreatedAt.Format(time.RFC3339),
			HasUpdate:        image.AvailableVersion != image.CurrentVersion && image.AvailableVersion != "",
			LatestImageID:    image.AvailableVersion,
			Severity:         image.Severity,
			Labels:           extractLabels(image.Metadata),
			Metadata:         convertInterfaceMapToJSONB(image.Metadata),
			PackageType:      image.PackageType,
			CurrentVersion:   image.CurrentVersion,
			AvailableVersion: image.AvailableVersion,
			EventType:        image.EventType,
			CreatedAtTime:    image.CreatedAt,
			Ports:            ports,
		}
		dockerInfo = append(dockerInfo, info)
	}

	c.JSON(http.StatusOK, gin.H{
		"docker_images": dockerInfo,
		"is_live":       isDockerRecentlyUpdated(result.Images),
		"total":         len(dockerInfo),
		"updates_available": countUpdates(dockerInfo),
	})
}

// Helper function to extract ports from metadata
func extractPorts(metadata models.JSONB) []models.DockerPort {
	if portsRaw, ok := metadata["ports"]; ok {
		if portsList, ok := portsRaw.([]interface{}); ok {
			ports := make([]models.DockerPort, 0, len(portsList))
			for _, p := range portsList {
				if portMap, ok := p.(map[string]interface{}); ok {
					port := models.DockerPort{}
					if ip, ok := portMap["ip"].(string); ok {
						port.HostIP = ip
					}
					if pub, ok := portMap["public_port"]; ok {
						switch v := pub.(type) {
						case float64:
							hp := int(v)
							port.HostPort = &hp
						case int:
							hp := v
							port.HostPort = &hp
						}
					}
					if priv, ok := portMap["private_port"]; ok {
						switch v := priv.(type) {
						case float64:
							port.ContainerPort = int(v)
						case int:
							port.ContainerPort = v
						}
					}
					if ptype, ok := portMap["type"].(string); ok {
						port.Protocol = ptype
					}
					ports = append(ports, port)
				}
			}
			return ports
		}
	}
	return nil
}

// Helper function to extract name from image name
func extractName(imageName string) string {
	// Simple implementation - split by ":" and return everything except last part
	parts := strings.Split(imageName, ":")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], ":")
	}
	return imageName
}

// Helper function to extract tag from image name
func extractTag(imageName string) string {
	// Simple implementation - split by ":" and return last part
	parts := strings.Split(imageName, ":")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return "latest"
}

// Helper function to parse image size from metadata
func parseImageSize(metadata models.JSONB) int64 {
	// Check if size is stored in metadata
	if sizeStr, ok := metadata["size"].(string); ok {
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			return size
		}
	}
	return 0
}

// Helper function to extract labels from metadata
func extractLabels(metadata models.JSONB) map[string]string {
	labels := make(map[string]string)
	if labelsData, ok := metadata["labels"].(map[string]interface{}); ok {
		for k, v := range labelsData {
			if str, ok := v.(string); ok {
				labels[k] = str
			}
		}
	}
	return labels
}

// Helper function to check if Docker images are recently updated
func isDockerRecentlyUpdated(images []models.StoredDockerImage) bool {
	if len(images) == 0 {
		return false
	}

	// Check if any image was updated in the last 5 minutes
	now := time.Now().UTC()
	for _, image := range images {
		if now.Sub(image.CreatedAt) < 5*time.Minute {
			return true
		}
	}
	return false
}

// Helper function to count available updates
func countUpdates(images []models.DockerImageInfo) int {
	count := 0
	for _, image := range images {
		if image.HasUpdate {
			count++
		}
	}
	return count
}

// Helper function to convert map[string]interface{} to models.JSONB
func convertToJSONB(data map[string]interface{}) models.JSONB {
	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = v
	}
	return models.JSONB(result)
}

// Helper function to convert map[string]interface{} to models.JSONB
func convertInterfaceMapToJSONB(data models.JSONB) models.JSONB {
	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = v
	}
	return models.JSONB(result)
}

// GetAgentDockerContainers returns containers for a specific agent.
func (h *DockerReportsHandler) GetAgentDockerContainers(c *gin.Context) {
	agentIDStr := c.Param("agentId")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	containers, err := h.dockerQueries.GetDockerContainers(agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch containers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"containers": containers, "total": len(containers)})
}

// GetAgentDockerStacks returns stacks for a specific agent.
func (h *DockerReportsHandler) GetAgentDockerStacks(c *gin.Context) {
	agentIDStr := c.Param("agentId")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	stacks, err := h.dockerQueries.GetDockerStacks(agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch stacks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"stacks": stacks, "total": len(stacks)})
}

// GetDockerContainersFleet returns containers across all agents.
func (h *DockerReportsHandler) GetDockerContainersFleet(c *gin.Context) {
	containers, err := h.dockerQueries.GetDockerContainersFleet()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch fleet containers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"containers": containers, "total": len(containers)})
}

// GetDockerStacksFleet returns stacks across all agents.
func (h *DockerReportsHandler) GetDockerStacksFleet(c *gin.Context) {
	stacks, err := h.dockerQueries.GetDockerStacksFleet()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch fleet stacks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"stacks": stacks, "total": len(stacks)})
}

