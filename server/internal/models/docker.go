package models

import (
	"time"
	"github.com/gofrs/uuid/v5"
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
	Severity         string       `json:"severity,omitempty"`
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
	Containers  []DockerContainer `json:"containers"`
	Images      []DockerImage     `json:"images"`
	TotalImages int               `json:"total_images"`
	Total       int               `json:"total"`
	Page        int               `json:"page"`
	PageSize    int               `json:"page_size"`
	TotalPages  int               `json:"total_pages"`
}

// DockerImage represents a Docker image
type DockerImage struct {
	ID              string    `json:"id"`
	Repository      string    `json:"repository"`
	Tag             string    `json:"tag"`
	Size            int64     `json:"size"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	AgentID         string    `json:"agent_id"`
	AgentName       string    `json:"agent_name,omitempty"`
	Severity        string    `json:"severity,omitempty"`
	UpdateAvailable bool      `json:"update_available"`
	CurrentVersion  string    `json:"current_version"`
	AvailableVersion string  `json:"available_version"`
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

// AgentDockerImage represents a Docker image as sent by the agent.
// Fields match the agent's client.DockerReportItem JSON tags so the
// payload deserializes correctly.
type AgentDockerImage struct {
	PackageType      string                 `json:"package_type"`
	PackageName      string                 `json:"package_name"`
	CurrentVersion   string                 `json:"current_version"`
	AvailableVersion string                 `json:"available_version"`
	Severity         string                 `json:"severity"`
	RepositorySource string                 `json:"repository_source"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// DockerReportRequest is sent by agents when reporting Docker image updates
type DockerReportRequest struct {
	CommandID     string                   `json:"command_id"`
	Timestamp     time.Time                `json:"timestamp"`
	Images        []AgentDockerImage       `json:"images"`
	Containers    []AgentDockerContainer   `json:"containers,omitempty"`
	Stacks        []AgentDockerStack       `json:"stacks,omitempty"`
	EngineVersion string                   `json:"engine_version,omitempty"`
}

// AgentDockerContainer represents a container reported by an agent.
type AgentDockerContainer struct {
	ContainerID string            `json:"container_id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	ImageID     string            `json:"image_id"`
	State       string            `json:"state"`
	Health      string            `json:"health"`
	StackName   string            `json:"stack_name"`
	Ports       string            `json:"ports"`
	CreatedAt   int64             `json:"created_at"`
	Labels      map[string]string `json:"labels"`
}

// AgentDockerStack represents a compose stack derived from container labels.
type AgentDockerStack struct {
	Name           string `json:"name"`
	ContainerCount int    `json:"container_count"`
	RunningCount   int    `json:"running_count"`
}

// StoredDockerContainer is a container row persisted in the docker_containers table.
type StoredDockerContainer struct {
	ID          uuid.UUID `json:"id" db:"id"`
	AgentID     uuid.UUID `json:"agent_id" db:"agent_id"`
	ContainerID string    `json:"container_id" db:"container_id"`
	Name        string    `json:"name" db:"name"`
	Image       string    `json:"image" db:"image"`
	ImageID     string    `json:"image_id" db:"image_id"`
	State       string    `json:"state" db:"state"`
	Health      string    `json:"health" db:"health"`
	StackName   string    `json:"stack_name" db:"stack_name"`
	Ports       string    `json:"ports" db:"ports"`
	CreatedAt   int64     `json:"created_at_epoch" db:"created_at_epoch"`
	Labels      JSONB     `json:"labels" db:"labels"`
	LastSeenAt  time.Time `json:"last_seen_at" db:"last_seen_at"`
}

// StoredDockerStack is a stack row persisted in the docker_stacks table.
type StoredDockerStack struct {
	ID             uuid.UUID `json:"id" db:"id"`
	AgentID        uuid.UUID `json:"agent_id" db:"agent_id"`
	Name           string    `json:"name" db:"name"`
	ContainerCount int       `json:"container_count" db:"container_count"`
	RunningCount   int       `json:"running_count" db:"running_count"`
	LastSeenAt     time.Time `json:"last_seen_at" db:"last_seen_at"`
}

// DockerImageInfo represents detailed Docker image information for API responses
type DockerImageInfo struct {
	ID                string                 `json:"id"`
	AgentID           string                 `json:"agent_id"`
	ImageName         string                 `json:"image_name"`
	ImageTag          string                 `json:"image_tag"`
	ImageID           string                 `json:"image_id"`
	RepositorySource  string                 `json:"repository_source"`
	SizeBytes         int64                  `json:"size_bytes"`
	CreatedAt         string                 `json:"created_at"`
	HasUpdate         bool                   `json:"has_update"`
	LatestImageID     string                 `json:"latest_image_id"`
	Severity          string                 `json:"severity"`
	Labels            map[string]string      `json:"labels"`
	Metadata          map[string]interface{} `json:"metadata"`
	PackageType       string                 `json:"package_type"`
	CurrentVersion    string                 `json:"current_version"`
	AvailableVersion  string                 `json:"available_version"`
	EventType         string                 `json:"event_type"`
	CreatedAtTime     time.Time              `json:"created_at_time"`
	Ports             []DockerPort           `json:"ports,omitempty"`
	ContainerName     string                 `json:"container_name,omitempty"`
}

// DockerImageUpdate represents a Docker image update from agent scans
type DockerImageUpdate struct {
	PackageType      string            `json:"package_type"`      // "docker_image"
	PackageName      string            `json:"package_name"`      // image name:tag
	CurrentVersion   string            `json:"current_version"`   // current image ID
	AvailableVersion string            `json:"available_version"` // latest image ID
	Severity         string            `json:"severity"`          // "low", "moderate", "high", "critical"
	RepositorySource string            `json:"repository_source"` // registry URL
	Metadata         map[string]string `json:"metadata"`
}

// StoredDockerImage represents a Docker image update in the database
type StoredDockerImage struct {
	ID               uuid.UUID         `json:"id" db:"id"`
	AgentID          uuid.UUID         `json:"agent_id" db:"agent_id"`
	PackageType      string            `json:"package_type" db:"package_type"`
	PackageName      string            `json:"package_name" db:"package_name"`
	CurrentVersion   string            `json:"current_version" db:"current_version"`
	AvailableVersion string            `json:"available_version" db:"available_version"`
	Severity         string            `json:"severity" db:"severity"`
	RepositorySource string            `json:"repository_source" db:"repository_source"`
	Metadata         JSONB             `json:"metadata" db:"metadata"`
	EventType        string            `json:"event_type" db:"event_type"`
	CreatedAt        time.Time         `json:"created_at" db:"created_at"`
}

// DockerFilter represents filtering options for Docker image queries
type DockerFilter struct {
	AgentID       *uuid.UUID `json:"agent_id,omitempty"`
	ImageName     *string    `json:"image_name,omitempty"`
	Registry      *string    `json:"registry,omitempty"`
	Severity      *string    `json:"severity,omitempty"`
	HasUpdates    *bool      `json:"has_updates,omitempty"`
	Limit         *int       `json:"limit,omitempty"`
	Offset        *int       `json:"offset,omitempty"`
}

// DockerResult represents the result of a Docker image query
type DockerResult struct {
	Images  []StoredDockerImage `json:"images"`
	Total   int                 `json:"total"`
	Page    int                 `json:"page"`
	PerPage int                 `json:"per_page"`
}