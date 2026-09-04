#!/bin/bash
set -e

# Create config directory if it doesn't exist
mkdir -p /app/config

# Execute the main command
exec "$@"
