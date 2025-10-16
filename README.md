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
- Linux agents with APT + Docker scanning
- Local CLI tools for agent management
- Update installation system (alpha)

## What This Isn't

- Not ready for public use
- Not documented for external users
- Not supported or maintained for others
- Not stable (active development)

## Current Capabilities

### Working Features
- Server backend with REST API
- Agent registration and check-in
- Update discovery for APT packages and Docker images
- Update approval workflow
- Web dashboard with agent management
- Local CLI tools (--scan, --status, --list-updates, --export)
- Update installation system (alpha quality)

### Known Limitations
- Update installation is minimally tested
- DNF/RPM scanner incomplete
- No rate limiting on API endpoints
- No Windows agent support
- No real-time WebSocket updates

## Screenshots

### Default Dashboard
![Default Dashboard](Screenshots/RedFlag%20Default%20Dashboard.png)
Main overview showing agent status, system metrics, and update statistics

### Updates Management
![Updates Dashboard](Screenshots/RedFlag%20Updates%20Dashboard.png)
Comprehensive update listing with filtering, approval, and bulk operations

### Agent Details
![Agent Dashboard](Screenshots/RedFlag%20Agent%20Dashboard.png)
Detailed agent information including system specs, last check-in, and individual update management

### Docker Container Management
![Docker Dashboard](Screenshots/RedFlag%20Docker%20Dashboard.png)
Docker-specific interface for container image updates and management

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
│Linux │  │Linux│  │Linux │
│Agent │  │Agent│  │Agent │
└──────┘  └─────┘  └──────┘
```

## Project Structure

```
RedFlag/
├── aggregator-server/      # Go server (Gin + PostgreSQL)
│   ├── cmd/server/         # Main entry point
│   ├── internal/
│   │   ├── api/            # HTTP handlers & middleware
│   │   ├── database/       # Database layer & migrations
│   │   ├── models/         # Data models
│   │   └── config/         # Configuration
│   └── go.mod

├── aggregator-agent/       # Go agent
│   ├── cmd/agent/          # Main entry point
│   ├── internal/
│   │   ├── client/         # API client
│   │   ├── installer/       # Update installers (APT, DNF, Docker)
│   │   ├── scanner/        # Update scanners (APT, Docker, DNF/RPM)
│   │   ├── system/         # System information collection
│   │   └── config/         # Configuration
│   └── go.mod

├── aggregator-web/         # React dashboard
├── docker-compose.yml      # PostgreSQL for local dev
├── Makefile                # Common tasks
└── README.md               # This file
```

## Database Schema

Key Tables:
- `agents` - Registered agents with system metadata
- `update_packages` - Discovered updates
- `agent_commands` - Command queue for agents
- `update_logs` - Execution logs
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
  "token": "jwt-token",
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

## Security

- Agent Authentication: JWT tokens with 24h expiry
- Pull-based Model: Agents poll server (firewall-friendly)
- Command Validation: Whitelisted commands only
- TLS Required: Production deployments must use HTTPS

## License

MIT License - see LICENSE file for details.

This is private development software. Use at your own risk.