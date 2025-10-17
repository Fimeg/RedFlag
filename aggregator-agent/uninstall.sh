#!/bin/bash
set -e

# RedFlag Agent Uninstallation Script
# This script removes the RedFlag agent service and configuration

AGENT_USER="redflag-agent"
AGENT_HOME="/var/lib/redflag-agent"
AGENT_BINARY="/usr/local/bin/redflag-agent"
SUDOERS_FILE="/etc/sudoers.d/redflag-agent"
SERVICE_FILE="/etc/systemd/system/redflag-agent.service"

echo "=== RedFlag Agent Uninstallation ==="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This script must be run as root (use sudo)"
    exit 1
fi

# Stop and disable service
if systemctl is-active --quiet redflag-agent; then
    echo "Stopping redflag-agent service..."
    systemctl stop redflag-agent
    echo "✓ Service stopped"
fi

if systemctl is-enabled --quiet redflag-agent; then
    echo "Disabling redflag-agent service..."
    systemctl disable redflag-agent
    echo "✓ Service disabled"
fi

# Remove service file
if [ -f "$SERVICE_FILE" ]; then
    echo "Removing systemd service file..."
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    echo "✓ Service file removed"
fi

# Remove sudoers configuration
if [ -f "$SUDOERS_FILE" ]; then
    echo "Removing sudoers configuration..."
    rm -f "$SUDOERS_FILE"
    echo "✓ Sudoers configuration removed"
fi

# Remove binary
if [ -f "$AGENT_BINARY" ]; then
    echo "Removing agent binary..."
    rm -f "$AGENT_BINARY"
    echo "✓ Agent binary removed"
fi

# Optionally remove user (commented out by default to preserve logs/data)
# if id "$AGENT_USER" &>/dev/null; then
#     echo "Removing user $AGENT_USER..."
#     userdel -r "$AGENT_USER"
#     echo "✓ User removed"
# fi

echo ""
echo "=== Uninstallation Complete ==="
echo ""
echo "Note: The $AGENT_USER user and $AGENT_HOME directory have been preserved."
echo "To completely remove them, run:"
echo "  sudo userdel -r $AGENT_USER"
echo ""
