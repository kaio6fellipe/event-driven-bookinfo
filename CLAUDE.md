# event-driven-bookinfo — Claude Code Project Instructions

## Project Overview

Go hexagonal architecture monorepo adapting Istio's Bookinfo as a **book review system with event-driven architecture**. Services are plain REST APIs — all event-driven complexity (Kafka consumers, retries, DLQ) is abstracted by Argo Events EventSources and Sensors. Full observability: structured logging, distributed tracing, metrics, continuous profiling.

**Module**: `github.com/kaio6fellipe/event-driven-bookinfo`

## Services

| Service | Type | Port | Description |
|---|---|---|---|
| **productpage** | BFF (Go + HTMX) | :8080 web / :9090 admin | Aggregator; fans out sync reads to details + reviews; renders HTML with HTMX |
| **details** | Backend (hex arch) | :8080 / :9090 admin | Book metadata CRUD |
| **reviews** | Backend (hex arch) | :8080 / :9090 admin | User reviews; sync call to ratings service |
| **ratings** | Backend (hex arch) | :8080 / :9090 admin | Star ratings |
| **notification** | Backend (hex arch) | :8080 / :9090 admin | Event consumer audit log |

## Architecture

- **Hexagonal architecture** (ports & adapters): domain -> ports -> service -> adapters -> cmd
- **Event-driven writes**: Envoy Gateway CQRS routing (POST -> EventSources) -> Kafka EventBus -> Sensors -> HTTP triggers to write services
- **Sync reads**: productpage fans out GET calls to details and reviews; reviews fans out to ratings
- **Storage**: swappable via `STORAGE_BACKEND` env var — `memory` (default, single replica) or `postgres` (horizontally scalable)
- **Admin port** (:9090): `/metrics`, `/debug/pprof/*`, `/healthz`, `/readyz` — isolated from business API
- **CQRS deployments** (local k8s): each backend service has separate read and write Deployments; read serves GET via gateway, write receives POST from Argo Events sensors. The Envoy Gateway acts as the CQRS routing boundary (GET -> read services, POST -> EventSource webhooks)
- **Local k8s** (`make run-k8s`): k3d cluster with Envoy Gateway API, Strimzi Kafka (KRaft), full observability stack (Prometheus, Grafana, Tempo, Loki, Alloy)

## Build Commands

```bash
make build-all          # Build all 5 service binaries to bin/
make test               # go test -race -count=1 ./...
make lint               # golangci-lint run
make e2e                # Docker Compose + shell smoke tests (memory backend)
make docker-build-all   # Build all 5 Docker images
```

## Local Kubernetes

```bash
make run-k8s            # Full local k8s: k3d + platform + observability + apps
make stop-k8s           # Delete k3d cluster
make k8s-rebuild        # Fast iteration: rebuild images + redeploy (skip infra)
make k8s-status         # Pod status + access URLs
make k8s-logs           # Tail bookinfo namespace logs
```

**Namespaces:** `platform` (Kafka, Argo Events, Gateway), `envoy-gateway-system` (Envoy Gateway), `observability` (Prometheus, Grafana, Tempo, Loki, Alloy), `bookinfo` (apps, PostgreSQL, EventSources, Sensors, HTTPRoutes)

**CQRS split:** details, reviews, ratings each have read + write Deployments. productpage is read-only. notification is write-only. Sensors target `-write` services.

**Context safety:** All kubectl/helm calls use `--context=k3d-bookinfo-local`. Never mutates the user's active context.

**Access:** Productpage http://localhost:8080, Webhooks POST http://localhost:8080/v1/* (method-based CQRS routing), Grafana http://localhost:3000, Prometheus http://localhost:9090

## Deploy Structure

```
deploy/
├── <service>/base/              # Deployment, Service, ConfigMap, EventSource, Sensor (details/reviews/ratings)
├── <service>/overlays/local/    # CQRS read/write split, eventsource-service, local patches
├── gateway/base/                # Gateway, GatewayClass, ReferenceGrant
├── gateway/overlays/local/      # HTTPRoutes for bookinfo
├── observability/local/         # Helm values: Prometheus, Grafana, Tempo, Loki, Alloy
├── platform/local/              # Helm values: Strimzi, Argo Events; Kafka CRDs; EventBus
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

**productpage** (depends on details + reviews):
```bash
SERVICE_NAME=productpage HTTP_PORT=8080 ADMIN_PORT=9090 \
  DETAILS_SERVICE_URL=http://localhost:8082 \
  REVIEWS_SERVICE_URL=http://localhost:8083 \
  go run ./services/productpage/cmd/
```

Optional env vars (all services): `LOG_LEVEL` (debug/info/warn/error, default info), `OTEL_EXPORTER_OTLP_ENDPOINT`, `PYROSCOPE_SERVER_ADDRESS`, `STORAGE_BACKEND` (memory/postgres), `DATABASE_URL`.

## Shared Packages (`pkg/`)

| Package | Purpose |
|---|---|
| `pkg/config` | Env-based config struct: ServiceName, HTTPPort, AdminPort, LogLevel, StorageBackend, DatabaseURL, OTLPEndpoint, PyroscopeServerAddress |
| `pkg/health` | `/healthz` (liveness) and `/readyz` (readiness with optional check functions) |
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

## Hex Arch Boundary Rules

- **Domain** (`core/domain/`): no imports from `adapter/`, no framework imports, pure Go types only.
- **Ports** (`core/port/`): interfaces only, no implementations. Depend on domain types.
- **Service** (`core/service/`): implements inbound ports, depends on outbound port interfaces. No HTTP/DB imports.
- **Adapters** (`adapter/`): implement ports. HTTP adapters call service via inbound port. Outbound adapters implement repository/client interfaces.
- **cmd/main.go**: only composition root — wires adapters + service + server. No business logic.
