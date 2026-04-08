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
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
