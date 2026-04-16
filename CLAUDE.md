# event-driven-bookinfo — Claude Code Project Instructions

## Project Overview

Go hexagonal architecture monorepo adapting Istio's Bookinfo as a **book review system with event-driven architecture**. Services are plain REST APIs — all event-driven complexity (Kafka consumers, retries) is abstracted by Argo Events EventSources and Sensors. Failed events that exhaust sensor retries are captured by the `dlqueue` service for inspection and replay. A standalone `ingestion` service polls the Open Library API and publishes `book-added` events through the Gateway, demonstrating the system as a self-hosted event broker. Full observability: structured logging, distributed tracing, metrics, continuous profiling.

**Module**: `github.com/kaio6fellipe/event-driven-bookinfo`

## Services

| Service | Type | Port | Description |
|---|---|---|---|
| **productpage** | BFF (Go + HTMX) | :8080 web / :9090 admin | Aggregator; fans out sync reads to details + reviews; renders HTML with HTMX; pending review cache via Redis |
| **details** | Backend (hex arch) | :8080 / :9090 admin | Book metadata CRUD |
| **reviews** | Backend (hex arch) | :8080 / :9090 admin | User reviews; sync call to ratings service |
| **ratings** | Backend (hex arch) | :8080 / :9090 admin | Star ratings |
| **notification** | Backend (hex arch) | :8080 / :9090 admin | Event consumer audit log |
| **dlqueue** | Backend (hex arch) | :8080 / :9090 admin | Dead letter queue for failed sensor deliveries; REST API for inspection, replay, and resolution |
| **ingestion** | Producer (hex arch) | :8080 / :9090 admin | Open Library scraper; polls on interval, publishes book-added events to the Gateway |

## Architecture

- **Hexagonal architecture** (ports & adapters): domain -> ports -> service -> adapters -> cmd
- **Event-driven writes**: Envoy Gateway CQRS routing (POST -> EventSources) -> Kafka EventBus -> Sensors -> HTTP triggers to write services
- **Sync reads**: productpage fans out GET calls to details and reviews; reviews fans out to ratings
- **Storage**: swappable via `STORAGE_BACKEND` env var — `memory` (default, single replica) or `postgres` (horizontally scalable)
- **Admin port** (:9090): `/metrics`, `/debug/pprof/*`, `/healthz`, `/readyz` — isolated from business API
- **CQRS deployments** (local k8s): each backend service has separate read and write Deployments; read serves GET via gateway, write receives POST from Argo Events sensors. The Envoy Gateway acts as the CQRS routing boundary (GET -> read services, POST -> EventSource webhooks)
- **Pending review cache**: productpage stores submitted reviews in Redis immediately after async POST; merges into read responses with "Processing" badge; HTMX auto-polls to reconcile when confirmed. Disabled when `REDIS_URL` is unset.
- **Local k8s** (`make run-k8s`): k3d cluster with Envoy Gateway API, Strimzi Kafka (KRaft), full observability stack (Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy)
- **DLQ**: sensor `dlqTrigger` → `dlq-event-received` EventSource → `dlqueue-write` → PostgreSQL. Dedup by natural key (`sensor_name + failed_trigger + SHA-256(payload)`). State machine: `pending → replayed → resolved / poisoned`. REST API at `/v1/events` supports list/get/replay/resolve/reset plus batch operations.
- **Idempotency**: all write services (reviews, ratings, details, notification, dlqueue) dedupe on client-supplied `idempotency_key` or derived natural key (SHA-256 of business fields). Prerequisite for safe DLQ replay — CloudEvents `id` cannot be used because Argo Events regenerates it per EventSource pass.
- **Ingestion**: `ingestion` service polls Open Library on `POLL_INTERVAL` → for each query in `SEARCH_QUERIES` → `POST {GATEWAY_URL}/v1/details` with `idempotency_key=ingestion-isbn-<ISBN>`. Stateless, single deployment, no CQRS split, no EventSource/Sensor of its own. Exercises the full write pipeline end-to-end.

## Build Commands

```bash
make build-all          # Build all 7 service binaries to bin/
make test               # go test -race -count=1 ./...
make lint               # golangci-lint run
make e2e                # Docker Compose + shell smoke tests (memory backend)
make docker-build-all   # Build all 7 Docker images
```

## Helm Commands

