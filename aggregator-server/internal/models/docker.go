package models

import (
	"time"
)

// DockerPort represents a port mapping in a Docker container
type DockerPort struct {
	ContainerPort int    `json:"container_port"`
	HostPort      *int   `json:"host_port,omitempty"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"host_ip"`
}

// DockerContainer represents a Docker container with its image information
type DockerContainer struct {
	ID               string       `json:"id"`
	ContainerID      string       `json:"container_id"`
	Image            string       `json:"image"`
	Tag              string       `json:"tag"`
	AgentID          string       `json:"agent_id"`
	AgentName        string       `json:"agent_name,omitempty"`
	AgentHostname    string       `json:"agent_hostname,omitempty"`
	Status           string       `json:"status"`
	State            string       `json:"state,omitempty"`
	Ports            []DockerPort `json:"ports,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	UpdateAvailable  bool         `json:"update_available"`
	CurrentVersion   string       `json:"current_version,omitempty"`
	AvailableVersion string       `json:"available_version,omitempty"`
}

// DockerContainerListResponse represents the response for container listing
type DockerContainerListResponse struct {
	Containers   []DockerContainer `json:"containers"`
	Images       []DockerContainer `json:"images"`       // Alias for containers to match frontend expectation
	TotalImages  int               `json:"total_images"`
	Total        int               `json:"total"`
	Page         int               `json:"page"`
	PageSize     int               `json:"page_size"`
	TotalPages   int               `json:"total_pages"`
}

// DockerImage represents a Docker image
type DockerImage struct {
	ID          string    `json:"id"`
	Repository  string    `json:"repository"`
	Tag         string    `json:"tag"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	AgentID     string    `json:"agent_id"`
	AgentName   string    `json:"agent_name,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion string  `json:"current_version"`
	AvailableVersion string `json:"available_version"`
}

// DockerStats represents Docker statistics across all agents
type DockerStats struct {
	TotalContainers     int    `json:"total_containers"`
	TotalImages         int    `json:"total_images"`
	UpdatesAvailable    int    `json:"updates_available"`
	PendingApproval     int    `json:"pending_approval"`
	CriticalUpdates     int    `json:"critical_updates"`
	AgentsWithContainers int   `json:"agents_with_containers"`
}

// DockerUpdateRequest represents a request to update Docker images
type DockerUpdateRequest struct {
	ContainerID string `json:"container_id" binding:"required"`
	ImageID     string `json:"image_id" binding:"required"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// BulkDockerUpdateRequest represents a bulk update request for Docker images
type BulkDockerUpdateRequest struct {
	Updates     []struct {
		ContainerID string `json:"container_id" binding:"required"`
		ImageID     string `json:"image_id" binding:"required"`
	} `json:"updates" binding:"required"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}