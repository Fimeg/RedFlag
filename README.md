# RedFlag (Aggregator)

**ALPHA RELEASE - v0.1.16**
Self-hosted update management platform for homelabs and small teams

## Status

- **Core Features Working**: Update management, agent registration, web dashboard
- **Alpha Deployment Ready**: Setup wizard and configuration system implemented
- **Cross-Platform Support**: Linux and Windows agents
- **In Development**: Enhanced features and polish
- **Alpha Software**: Expect some rough edges, backup your data

## What RedFlag Is

A self-hosted, cross-platform update management platform built for homelabs and small teams:

- Go Server Backend with PostgreSQL database
- React Web Dashboard with real-time updates
- Cross-Platform Agents (Linux APT/DNF/Docker, Windows Updates/Winget)
- Secure Authentication with registration tokens and refresh tokens
- System Monitoring with real-time status and audit trails
- User-Adjustable Rate Limiting with TLS support

## Key Features

### Alpha Features
- Secure Server Setup: `./redflag-server --setup` with user-provided secrets
- Registration Token System: One-time tokens for secure agent enrollment
- Rate Limiting: User-adjustable API security with sensible defaults
- Cross-Platform Agents: Linux and Windows with unified architecture
- Real-Time Heartbeat: Rapid polling for interactive operations
- Dependency Management: Safe update installation with dry-run checking
- Audit Logging: Complete activity tracking and history
- Proxy Support: HTTP/HTTPS/SOCKS5 proxy support for restricted networks

### Update Management
- Package Managers: APT, DNF, Docker images, Windows Updates, Winget
- Update Discovery: Automatic scanning with severity classification
- Approval Workflow: Controlled update deployment with confirmation
- Bulk Operations: Multi-agent management and batch operations
- Rollback Support: Failed update tracking and retry capabilities

### Deployment
- Configuration Management: CLI flags → environment → config file → defaults
- Service Integration: systemd service management on Linux
- Cross-Platform Installers: One-liner deployment scripts
- Container Support: Docker and Kubernetes deployment options

## Architecture

```
┌─────────────────┐
│  Web Dashboard  │  React + TypeScript + TailwindCSS
│  + Rate Limiting │  + Registration Token Management
└────────┬────────┘
         │ HTTPS with TLS + User Authentication
┌────────▼────────┐
│  Server (Go)    │  Alpha with PostgreSQL
│  + Rate Limits  │  + Registration Tokens + Setup Wizard
│  + JWT Auth     │  + Heartbeat System + Comprehensive API
└────────┬────────┘
         │ Pull-based (agents check in every 5 min) + Rapid Polling
    ┌────┴────┬────────┐
    │         │        │
┌───▼──┐  ┌──▼──┐  ┌──▼───┐
│Linux │  │Windows│  │Linux │
│Agent │  │Agent  │  │Agent │
│+Proxy│  │+Proxy│  │+Proxy│
└──────┘  └───────┘  └──────┘
```

## Prerequisites

- **Go 1.21+** (for building from source)
- **Docker & Docker Compose** (for PostgreSQL database)
- **Linux** (server deployment platform)

## Quick Start

### 1. Server Setup (Docker - Recommended)
```bash
# Clone repository
git clone https://github.com/Fimeg/RedFlag.git
cd RedFlag

# Start database and server
docker-compose up -d

# Setup server (one-time)
docker-compose exec server ./redflag-server --setup

# Run database migrations
docker-compose exec server ./redflag-server --migrate

# Restart server with configuration
docker-compose restart server

# Access: http://localhost:8080
# Admin: http://localhost:8080/admin
```

### 2. Manual Setup (Development)
```bash
# Build components
make build-all

# Start database
docker-compose up -d postgres

# Setup server
cd aggregator-server && sudo ./redflag-server --setup

# Run migrations
./redflag-server --migrate

# Start server
./redflag-server
```

### 2. Agent Deployment (Linux)
```bash
# Option 1: One-liner with registration token
sudo bash -c 'curl -sfL https://redflag.wiuf.net/install | bash -s -- rf-tok-abc123'

# Option 2: Manual installation
sudo ./install.sh --server https://redflag.wiuf.net:8080 --token rf-tok-abc123

# Option 3: Advanced configuration with proxy
sudo ./redflag-agent --server https://redflag.wiuf.net:8080 \
                      --token rf-tok-abc123 \
                      --proxy-http http://proxy.company.com:8080 \
                      --organization "my-homelab" \
                      --tags "production,webserver"
```