```bash
make helm-lint            # Lint chart with all per-service values files
make helm-template SERVICE=ratings  # Dry-run render for a specific service
helm upgrade --install ratings charts/bookinfo-service -f deploy/ratings/values-local.yaml -n bookinfo
```

## Local Kubernetes

```bash
make run-k8s            # Full local k8s: k3d + platform + observability + apps
make stop-k8s           # Delete k3d cluster
make k8s-rebuild        # Fast iteration: rebuild images + redeploy (skip infra)
make k8s-status         # Pod status + access URLs
make k8s-logs           # Tail bookinfo namespace logs
```

**Namespaces:** `platform` (Kafka, Argo Events, Gateway), `envoy-gateway-system` (Envoy Gateway), `observability` (Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy), `bookinfo` (apps, PostgreSQL, EventSources, Sensors, HTTPRoutes)

**CQRS split:** details, reviews, ratings each have read + write Deployments. productpage is read-only. notification is write-only. Sensors target `-write` services.

**Context safety:** All kubectl/helm calls use `--context=k3d-bookinfo-local`. Never mutates the user's active context.

**Access:** Productpage http://localhost:8080, Webhooks POST http://localhost:8080/v1/* (method-based CQRS routing), Grafana http://localhost:3000, Prometheus http://localhost:9090, Headlamp http://localhost:4466

## Deploy Structure

```
charts/
  bookinfo-service/          # Reusable Helm chart for all 7 services
    Chart.yaml
    values.yaml
    templates/
    ci/                      # chart-testing test values
deploy/
├── <service>/values-local.yaml  # Per-service Helm values for local k8s
├── gateway/base/                # Gateway, GatewayClass, ReferenceGrant
├── observability/local/         # Helm values: Prometheus, Grafana, Tempo, Loki, Alloy
├── platform/local/              # Helm values: Strimzi, Argo Events; Kafka CRDs; EventBus
├── redis/local/                 # Helm values: Bitnami Redis
└── postgres/local/              # StatefulSet, Service, init ConfigMap
```

## Run Locally

**ratings** (no dependencies):
```bash
SERVICE_NAME=ratings HTTP_PORT=8081 ADMIN_PORT=9091 go run ./services/ratings/cmd/
```

**details** (no dependencies):
```bash
SERVICE_NAME=details HTTP_PORT=8082 ADMIN_PORT=9092 go run ./services/details/cmd/
```

**reviews** (depends on ratings):
```bash
SERVICE_NAME=reviews HTTP_PORT=8083 ADMIN_PORT=9093 RATINGS_SERVICE_URL=http://localhost:8081 go run ./services/reviews/cmd/
```

**notification** (no dependencies):
```bash
SERVICE_NAME=notification HTTP_PORT=8084 ADMIN_PORT=9094 go run ./services/notification/cmd/
```

**dlqueue** (no dependencies):
```bash
SERVICE_NAME=dlqueue HTTP_PORT=8085 ADMIN_PORT=9095 go run ./services/dlqueue/cmd/
```

**ingestion** (publishes to Gateway; requires Gateway/productpage reachable):
```bash
SERVICE_NAME=ingestion HTTP_PORT=8086 ADMIN_PORT=9096 \
  GATEWAY_URL=http://localhost:8080 \
  POLL_INTERVAL=5m \
  SEARCH_QUERIES=programming,golang \
  MAX_RESULTS_PER_QUERY=10 \
  go run ./services/ingestion/cmd/
```

**productpage** (depends on details + reviews):
```bash
SERVICE_NAME=productpage HTTP_PORT=8080 ADMIN_PORT=9090 \
  DETAILS_SERVICE_URL=http://localhost:8082 \
  REVIEWS_SERVICE_URL=http://localhost:8083 \
  go run ./services/productpage/cmd/
```

Optional env vars (all services): `LOG_LEVEL` (debug/info/warn/error, default info), `OTEL_EXPORTER_OTLP_ENDPOINT`, `PYROSCOPE_SERVER_ADDRESS`, `STORAGE_BACKEND` (memory/postgres), `DATABASE_URL`. Productpage-specific: `REDIS_URL` (enables pending review cache; disabled when unset). Ingestion-specific: `GATEWAY_URL`, `POLL_INTERVAL`, `SEARCH_QUERIES`, `MAX_RESULTS_PER_QUERY`.

## Shared Packages (`pkg/`)

