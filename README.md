# RedFlag

> **⚠️ ALPHA SOFTWARE - NOT READY FOR PRODUCTION**
>
> This is experimental software in active development. Features may be broken, bugs are expected, and breaking changes happen frequently. Use at your own risk, preferably on test systems only. Seriously, don't put this in production yet.  A large update will be released soon which has some fairly significant changes. Recommendation is to hold off unless ALPHA Testing, knowing that a reinstall of all agents and server is most likely.

**Update 11/13/2025:** With much consternation - my deveopment machine's primary /home (ssd) has failed and I am rebuilding my dev eviornment. Quite luckily, I saved a backup of my builds offsite but I realize I had failed to save my nearly 50 md files for technical debt, features etc. I'm now experiencing joy to restore some of those. - worst it'll slow project a little, best means we get to have a second look through every file now. I'll release the build soon - but I need to make sure its both the latest, and as feature complete in function as the current. More pictures and updates on our discord. https://discord.gg/mtaU98fVqr Cheers!

**Self-hosted update management for homelabs**

Cross-platform agents • Web dashboard • Single binary deployment • No enterprise BS

```
v0.1.18 - Alpha Release
```

**Latest:** Enhanced disk detection, redesigned agent UI with workflow tabs, improved cache invalidation. Testing kernel updates on cloned test benches - help find bugs. [Update instructions below](#updating).

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

| Live Operations | History Tracking | Docker Integration |
|-----------------|------------------|-------------------|
| ![Live Ops](Screenshots/RedFlag%20Live%20Operations%20-%20Failed%20Dashboard.png) | ![History](Screenshots/RedFlag%20History%20Dashboard.png) | ![Docker](Screenshots/RedFlag%20Docker%20Dashboard.png) |

<details>
<summary><b>More Screenshots</b> (click to expand)</summary>

| Heartbeat System | Registration Tokens | Settings Page |
|------------------|---------------------|---------------|
| ![Heartbeat](Screenshots/RedFlag%20Heartbeat%20System.png) | ![Tokens](Screenshots/RedFlag%20Registration%20Tokens.jpg) | ![Settings](Screenshots/RedFlag%20Settings%20Page.jpg) |

| Linux Update History | Windows Agent Details | Agent List |
|---------------------|----------------------|------------|
| ![Linux History](Screenshots/RedFlag%20Linux%20Agent%20History%20Extended.png) | ![Windows Agent](Screenshots/RedFlag%20Windows%20Agent%20Details.png) | ![Agent List](Screenshots/RedFlag%20Agent%20List.png) |

| Windows Update History |
|------------------------|
| ![Windows History](Screenshots/RedFlag%20Windows%20Agent%20History%20Extended.png) |

</details>

---

## Quick Start

### Server Deployment (Docker)

```bash
# Clone and configure
git clone https://github.com/Fimeg/RedFlag.git
cd RedFlag
cp config/.env.bootstrap.example config/.env
docker-compose build
docker-compose up -d

# Access web UI and run setup
open http://localhost:3000
# Follow setup wizard, then copy generated .env content

# Restart with new configuration
docker-compose down
docker-compose up -d
```

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

### Updating

To update to the latest version:

```bash
git pull && docker-compose down && docker-compose build --no-cache && docker-compose up -d
```

---

<details>
<summary><b>Full Reinstall (Nuclear Option)</b></summary>

If things get really broken or you want to start completely fresh:

```bash
docker-compose down -v --remove-orphans && \
  rm config/.env && \
  docker-compose build --no-cache && \
  cp config/.env.bootstrap.example config/.env && \
  docker-compose up -d
```

**What this does:**
- `down -v` - Stops containers and **wipes all data** (including the database)
- `--remove-orphans` - Cleans up leftover containers
- `rm config/.env` - Removes old server config
- `build --no-cache` - Rebuilds images from scratch
- `cp config/.env.bootstrap.example` - Resets to bootstrap mode for setup wizard
- `up -d` - Starts fresh in background

**Warning:** This deletes everything - all agents, update history, configurations. You'll need to handle existing agents:

**Option 1 - Re-register agents:**
- Remove agent config: `sudo rm /etc/aggregator/config.json` (Linux) or `C:\ProgramData\RedFlag\config.json` (Windows)
- Re-run the one-liner installer with new registration token
- Scripts handle override/update automatically (one agent per OS install)

**Option 2 - Clean uninstall/reinstall:**
- Uninstall agent completely first
- Then run installer with new token

</details>

---

<details>
<summary><b>Full Uninstall</b></summary>

**Uninstall Server:**
```bash
docker-compose down -v --remove-orphans
rm config/.env
```

**Uninstall Linux Agent:**
```bash
# Using uninstall script (recommended)
sudo bash aggregator-agent/uninstall.sh

# Remove agent configuration
sudo rm /etc/aggregator/config.json

# Remove agent user (optional - preserves logs)
sudo userdel -r redflag-agent
```

**Uninstall Windows Agent:**
```powershell
# Stop and remove service
Stop-Service RedFlagAgent
sc.exe delete RedFlagAgent

# Remove files
Remove-Item "C:\Program Files\RedFlag\redflag-agent.exe"
Remove-Item "C:\ProgramData\RedFlag\config.json"
```

</details>

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
