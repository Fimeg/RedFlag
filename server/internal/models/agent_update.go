package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// AgentUpdatePackage represents a signed agent binary package
type AgentUpdatePackage struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Version      string    `json:"version" db:"version"`
	Platform     string    `json:"platform" db:"platform"`
	Architecture string    `json:"architecture" db:"architecture"`
	BinaryPath   string    `json:"binary_path" db:"binary_path"`
	Signature    string    `json:"signature" db:"signature"`
	Checksum     string    `json:"checksum" db:"checksum"`
	FileSize     int64     `json:"file_size" db:"file_size"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	CreatedBy    string    `json:"created_by" db:"created_by"`
	IsActive     bool      `json:"is_active" db:"is_active"`
}

// AgentUpdateRequest represents a request to update an agent
type AgentUpdateRequest struct {
	AgentID   uuid.UUID `json:"agent_id,omitempty"`         // Optional when agent ID is in URL path
	Version   string    `json:"version" binding:"required"`
	Platform  string    `json:"platform" binding:"required"`
	Scheduled *string   `json:"scheduled_at,omitempty"`
	Nonce     string    `json:"nonce" binding:"required"` // Required security nonce to prevent replay attacks
}

// BulkAgentUpdateRequest represents a bulk update request
type BulkAgentUpdateRequest struct {
	AgentIDs  []uuid.UUID `json:"agent_ids" binding:"required"`
	Version   string      `json:"version" binding:"required"`
	Platform  string      `json:"platform" binding:"required"`
	Scheduled *string     `json:"scheduled_at,omitempty"`
}

// AgentUpdateResponse represents the response for an update request
type AgentUpdateResponse struct {
	Message        string `json:"message"`
	UpdateID       string `json:"update_id,omitempty"`
	DownloadURL    string `json:"download_url,omitempty"`
	Signature      string `json:"signature,omitempty"`
	Checksum       string `json:"checksum,omitempty"`
	FileSize       int64  `json:"file_size,omitempty"`
	EstimatedTime  int    `json:"estimated_time_seconds,omitempty"`
}

// SignatureVerificationRequest represents a request to verify an agent's binary signature
type SignatureVerificationRequest struct {
	AgentID      uuid.UUID `json:"agent_id" binding:"required"`
	BinaryPath   string    `json:"binary_path" binding:"required"`
	MachineID    string    `json:"machine_id" binding:"required"`
	PublicKey    string    `json:"public_key" binding:"required"`
	Signature    string    `json:"signature" binding:"required"`
}

// SignatureVerificationResponse represents the response for signature verification
type SignatureVerificationResponse struct {
	Valid       bool   `json:"valid"`
	AgentID     string `json:"agent_id"`
	MachineID   string `json:"machine_id"`
	Fingerprint string `json:"fingerprint"`
	Message     string `json:"message"`
}