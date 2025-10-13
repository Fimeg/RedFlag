.PHONY: help db-up db-down server agent clean

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

db-up: ## Start PostgreSQL database
	docker-compose up -d aggregator-db
	@echo "Waiting for database to be ready..."
	@sleep 3

db-down: ## Stop PostgreSQL database
	docker-compose down

server: ## Build and run the server
	cd aggregator-server && go mod tidy && go run cmd/server/main.go

agent: ## Build and run the agent
	cd aggregator-agent && go mod tidy && go run cmd/agent/main.go

build-server: ## Build server binary
	cd aggregator-server && go build -o bin/aggregator-server cmd/server/main.go

build-agent: ## Build agent binary
	cd aggregator-agent && go build -o bin/aggregator-agent cmd/agent/main.go

clean: ## Clean build artifacts
	rm -rf aggregator-server/bin aggregator-agent/bin

test: ## Run tests
	cd aggregator-server && go test ./...
	cd aggregator-agent && go test ./...
