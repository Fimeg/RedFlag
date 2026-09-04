.PHONY: help db-up db-down server agent clean kernel-enforcer test lint version up rebuild rebuild-clean down logs prune-cruft

# docker compose v2 (override with COMPOSE=docker-compose for the v1 plugin)
COMPOSE ?= docker compose

VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
AGENT_LDFLAGS := -X github.com/Fimeg/RedFlag/agent/internal/version.Version=$(VERSION) \
	-X github.com/Fimeg/RedFlag/agent/internal/version.ConfigVersion=$(VERSION) \
	-X github.com/Fimeg/RedFlag/agent/internal/version.BuildTime=$(BUILD_TIME)
SERVER_LDFLAGS := -X github.com/Fimeg/RedFlag/server/internal/version/versions.AgentVersion=$(VERSION) \
	-X github.com/Fimeg/RedFlag/server/internal/version/versions.ConfigVersion=$(VERSION)

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

version: ## Print current version
	@echo "VERSION=$(VERSION)"

up: ## Start the stack (NO rebuild — reuses the existing image)
	$(COMPOSE) up -d

rebuild: ## Rebuild changed layers and restart (the everyday command)
	$(COMPOSE) up -d --build

rebuild-clean: ## Full no-cache rebuild from scratch, then restart
	$(COMPOSE) build --no-cache && $(COMPOSE) up -d

down: ## Stop the stack
	$(COMPOSE) down

logs: ## Tail the server logs
	$(COMPOSE) logs -f server

prune-cruft: ## Remove retired RedFlag images (redflag-web, desktop-stage-test)
	-docker image rm redflag-web:latest redflag-desktop-stage-test:latest 2>/dev/null; true

fetch-desktop-windows: ## Fetch+verify the signed Windows Desktop from the latest release into ./dist
	@mkdir -p dist
	sh scripts/fetch-desktop-windows.sh amd64 ./dist
	@ls -lh dist/redflag-desktop.exe 2>/dev/null || echo "no Windows Desktop in the latest release yet"

db-up: ## Start PostgreSQL database
	docker-compose up -d postgres
	@echo "Waiting for database to be ready..."
	@sleep 3

db-down: ## Stop PostgreSQL database
	docker-compose down

server: ## Build and run the server
	cd server && go mod tidy && go run ./cmd/server/

agent: ## Build and run the agent
	cd agent && go mod tidy && go run cmd/agent/main.go

build-server: ## Build server binary
	cd server && go mod tidy && go build -ldflags "$(SERVER_LDFLAGS)" -o bin/server ./cmd/server/

build-agent: ## Build agent binary with version injection
	cd agent && go mod tidy && go build -ldflags "$(AGENT_LDFLAGS)" -o bin/agent ./cmd/agent/

clean: ## Clean build artifacts
	rm -rf server/bin agent/bin

build-all: ## Build all components with version from git tag
	@echo "Building all components at version $(VERSION)..."
	cd server && go mod tidy && go build -ldflags "$(SERVER_LDFLAGS)" -o redflag-server ./cmd/server/
	cd agent && go mod tidy && go build -ldflags "$(AGENT_LDFLAGS)" -o redflag-agent ./cmd/agent/
	@echo "Build complete!"

test: ## Run all tests
	cd server && go test -race -count=1 ./...
	cd agent && go test -race -count=1 ./...
	cd helper && cargo test

lint: ## Run linters and vet
	cd server && go vet ./...
	cd agent && go vet ./...
	cd helper && cargo clippy -- -D warnings

kernel-enforcer: ## Build eBPF kernel enforcer
	@echo "Building eBPF kernel enforcer..."
	@cd agent && clang -O2 -g -target bpf -mllvm -bpf-stack-size=4096 -c pkg-gate/pkg-gate.c -o pkg-gate/pkg-gate.o \
		-I/usr/src/kernels/$(shell uname -r)/vmlinux.h \
		-I/usr/src/kernels/$(shell uname -r)/tools/lib/bpf \
		-I/usr/src/kernels/$(shell uname -r)/tools/bpf/resolve_btfids/libbpf/include \
		-I/usr/src/kernels/$(shell uname -r)/tools/bpf/resolve_btfids/libbpf \
		-I/usr/src/kernels/$(shell uname -r)
	@echo "eBPF enforcer built successfully"

kernel-enforcer-clean: ## Clean eBPF kernel enforcer artifacts
	@echo "Cleaning eBPF kernel enforcer..."
	@cd agent && rm -f pkg-gate/pkg-gate.o
	@echo "Clean complete"
