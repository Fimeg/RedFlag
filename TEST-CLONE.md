# Test Fresh Clone Instructions

## Prerequisites
- Go 1.21+ must be installed
- Docker & Docker Compose must be running

## Quick Test on New Machine/Location

```bash
# Clone fresh
git clone https://github.com/Fimeg/RedFlag.git
cd RedFlag

# Build components (requires Go)
make build-all

# Start database
docker-compose up -d postgres

# Configure server (interactive)
cd aggregator-server
./redflag-server --setup

# Run database migrations
./redflag-server --migrate

# Start server
./redflag-server

# In another terminal, generate and deploy agent
cd ../aggregator-agent

# Get token from admin UI: http://localhost:8080/admin
# Then deploy:
./aggregator-agent --server http://localhost:8080 --token <YOUR_TOKEN>
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