### 3. Windows Agent Deployment
```powershell
# PowerShell one-liner
iwr https://redflag.wiuf.net/install.ps1 | iex -Arguments '--server https://redflag.wiuf.net:8080 --token rf-tok-abc123'

# Or manual download and install
.\redflag-agent.exe --server https://redflag.wiuf.net:8080 --token rf-tok-abc123
```

## Agent Configuration Options

### CLI Flags (Highest Priority)
```bash
./redflag-agent --server https://redflag.wiuf.net \
                --token rf-tok-abc123 \
                --proxy-http http://proxy.company.com:8080 \
                --proxy-https https://proxy.company.com:8080 \
                --log-level debug \
                --organization "my-homelab" \
                --tags "production,webserver" \
                --name "redflag-server-01" \
                --insecure-tls
```

### Environment Variables
```bash
export REDFLAG_SERVER_URL="https://redflag.wiuf.net"
export REDFLAG_REGISTRATION_TOKEN="rf-tok-abc123"
export REDFLAG_HTTP_PROXY="http://proxy.company.com:8080"
export REDFLAG_HTTPS_PROXY="https://proxy.company.com:8080"
export REDFLAG_NO_PROXY="localhost,127.0.0.1"
export REDFLAG_LOG_LEVEL="info"
export REDFLAG_ORGANIZATION="my-homelab"
```

### Configuration File
```json
{
  "server_url": "https://redflag.wiuf.net",
  "registration_token": "rf-tok-abc123",
  "proxy": {
    "enabled": true,
    "http": "http://proxy.company.com:8080",
    "https": "https://proxy.company.com:8080",
    "no_proxy": "localhost,127.0.0.1"
  },
  "network": {
    "timeout": "30s",
    "retry_count": 3,
    "retry_delay": "5s"
  },
  "logging": {
    "level": "info",
    "max_size": 100,
    "max_backups": 3
  },
  "tags": ["production", "webserver"],
  "organization": "my-homelab",
  "display_name": "redflag-server-01"
}
```

## Web Dashboard Features

### Agent Management
- Real-time Status: Online/offline with heartbeat indicators
- System Information: CPU, memory, disk usage, OS details
- Version Tracking: Agent versions and update availability
- Metadata Management: Tags, organizations, display names
- Bulk Operations: Multi-agent scanning and updates

### Update Management
- Severity Classification: Critical, high, medium, low priority updates
- Approval Workflow: Controlled update deployment with dependencies
- Dependency Resolution: Safe installation with conflict checking
- Batch Operations: Approve/install multiple updates
- Audit Trail: Complete history of all operations

### Settings & Administration
- Registration Tokens: Generate and manage secure enrollment tokens
- Rate Limiting: User-adjustable API security settings
- Authentication: Secure login with session management
- Audit Logging: Comprehensive activity tracking
- Server Configuration: Admin settings and system controls

## API Reference

### Registration Token Management
```bash
# Generate registration token
curl -X POST https://redflag.wiuf.net/api/v1/admin/registration-tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"label": "Production Servers", "expires_in": "24h"}'

# List tokens
curl -X GET https://redflag.wiuf.net/api/v1/admin/registration-tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Revoke token
curl -X DELETE https://redflag.wiuf.net/api/v1/admin/registration-tokens/rf-tok-abc123 \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### Rate Limit Management
```bash
# View current settings
curl -X GET https://redflag.wiuf.net/api/v1/admin/rate-limits \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Update settings
curl -X PUT https://redflag.wiuf.net/api/v1/admin/rate-limits \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "agent_registration": {"requests": 10, "window": "1m", "enabled": true},
    "admin_operations": {"requests": 200, "window": "1m", "enabled": true}
  }'
