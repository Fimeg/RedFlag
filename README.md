# RedFlag

> **⚠️ ALPHA SOFTWARE - NOT READY FOR PRODUCTION**
>
> This is experimental software in active development. Features may be broken, bugs are expected, and breaking changes happen frequently. Use at your own risk, preferably on test systems only. Seriously, don't put this in production yet.

**Self-hosted update management for homelabs**

Cross-platform agents • Web dashboard • Single binary deployment • No enterprise BS

```
v0.1.17 - Alpha Release
```

---

## What It Does

RedFlag lets you manage software updates across all your servers from one dashboard. Track pending updates, approve installs, and monitor system health without SSHing into every machine.

**Supported Platforms:**
- Linux (APT, DNF, Docker)
- Windows (Windows Update, Winget)
- Future: Proxmox integration planned

**Built With:**
- Go backend + PostgreSQL
- React dashboard
- Pull-based agents (firewall-friendly)
- JWT auth with refresh tokens

---

## Screenshots

| Dashboard | Agent Details | Update Management |
|-----------|---------------|-------------------|
| ![Dashboard](Screenshots/RedFlag%20Default%20Dashboard.png) | ![Linux Agent](Screenshots/RedFlag%20Linux%20Agent%20Details.png) | ![Updates](Screenshots/RedFlag%20Updates%20Dashboard.png) |

| Windows Support | History Tracking | Docker Integration |
|-----------------|------------------|-------------------|
| ![Windows Agent](Screenshots/RedFlag%20Windows%20Agent%20Details.png) | ![History](Screenshots/RedFlag%20History%20Dashboard.png) | ![Docker](Screenshots/RedFlag%20Docker%20Dashboard.png) |

---

## Quick Start

### Server Deployment (Docker)

```bash
# Clone and start
git clone https://github.com/Fimeg/RedFlag.git
cd RedFlag
docker-compose up -d

# Access web UI
open http://localhost:3000

# Follow setup wizard to create admin account
```

The setup wizard runs automatically on first launch. It'll generate secure secrets and walk you through creating an admin account.

---

### Agent Installation

**Linux (one-liner):**
```bash
curl -sfL https://your-server.com/install | sudo bash -s -- your-registration-token
```

**Windows (PowerShell):**
```powershell
iwr https://your-server.com/install.ps1 | iex
```

**Manual installation:**
```bash
# Download agent binary
wget https://your-server.com/download/linux/amd64/redflag-agent

# Register and install
chmod +x redflag-agent
sudo ./redflag-agent --server https://your-server.com --token your-token --register
```

Get registration tokens from the web dashboard under **Settings → Token Management**.

---

## Key Features

✓ **Secure by Default** - Registration tokens, JWT auth, rate limiting
✓ **Idempotent Installs** - Re-running installers won't create duplicate agents
✓ **Real-time Heartbeat** - Interactive operations with rapid polling
✓ **Dependency Handling** - Dry-run checks before installing updates
✓ **Multi-seat Tokens** - One token can register multiple agents
✓ **Audit Trails** - Complete history of all operations
✓ **Proxy Support** - HTTP/HTTPS/SOCKS5 for restricted networks
✓ **Native Services** - systemd on Linux, Windows Services on Windows

---

## Architecture

```
┌─────────────────┐
│  Web Dashboard  │  React + TypeScript
│  Port: 3000     │
└────────┬────────┘
         │ HTTPS + JWT Auth
┌────────▼────────┐
│  Server (Go)    │  PostgreSQL
│  Port: 8080     │
└────────┬────────┘
         │ Pull-based (agents check in every 5 min)
    ┌────┴────┬────────┐
    │         │        │
┌───▼──┐  ┌──▼──┐  ┌──▼───┐
│Linux │  │Windows│ │Linux │
│Agent │  │Agent  │ │Agent │
└──────┘  └───────┘ └──────┘
```

---

## Documentation

- **[API Reference](docs/API.md)** - Complete API documentation
- **[Configuration](docs/CONFIGURATION.md)** - CLI flags, env vars, config files
- **[Architecture](docs/ARCHITECTURE.md)** - System design and database schema
- **[Development](docs/DEVELOPMENT.md)** - Build from source, testing, contributing

---

## Security Notes

RedFlag uses:
- **Registration tokens** - One-time use tokens for secure agent enrollment
- **Refresh tokens** - 90-day sliding window, auto-renewal for active agents
- **SHA-256 hashing** - All tokens hashed at rest
- **Rate limiting** - Configurable API protection
- **Minimal privileges** - Agents run with least required permissions

For production deployments:
1. Change default admin password
2. Use HTTPS/TLS
3. Generate strong JWT secrets (setup wizard does this)
4. Configure firewall rules
5. Enable rate limiting

---

## Current Status

**What Works:**
- ✅ Cross-platform agent registration and updates
- ✅ Update scanning for all supported package managers
- ✅ Dry-run dependency checking before installation
- ✅ Real-time heartbeat and rapid polling
- ✅ Multi-seat registration tokens
- ✅ Native service integration (systemd, Windows Services)
- ✅ Web dashboard with full agent management
- ✅ Docker integration for container image updates

**Known Issues:**
- Windows Winget detection needs debugging
- Some Windows Updates may reappear after installation (known Windows Update quirk)

**Planned Features:**
- Proxmox VM/container integration
- Agent auto-update system
- WebSocket real-time updates
- Mobile-responsive dashboard improvements

---

## Development

```bash
# Start local development environment
make db-up
make server   # Terminal 1
make agent    # Terminal 2
make web      # Terminal 3
```

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for detailed build instructions.

---

## Alpha Release Notice

This is alpha software built for homelabs and self-hosters. It's functional and actively used, but:

- Expect occasional bugs
- Backup your data
- Security model is solid but not audited
- Breaking changes may happen between versions
- Documentation is a work in progress

That said, it works well for its intended use case. Issues and feedback welcome!

---

## License

MIT License - See [LICENSE](LICENSE) for details

**Third-Party Components:**
- Windows Update integration based on [windowsupdate](https://github.com/ceshihao/windowsupdate) (Apache 2.0)

---

## Project Goals

RedFlag aims to be:
- **Simple** - Deploy in 5 minutes, understand in 10
- **Honest** - No enterprise marketing speak, just useful software
- **Homelab-first** - Built for real use cases, not investor pitches
- **Self-hosted** - Your data, your infrastructure

If you're looking for an enterprise-grade solution with SLAs and support contracts, this isn't it. If you want to manage updates across your homelab without SSH-ing into every server, welcome aboard.

---

**Made with ☕ for homelabbers, by homelabbers**
