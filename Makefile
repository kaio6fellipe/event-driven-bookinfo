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

# ─── Kubernetes (local) ──────────────────────────────────────────────────────

K8S_CLUSTER    := bookinfo-local
K8S_CONTEXT    := k3d-$(K8S_CLUSTER)
K8S_NS_PLATFORM     := platform
K8S_NS_OBSERVABILITY := observability
K8S_NS_BOOKINFO     := bookinfo
KUBECTL        := kubectl --context=$(K8S_CONTEXT)
HELM           := helm --kube-context=$(K8S_CONTEXT)

# Context safety guard — call at the top of every k8s target.
define k8s-guard
	@if ! kubectl config get-contexts $(K8S_CONTEXT) >/dev/null 2>&1; then \
		printf "$(RED)ERROR: context '$(K8S_CONTEXT)' not found.$(NC)\n"; \
		printf "  Run $(CYAN)make k8s-cluster$(NC) first.\n"; \
		exit 1; \
	fi
	@if ! echo "$(K8S_CONTEXT)" | grep -q '^k3d-'; then \
		printf "$(RED)ERROR: context '$(K8S_CONTEXT)' is not a k3d cluster. Refusing to proceed.$(NC)\n"; \
		exit 1; \
	fi
endef

.PHONY: k8s-cluster
k8s-cluster: ## Create k3d cluster with port mappings for Gateway + observability
	@if k3d cluster list $(K8S_CLUSTER) >/dev/null 2>&1; then \
		printf "$(GREEN)Cluster '$(K8S_CLUSTER)' already exists.$(NC)\n"; \
	else \
		printf "$(BOLD)Creating k3d cluster '$(K8S_CLUSTER)'...$(NC)\n"; \
		k3d cluster create $(K8S_CLUSTER) \
			--api-port 6550 \
			-p "8080:80@loadbalancer" \
			-p "8443:443@loadbalancer" \
			-p "3000:30300@server:0" \
			-p "9090:30900@server:0" \
			--k3s-arg "--disable=traefik@server:0" \
			--wait; \
	fi
	@printf "$(BOLD)Verifying cluster...$(NC)\n"
	$(KUBECTL) cluster-info
	@printf "\n$(GREEN)$(BOLD)Cluster '$(K8S_CLUSTER)' ready.$(NC)\n\n"

.PHONY: stop-k8s
stop-k8s: ## Delete k3d cluster and all resources
	@if k3d cluster list $(K8S_CLUSTER) >/dev/null 2>&1; then \
		printf "$(BOLD)Deleting cluster '$(K8S_CLUSTER)'...$(NC)\n"; \
		k3d cluster delete $(K8S_CLUSTER); \
		printf "$(GREEN)Cluster deleted.$(NC)\n"; \
	else \
		printf "Cluster '$(K8S_CLUSTER)' does not exist.\n"; \
	fi

.PHONY: k8s-platform
k8s-platform: ## Install platform: Envoy Gateway, Strimzi, Kafka, Argo Events
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Platform Layer ═══$(NC)\n\n"
	@$(KUBECTL) create namespace $(K8S_NS_PLATFORM) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@printf "$(BOLD)[1/5] Installing Envoy Gateway...$(NC)\n"
	@$(HELM) upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
		--version v1.7.0 \
		-n envoy-gateway-system --create-namespace \
		--wait --timeout 120s
	@printf "  $(GREEN)Envoy Gateway controller ready.$(NC)\n"
	@printf "$(BOLD)[2/5] Installing Strimzi operator...$(NC)\n"
	@$(HELM) repo add strimzi https://strimzi.io/charts/ --force-update 2>/dev/null || true
	@$(HELM) upgrade --install strimzi strimzi/strimzi-kafka-operator \
		-n $(K8S_NS_PLATFORM) \
		-f deploy/platform/local/strimzi-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Strimzi operator ready.$(NC)\n"
	@printf "$(BOLD)[3/5] Deploying Kafka cluster (KRaft)...$(NC)\n"
	@$(KUBECTL) apply -f deploy/platform/local/kafka-nodepool.yaml
	@$(KUBECTL) apply -f deploy/platform/local/kafka-cluster.yaml
	@printf "  Waiting for Kafka cluster to be ready (this takes ~60-90s)...\n"
	@$(KUBECTL) wait kafka/bookinfo-kafka -n $(K8S_NS_PLATFORM) \
		--for=condition=Ready --timeout=300s
	@printf "  $(GREEN)Kafka cluster ready.$(NC)\n"
	@printf "$(BOLD)[4/5] Installing Argo Events controller...$(NC)\n"
	@$(HELM) repo add argo https://argoproj.github.io/argo-helm --force-update 2>/dev/null || true
	@$(HELM) upgrade --install argo-events argo/argo-events \
		-n $(K8S_NS_PLATFORM) \
		-f deploy/platform/local/argo-events-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Argo Events controller ready.$(NC)\n"
	@printf "$(BOLD)[5/5] Applying Gateway default-gw...$(NC)\n"
	@$(KUBECTL) apply -k deploy/gateway/base/
	@printf "  Waiting for Gateway to be programmed...\n"
	@$(KUBECTL) wait gateway/default-gw -n $(K8S_NS_PLATFORM) \
		--for=condition=Programmed --timeout=120s
	@printf "  $(GREEN)Gateway default-gw programmed.$(NC)\n"
	@printf "\n$(GREEN)$(BOLD)Platform layer complete.$(NC)\n\n"