```

## Security

### Authentication & Authorization
- Registration Tokens: One-time use tokens prevent unauthorized agent enrollment
- Refresh Token Authentication: 90-day sliding window with 24h access tokens
- SHA-256 token hashing for secure storage
- Admin authentication for server access and management

### Network Security
- Rate Limiting: Configurable API protection with sensible defaults
- TLS Support: Certificate validation and client certificate support
- Pull-based Model: Agents poll server (firewall-friendly)
- HTTPS Required: Production deployments must use TLS

### System Hardening
- Minimal Privilege Execution: Agents run with least required privileges
- Command Validation: Whitelisted commands only
- Secure Defaults: Hardened configurations out of the box
- Security Hardening: Minimal privilege execution and sudoers management

### Audit & Monitoring
- Audit Trails: Complete logging of all activities
- Token Renewal: `/renew` endpoint prevents daily re-registration
- Activity Tracking: Comprehensive monitoring and alerting
- Access Logs: Full audit trail of user and agent actions

## Docker Deployment

```yaml
# docker-compose.yml
version: '3.8'
services:
  redflag-server:
    build: ./aggregator-server
    ports:
      - "8080:8080"
    environment:
      - REDFLAG_SERVER_HOST=0.0.0.0
      - REDFLAG_SERVER_PORT=8080
      - REDFLAG_DB_HOST=postgres
      - REDFLAG_DB_PORT=5432
      - REDFLAG_DB_NAME=redflag
      - REDFLAG_DB_USER=redflag
      - REDFLAG_DB_PASSWORD=secure-password
    depends_on:
      - postgres
    volumes:
      - ./redflag-data:/etc/redflag
      - ./logs:/app/logs

  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: redflag
      POSTGRES_USER: redflag
      POSTGRES_PASSWORD: secure-password
    volumes:
      - postgres-data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
```

## Project Structure

```
RedFlag/
├── aggregator-server/          # Go server backend
│   ├── cmd/server/            # Main server entry point
│   ├── internal/
│   │   ├── api/               # REST API handlers and middleware
│   │   │   └── handlers/       # API endpoint implementations
│   │   ├── database/          # Database layer with migrations
│   │   │   ├── migrations/     # Database schema migrations
│   │   │   └── queries/        # Database query functions
│   │   ├── models/            # Data models and structs
│   │   ├── services/          # Business logic services
│   │   └── config/            # Configuration management
│   └── redflag-server          # Server binary

├── aggregator-agent/           # Cross-platform Go agent
│   ├── cmd/agent/             # Agent main entry point
│   ├── internal/
│   │   ├── client/           # HTTP client with token renewal
│   │   ├── config/           # Enhanced configuration system
│   │   ├── scanner/          # Update scanners for each platform
│   │   ├── installer/        # Package installers
│   │   └── system/           # System information collection
│   ├── install.sh             # Linux installation script
│   └── redflag-agent           # Agent binary

