package handlers

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// DownloadHandler handles agent binary downloads
type DownloadHandler struct {
	agentDir string
}

func NewDownloadHandler(agentDir string) *DownloadHandler {
	return &DownloadHandler{
		agentDir: agentDir,
	}
}

// DownloadAgent serves agent binaries for different platforms
func (h *DownloadHandler) DownloadAgent(c *gin.Context) {
	platform := c.Param("platform")

	// Validate platform to prevent directory traversal
	validPlatforms := map[string]bool{
		"linux-amd64":   true,
		"linux-arm64":   true,
		"windows-amd64": true,
		"windows-arm64": true,
		"darwin-amd64":  true,
		"darwin-arm64":  true,
	}

	if !validPlatforms[platform] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid platform"})
		return
	}

	filename := "redflag-agent"
	if strings.HasPrefix(platform, "windows") {
		filename += ".exe"
	}

	agentPath := filepath.Join(h.agentDir, filename)
	c.File(agentPath)
}

// InstallScript serves the installation script
func (h *DownloadHandler) InstallScript(c *gin.Context) {
	platform := c.Param("platform")

	// Validate platform
	validPlatforms := map[string]bool{
		"linux": true,
		"darwin": true,
		"windows": true,
	}

	if !validPlatforms[platform] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid platform"})
		return
	}

	scriptContent := h.generateInstallScript(platform, c.Request.Host)
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, scriptContent)
}

func (h *DownloadHandler) generateInstallScript(platform, serverHost string) string {
	baseURL := "http://" + serverHost

	switch platform {
	case "linux":
		return `#!/bin/bash
set -e

REDFLAG_SERVER="` + baseURL + `"
AGENT_DIR="/usr/local/bin"
SERVICE_NAME="redflag-agent"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root or with sudo"
    exit 1
fi

echo "Installing RedFlag agent from ${REDFLAG_SERVER}..."

# Download agent
curl -sfL "${REDFLAG_SERVER}/api/v1/downloads/linux-amd64" -o "${AGENT_DIR}/redflag-agent"
chmod +x "${AGENT_DIR}/redflag-agent"

echo "Agent downloaded. Please visit ${REDFLAG_SERVER}/admin to get a registration token."
echo "Then run: ${AGENT_DIR}/redflag-agent --server ${REDFLAG_SERVER} --token <YOUR_TOKEN>"`

	case "darwin":
		return `#!/bin/bash
set -e

REDFLAG_SERVER="` + baseURL + `"
AGENT_DIR="/usr/local/bin"

echo "Installing RedFlag agent from ${REDFLAG_SERVER}..."

# Download agent
curl -sfL "${REDFLAG_SERVER}/api/v1/downloads/darwin-amd64" -o "${AGENT_DIR}/redflag-agent"
chmod +x "${AGENT_DIR}/redflag-agent"

echo "Agent downloaded. Please visit ${REDFLAG_SERVER}/admin to get a registration token."
echo "Then run: ${AGENT_DIR}/redflag-agent --server ${REDFLAG_SERVER} --token <YOUR_TOKEN>"`

	case "windows":
		return `@echo off
set REDFLAG_SERVER=` + baseURL + `

echo Downloading RedFlag agent from %REDFLAG_SERVER%...
curl -sfL "%REDFLAG_SERVER%/api/v1/downloads/windows-amd64" -o redflag-agent.exe

echo Agent downloaded. Please visit %REDFLAG_SERVER%/admin to get a registration token.
echo Then run: redflag-agent.exe --server %REDFLAG_SERVER% --token <YOUR_TOKEN%`

	default:
		return "# Unsupported platform"
	}
}