# RedFlag (Aggregator)

⚠️ PRIVATE DEVELOPMENT - NOT FOR PUBLIC USE

This is a private development repository for version retention only.

## Status

- **Active Development**: In progress
- **Not Production Ready**: Do not use
- **Breaking Changes Expected**: APIs will change
- **No Support Available**: This is not released software

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
- No rate limiting on API endpoints (security improvement needed)
- No real-time WebSocket updates
- Proxmox integration is broken (needs complete rewrite)
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

## Architecture

```
┌─────────────────┐
│  Web Dashboard  │  React + TypeScript + TailwindCSS
└────────┬────────┘
         │ HTTPS
┌────────▼────────┐
│  Server (Go)    │  Production Ready with PostgreSQL
│  + PostgreSQL   │
└────────┬────────┘
         │ Pull-based (agents check in every 5 min)
    ┌────┴────┬────────┐
    │         │        │
┌───▼──┐  ┌──▼──┐  ┌──▼───┐
│Linux │  │Windows│  │Linux │
│Agent │  │Agent  │  │Agent │
└──────┘  └───────┘  └──────┘
```

## Project Structure

```
RedFlag/
├── aggregator-server/      # Go server (Gin + PostgreSQL)
│   ├── cmd/server/         # Main entry point
│   ├── internal/
│   │   ├── api/            # HTTP handlers & middleware
│   │   │   └── handlers/   # API endpoint handlers
│   │   ├── database/       # Database layer & migrations
│   │   │   ├── migrations/ # Database schema migrations
│   │   │   └── queries/    # Database query functions
│   │   ├── models/         # Data models and structs
│   │   ├── services/       # Business logic services
│   │   ├── utils/          # Utility functions
│   │   └── config/         # Configuration management
│   └── go.mod

├── aggregator-agent/       # Go agent (cross-platform)
│   ├── cmd/agent/          # Main entry point
│   ├── internal/
│   │   ├── cache/          # Local cache system for offline viewing
│   │   ├── client/         # API client with token renewal
│   │   ├── config/         # Configuration management
│   │   ├── display/        # Terminal output formatting
│   │   ├── installer/      # Update installers
│   │   │   ├── apt.go      # APT package installer
│   │   │   ├── dnf.go      # DNF package installer
│   │   │   ├── docker.go   # Docker image installer
│   │   │   ├── windows.go  # Windows installer base
│   │   │   ├── winget.go   # Winget package installer
│   │   │   ├── security.go # Security utilities
│   │   │   └── sudoers.go  # Sudo management
│   │   ├── scanner/        # Update scanners
│   │   │   ├── apt.go      # APT package scanner
│   │   │   ├── dnf.go      # DNF package scanner
│   │   │   ├── docker.go   # Docker image scanner
│   │   │   ├── registry.go # Docker registry client
│   │   │   ├── windows.go  # Windows Update scanner
│   │   │   ├── winget.go   # Winget package scanner
│   │   │   └── windows_*.go # Windows Update API components
│   │   ├── system/         # System information collection
│   │   │   ├── info.go     # System metrics
│   │   │   └── windows.go  # Windows system info
│   │   └── executor/       # Command execution
│   ├── install.sh          # Linux installation script
│   ├── uninstall.sh        # Linux uninstallation script
│   └── go.mod

├── aggregator-web/         # React dashboard
├── docker-compose.yml      # PostgreSQL for local dev
├── Makefile                # Common tasks
└── README.md               # This file
```

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

## Security

- Agent Authentication: Refresh token system with 90-day sliding window + 24h access tokens
- SHA-256 token hashing for secure storage
- Pull-based Model: Agents poll server (firewall-friendly)
- Command Validation: Whitelisted commands only
- TLS Required: Production deployments must use HTTPS
- Token Renewal: `/renew` endpoint prevents daily re-registration

## License

MIT License - see LICENSE file for details.

This is private development software. Use at your own risk.