.PHONY: k8s-observability
k8s-observability: ## Install observability: Prometheus, Grafana, Tempo, Loki, Alloy
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Observability Layer ═══$(NC)\n\n"
	@$(KUBECTL) create namespace $(K8S_NS_OBSERVABILITY) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@printf "$(BOLD)[1/5] Installing kube-prometheus-stack...$(NC)\n"
	@$(HELM) repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update 2>/dev/null || true
	@$(HELM) upgrade --install prometheus prometheus-community/kube-prometheus-stack \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/kube-prometheus-stack-values.yaml \
		--wait --timeout 300s
	@printf "  $(GREEN)kube-prometheus-stack ready.$(NC)\n"
	@printf "$(BOLD)[2/5] Installing Tempo...$(NC)\n"
	@$(HELM) repo add grafana https://grafana.github.io/helm-charts --force-update 2>/dev/null || true
	@$(HELM) upgrade --install tempo grafana/tempo \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/tempo-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Tempo ready.$(NC)\n"
	@printf "$(BOLD)[3/5] Installing Loki...$(NC)\n"
	@$(HELM) upgrade --install loki grafana/loki \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/loki-values.yaml \
		--wait --timeout 300s
	@printf "  $(GREEN)Loki ready.$(NC)\n"
	@printf "$(BOLD)[4/5] Installing Alloy (logs)...$(NC)\n"
	@$(HELM) upgrade --install alloy-logs grafana/alloy \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/alloy-logs-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Alloy (logs) ready.$(NC)\n"
	@printf "$(BOLD)[5/5] Installing Alloy (metrics+traces)...$(NC)\n"
	@$(HELM) upgrade --install alloy-metrics-traces grafana/alloy \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/alloy-metrics-traces-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Alloy (metrics+traces) ready.$(NC)\n"
	@printf "\n$(GREEN)$(BOLD)Observability layer complete.$(NC)\n\n"

.PHONY: k8s-deploy
k8s-deploy: ## Build images, import to k3d, deploy apps + Argo Events + HTTPRoutes
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Application Layer ═══$(NC)\n\n"
	@$(KUBECTL) create namespace $(K8S_NS_BOOKINFO) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@printf "$(BOLD)[1/6] Building Docker images...$(NC)\n"
	@for svc in $(SERVICES); do \
		printf "  Building $$svc...\n"; \
		docker build -f build/Dockerfile.$$svc -t event-driven-bookinfo/$$svc:local . || exit 1; \
	done
	@printf "$(BOLD)[2/6] Importing images to k3d...$(NC)\n"
	@for svc in $(SERVICES); do \
		k3d image import event-driven-bookinfo/$$svc:local -c $(K8S_CLUSTER) || exit 1; \
	done
	@printf "  $(GREEN)Images imported.$(NC)\n"
	@printf "$(BOLD)[3/6] Deploying PostgreSQL...$(NC)\n"
	@$(KUBECTL) apply -k deploy/postgres/local/
	@$(KUBECTL) wait statefulset/postgres -n $(K8S_NS_BOOKINFO) \
		--for=jsonpath='{.status.readyReplicas}'=1 --timeout=120s
	@printf "  $(GREEN)PostgreSQL ready.$(NC)\n"
	@printf "$(BOLD)[4/6] Deploying services...$(NC)\n"
	@for svc in $(SERVICES); do \
		printf "  Applying $$svc local overlay...\n"; \
		$(KUBECTL) apply -k deploy/$$svc/overlays/local/ || exit 1; \
	done
	@printf "$(BOLD)[5/6] Deploying Argo Events resources...$(NC)\n"
	@$(KUBECTL) kustomize deploy/argo-events/overlays/local/ --load-restrictor LoadRestrictionsNone | $(KUBECTL) apply -f -
	@printf "$(BOLD)[6/6] Applying HTTPRoutes...$(NC)\n"
	@$(KUBECTL) apply -k deploy/gateway/overlays/local/
	@printf "\n$(BOLD)Waiting for deployments...$(NC)\n"
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification; do \
		$(KUBECTL) wait deployment/$$dep -n $(K8S_NS_BOOKINFO) \
			--for=condition=Available --timeout=120s || true; \
	done
	@printf "\n$(GREEN)$(BOLD)Application layer complete.$(NC)\n\n"

