package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/config"
	"github.com/gin-gonic/gin"
)

// DownloadHandler handles agent binary downloads
type DownloadHandler struct {
	agentDir string
	config   *config.Config
}

func NewDownloadHandler(agentDir string, cfg *config.Config) *DownloadHandler {
	return &DownloadHandler{
		agentDir: agentDir,
		config:   cfg,
	}
}

// getServerURL determines the server URL with proper protocol detection
func (h *DownloadHandler) getServerURL(c *gin.Context) string {
	// Priority 1: Use configured public URL if set
	if h.config.Server.PublicURL != "" {
		return h.config.Server.PublicURL
	}

	// Priority 2: Detect from request with TLS/proxy awareness
	scheme := "http"

	// Check if TLS is enabled in config
	if h.config.Server.TLS.Enabled {
		scheme = "https"
	}

	// Check if request came through HTTPS (direct or via proxy)
	if c.Request.TLS != nil {
		scheme = "https"
	}

	// Check X-Forwarded-Proto for reverse proxy setups
	if forwardedProto := c.GetHeader("X-Forwarded-Proto"); forwardedProto == "https" {
		scheme = "https"
	}

	// Use the Host header exactly as received (includes port if present)
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

// DownloadAgent serves agent binaries for different platforms
func (h *DownloadHandler) DownloadAgent(c *gin.Context) {
	platform := c.Param("platform")

	// Validate platform to prevent directory traversal (removed darwin - no macOS support)
	validPlatforms := map[string]bool{
		"linux-amd64":   true,
		"linux-arm64":   true,
		"windows-amd64": true,
		"windows-arm64": true,
	}

	if !validPlatforms[platform] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unsupported platform"})
		return
	}

	// Build filename based on platform
	filename := "redflag-agent"
	if strings.HasPrefix(platform, "windows") {
		filename += ".exe"
	}

	// Serve from platform-specific directory: binaries/{platform}/redflag-agent
	agentPath := filepath.Join(h.agentDir, "binaries", platform, filename)

	// Check if file exists
	if _, err := os.Stat(agentPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent binary not found"})
		return
	}

	// Handle both GET and HEAD requests
	if c.Request.Method == "HEAD" {
		c.Status(http.StatusOK)
		return
	}

	c.File(agentPath)
}

// InstallScript serves the installation script
func (h *DownloadHandler) InstallScript(c *gin.Context) {
	platform := c.Param("platform")

	// Validate platform (removed darwin - no macOS support)
	validPlatforms := map[string]bool{
		"linux":   true,
		"windows": true,
	}

	if !validPlatforms[platform] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unsupported platform"})
		return
	}

	serverURL := h.getServerURL(c)
	scriptContent := h.generateInstallScript(platform, serverURL)
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, scriptContent)
}

