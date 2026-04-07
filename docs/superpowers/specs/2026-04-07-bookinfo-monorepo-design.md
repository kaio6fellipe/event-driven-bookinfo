# Go Hexagonal Architecture Monorepo — Bookinfo E-Commerce

> Supersedes `repo-restructure.md`

## Context

The repository `go-http-server` (to be renamed `event-driven-bookinfo`) is currently a single-file Go HTTP server with OpenTelemetry tracing. The goal is to restructure it into a **professional hexagonal architecture monorepo** adapting Istio's [Bookinfo](https://github.com/istio/istio/tree/master/samples/bookinfo) sample application as an event-driven e-commerce system.

The original Bookinfo is polyglot (Python, Ruby, Java, Node.js). We rewrite all services in Go with hexagonal architecture, add full observability (metrics, structured logging, continuous profiling), and introduce event-driven writes via Argo Events with Kafka EventBus.

**Current state**: `cmd/main.go` (HTTP server on :8090 with OTel tracing), broken `Dockerfile`, `go.mod` (module `http-server`, Go 1.25.0), minimal README.

---

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Repository name | `event-driven-bookinfo` (renamed from `go-http-server`) | Descriptive name reflecting the event-driven Bookinfo architecture |
| Module name | `github.com/kaio6fellipe/event-driven-bookinfo` | Matches GitHub remote, professional Go convention |
| Single vs multi go.mod | Single root `go.mod` | Simpler for tightly-coupled monorepo |
| HTTP framework | Standard library (`net/http`) | Go 1.22+ ServeMux has method routing + path params |
| Base Docker image | `scratch` for backend services, `gcr.io/distroless/static-debian12:nonroot` for productpage (needs embedded templates) | Smallest possible images; scratch for pure Go binaries with `CGO_ENABLED=0`, distroless only when static assets needed |
| Container registry | GitHub Container Registry (`ghcr.io/kaio6fellipe/event-driven-bookinfo/<service>`) | Colocated with source, free for public repos, integrated with GitHub Actions |
| UUID generation | `github.com/google/uuid` | Already indirect dep, promote to direct |
| OTel in tests | No-op when `OTEL_EXPORTER_OTLP_ENDPOINT` unset | Tests don't require running collector |
| K8s manifests | Kustomize with base + overlays (dev/staging/prod) | Industry standard, no Helm templating complexity |
| E2E tests | Shell scripts (smoke tests) + Docker Compose | Lightweight, no extra Go deps, easy to run in CI |
| Health probes | Separate `/healthz` (liveness) and `/readyz` (readiness) | K8s best practice: different probe semantics |
| Metrics pipeline | OTel Metrics SDK -> Prometheus exporter | Unified telemetry API for traces + metrics; Prometheus-compatible scraping |
| Structured logging | `log/slog` (stdlib) with JSON output | Zero deps, Go community standard, OTel bridge available |
| Log-trace correlation | `otelslog` bridge + request-scoped context logger | Automatic trace_id/span_id injection, per-request enrichment |
| Profiling (local dev) | `github.com/grafana/pyroscope-go` SDK (push mode) | No-op when PYROSCOPE_SERVER_ADDRESS unset, matches OTel pattern |
| Profiling (production) | Grafana Alloy DaemonSet with `pyroscope.ebpf` | Zero app changes, auto-discovers pods via annotations. Alloy is external infra, not in this repo |
| Admin port | Separate listener (:9090) for /metrics, /debug/pprof/*, /healthz, /readyz | Isolates observability from business API, K8s best practice |
| Middleware order | logging -> metrics -> tracing -> handler | Ensures all requests are logged and metered even on panic |
| Business metrics | Per-service custom counters/gauges via OTel API | Domain-level observability |
| Runtime metrics | OTel runtime instrumentation | Goroutines, GC, memory exposed automatically |
| Frontend | Go + html/template + HTMX | Single binary, no JS toolchain, fits Go monorepo |
| Event-driven writes | Argo Events (webhook EventSources -> Kafka EventBus -> Sensors -> HTTP triggers) | Async writes, sync reads. Realistic e-commerce pattern |
| OTel trace propagation | Graceful degradation — extract traceparent from CloudEvent extensions if present, otherwise start new trace | Works with upstream Argo Events today, full E2E tracing when PR #3961 merges |
| Reviews versioning | Dropped (single version, always includes ratings) | Istio traffic routing demo not in scope |
| Storage backend | Swappable via `STORAGE_BACKEND` env var: `memory` (default) or `postgres` | Hex arch showcase — same outbound port, two adapters. In-memory for zero-dep local dev/tests, Postgres for production persistence |
| Replica constraint | `memory` backend: 1 replica only (no shared state). `postgres` backend: horizontally scalable | In-memory storage is pod-local and cannot be shared across replicas |

---

## Service Architecture

### Service Topology

```
                    ┌─────────────────────────────────────┐
                    │         productpage (BFF)            │
                    │  Go + html/template + HTMX           │
                    │  :8080 (web) / :9090 (admin)        │
                    └──────┬──────────────┬───────────────┘
                           │ sync GET     │ sync GET
                           ▼              ▼
                 ┌─────────────┐  ┌──────────────┐
                 │   details   │  │   reviews    │
                 │   :8080     │  │   :8080      │
                 └─────────────┘  └──────┬───────┘
                                         │ sync GET
                                         ▼
                                  ┌──────────────┐
                                  │   ratings    │
                                  │   :8080      │
                                  └──────────────┘

Event-driven writes (async):

External webhook POST
        │
        ▼
[Argo Events: EventSource (webhook)]
        │
        ▼ Kafka EventBus (CloudEvent with optional traceparent)
        │
        ▼
[Sensor: review-submitted]
  ├─► HTTP Trigger → reviews POST /v1/reviews
  └─► HTTP Trigger → notification-service POST /v1/notifications
                      (notify: "new review posted")

[Sensor: rating-submitted]
  ├─► HTTP Trigger → ratings POST /v1/ratings
  └─► HTTP Trigger → notification-service POST /v1/notifications
                      (notify: "new rating posted")

[Sensor: book-added]
  ├─► HTTP Trigger → details POST /v1/details
  └─► HTTP Trigger → notification-service POST /v1/notifications
                      (notify: "new book added")
```

### Service Breakdown

| Service | Type | Sync API | Event-driven writes | Storage |
|---|---|---|---|---|
| **productpage** | BFF (Go + HTMX) | `GET /` (HTML), `GET /v1/products`, `GET /v1/products/{id}` — fans out to details + reviews | — | None (aggregator) |
| **details** | Backend | `GET /v1/details/{id}` | `POST /v1/details` (new book) | In-memory or PostgreSQL |
| **reviews** | Backend | `GET /v1/reviews/{id}` (includes ratings via sync call) | `POST /v1/reviews` (new review) | In-memory or PostgreSQL |
| **ratings** | Backend | `GET /v1/ratings/{id}` | `POST /v1/ratings` (new rating) | In-memory or PostgreSQL |
| **notification** | Event consumer only | `GET /v1/notifications`, `GET /v1/notifications/{id}` (audit log) | `POST /v1/notifications` (triggered by sensors) | In-memory or PostgreSQL |

**Storage selection**: controlled by `STORAGE_BACKEND` env var (`memory` or `postgres`). When using `memory`, the service must run as a single replica (no HPA). When using `postgres`, services can scale horizontally.

### Business Metrics per Service

| Service | Metrics |
|---|---|
| **ratings** | `ratings_submitted_total` (counter) |
| **details** | `books_added_total` (counter) |
| **reviews** | `reviews_submitted_total` (counter) |
| **notification** | `notifications_dispatched_total` (counter, by channel), `notifications_failed_total` (counter, by channel), `notifications_by_status` (up-down counter, by status) |
| **productpage** | HTTP middleware metrics only (no domain metrics — it's an aggregator) |

---

## Target Directory Structure

```
event-driven-bookinfo/
├── .github/workflows/
│   ├── ci.yml                              # Lint + Test + Build on PRs
│   └── release.yml                         # GoReleaser on tag push
├── .claude/
│   ├── rules/
│   │   ├── code-style.md                   # Go formatting, naming, file organization
│   │   ├── testing.md                      # Table-driven tests, mocking, coverage
│   │   └── api-design.md                   # HTTP handler patterns, status codes, errors
│   ├── agents/
│   │   ├── code-reviewer.md                # Read-only Go code review (sonnet)
│   │   └── test-writer.md                  # Table-driven test generation (haiku)
│   └── skills/
│       ├── test-service/SKILL.md           # /test-service <name> - full test suite
│       └── new-service/SKILL.md            # /new-service <name> - scaffold hex arch service
├── services/
│   ├── productpage/                        # BFF — Go + html/template + HTMX
│   │   ├── cmd/main.go
│   │   ├── internal/
│   │   │   ├── client/                     # HTTP clients for backend services
│   │   │   │   ├── details.go
│   │   │   │   ├── reviews.go
│   │   │   │   └── ratings.go
│   │   │   ├── handler/
│   │   │   │   ├── pages.go                # HTML page handlers (GET /)
│   │   │   │   └── api.go                  # JSON API handlers (GET /v1/products/*)
│   │   │   └── model/
│   │   │       └── product.go              # Aggregated view models
│   │   └── templates/
│   │       ├── layout.html                 # Base layout
│   │       ├── productpage.html            # Product listing / detail page
│   │       └── partials/
│   │           ├── details.html            # HTMX partial for book details
│   │           ├── reviews.html            # HTMX partial for reviews list
│   │           └── rating-form.html        # HTMX partial for submit rating
│   ├── details/                            # Book metadata (hex arch)
│   │   ├── cmd/main.go
│   │   └── internal/
│   │       ├── core/
│   │       │   ├── domain/detail.go
│   │       │   ├── port/inbound.go
│   │       │   ├── port/outbound.go
│   │       │   └── service/
│   │       │       ├── detail_service.go
│   │       │       └── detail_service_test.go
│   │       └── adapter/
│   │           ├── inbound/http/
│   │           │   ├── handler.go
│   │           │   ├── handler_test.go
│   │           │   └── dto.go
│   │           └── outbound/
│   │               ├── memory/
│   │               │   └── detail_repository.go
│   │               └── postgres/
│   │                   └── detail_repository.go
│   ├── reviews/                            # User reviews (hex arch)
│   │   ├── cmd/main.go
│   │   └── internal/
│   │       ├── core/
│   │       │   ├── domain/review.go
│   │       │   ├── port/inbound.go
│   │       │   ├── port/outbound.go        # ReviewRepository + RatingsClient
│   │       │   └── service/
│   │       │       ├── review_service.go
│   │       │       └── review_service_test.go
│   │       └── adapter/
│   │           ├── inbound/http/
│   │           │   ├── handler.go
│   │           │   ├── handler_test.go
│   │           │   └── dto.go
│   │           └── outbound/
│   │               ├── memory/
│   │               │   └── review_repository.go
│   │               ├── postgres/
│   │               │   └── review_repository.go
│   │               └── http/
│   │                   └── ratings_client.go   # Sync HTTP client to ratings service
│   ├── ratings/                            # Star ratings (hex arch)
│   │   ├── cmd/main.go
│   │   └── internal/
│   │       ├── core/
│   │       │   ├── domain/rating.go
│   │       │   ├── port/inbound.go
│   │       │   ├── port/outbound.go
│   │       │   └── service/
│   │       │       ├── rating_service.go
│   │       │       └── rating_service_test.go
│   │       └── adapter/
│   │           ├── inbound/http/
│   │           │   ├── handler.go
│   │           │   ├── handler_test.go
│   │           │   └── dto.go
│   │           └── outbound/
│   │               ├── memory/
│   │               │   └── rating_repository.go
│   │               └── postgres/
│   │                   └── rating_repository.go
│   └── notification/                       # Event consumer (hex arch)
│       ├── cmd/main.go
│       └── internal/
│           ├── core/
│           │   ├── domain/notification.go
│           │   ├── port/inbound.go
│           │   ├── port/outbound.go        # NotificationRepository + NotificationDispatcher
│           │   └── service/
│           │       ├── notification_service.go
│           │       └── notification_service_test.go
│           └── adapter/
│               ├── inbound/http/
│               │   ├── handler.go
│               │   ├── handler_test.go
│               │   └── dto.go
│               └── outbound/
│                   ├── memory/
│                   │   └── notification_repository.go
│                   ├── postgres/
│                   │   └── notification_repository.go
│                   └── log/
│                       └── dispatcher.go   # Log-based notification dispatcher
├── pkg/
│   ├── config/config.go                    # Env-based config: ServiceName, HTTPPort, AdminPort, LogLevel, OTLPEndpoint, PyroscopeServerAddress
│   ├── health/health.go                    # /healthz (liveness) and /readyz (readiness) handlers
│   ├── logging/
│   │   ├── logging.go                      # slog setup, JSON handler, otelslog bridge, FromContext/WithContext
│   │   ├── middleware.go                   # HTTP middleware: request-scoped logger with request_id, trace_id, span_id
│   │   └── logging_test.go
│   ├── metrics/
│   │   ├── metrics.go                      # OTel meter provider setup + Prometheus exporter
│   │   ├── middleware.go                   # HTTP middleware: request count, latency histogram, in-flight gauge
│   │   ├── runtime.go                      # Go runtime metrics (goroutines, GC, memory)
│   │   └── metrics_test.go
│   ├── profiling/
│   │   ├── profiling.go                    # Pyroscope SDK wrapper, no-op when endpoint unset
│   │   └── profiling_test.go
│   ├── server/server.go                    # Dual-port server: API (:HTTPPort) + admin (:AdminPort), graceful shutdown
│   └── telemetry/telemetry.go              # OTel tracing setup (extracted from current main.go), no-op when OTLP endpoint unset
├── deploy/
│   ├── productpage/
│   │   ├── base/
│   │   │   ├── kustomization.yaml
│   │   │   ├── deployment.yaml             # Dual ports (8080 + 9090), Pyroscope annotations, probes on admin port
│   │   │   ├── service.yaml                # ClusterIP, port 80 -> 8080 (API only)
│   │   │   └── configmap.yaml              # HTTP_PORT, ADMIN_PORT, LOG_LEVEL, OTEL/PYROSCOPE endpoints, backend service URLs
│   │   └── overlays/
│   │       ├── dev/
│   │       │   ├── kustomization.yaml
│   │       │   └── deployment-patch.yaml
│   │       ├── staging/
│   │       │   ├── kustomization.yaml
│   │       │   └── deployment-patch.yaml
│   │       └── production/
│   │           ├── kustomization.yaml
│   │           ├── deployment-patch.yaml
│   │           └── hpa.yaml
│   ├── details/
│   │   ├── base/ (same pattern)
│   │   └── overlays/ (same pattern)
│   ├── reviews/
│   │   ├── base/ (same pattern)
│   │   └── overlays/ (same pattern)
│   ├── ratings/
│   │   ├── base/ (same pattern)
│   │   └── overlays/ (same pattern)
│   ├── notification/
│   │   ├── base/ (same pattern)
│   │   └── overlays/ (same pattern)
│   └── argo-events/
│       ├── eventsources/
│       │   ├── book-added.yaml             # Webhook EventSource
│       │   ├── review-submitted.yaml       # Webhook EventSource
│       │   └── rating-submitted.yaml       # Webhook EventSource
│       └── sensors/
│           ├── book-added-sensor.yaml      # Triggers: details + notification
│           ├── review-submitted-sensor.yaml # Triggers: reviews + notification
│           └── rating-submitted-sensor.yaml # Triggers: ratings + notification
├── test/
│   └── e2e/
│       ├── docker-compose.yml              # All 5 services (memory backend) + admin port mappings
│       ├── docker-compose.postgres.yml     # Override: adds PostgreSQL + sets STORAGE_BACKEND=postgres
│       ├── run-tests.sh                    # Main test runner
│       ├── test-productpage.sh             # HTML page load + HTMX partials
│       ├── test-details.sh                 # Details CRUD + health
│       ├── test-reviews.sh                 # Reviews CRUD + ratings integration
│       ├── test-ratings.sh                 # Ratings CRUD + health
│       └── test-notification.sh            # Notification dispatch + audit log
├── CLAUDE.md                               # Project instructions for Claude Code
├── .golangci.yml                           # Production linter config (v2)
├── .goreleaser.yaml                        # Multi-binary release config (v2)
├── Dockerfile.productpage
├── Dockerfile.details
├── Dockerfile.reviews
├── Dockerfile.ratings
├── Dockerfile.notification
├── Makefile                                # build, test, lint, docker, e2e, deploy targets
├── go.mod                                  # Single module: github.com/kaio6fellipe/event-driven-bookinfo
├── go.sum
├── .gitignore
├── LICENSE
└── README.md
```

---

## Shared Packages Detail

### pkg/config/config.go

```go
type Config struct {
    ServiceName            string // SERVICE_NAME (required)
    HTTPPort               string // HTTP_PORT (default "8080")
    AdminPort              string // ADMIN_PORT (default "9090")
    LogLevel               string // LOG_LEVEL (default "info") — debug/info/warn/error
    StorageBackend         string // STORAGE_BACKEND (default "memory") — "memory" or "postgres"
    DatabaseURL            string // DATABASE_URL (required when STORAGE_BACKEND=postgres)
    OTLPEndpoint           string // OTEL_EXPORTER_OTLP_ENDPOINT (optional, enables tracing)
    PyroscopeServerAddress string // PYROSCOPE_SERVER_ADDRESS (optional, enables SDK profiling)
}
```

### pkg/logging/logging.go

- `New(level string, serviceName string) *slog.Logger` — creates a JSON slog handler wrapped with `otelslog` bridge for trace/span ID injection
- Default fields: `service`, `version` (from build info)
- `FromContext(ctx) *slog.Logger` — retrieves request-scoped logger
- `WithContext(ctx, logger) context.Context` — stores logger in context

### pkg/logging/middleware.go

HTTP middleware that:
1. Extracts or generates a `request_id`
2. Creates a child logger with `request_id`, `method`, `path`, `remote_addr`
3. Stores it in context via `WithContext`
4. Logs request start (debug) and completion (info) with status code + duration

### pkg/metrics/metrics.go

- `Setup(serviceName string) (*prometheus.Exporter, error)` — creates OTel meter provider with Prometheus exporter, registers as global
- Returns the exporter's `http.Handler` for mounting on the admin port at `/metrics`

### pkg/metrics/middleware.go

HTTP middleware recording via OTel metrics API:
- `http_server_request_duration_seconds` (histogram) — labeled by method, route, status
- `http_server_requests_total` (counter) — labeled by method, route, status
- `http_server_active_requests` (gauge) — labeled by method

### pkg/metrics/runtime.go

- `RegisterRuntimeMetrics()` — registers Go runtime metrics via `go.opentelemetry.io/contrib/instrumentation/runtime`. Exposes goroutine count, GC stats, memory usage.

### pkg/profiling/profiling.go

- `Start(cfg Config) (func(), error)` — starts Pyroscope SDK if `PyroscopeServerAddress` is set, returns a stop function. No-op stop if address is empty.
- Enables: CPU, alloc objects/space, inuse objects/space, goroutines, mutex count/duration, block count/duration
- Sets `runtime.SetMutexProfileFraction(5)` and `runtime.SetBlockProfileRate(5)`

### pkg/health/health.go

- `LivenessHandler()` — returns `{"status":"ok"}`
- `ReadinessHandler(checks ...func() error)` — runs optional check functions, returns 200 or 503. When using Postgres, a `db.Ping()` check is registered

### pkg/server/server.go

Manages two HTTP listeners:
- **API server** (`:HTTPPort`) — business routes with middleware chain: logging -> metrics -> tracing -> handler
- **Admin server** (`:AdminPort`) — `/metrics`, `/debug/pprof/*`, `/healthz`, `/readyz`
- Both shut down gracefully on SIGINT/SIGTERM
- `Run(ctx, cfg, registerAPIRoutes)` — caller registers business routes only, admin routes are automatic

### pkg/telemetry/telemetry.go

- `Setup(ctx, serviceName) (shutdown func(ctx), err)` — extracted from current `initTracer()`. Parameterized by service name.
- Returns no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset.

---

## Service Wiring Pattern

Each service's `cmd/main.go` composition root:

```
main() {
    1. Load config              → pkg/config.Load()
    2. Setup logging            → pkg/logging.New(level, serviceName)
    3. Setup tracing            → pkg/telemetry.Setup(ctx, serviceName)     [no-op if OTLP unset]
    4. Setup metrics            → pkg/metrics.Setup(serviceName)
    5. Start profiling          → pkg/profiling.Start(cfg)                  [no-op if Pyroscope unset]
    6. Register runtime metrics → pkg/metrics.RegisterRuntimeMetrics()
    7. Wire hex arch layers     → select repo adapter (memory or postgres) → service → HTTP handler
    8. Run server               → pkg/server.Run(ctx, cfg, handler.RegisterRoutes)
}
```

Service layer methods accept `context.Context` and use `logging.FromContext(ctx)` for request-scoped logging. Domain errors logged at warn, infrastructure errors at error. No `fmt.Print*` anywhere.

---

## Kubernetes Manifests

### Base Deployment (all services)

```yaml
ports:
  - name: http
    containerPort: 8080
  - name: admin
    containerPort: 9090

livenessProbe:
  httpGet:
    path: /healthz
    port: admin
  initialDelaySeconds: 10
  periodSeconds: 10
  failureThreshold: 5

readinessProbe:
  httpGet:
    path: /readyz
    port: admin
  initialDelaySeconds: 5
  periodSeconds: 5
  failureThreshold: 3

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

Pod annotations for Pyroscope scraping (via Grafana Alloy eBPF):

```yaml
annotations:
  profiles.grafana.com/cpu.scrape: "true"
  profiles.grafana.com/cpu.port: "9090"
  profiles.grafana.com/memory.scrape: "true"
  profiles.grafana.com/memory.port: "9090"
  profiles.grafana.com/goroutine.scrape: "true"
  profiles.grafana.com/goroutine.port: "9090"
  profiles.grafana.com/block.scrape: "true"
  profiles.grafana.com/block.port: "9090"
  profiles.grafana.com/mutex.scrape: "true"
  profiles.grafana.com/mutex.port: "9090"
```

### K8s Service

ClusterIP, exposes API port only (80 -> 8080). Admin port is pod-internal, scraped via annotations.

### ConfigMap

```yaml
data:
  HTTP_PORT: "8080"
  ADMIN_PORT: "9090"
  LOG_LEVEL: "info"
  STORAGE_BACKEND: "memory"
  DATABASE_URL: ""                           # set per overlay when STORAGE_BACKEND=postgres
  OTEL_EXPORTER_OTLP_ENDPOINT: ""
  PYROSCOPE_SERVER_ADDRESS: ""
```

productpage additionally needs:
```yaml
  DETAILS_SERVICE_URL: "http://details.default.svc.cluster.local"
  REVIEWS_SERVICE_URL: "http://reviews.default.svc.cluster.local"
```

### Overlays

| Overlay | Replicas | Resources | Storage | Extras |
|---|---|---|---|---|
| dev | 1 | requests only (no limits) | memory (default) | LOG_LEVEL=debug |
| staging | 2 | same as base | postgres | LOG_LEVEL=info, DATABASE_URL from Secret |
| production | 3 | higher limits (cpu 1, mem 512Mi) | postgres | HPA (min 3, max 10, target 70% CPU), LOG_LEVEL=warn, DATABASE_URL from Secret |

**Note**: dev overlay uses `memory` backend with 1 replica. Staging and production use `postgres` with horizontal scaling. HPA is only enabled when using `postgres` backend.

---

## Argo Events Manifests

EventBus (Kafka) already exists externally. This repo provides EventSources and Sensors only.

### EventSources (webhooks)

```
deploy/argo-events/eventsources/
├── book-added.yaml              # Webhook listening for new book events
├── review-submitted.yaml        # Webhook listening for new review events
└── rating-submitted.yaml        # Webhook listening for new rating events
```

Each EventSource creates a webhook endpoint that receives external HTTP POSTs, converts them to CloudEvents, and publishes to the Kafka EventBus.

### Sensors (HTTP triggers)

```
deploy/argo-events/sensors/
├── book-added-sensor.yaml       # On book-added → POST details + POST notification
├── review-submitted-sensor.yaml # On review-submitted → POST reviews + POST notification
└── rating-submitted-sensor.yaml # On rating-submitted → POST ratings + POST notification
```

Each sensor listens on the Kafka EventBus for specific events and fires HTTP triggers to the target services. Payload is mapped from CloudEvent data fields to the service's expected request body.

OTel trace context: sensors forward `traceparent`/`tracestate` from CloudEvent extensions as HTTP headers when present (requires PR #3961). Services extract trace context if present, otherwise start a new trace (graceful degradation).

---

## Implementation Phases

| Phase | Description | Verify |
|---|---|---|
| **1** | **Shared packages** — `pkg/config`, `pkg/telemetry` (tracing), `pkg/metrics` (OTel -> Prometheus), `pkg/logging` (slog + otelslog + context middleware), `pkg/profiling` (Pyroscope SDK wrapper), `pkg/health`, `pkg/server` (dual-port: API + admin) | `go build ./pkg/...`, `go test ./pkg/...` |
| **2** | **ratings** — simplest domain (id + reviewer->stars map). Full hex arch with in-memory adapter. Business metric: `ratings_submitted_total` | Build + test + curl |
| **3** | **details** — book metadata CRUD. Full hex arch with in-memory adapter. Business metric: `books_added_total` | Build + test + curl |
| **4** | **reviews** — reviews with sync call to ratings (outbound HTTP client port). In-memory adapter. Business metric: `reviews_submitted_total` | Build + test + curl |
| **5** | **notification** — event consumer only + audit log. Log-based dispatcher. In-memory adapter. Business metrics: `notifications_dispatched_total`, `notifications_failed_total`, `notifications_by_status` | Build + test + curl |
| **6** | **productpage** — Go + html/template + HTMX. HTTP clients for details/reviews. Fans out sync reads, renders HTML. HTMX partials for rating submission | Build + test + browser |
| **7** | **PostgreSQL adapters** — add `outbound/postgres/` adapter for each backend service (details, reviews, ratings, notification). Schema migrations via `golang-migrate`. Composition root selects adapter via `STORAGE_BACKEND` env var | `go test ./... -tags=integration` with docker Postgres |
| **8** | **Cleanup & module update** — delete old `cmd/main.go` and `Dockerfile`, rename repo to `event-driven-bookinfo`, rename module to `github.com/kaio6fellipe/event-driven-bookinfo`, promote uuid + pgx deps, `go mod tidy`, update `.gitignore` (add `bin/`, `dist/`, `coverage.html`) | `go build ./...`, `go test ./...` |
| **9** | **Build & CI/CD tooling** — Makefile (build, test, lint, docker, e2e, deploy targets), 5 Dockerfiles (multi-stage, `scratch` for backend / distroless for productpage), `.golangci.yml` (v2), `.goreleaser.yaml` (v2, 5 builds, push to `ghcr.io`), GitHub Actions (`ci.yml`, `release.yml` with GHCR push) | `make lint`, `make build-all`, `goreleaser check` |
| **10** | **K8s manifests** — Kustomize base + overlays for all 5 services. Dual ports, Pyroscope annotations, probes on admin port. Staging/prod overlays set `STORAGE_BACKEND=postgres` + DATABASE_URL from Secret | `kustomize build` for each service/overlay |
| **11** | **Argo Events manifests** — 3 EventSources (webhooks) + 3 Sensors (HTTP triggers) | `kubectl apply --dry-run=client` |
| **12** | **E2E tests** — docker-compose (memory backend) + docker-compose.postgres.yml (postgres backend) + shell scripts. Sync read flows + write via direct POST (simulating sensor HTTP triggers) | `make e2e` and `make e2e-postgres` |
| **13** | **Claude Code config** — CLAUDE.md, `.claude/rules/` (code-style, testing, api-design), `.claude/agents/` (code-reviewer, test-writer), `.claude/skills/` (test-service, new-service) | Load and verify |
| **14** | **Documentation** — README rewrite with architecture diagrams, event flow, quick start, Makefile reference, K8s deployment guide, storage backend configuration | Review |

### Phase ordering rationale

Services are built bottom-up through the dependency DAG: ratings (no deps) -> details (no deps) -> reviews (depends on ratings) -> notification (no deps but event-only) -> productpage (depends on all). All services are built first with in-memory adapters (phases 2-6), then PostgreSQL adapters are added (phase 7) to demonstrate hex arch adapter swapping.

---

## Existing Code to Reuse

| Current Code | New Location |
|---|---|
| `initTracer()` from `cmd/main.go:21-48` | `pkg/telemetry/telemetry.go` (parameterized by service name) |
| Signal handling from `cmd/main.go:66-68` | `pkg/server/server.go` (generalized, dual-port) |
| Health handler from `cmd/main.go:60-62` | `pkg/health/health.go` (split into liveness + readiness, returns JSON) |
| OTel handler wrapping from `cmd/main.go:74-75` | `pkg/server/server.go` (automatic wrapping via middleware chain) |

## Files to Delete

| File | Reason |
|---|---|
| `cmd/main.go` | Code distributed to `pkg/` and `services/` |
| `Dockerfile` | Replaced by 5 per-service Dockerfiles |
| `repo-restructure.md` | Superseded by this spec |

---

## Container Images

### Multi-stage Build Strategy

All Dockerfiles use multi-stage builds. The builder stage compiles the Go binary; the final stage contains only the binary and minimal runtime dependencies.

**Backend services** (details, reviews, ratings, notification) — `FROM scratch`:
```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY pkg/ ./pkg/
COPY services/<name>/ ./services/<name>/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/<name> ./services/<name>/cmd/

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/<name> /bin/<name>
USER 65534:65534
ENTRYPOINT ["/bin/<name>"]
```

- `CGO_ENABLED=0` produces a fully static binary — no libc needed
- CA certs copied from builder for HTTPS outbound calls (OTel, Pyroscope)
- Non-root user via numeric UID (scratch has no `/etc/passwd`)
- `-ldflags="-s -w"` strips debug info for smaller binary

**productpage** — `FROM gcr.io/distroless/static-debian12:nonroot`:
```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY pkg/ ./pkg/
COPY services/productpage/ ./services/productpage/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/productpage ./services/productpage/cmd/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/productpage /bin/productpage
COPY services/productpage/templates/ /templates/
ENTRYPOINT ["/bin/productpage"]
```

- Distroless used because productpage needs embedded HTML templates copied into the image
- Templates directory copied alongside the binary
- Distroless provides CA certs, timezone data, and non-root user built-in

### Container Registry

Images are pushed to GitHub Container Registry (GHCR):

```
ghcr.io/kaio6fellipe/event-driven-bookinfo/productpage:<tag>
ghcr.io/kaio6fellipe/event-driven-bookinfo/details:<tag>
ghcr.io/kaio6fellipe/event-driven-bookinfo/reviews:<tag>
ghcr.io/kaio6fellipe/event-driven-bookinfo/ratings:<tag>
ghcr.io/kaio6fellipe/event-driven-bookinfo/notification:<tag>
```

Tags follow semver from GoReleaser (`v1.0.0`, `v1.0.0-rc.1`). The `release.yml` GitHub Action authenticates to GHCR via `GITHUB_TOKEN`, builds multi-platform images (linux/amd64, linux/arm64), and pushes on tag.

---

## Verification Plan

1. **After each service**: `go build ./services/<name>/cmd/` compiles, `go test ./services/<name>/...` passes
2. **After all code**: `go test -race -count=1 ./...` passes
3. **Makefile**: `make lint` passes, `make test` passes, `make build-all` produces 5 binaries in `bin/`
4. **Docker**: `make docker-build-all` builds 5 images successfully
5. **GoReleaser**: `goreleaser check` validates config
6. **Kustomize**: `kustomize build deploy/<service>/overlays/dev` renders valid YAML for each service/overlay
7. **Argo Events**: `kubectl apply --dry-run=client` on all EventSources and Sensors
8. **E2E**: `make e2e` runs docker-compose, executes all test scripts, all pass
9. **Claude Code**: CLAUDE.md loads correctly, agents/skills are discoverable
10. **Browser test**: Run productpage locally, load in browser, verify HTML renders with HTMX partials
