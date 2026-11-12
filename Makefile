# apm_master developer tasks. `make help` lists them.
GO            ?= go
BIN           ?= bin
INTEGRATION_DSN ?= pgx5://apm_user:apm_password@localhost:5433/apm_db?sslmode=disable

.PHONY: help build fmt vet lint test test-race test-int test-e2e migrate-up migrate-status run deps-up deps-down

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-14s %s\n", $$1, $$2}'

build: ## Build apm, scheduler and migrate into ./bin
	$(GO) build -o $(BIN)/apm ./cmd/apm
	$(GO) build -o $(BIN)/scheduler ./cmd/scheduler
	$(GO) build -o $(BIN)/migrate ./cmd/migrate

fmt: ## Format code
	gofmt -w .

vet: ## Run go vet
	$(GO) vet ./...

lint: ## Run golangci-lint (see .golangci.yml)
	golangci-lint run ./...

test: ## Unit tests (no external services needed)
	$(GO) test ./...

test-race: ## Unit tests with the race detector
	$(GO) test -race ./...

test-int: ## Unit + DB-backed tests against INTEGRATION_DSN
	APM_INTEGRATION_DSN=$(INTEGRATION_DSN) $(GO) test -count=1 -p 1 ./...

test-e2e: ## End-to-end tests against a running server (APM_E2E_URL)
	$(GO) test -tags=e2e ./test/e2e/...

migrate-up: ## Apply migrations
	$(GO) run ./cmd/migrate -action up

migrate-status: ## Show migration version
	$(GO) run ./cmd/migrate -action status

run: ## Run the API locally
	$(GO) run ./cmd/apm

deps-up: ## Start Postgres, Redis, Kafka and the Mailpit mail catcher in Docker
	docker compose -f deploy/docker/docker-compose.yml up -d postgres redis kafka mailpit

deps-down: ## Stop the Docker dependencies
	docker compose -f deploy/docker/docker-compose.yml down
