package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Agent represents a registered update agent
type Agent struct {
	ID             uuid.UUID `json:"id" db:"id"`
	Hostname       string    `json:"hostname" db:"hostname"`
	OSType         string    `json:"os_type" db:"os_type"`
	OSVersion      string    `json:"os_version" db:"os_version"`
	OSArchitecture string    `json:"os_architecture" db:"os_architecture"`
	AgentVersion   string    `json:"agent_version" db:"agent_version"`
	LastSeen       time.Time `json:"last_seen" db:"last_seen"`
	Status         string    `json:"status" db:"status"`
	Metadata       JSONB     `json:"metadata" db:"metadata"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// AgentSpecs represents system specifications for an agent
type AgentSpecs struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	AgentID           uuid.UUID       `json:"agent_id" db:"agent_id"`
	CPUModel          string          `json:"cpu_model" db:"cpu_model"`
	CPUCores          int             `json:"cpu_cores" db:"cpu_cores"`
	MemoryTotalMB     int             `json:"memory_total_mb" db:"memory_total_mb"`
	DiskTotalGB       int             `json:"disk_total_gb" db:"disk_total_gb"`
	DiskFreeGB        int             `json:"disk_free_gb" db:"disk_free_gb"`
	NetworkInterfaces JSONB           `json:"network_interfaces" db:"network_interfaces"`
	DockerInstalled   bool            `json:"docker_installed" db:"docker_installed"`
	DockerVersion     string          `json:"docker_version" db:"docker_version"`
	PackageManagers   StringArray     `json:"package_managers" db:"package_managers"`
	CollectedAt       time.Time       `json:"collected_at" db:"collected_at"`
}

// AgentRegistrationRequest is the payload for agent registration
type AgentRegistrationRequest struct {
	Hostname       string            `json:"hostname" binding:"required"`
	OSType         string            `json:"os_type" binding:"required"`
	OSVersion      string            `json:"os_version"`
	OSArchitecture string            `json:"os_architecture"`
	AgentVersion   string            `json:"agent_version" binding:"required"`
	Metadata       map[string]string `json:"metadata"`
}

// AgentRegistrationResponse is returned after successful registration
type AgentRegistrationResponse struct {
	AgentID uuid.UUID              `json:"agent_id"`
	Token   string                 `json:"token"`
	Config  map[string]interface{} `json:"config"`
}

// JSONB type for PostgreSQL JSONB columns
type JSONB map[string]interface{}

// Value implements driver.Valuer for database storage
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner for database retrieval
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// StringArray type for PostgreSQL text[] columns
type StringArray []string

// Value implements driver.Valuer
func (s StringArray) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan implements sql.Scanner
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, s)
}
