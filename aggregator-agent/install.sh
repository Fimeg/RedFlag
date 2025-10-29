#!/bin/bash
set -e

# RedFlag Agent Installation Script
# This script installs the RedFlag agent as a systemd service with proper permissions

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_USER="redflag-agent"
AGENT_HOME="/var/lib/redflag-agent"
AGENT_BINARY="/usr/local/bin/redflag-agent"
SUDOERS_FILE="/etc/sudoers.d/redflag-agent"
SERVICE_FILE="/etc/systemd/system/redflag-agent.service"

echo "=== RedFlag Agent Installation ==="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This script must be run as root (use sudo)"
    exit 1
fi

# Function to create user if doesn't exist
create_user() {
    if id "$AGENT_USER" &>/dev/null; then
        echo "✓ User $AGENT_USER already exists"
    else
        echo "Creating system user $AGENT_USER..."
        useradd -r -s /bin/false -d "$AGENT_HOME" -m "$AGENT_USER"
        echo "✓ User $AGENT_USER created"
    fi

    # Add user to docker group for Docker update scanning
    if getent group docker &>/dev/null; then
        echo "Adding $AGENT_USER to docker group..."
        usermod -aG docker "$AGENT_USER"
        echo "✓ User $AGENT_USER added to docker group"
    else
        echo "⚠ Docker group not found - Docker updates will not be available"
        echo "  (Install Docker first, then reinstall the agent to enable Docker support)"
    fi
}

# Function to build agent binary
build_agent() {
    echo "Building agent binary..."
    cd "$SCRIPT_DIR"
    go build -o redflag-agent ./cmd/agent
    echo "✓ Agent binary built"
}

# Function to install agent binary
install_binary() {
    echo "Installing agent binary to $AGENT_BINARY..."
    cp "$SCRIPT_DIR/redflag-agent" "$AGENT_BINARY"
    chmod 755 "$AGENT_BINARY"
    chown root:root "$AGENT_BINARY"
    echo "✓ Agent binary installed"
}

# Function to install sudoers configuration
install_sudoers() {
    echo "Installing sudoers configuration..."
    cat > "$SUDOERS_FILE" <<'EOF'
# RedFlag Agent minimal sudo permissions
# This file is generated automatically during RedFlag agent installation

# APT package management commands
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get update
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get install -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get upgrade -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get install --dry-run --yes *

# DNF package management commands
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf makecache
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf install -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf upgrade -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf install --assumeno --downloadonly *

# Docker operations
redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker pull *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker image inspect *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker manifest inspect *
EOF

    chmod 440 "$SUDOERS_FILE"

    # Validate sudoers file
    if visudo -c -f "$SUDOERS_FILE"; then
        echo "✓ Sudoers configuration installed and validated"
    else
        echo "ERROR: Sudoers configuration is invalid"
        rm -f "$SUDOERS_FILE"
        exit 1
    fi
}

# Function to install systemd service
install_service() {
    echo "Installing systemd service..."
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=RedFlag Update Agent
After=network.target

[Service]
Type=simple
User=$AGENT_USER
Group=$AGENT_USER
WorkingDirectory=$AGENT_HOME
ExecStart=$AGENT_BINARY
Restart=always
RestartSec=30

# Security hardening
# NoNewPrivileges=true - DISABLED: Prevents sudo from working
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$AGENT_HOME /var/log /etc/aggregator
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

    chmod 644 "$SERVICE_FILE"
    echo "✓ Systemd service installed"
}

# Function to start and enable service
start_service() {
    echo "Reloading systemd daemon..."
    systemctl daemon-reload

    # Stop service if running
    if systemctl is-active --quiet redflag-agent; then
        echo "Stopping existing service..."
        systemctl stop redflag-agent
    fi

    echo "Enabling and starting redflag-agent service..."
    systemctl enable redflag-agent
    systemctl start redflag-agent

    # Wait a moment for service to start
    sleep 2

    echo "✓ Service started"
}

# Function to show status
show_status() {
    echo ""
    echo "=== Service Status ==="
    systemctl status redflag-agent --no-pager -l
    echo ""
    echo "=== Recent Logs ==="
    journalctl -u redflag-agent -n 20 --no-pager
}

# Function to register agent
register_agent() {
    local server_url="${1:-http://localhost:8080}"

    echo "Registering agent with server at $server_url..."

    # Create config directory
    mkdir -p /etc/aggregator

    # Register agent (run as regular binary, not as service)
    if "$AGENT_BINARY" -register -server "$server_url"; then
        echo "✓ Agent registered successfully"
    else
        echo "ERROR: Agent registration failed"
        echo "Please ensure the RedFlag server is running at $server_url"
        exit 1
    fi
}

# Main installation flow
SERVER_URL="${1:-http://localhost:8080}"

echo "Step 1: Creating system user..."
create_user

echo ""
echo "Step 2: Building agent binary..."
build_agent

echo ""
echo "Step 3: Installing agent binary..."
install_binary

echo ""
echo "Step 4: Registering agent with server..."
register_agent "$SERVER_URL"

echo ""
echo "Step 5: Setting config file permissions..."
chown redflag-agent:redflag-agent /etc/aggregator/config.json
chmod 600 /etc/aggregator/config.json

echo ""
echo "Step 6: Installing sudoers configuration..."
install_sudoers

echo ""
echo "Step 7: Installing systemd service..."
install_service

echo ""
echo "Step 8: Starting service..."
start_service

echo ""
echo "=== Installation Complete ==="
echo ""
echo "The RedFlag agent is now installed and running as a systemd service."
echo "Server URL: $SERVER_URL"
echo ""
echo "Useful commands:"
echo "  - Check status:  sudo systemctl status redflag-agent"
echo "  - View logs:     sudo journalctl -u redflag-agent -f"
echo "  - Restart:       sudo systemctl restart redflag-agent"
echo "  - Stop:          sudo systemctl stop redflag-agent"
echo "  - Disable:       sudo systemctl disable redflag-agent"
echo ""
echo "Note: To re-register with a different server, edit /etc/aggregator/config.json"
echo ""

show_status
