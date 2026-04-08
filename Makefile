.DEFAULT_GOAL := help

MODULE := github.com/kaio6fellipe/event-driven-bookinfo
SERVICES := productpage details reviews ratings notification

# ─── Build ──────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build a single service: make build SERVICE=<name>
ifndef SERVICE
	$(error SERVICE is not set. Usage: make build SERVICE=<name>)
endif
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(SERVICE) ./services/$(SERVICE)/cmd/

.PHONY: build-all
build-all: ## Build all 5 services
	@for svc in $(SERVICES); do \
		echo "Building $$svc..."; \
		CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$$svc ./services/$$svc/cmd/ || exit 1; \
	done
	@echo "All services built successfully."

# ─── Test ───────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: test-cover
test-cover: ## Run tests with coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

.PHONY: test-race
test-race: ## Run tests with race detector
	go test -race -count=1 ./...

# ─── Quality ────────────────────────────────────────────────────────────────

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format all Go source files
	gofmt -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: mod-tidy
mod-tidy: ## Tidy go module dependencies
	go mod tidy

# ─── Security ──────────────────────────────────────────────────────────────

GOVULNCHECK := $(shell go env GOPATH)/bin/govulncheck

.PHONY: vuln
vuln: ## Scan Go dependencies for known CVEs (requires govulncheck)
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || { echo "Installing govulncheck..."; go install golang.org/x/vuln/cmd/govulncheck@latest; }
	$(GOVULNCHECK) ./...

.PHONY: gitleaks
gitleaks: ## Scan git history for leaked secrets (requires gitleaks)
	gitleaks detect --source . -v

.PHONY: trivy
trivy: ## Scan Docker images for vulnerabilities (requires trivy)
	@for svc in $(SERVICES); do \
		echo "Scanning event-driven-bookinfo/$$svc:latest..."; \
		trivy image --severity HIGH,CRITICAL event-driven-bookinfo/$$svc:latest || exit 1; \
	done

# ─── Docker ─────────────────────────────────────────────────────────────────

.PHONY: docker-build
docker-build: ## Build Docker image for one service: make docker-build SERVICE=<name>
ifndef SERVICE
	$(error SERVICE is not set. Usage: make docker-build SERVICE=<name>)
endif
	docker build -f build/Dockerfile.$(SERVICE) -t event-driven-bookinfo/$(SERVICE):latest .

.PHONY: docker-build-all
docker-build-all: ## Build Docker images for all 5 services
	@for svc in $(SERVICES); do \
		echo "Building Docker image for $$svc..."; \
		docker build -f build/Dockerfile.$$svc -t event-driven-bookinfo/$$svc:latest . || exit 1; \
	done
	@echo "All Docker images built successfully."

# ─── Run ───────────────────────────────────────────────────────────────────

