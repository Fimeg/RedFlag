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

### Main Dashboard
![Main Dashboard](Screenshots/RedFlag%20Default%20Dashboard.png)
Overview showing agent status, system metrics, and update statistics

### Updates Management
![Updates Dashboard](Screenshots/RedFlag%20Updates%20Dashboard.png)
Comprehensive update listing with filtering, approval, and dependency confirmation

### Agent Details
![Agent Details](Screenshots/RedFlag%20Agent%20Details.png)
Detailed agent information including system specs, last check-in, and individual update management

### Windows Agent Support
![Windows Agent](Screenshots/RedFlag%20Windows%20Agent%20Details.png)
Cross-platform support for Windows Updates and Winget package management

### History & Audit Trail
![History Dashboard](Screenshots/RedFlag%20History%20Dashboard.png)
Complete audit trail of all update activities and command execution

### Live Operations
![Live Operations](Screenshots/RedFlag%20Live%20Operations%20-%20Failed%20Dashboard.png)
Real-time view of update operations with success/failure tracking

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