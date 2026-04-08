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

# Port assignments for local development
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

# Colors
GREEN  := \033[0;32m
RED    := \033[0;31m
YELLOW := \033[1;33m
CYAN   := \033[0;36m
BOLD   := \033[1m
NC     := \033[0m

# start-service: launch a service in the background if its HTTP port is not already in use
# Usage: $(call start-service,name,http_port,admin_port,extra_env)
define start-service
	@if curl -sf http://localhost:$(3)/healthz > /dev/null 2>&1; then \
		printf "  $(CYAN)$(1)$(NC) already running on :$(2)\n"; \
	else \
		printf "  Starting $(CYAN)$(1)$(NC) on :$(2) (admin :$(3))...\n"; \
		SERVICE_NAME=$(1) HTTP_PORT=$(2) ADMIN_PORT=$(3) $(4) \
			go run ./services/$(1)/cmd/ > /tmp/bookinfo-$(1).log 2>&1 & \
	fi
endef

# wait-healthy: poll a service's admin healthz endpoint with timeout
# Usage: $(call wait-healthy,name,admin_port)
define wait-healthy
	@timeout=15; elapsed=0; \
	while [ $$elapsed -lt $$timeout ]; do \
		if curl -sf http://localhost:$(2)/healthz > /dev/null 2>&1; then \
			break; \
		fi; \
		sleep 1; \
		elapsed=$$((elapsed + 1)); \
	done
endef

.PHONY: run
run: ## Start all services locally (background, memory backend)
	@printf "\n$(BOLD)Starting bookinfo services...$(NC)\n\n"
	$(call start-service,ratings,$(RATINGS_HTTP_PORT),$(RATINGS_ADMIN_PORT),)
	$(call start-service,details,$(DETAILS_HTTP_PORT),$(DETAILS_ADMIN_PORT),)
	$(call wait-healthy,ratings,$(RATINGS_ADMIN_PORT))
	$(call wait-healthy,details,$(DETAILS_ADMIN_PORT))
	$(call start-service,reviews,$(REVIEWS_HTTP_PORT),$(REVIEWS_ADMIN_PORT),RATINGS_SERVICE_URL=http://localhost:$(RATINGS_HTTP_PORT))
	$(call start-service,notification,$(NOTIFICATION_HTTP_PORT),$(NOTIFICATION_ADMIN_PORT),)
	$(call wait-healthy,reviews,$(REVIEWS_ADMIN_PORT))
	$(call wait-healthy,notification,$(NOTIFICATION_ADMIN_PORT))
	$(call start-service,productpage,$(PRODUCTPAGE_HTTP_PORT),$(PRODUCTPAGE_ADMIN_PORT),DETAILS_SERVICE_URL=http://localhost:$(DETAILS_HTTP_PORT) REVIEWS_SERVICE_URL=http://localhost:$(REVIEWS_HTTP_PORT))
	$(call wait-healthy,productpage,$(PRODUCTPAGE_ADMIN_PORT))
	@printf "\n$(BOLD)Service Status$(NC)\n"
	@printf "  %-14s %-10s %-10s %s\n" "SERVICE" "API" "ADMIN" "STATUS"
	@printf "  %-14s %-10s %-10s %s\n" "──────────────" "──────────" "──────────" "──────"
	@for entry in \
		"ratings:$(RATINGS_HTTP_PORT):$(RATINGS_ADMIN_PORT)" \
		"details:$(DETAILS_HTTP_PORT):$(DETAILS_ADMIN_PORT)" \
		"reviews:$(REVIEWS_HTTP_PORT):$(REVIEWS_ADMIN_PORT)" \
		"notification:$(NOTIFICATION_HTTP_PORT):$(NOTIFICATION_ADMIN_PORT)" \
		"productpage:$(PRODUCTPAGE_HTTP_PORT):$(PRODUCTPAGE_ADMIN_PORT)"; \
	do \
		name=$${entry%%:*}; rest=$${entry#*:}; \
		http=$${rest%%:*}; admin=$${rest#*:}; \
		if curl -sf http://localhost:$$admin/healthz > /dev/null 2>&1; then \
			printf "  $(GREEN)%-14s$(NC) :%-9s :%-9s $(GREEN)healthy$(NC)\n" "$$name" "$$http" "$$admin"; \
		else \
			printf "  $(RED)%-14s$(NC) :%-9s :%-9s $(RED)failed$(NC)  (check /tmp/bookinfo-$$name.log)\n" "$$name" "$$http" "$$admin"; \
		fi; \
	done
	@printf "\n  Logs: /tmp/bookinfo-{service}.log\n\n"

.PHONY: stop
stop: ## Stop all locally running services
	@echo "Stopping bookinfo services..."
	@for port in $(PRODUCTPAGE_HTTP_PORT) $(PRODUCTPAGE_ADMIN_PORT) \
	             $(REVIEWS_HTTP_PORT) $(REVIEWS_ADMIN_PORT) \
	             $(NOTIFICATION_HTTP_PORT) $(NOTIFICATION_ADMIN_PORT) \
	             $(DETAILS_HTTP_PORT) $(DETAILS_ADMIN_PORT) \
	             $(RATINGS_HTTP_PORT) $(RATINGS_ADMIN_PORT); do \
		pid=$$(lsof -ti :$$port 2>/dev/null); \
		if [ -n "$$pid" ]; then kill $$pid 2>/dev/null || true; fi; \
	done
	@echo "All services stopped."

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