# Colors
GREEN  := \033[0;32m
RED    := \033[0;31m
CYAN   := \033[0;36m
BOLD   := \033[1m
NC     := \033[0m

# Port assignments (must match docker-compose.yml host ports)
RATINGS_HTTP_PORT      := 8081
RATINGS_ADMIN_PORT     := 9091
DETAILS_HTTP_PORT      := 8082
DETAILS_ADMIN_PORT     := 9092
REVIEWS_HTTP_PORT      := 8083
REVIEWS_ADMIN_PORT     := 9093
NOTIFICATION_HTTP_PORT := 8084
NOTIFICATION_ADMIN_PORT:= 9094
PRODUCTPAGE_HTTP_PORT  := 8080
PRODUCTPAGE_ADMIN_PORT := 9090

.PHONY: run
run: ## Start all services via docker compose (detached, builds images)
	@printf "\n$(BOLD)Starting bookinfo services...$(NC)\n\n"
	@docker compose up -d --build
	@printf "\n$(BOLD)Waiting for services to be healthy...$(NC)\n\n"
	@failed=0; \
	for entry in \
		"ratings:$(RATINGS_HTTP_PORT):$(RATINGS_ADMIN_PORT)" \
		"details:$(DETAILS_HTTP_PORT):$(DETAILS_ADMIN_PORT)" \
		"reviews:$(REVIEWS_HTTP_PORT):$(REVIEWS_ADMIN_PORT)" \
		"notification:$(NOTIFICATION_HTTP_PORT):$(NOTIFICATION_ADMIN_PORT)" \
		"productpage:$(PRODUCTPAGE_HTTP_PORT):$(PRODUCTPAGE_ADMIN_PORT)"; \
	do \
		name=$${entry%%:*}; rest=$${entry#*:}; \
		http=$${rest%%:*}; admin=$${rest#*:}; \
		elapsed=0; ready=0; \
		while [ $$elapsed -lt 15 ]; do \
			if curl -sf http://localhost:$$admin/healthz > /dev/null 2>&1; then \
				ready=1; break; \
			fi; \
			sleep 1; elapsed=$$((elapsed + 1)); \
		done; \
		if [ $$ready -eq 1 ]; then \
			printf "  $(GREEN)%-14s$(NC) :%-9s :%-9s $(GREEN)healthy$(NC)\n" "$$name" "$$http" "$$admin"; \
		else \
			printf "  $(RED)%-14s$(NC) :%-9s :%-9s $(RED)failed$(NC)  (docker compose logs $$name)\n" "$$name" "$$http" "$$admin"; \
			failed=1; \
		fi; \
	done; \
	printf "\n"; \
	if [ $$failed -eq 0 ]; then \
		printf "  $(GREEN)$(BOLD)All services healthy!$(NC)\n"; \
	else \
		printf "  $(RED)Some services failed to start.$(NC) Run $(CYAN)make run-logs$(NC) to check.\n"; \
	fi; \
	printf "  Logs: $(CYAN)make run-logs$(NC)\n"; \
	printf "  App:  $(CYAN)http://localhost:$(PRODUCTPAGE_HTTP_PORT)$(NC)\n\n"
	@$(MAKE) --no-print-directory seed

.PHONY: stop
stop: ## Stop all services and remove containers (keeps data)
	docker compose down

.PHONY: clean-data
clean-data: ## Stop all services and remove postgres data volume
	docker compose down -v

.PHONY: seed
seed: ## Seed postgres with sample data (idempotent)
	@printf "\n$(BOLD)Seeding databases...$(NC)\n\n"
	@for svc in details ratings reviews notification; do \
		seed_file="services/$$svc/seeds/seed.sql"; \
		if [ -f "$$seed_file" ]; then \
			docker compose exec -T postgres psql -U bookinfo -d bookinfo_$$svc -f /dev/stdin < "$$seed_file" > /dev/null 2>&1; \
			printf "  $(GREEN)%-14s$(NC) seeded\n" "$$svc"; \
		fi; \
	done
	@printf "\n"

.PHONY: migrate
migrate: ## Re-run database migrations (restarts backend services)
	@printf "\n$(BOLD)Running migrations...$(NC)\n\n"
	docker compose restart ratings details reviews notification
	@printf "\n  Migrations applied (services restarted).\n\n"

.PHONY: run-logs
run-logs: ## Tail logs from all services (Ctrl+C to stop)
	docker compose logs -f

# ─── E2E ────────────────────────────────────────────────────────────────────

.PHONY: e2e
e2e: ## Run E2E tests via docker-compose
	bash test/e2e/run-tests.sh

.PHONY: e2e-postgres
e2e-postgres: ## Run E2E tests with PostgreSQL backend
	COMPOSE_FILE="docker-compose.yml docker-compose.postgres.yml" bash test/e2e/run-tests.sh

# ─── Cleanup ────────────────────────────────────────────────────────────────

.PHONY: clean
clean: ## Remove build output directories
	rm -rf bin/ dist/

# ─── Help ───────────────────────────────────────────────────────────────────

.PHONY: help
help: ## List all available targets
	@echo "Usage: make <target> [SERVICE=<name>]"
	@echo ""
	@echo "Services: $(SERVICES)"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