func (h *DownloadHandler) generateInstallScript(platform, baseURL string) string {
	switch platform {
	case "linux":
		return `#!/bin/bash
set -e

# RedFlag Agent Installation Script
# This script installs the RedFlag agent as a systemd service with proper security hardening

REDFLAG_SERVER="` + baseURL + `"
AGENT_USER="redflag-agent"
AGENT_HOME="/var/lib/redflag-agent"
AGENT_BINARY="/usr/local/bin/redflag-agent"
SUDOERS_FILE="/etc/sudoers.d/redflag-agent"
SERVICE_FILE="/etc/systemd/system/redflag-agent.service"
CONFIG_DIR="/etc/aggregator"

echo "=== RedFlag Agent Installation ==="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This script must be run as root (use sudo)"
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        DOWNLOAD_ARCH="amd64"
        ;;
    aarch64|arm64)
        DOWNLOAD_ARCH="arm64"
        ;;
    *)
        echo "ERROR: Unsupported architecture: $ARCH"
        echo "Supported: x86_64 (amd64), aarch64 (arm64)"
        exit 1
        ;;
esac

echo "Detected architecture: $ARCH (using linux-$DOWNLOAD_ARCH)"
echo ""

# Step 1: Create system user
echo "Step 1: Creating system user..."
if id "$AGENT_USER" &>/dev/null; then
    echo "✓ User $AGENT_USER already exists"
else
    useradd -r -s /bin/false -d "$AGENT_HOME" -m "$AGENT_USER"
    echo "✓ User $AGENT_USER created"
fi

# Create home directory if it doesn't exist
if [ ! -d "$AGENT_HOME" ]; then
    mkdir -p "$AGENT_HOME"
    chown "$AGENT_USER:$AGENT_USER" "$AGENT_HOME"
    echo "✓ Home directory created"
fi

# Stop existing service if running (to allow binary update)
if systemctl is-active --quiet redflag-agent 2>/dev/null; then
    echo ""
    echo "Existing service detected - stopping to allow update..."
    systemctl stop redflag-agent
    sleep 2
    echo "✓ Service stopped"
fi

# Step 2: Download agent binary
echo ""
echo "Step 2: Downloading agent binary..."
echo "Downloading from ${REDFLAG_SERVER}/api/v1/downloads/linux-${DOWNLOAD_ARCH}..."

# Download to temporary file first (to avoid root permission issues)
TEMP_FILE="/tmp/redflag-agent-${DOWNLOAD_ARCH}"
echo "Downloading to temporary file: $TEMP_FILE"

# Try curl first (most reliable)
if curl -sL "${REDFLAG_SERVER}/api/v1/downloads/linux-${DOWNLOAD_ARCH}" -o "$TEMP_FILE"; then
    echo "✓ Download successful, moving to final location"
    mv "$TEMP_FILE" "${AGENT_BINARY}"
    chmod 755 "${AGENT_BINARY}"
    chown root:root "${AGENT_BINARY}"
    echo "✓ Agent binary downloaded and installed"
else
    echo "✗ Download with curl failed"
    # Fallback to wget if available
    if command -v wget >/dev/null 2>&1; then
        echo "Trying wget fallback..."
        if wget -q "${REDFLAG_SERVER}/api/v1/downloads/linux-${DOWNLOAD_ARCH}" -O "$TEMP_FILE"; then
            echo "✓ Download successful with wget, moving to final location"
            mv "$TEMP_FILE" "${AGENT_BINARY}"
            chmod 755 "${AGENT_BINARY}"
            chown root:root "${AGENT_BINARY}"
            echo "✓ Agent binary downloaded and installed (using wget fallback)"
        else
            echo "ERROR: Failed to download agent binary"
            echo "Both curl and wget failed"
            echo "Please ensure ${REDFLAG_SERVER} is accessible"
            # Clean up temp file if it exists
            rm -f "$TEMP_FILE"
            exit 1
        fi
    else
        echo "ERROR: Failed to download agent binary"
        echo "curl failed and wget is not available"
        echo "Please ensure ${REDFLAG_SERVER} is accessible"
        # Clean up temp file if it exists
        rm -f "$TEMP_FILE"
        exit 1
    fi
fi

# Clean up temp file if it still exists
rm -f "$TEMP_FILE"

# Set SELinux context for binary if SELinux is enabled
if command -v getenforce >/dev/null 2>&1 && [ "$(getenforce)" != "Disabled" ]; then
    echo "SELinux detected, setting file context for binary..."
    restorecon -v "${AGENT_BINARY}" 2>/dev/null || true
    echo "✓ SELinux context set for binary"
fi

# Step 3: Install sudoers configuration
echo ""
echo "Step 3: Installing sudoers configuration..."
cat > "$SUDOERS_FILE" <<'SUDOERS_EOF'
# RedFlag Agent minimal sudo permissions
# This file grants the redflag-agent user limited sudo access for package management
# Generated automatically during RedFlag agent installation

# APT package management commands (Debian/Ubuntu)
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get update
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get install -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get upgrade -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get install --dry-run --yes *

# DNF package management commands (RHEL/Fedora/Rocky/Alma)
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf makecache
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf install -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf upgrade -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf install --assumeno --downloadonly *

# Docker operations
redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker pull *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker image inspect *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker manifest inspect *
SUDOERS_EOF

chmod 440 "$SUDOERS_FILE"

# Validate sudoers file
if visudo -c -f "$SUDOERS_FILE" &>/dev/null; then
    echo "✓ Sudoers configuration installed and validated"
else
    echo "ERROR: Sudoers configuration is invalid"
    rm -f "$SUDOERS_FILE"
    exit 1
fi

# Step 4: Create configuration directory
echo ""
echo "Step 4: Creating configuration directory..."
mkdir -p "$CONFIG_DIR"
chown "$AGENT_USER:$AGENT_USER" "$CONFIG_DIR"
chmod 755 "$CONFIG_DIR"
echo "✓ Configuration directory created"

# Set SELinux context for config directory if SELinux is enabled
if command -v getenforce >/dev/null 2>&1 && [ "$(getenforce)" != "Disabled" ]; then
    echo "Setting SELinux context for config directory..."
    restorecon -Rv "$CONFIG_DIR" 2>/dev/null || true
    echo "✓ SELinux context set for config directory"
fi

# Step 5: Install systemd service
echo ""
echo "Step 5: Installing systemd service..."
cat > "$SERVICE_FILE" <<SERVICE_EOF
[Unit]
Description=RedFlag Update Agent
After=network.target
Documentation=https://github.com/Fimeg/RedFlag

[Service]
Type=simple
User=$AGENT_USER
Group=$AGENT_USER
WorkingDirectory=$AGENT_HOME
ExecStart=$AGENT_BINARY
Restart=always
RestartSec=30

# Security hardening
# NoNewPrivileges=true - DISABLED: Prevents sudo from working, which agent needs for package management
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$AGENT_HOME /var/log $CONFIG_DIR
PrivateTmp=true

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=redflag-agent

[Install]
WantedBy=multi-user.target
SERVICE_EOF

chmod 644 "$SERVICE_FILE"
echo "✓ Systemd service installed"

# Step 6: Register agent with server
echo ""
echo "Step 6: Agent registration"
echo "=========================================="
echo ""

# Check if token was provided as parameter (for one-liner support)
if [ -n "$1" ]; then
    REGISTRATION_TOKEN="$1"
    echo "Using provided registration token"
else
    # Check if stdin is a terminal (not being piped)
    if [ -t 0 ]; then
        echo "Registration token required to enroll this agent with the server."
        echo ""
        echo "To get a token:"
        echo "  1. Visit: ${REDFLAG_SERVER}/settings/tokens"
        echo "  2. Copy the active token from the list"
        echo ""
        echo "Enter registration token (or press Enter to skip):"
        read -p "> " REGISTRATION_TOKEN
    else
        echo ""
        echo "IMPORTANT: Registration token required!"
        echo ""
        echo "Since you're running this via pipe, you need to:"
        echo ""
        echo "Option 1 - One-liner with token:"
        echo "  curl -sfL ${REDFLAG_SERVER}/api/v1/install/linux | sudo bash -s -- YOUR_TOKEN"
        echo ""
        echo "Option 2 - Download and run interactively:"
        echo "  curl -sfL ${REDFLAG_SERVER}/api/v1/install/linux -o install.sh"
        echo "  chmod +x install.sh"
        echo "  sudo ./install.sh"
        echo ""
        echo "Skipping registration for now."
        echo "Please register manually after installation."
    fi
fi

# Check if agent is already registered
if [ -f "$CONFIG_DIR/config.json" ]; then
    echo ""
    echo "[INFO] Agent already registered - configuration file exists"
    echo "[INFO] Skipping registration to preserve agent history"
    echo "[INFO] If you need to re-register, delete: $CONFIG_DIR/config.json"
    echo ""
elif [ -n "$REGISTRATION_TOKEN" ]; then
    echo ""
    echo "Registering agent..."

    # Create config file and register
    cat > "$CONFIG_DIR/config.json" <<EOF
{
  "server_url": "${REDFLAG_SERVER}",
  "registration_token": "${REGISTRATION_TOKEN}"
}
EOF

    # Set proper permissions
    chown "$AGENT_USER:$AGENT_USER" "$CONFIG_DIR/config.json"
    chmod 600 "$CONFIG_DIR/config.json"

    # Run agent registration as the agent user with explicit server and token
    echo "Running: sudo -u $AGENT_USER ${AGENT_BINARY} --server ${REDFLAG_SERVER} --token $REGISTRATION_TOKEN --register"
    if sudo -u "$AGENT_USER" "${AGENT_BINARY}" --server "${REDFLAG_SERVER}" --token "$REGISTRATION_TOKEN" --register; then
        echo "✓ Agent registered successfully"

        # Update config file with the new agent credentials
        if [ -f "$CONFIG_DIR/config.json" ]; then
            chown "$AGENT_USER:$AGENT_USER" "$CONFIG_DIR/config.json"
            chmod 600 "$CONFIG_DIR/config.json"
            echo "✓ Configuration file updated and secured"
        fi
    else
        echo "ERROR: Agent registration failed"
        echo "Please check the token and server URL, then try again"
        echo ""
        echo "To retry manually:"
        echo "  sudo -u $AGENT_USER ${AGENT_BINARY} --server ${REDFLAG_SERVER} --token $REGISTRATION_TOKEN --register"
        exit 1
    fi
else
    echo ""
    echo "Skipping registration. You'll need to register manually before starting the service."
    echo ""
    echo "To register later:"
    echo "  1. Visit ${REDFLAG_SERVER}/settings/tokens"
    echo "  2. Copy a registration token"
    echo "  3. Run: sudo -u $AGENT_USER ${AGENT_BINARY} --server ${REDFLAG_SERVER} --token YOUR_TOKEN"
    echo ""
    echo "Installation will continue, but the service will not start until registered."
fi

# Step 7: Enable and start service
echo ""
echo "Step 7: Enabling and starting service..."
systemctl daemon-reload

# Check if agent is registered
if [ -f "$CONFIG_DIR/config.json" ]; then
    systemctl enable redflag-agent
    systemctl restart redflag-agent

    # Wait for service to start
    sleep 2

    if systemctl is-active --quiet redflag-agent; then
        echo "✓ Service started successfully"
    else
        echo "⚠ Service failed to start. Check logs:"
        echo "  sudo journalctl -u redflag-agent -n 50"
        exit 1
    fi
else
    echo "⚠ Service not started (agent not registered)"
    echo "  Run registration command above, then:"
    echo "  sudo systemctl enable redflag-agent"
    echo "  sudo systemctl start redflag-agent"
fi

# Step 8: Show status
echo ""
echo "=== Installation Complete ==="
echo ""
echo "The RedFlag agent has been installed with the following security features:"
echo "  ✓ Dedicated system user (redflag-agent)"
echo "  ✓ Limited sudo access via /etc/sudoers.d/redflag-agent"
echo "  ✓ Systemd service with security hardening"
echo "  ✓ Protected configuration directory"
echo ""
if systemctl is-active --quiet redflag-agent; then
    echo "Service Status: ✓ RUNNING"
    echo ""
    systemctl status redflag-agent --no-pager -l | head -n 15
    echo ""
else
    echo "Service Status: ⚠ NOT RUNNING (waiting for registration)"
    echo ""
fi
echo "Useful commands:"
echo "  Check status:  sudo systemctl status redflag-agent"
echo "  View logs:     sudo journalctl -u redflag-agent -f"
echo "  Restart:       sudo systemctl restart redflag-agent"
echo "  Stop:          sudo systemctl stop redflag-agent"
echo ""
echo "Configuration:"
echo "  Config file:   $CONFIG_DIR/config.json"
echo "  Binary:        $AGENT_BINARY"
echo "  Service:       $SERVICE_FILE"
echo "  Sudoers:       $SUDOERS_FILE"
echo ""
`

	case "windows":
		return `@echo off
REM RedFlag Agent Installation Script for Windows
REM This script downloads the agent and sets up Windows service
REM
REM Usage:
REM   install.bat                    - Interactive mode (prompts for token)
REM   install.bat YOUR_TOKEN_HERE    - Automatic mode (uses provided token)

set REDFLAG_SERVER=` + baseURL + `
set AGENT_DIR=%ProgramFiles%\RedFlag
set AGENT_BINARY=%AGENT_DIR%\redflag-agent.exe
set CONFIG_DIR=%ProgramData%\RedFlag

echo === RedFlag Agent Installation ===
echo.

REM Check for admin privileges
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo ERROR: This script must be run as Administrator
    echo Right-click and select "Run as administrator"
    pause
    exit /b 1
)

REM Detect architecture
if "%PROCESSOR_ARCHITECTURE%"=="AMD64" (
    set DOWNLOAD_ARCH=amd64
) else if "%PROCESSOR_ARCHITECTURE%"=="ARM64" (
    set DOWNLOAD_ARCH=arm64
) else (
    echo ERROR: Unsupported architecture: %PROCESSOR_ARCHITECTURE%
    echo Supported: AMD64, ARM64
    pause
    exit /b 1
)

echo Detected architecture: %PROCESSOR_ARCHITECTURE% (using windows-%DOWNLOAD_ARCH%)
echo.

REM Create installation directory
echo Creating installation directory...
if not exist "%AGENT_DIR%" mkdir "%AGENT_DIR%"
echo [OK] Installation directory created

REM Create config directory
if not exist "%CONFIG_DIR%" mkdir "%CONFIG_DIR%"
echo [OK] Configuration directory created

REM Grant full permissions to SYSTEM and Administrators on config directory
echo Setting permissions on configuration directory...
icacls "%CONFIG_DIR%" /grant "SYSTEM:(OI)(CI)F"
icacls "%CONFIG_DIR%" /grant "Administrators:(OI)(CI)F"
echo [OK] Permissions set
echo.

REM Stop existing service if running (to allow binary update)
sc query RedFlagAgent >nul 2>&1
if %errorLevel% equ 0 (
    echo Existing service detected - stopping to allow update...
    sc stop RedFlagAgent >nul 2>&1
    timeout /t 3 /nobreak >nul
    echo [OK] Service stopped
)

REM Download agent binary
echo Downloading agent binary...
echo From: %REDFLAG_SERVER%/api/v1/downloads/windows-%DOWNLOAD_ARCH%
curl -sfL "%REDFLAG_SERVER%/api/v1/downloads/windows-%DOWNLOAD_ARCH%" -o "%AGENT_BINARY%"
if %errorLevel% neq 0 (
    echo ERROR: Failed to download agent binary
    echo Please ensure %REDFLAG_SERVER% is accessible
    pause
    exit /b 1
)
echo [OK] Agent binary downloaded
echo.

REM Agent registration
echo === Agent Registration ===
echo.

REM Check if token was provided as command-line argument
if not "%1"=="" (
    set TOKEN=%1
    echo Using provided registration token
) else (
    echo IMPORTANT: You need a registration token to enroll this agent.
    echo.
    echo To get a token:
    echo   1. Visit: %REDFLAG_SERVER%/settings/tokens
    echo   2. Create a new registration token
    echo   3. Copy the token
    echo.
    set /p TOKEN="Enter registration token (or press Enter to skip): "
)

REM Check if agent is already registered
if exist "%CONFIG_DIR%\config.json" (
    echo.
    echo [INFO] Agent already registered - configuration file exists
    echo [INFO] Skipping registration to preserve agent history
    echo [INFO] If you need to re-register, delete: %CONFIG_DIR%\config.json
    echo.
) else if not "%TOKEN%"=="" (
    echo.
    echo === Registering Agent ===
    echo.

    REM Attempt registration
    "%AGENT_BINARY%" --server "%REDFLAG_SERVER%" --token "%TOKEN%" --register

    REM Check exit code
    if %errorLevel% equ 0 (
        echo [OK] Agent registered successfully
        echo [OK] Configuration saved to: %CONFIG_DIR%\config.json
        echo.
    ) else (
        echo.
        echo [ERROR] Registration failed
        echo.
        echo Please check:
        echo   1. Server is accessible: %REDFLAG_SERVER%
        echo   2. Registration token is valid and not expired
        echo   3. Token has available seats remaining
        echo.
        echo To try again:
        echo   "%AGENT_BINARY%" --server "%REDFLAG_SERVER%" --token "%TOKEN%" --register
        echo.
        pause
        exit /b 1
    )
) else (
    echo.
    echo [INFO] No registration token provided - skipping registration
    echo.
    echo To register later:
    echo   "%AGENT_BINARY%" --server "%REDFLAG_SERVER%" --token YOUR_TOKEN --register
)

REM Check if service already exists
echo.
echo === Configuring Windows Service ===
echo.
sc query RedFlagAgent >nul 2>&1
if %errorLevel% equ 0 (
    echo [INFO] RedFlag Agent service already installed
    echo [INFO] Service will be restarted with updated binary
    echo.
) else (
    echo Installing RedFlag Agent service...
    "%AGENT_BINARY%" -install-service
    if %errorLevel% equ 0 (
        echo [OK] Service installed successfully
        echo.

        REM Give Windows SCM time to register the service
        timeout /t 2 /nobreak >nul
    ) else (
        echo [ERROR] Failed to install service
        echo.
        pause
        exit /b 1
    )
)

REM Start the service if agent is registered
if exist "%CONFIG_DIR%\config.json" (
    echo Starting RedFlag Agent service...
    "%AGENT_BINARY%" -start-service
    if %errorLevel% equ 0 (
        echo [OK] RedFlag Agent service started
        echo.
        echo Agent is now running as a Windows service in the background.
        echo You can verify it is working by checking the agent status in the web UI.
    ) else (
        echo [WARNING] Failed to start service. You can start it manually:
        echo   "%AGENT_BINARY%" -start-service
        echo   Or use Windows Services: services.msc
    )
) else (
    echo [WARNING] Service not started (agent not registered)
    echo To register and start the service:
    echo   1. Register: "%AGENT_BINARY%" --server "%REDFLAG_SERVER%" --token YOUR_TOKEN --register
    echo   2. Start: "%AGENT_BINARY%" -start-service
)

echo.
echo === Installation Complete ===
echo.
echo The RedFlag agent has been installed as a Windows service.
echo Configuration file: %CONFIG_DIR%\config.json
echo Agent binary: %AGENT_BINARY%
echo.
echo Managing the RedFlag Agent service:
echo   Check status:     "%AGENT_BINARY%" -service-status
echo   Start manually:    "%AGENT_BINARY%" -start-service
echo   Stop service:     "%AGENT_BINARY%" -stop-service
echo   Remove service:   "%AGENT_BINARY%" -remove-service
echo.
echo Alternative management with Windows Services:
echo   Open services.msc and look for "RedFlag Update Agent"
echo.
echo To run the agent directly (for debugging):
echo   "%AGENT_BINARY%"
echo.
echo To verify the agent is working:
echo   1. Check the web UI for the agent status
echo   2. Look for recent check-ins from this machine
echo.
pause
`

	default:
		return "# Unsupported platform"
	}
}