├── aggregator-web/             # React dashboard
├── docker-compose.yml          # Development environment
├── Makefile                    # Common tasks
└── README.md                   # This file
```

## What This Is

A self-hosted, cross-platform update management platform built with:

- Go server backend + PostgreSQL
- React web dashboard with TypeScript
- Cross-platform agents (Linux APT/DNF/Docker, Windows Updates/Winget)
- Local CLI tools for agent management
- Update installation system with dependency management
- Refresh token authentication for stable agent identity

## What This Isn't

- Not ready for public use
- Not documented for external users
- Not supported or maintained for others
- Not stable (active development)

## Current Capabilities

### Working Features
- Server backend with REST API
- Cross-platform agent registration and check-in
- Update discovery for APT, DNF, Docker images, Windows Updates, and Winget packages
- Update approval workflow with dependency confirmation
- Web dashboard with agent management and real-time status
- Local CLI tools (--scan, --status, --list-updates, --export, --export=json/csv)
- Update installation system with dry-run dependency checking
- Beautiful terminal output with colors and severity indicators
- Local cache system for offline viewing of scan results
- Refresh token authentication for stable agent identity
- Event-sourced database architecture for scalability

### Known Limitations
- No real-time WebSocket updates
- Proxmox integration is not implemented in this version (planned for future release)
- Authentication system works but needs security hardening

## Screenshots

| Overview | Updates Management | Agent List |
|----------|-------------------|------------|
| ![Main Dashboard](Screenshots/RedFlag%20Default%20Dashboard.png) | ![Updates Dashboard](Screenshots/RedFlag%20Updates%20Dashboard.png) | ![Agent List](Screenshots/RedFlag%20Agent%20List.png) |
| System overview with metrics | Update approval with dependency workflow | Cross-platform agent management |

| Linux Agent Details | Windows Agent Details |
|-------------------|---------------------|
| ![Linux Agent Details](Screenshots/RedFlag%20Linux%20Agent%20Details.png) | ![Windows Agent Details](Screenshots/RedFlag%20Windows%20Agent%20Details.png) |
| Linux system specs and updates | Windows Updates and Winget support |

| History & Audit | Windows Agent History |
|----------------|----------------------|
| ![History Dashboard](Screenshots/RedFlag%20History%20Dashboard.png) | ![Windows Agent History](Screenshots/RedFlag%20Windows%20Agent%20History%20.png) |
| Complete audit trail of activities | Windows agent activity timeline |

| Live Operations | Docker Management |
|-----------------|------------------|
| ![Live Operations](Screenshots/RedFlag%20Live%20Operations%20-%20Failed%20Dashboard.png) | ![Docker Dashboard](Screenshots/RedFlag%20Docker%20Dashboard.png) |
| Real-time operation tracking | Container image update management |

## For Developers

This repository contains:

- **Server backend code** (`aggregator-server/`)
- **Agent code** (`aggregator-agent/`)
- **Web dashboard** (`aggregator-web/`)
- **Database migrations** and configuration

## Database Schema

Key Tables:
- `agents` - Registered agents with system metadata and version tracking
- `refresh_tokens` - Long-lived refresh tokens for stable agent identity
- `update_events` - Immutable event storage for update discoveries
- `current_package_state` - Optimized view of current update state
- `agent_commands` - Command queue for agents (scan, install, dry-run)
- `update_logs` - Execution logs with detailed results
- `agent_tags` - Agent tagging/grouping

## Configuration

### Server (.env)
```bash
SERVER_PORT=8080
DATABASE_URL=postgres://aggregator:aggregator@localhost:5432/aggregator?sslmode=disable
JWT_SECRET=change-me-in-production
CHECK_IN_INTERVAL=300    # seconds
OFFLINE_THRESHOLD=600    # seconds
```

### Agent (/etc/aggregator/config.json)
Auto-generated on registration:
```json
{
  "server_url": "http://localhost:8080",
  "agent_id": "uuid",
  "token": "jwt-access-token",
  "refresh_token": "long-lived-refresh-token",
  "check_in_interval": 300
}
```

## Development

### Makefile Commands
```bash
make help           # Show all commands
make db-up          # Start PostgreSQL
make db-down        # Stop PostgreSQL
make server         # Run server (with auto-reload)
make agent          # Run agent
make build-server   # Build server binary
make build-agent    # Build agent binary
make test           # Run tests
make clean          # Clean build artifacts
```

### Running Tests
```bash
cd aggregator-server && go test ./...
cd aggregator-agent && go test ./...
```

## API Usage

### List All Agents
```bash
curl http://localhost:8080/api/v1/agents
```

### Trigger Update Scan
```bash
curl -X POST http://localhost:8080/api/v1/agents/{agent-id}/scan
```

### List All Updates
```bash
# All updates
curl http://localhost:8080/api/v1/updates

# Filter by severity
curl http://localhost:8080/api/v1/updates?severity=critical

# Filter by status
curl http://localhost:8080/api/v1/updates?status=pending
```

### Approve an Update
```bash
curl -X POST http://localhost:8080/api/v1/updates/{update-id}/approve
```

### Token Renewal (Agent Authentication)
```bash
# Exchange refresh token for new access token
curl -X POST http://localhost:8080/api/v1/agents/renew \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "uuid",
    "refresh_token": "long-lived-token"
  }'
```

### Dependency Workflow
```bash
# Dry run to check dependencies (automatically triggered by install)
curl -X POST http://localhost:8080/api/v1/updates/{update-id}/approve

# Confirm dependencies and install
curl -X POST http://localhost:8080/api/v1/updates/{update-id}/confirm-dependencies
```

## License

MIT License - see LICENSE file for details.

This is private development software. Use at your own risk.

## Third-Party Licenses

### Windows Update Package (Apache 2.0)
This project includes a modified version of the `windowsupdate` package from https://github.com/ceshihao/windowsupdate

Copyright 2022 Zheng Dayu
Licensed under the Apache License, Version 2.0
Original package: https://github.com/ceshihao/windowsupdate

The package is included in `aggregator-agent/pkg/windowsupdate/` and has been modified for integration with RedFlag's update management system.