.PHONY: k8s-seed
k8s-seed: ## Seed databases in k8s PostgreSQL
	$(k8s-guard)
	@printf "\n$(BOLD)Seeding databases...$(NC)\n\n"
	@for svc in details ratings reviews notification; do \
		seed_file="services/$$svc/seeds/seed.sql"; \
		if [ -f "$$seed_file" ]; then \
			$(KUBECTL) exec -n $(K8S_NS_BOOKINFO) statefulset/postgres -- \
				psql -U bookinfo -d bookinfo_$$svc -c "$$(cat $$seed_file)" > /dev/null 2>&1; \
			printf "  $(GREEN)%-14s$(NC) seeded\n" "$$svc"; \
		fi; \
	done
	@printf "\n"

.PHONY: run-k8s
run-k8s: ## Full local k8s setup: cluster -> platform -> observability -> deploy
	@printf "\n$(BOLD)$(CYAN)════════════════════════════════════════$(NC)\n"
	@printf "$(BOLD)$(CYAN)  Bookinfo Local Kubernetes Environment  $(NC)\n"
	@printf "$(BOLD)$(CYAN)════════════════════════════════════════$(NC)\n\n"
	@$(MAKE) --no-print-directory k8s-cluster
	@$(MAKE) --no-print-directory k8s-platform
	@$(MAKE) --no-print-directory k8s-observability
	@$(MAKE) --no-print-directory k8s-deploy
	@$(MAKE) --no-print-directory k8s-seed
	@$(MAKE) --no-print-directory k8s-status

.PHONY: k8s-rebuild
k8s-rebuild: ## Fast iteration: rebuild images, reimport, rollout restart
	$(k8s-guard)
	@printf "\n$(BOLD)Rebuilding and redeploying...$(NC)\n\n"
	@for svc in $(SERVICES); do \
		printf "  Building $$svc...\n"; \
		docker build -f build/Dockerfile.$$svc -t event-driven-bookinfo/$$svc:local . || exit 1; \
	done
	@for svc in $(SERVICES); do \
		k3d image import event-driven-bookinfo/$$svc:local -c $(K8S_CLUSTER) || exit 1; \
	done
	@for svc in $(SERVICES); do \
		$(KUBECTL) apply -k deploy/$$svc/overlays/local/ || exit 1; \
	done
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification; do \
		$(KUBECTL) rollout restart deployment/$$dep -n $(K8S_NS_BOOKINFO) 2>/dev/null || true; \
	done
	@printf "\n$(BOLD)Waiting for rollouts...$(NC)\n"
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification; do \
		$(KUBECTL) rollout status deployment/$$dep -n $(K8S_NS_BOOKINFO) --timeout=120s 2>/dev/null || true; \
	done
	@printf "\n$(GREEN)$(BOLD)Rebuild complete.$(NC)\n\n"

.PHONY: k8s-status
k8s-status: ## Show pod status and access URLs
	$(k8s-guard)
	@printf "\n$(BOLD)Pod Status:$(NC)\n\n"
	@$(KUBECTL) get pods -n $(K8S_NS_BOOKINFO) -o wide 2>/dev/null || true
	@printf "\n$(BOLD)Platform:$(NC)\n\n"
	@$(KUBECTL) get pods -n $(K8S_NS_PLATFORM) 2>/dev/null || true
	@printf "\n$(BOLD)Observability:$(NC)\n\n"
	@$(KUBECTL) get pods -n $(K8S_NS_OBSERVABILITY) 2>/dev/null || true
	@printf "\n$(BOLD)Access URLs:$(NC)\n\n"
	@printf "  $(CYAN)Productpage:$(NC)  http://localhost:8080\n"
	@printf "  $(CYAN)Grafana:$(NC)      http://localhost:3000  (admin/admin)\n"
	@printf "  $(CYAN)Prometheus:$(NC)   http://localhost:9090\n"
	@printf "\n$(BOLD)Webhooks (via Gateway):$(NC)\n\n"
	@printf "  $(CYAN)book-added:$(NC)         curl -X POST http://localhost:8443/v1/book-added -H 'Content-Type: application/json' -d '{...}'\n"
	@printf "  $(CYAN)review-submitted:$(NC)   curl -X POST http://localhost:8443/v1/review-submitted -H 'Content-Type: application/json' -d '{...}'\n"
	@printf "  $(CYAN)rating-submitted:$(NC)   curl -X POST http://localhost:8443/v1/rating-submitted -H 'Content-Type: application/json' -d '{...}'\n"
	@printf "\n"

.PHONY: k8s-logs
k8s-logs: ## Tail logs from bookinfo namespace
	$(k8s-guard)
	$(KUBECTL) logs -n $(K8S_NS_BOOKINFO) -l part-of=event-driven-bookinfo -f --max-log-requests=10

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
