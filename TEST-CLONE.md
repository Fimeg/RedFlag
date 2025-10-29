# Test Fresh Clone Instructions

## Prerequisites
- Go 1.21+ must be installed
- Docker & Docker Compose must be running

## Quick Test on New Machine/Location

```bash
# Clone fresh
git clone https://github.com/Fimeg/RedFlag.git
cd RedFlag

# Docker deployment (recommended)
docker-compose up -d

# One-time server setup
docker-compose exec server ./redflag-server --setup

# Run database migrations
docker-compose exec server ./redflag-server --migrate

# Restart server with config
docker-compose restart server

# Test server: http://localhost:8080
# Admin: http://localhost:8080/admin
```

## What Should Work
- ✅ Server setup wizard creates .env file
- ✅ Database migrations run without errors
- ✅ Server starts on port 8080
- ✅ Admin interface accessible
- ✅ Can generate registration tokens
- ✅ Agent registers and appears in UI
- ✅ Agent shows system information
- ✅ Agent performs update scan

## Expected Breaking Changes
Old agents won't work - need fresh registration with tokens.

## Version Check
- Agent should report v0.1.16
- Server should show v0.1.16 as latest version