| Package | Purpose |
|---|---|
| `pkg/config` | Env-based config struct: ServiceName, HTTPPort, AdminPort, LogLevel, StorageBackend, DatabaseURL, RedisURL, OTLPEndpoint, PyroscopeServerAddress |
| `pkg/health` | `/healthz` (liveness) and `/readyz` (readiness with optional check functions) |
| `pkg/idempotency` | `Store` interface (`CheckAndRecord`) with memory + postgres adapters; `NaturalKey(fields...)` (SHA-256 with `0x1f` separator); `Resolve(explicitKey, fields...)` picks explicit when present, natural key otherwise |
| `pkg/logging` | slog + otelslog bridge, JSON output, `FromContext`/`WithContext`, request-scoped HTTP middleware |
| `pkg/metrics` | OTel meter provider + Prometheus exporter, HTTP middleware (duration, requests_total, active_requests), runtime metrics |
| `pkg/profiling` | Pyroscope SDK wrapper, no-op when `PYROSCOPE_SERVER_ADDRESS` is unset |
| `pkg/server` | Dual-port HTTP server (API + admin), graceful shutdown on SIGINT/SIGTERM |
| `pkg/telemetry` | OTel tracing setup, no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset |

## Service Structure (hex arch)

```
services/<name>/
├── cmd/main.go                        # Composition root
└── internal/
    ├── core/
    │   ├── domain/<entity>.go         # Pure domain types, no dependencies
    │   ├── port/inbound.go            # Service interfaces (called by adapters)
    │   └── port/outbound.go           # Repository/client interfaces (implemented by adapters)
    └── adapter/
        ├── inbound/http/
        │   ├── handler.go             # HTTP handlers, calls service via inbound port
        │   ├── handler_test.go        # httptest-based tests
        │   └── dto.go                 # Request/response types, separate from domain
        └── outbound/
            ├── memory/                # In-memory adapter (default)
            └── postgres/              # PostgreSQL adapter (STORAGE_BACKEND=postgres)
```

## Coding Standards

- **Error handling**: always wrap with `%w` — `fmt.Errorf("getting detail: %w", err)`. Never ignore errors with `_`.
- **Logging**: no `fmt.Print*` anywhere. Always use `logging.FromContext(ctx)` for structured, request-scoped logging.
- **Interfaces**: small and focused. Receiver names 1-2 chars. No "I" prefix on interface names.
- **Context**: always pass `ctx context.Context` as first argument to functions that do I/O.
- **Go idioms**: prefer early returns over deep nesting. Use table-driven tests. Keep functions small.

## Import Ordering

Group imports in this order (goimports enforces this):
1. Standard library
2. External dependencies
3. `github.com/kaio6fellipe/event-driven-bookinfo/pkg/` (shared packages)
4. `github.com/kaio6fellipe/event-driven-bookinfo/services/` (internal service packages)

## Git Workflow

Conventional commits scoped to the affected service or layer:
```
feat(ratings): add star rating endpoint
fix(reviews): handle missing ratings gracefully
feat(pkg/logging): add request_id to middleware
chore: update go.mod dependencies
```

## Release Process

Each service is released independently with its own version tag.

**Tag format**: `<service>-v<major>.<minor>.<patch>` (e.g., `details-v0.1.0`, `reviews-v1.2.3`)

**Auto-release on PR merge to `main`:**
1. `auto-tag.yml` detects which services changed (`services/<name>/` paths + `pkg/`/`go.mod`/`go.sum` triggers all)
2. Determines bump type: PR labels (`major`/`minor`) → conventional commits → default `patch`
3. Creates prefixed tag and dispatches `release.yml` via `workflow_dispatch`

**Manual release:** `gh workflow run release.yml -f service=<name> -f tag=<name>-v<X.Y.Z>`

**GoReleaser configs:** `services/<name>/.goreleaser.yaml` (per-service, GoReleaser OSS with `GORELEASER_CURRENT_TAG`)

**Version source of truth:** git tags (`git tag -l "<service>-v*"`)

## Hex Arch Boundary Rules

- **Domain** (`core/domain/`): no imports from `adapter/`, no framework imports, pure Go types only.
- **Ports** (`core/port/`): interfaces only, no implementations. Depend on domain types.
- **Service** (`core/service/`): implements inbound ports, depends on outbound port interfaces. No HTTP/DB imports.
- **Adapters** (`adapter/`): implement ports. HTTP adapters call service via inbound port. Outbound adapters implement repository/client interfaces.
- **cmd/main.go**: only composition root — wires adapters + service + server. No business logic.
