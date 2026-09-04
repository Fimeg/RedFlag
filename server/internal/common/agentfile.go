package common

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"
)

type AgentFile struct {
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	ModifiedTime time.Time `json:"modified_time"`
	Version      string    `json:"version,omitempty"`
	Checksum     string    `json:"checksum"`
	Required     bool      `json:"required"`
	Migrate      bool      `json:"migrate"`
	Description  string    `json:"description"`
}

// CalculateChecksum computes SHA256 checksum of a file
func CalculateChecksum(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// IsRequiredFile determines if a file is required for agent operation
func IsRequiredFile(path string) bool {
	requiredFiles := []string{
		"/etc/redflag/config.json",
		"/usr/local/bin/redflag-agent",
		"/etc/systemd/system/redflag-agent.service",
	}
	for _, rf := range requiredFiles {
		if path == rf {
			return true
		}
	}
	return false
}
