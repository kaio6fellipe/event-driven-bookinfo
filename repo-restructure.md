# Go Hexagonal Architecture Monorepo Restructure

## Context

The repository `go-http-server` is currently a single-file Go HTTP server with OpenTelemetry tracing. The goal is to restructure it into a **professional hexagonal architecture monorepo** with 3 example microservices, complete with production-grade tooling (Makefile, Dockerfiles, GitHub Actions, GoReleaser, golangci-lint), E2E tests, Kubernetes deployment manifests with Kustomize, and Claude Code project configuration.

**Current state**: `cmd/main.go` (HTTP server on :8090 with OTel), broken `Dockerfile`, `go.mod` (module `http-server`, Go 1.25.0), minimal README.

---

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Module name | `github.com/kaio6fellipe/go-http-server` | Matches GitHub remote, professional Go convention |
| Single vs multi go.mod | Single root `go.mod` | Simpler for tightly-coupled example monorepo |
| HTTP framework | Standard library (`net/http`) | Go 1.22+ ServeMux has method routing + path params. No framework needed. |
| Base Docker image | `gcr.io/distroless/static-debian12:nonroot` | Smaller than scratch with CA certs + tz data + non-root user built-in |
| 3 Services | user-service, order-service, notification-service | Each demonstrates different hex arch patterns |
| UUID generation | `github.com/google/uuid` (already indirect dep) | Promote to direct dependency |
| OTel in tests | No-op when `OTEL_EXPORTER_OTLP_ENDPOINT` unset | Tests don't require running collector |
| K8s manifests | Kustomize with base + overlays (dev/staging/prod) | Industry standard, no Helm templating complexity |
| E2E tests | Shell scripts (smoke tests) + Docker Compose for local env | Lightweight, no extra Go deps, easy to run in CI |
| Health probes | Separate `/healthz` (liveness) and `/readyz` (readiness) | K8s best practice: different probe semantics |

---

## Target Directory Structure

```
go-http-server/
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
│   ├── user-service/                       # CRUD users (full hex arch example)
│   │   ├── cmd/main.go
│   │   └── internal/
│   │       ├── core/
│   │       │   ├── domain/user.go          # Entities, value objects
│   │       │   ├── port/inbound.go         # UserService interface
│   │       │   ├── port/outbound.go        # UserRepository interface
│   │       │   └── service/                # Use case implementation
│   │       │       ├── user_service.go
│   │       │       └── user_service_test.go
│   │       └── adapter/
│   │           ├── inbound/http/           # HTTP handler (driving adapter)
│   │           │   ├── handler.go
│   │           │   ├── handler_test.go
│   │           │   └── dto.go
│   │           └── outbound/memory/        # In-memory repo (driven adapter)
│   │               └── user_repository.go
│   ├── order-service/                      # Order management (richer domain logic)
│   │   └── (same structure)                # + NotificationSender outbound port
│   └── notification-service/               # Notifications (event dispatch pattern)
│       └── (same structure)                # + Dispatcher outbound port
├── pkg/
│   ├── config/config.go                    # Env-based config loading
│   ├── health/health.go                    # Reusable /healthz and /readyz handlers
│   ├── server/server.go                    # HTTP server with graceful shutdown + OTel wrapping
│   └── telemetry/telemetry.go              # OTel setup (extracted from current main.go)
├── deploy/
│   ├── user-service/
│   │   ├── base/
│   │   │   ├── kustomization.yaml
│   │   │   ├── deployment.yaml
│   │   │   ├── service.yaml
│   │   │   └── configmap.yaml
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
│   ├── order-service/
│   │   ├── base/  (same pattern)
│   │   └── overlays/ (same pattern)
│   └── notification-service/
│       ├── base/  (same pattern)
│       └── overlays/ (same pattern)
├── test/
│   └── e2e/
│       ├── docker-compose.yml              # Spins up all 3 services
│       ├── run-tests.sh                    # Main test runner script
│       ├── test-user-service.sh            # User service CRUD + health tests
│       ├── test-order-service.sh           # Order service flow tests
│       └── test-notification-service.sh    # Notification service tests
├── CLAUDE.md                               # Project instructions for Claude Code
├── .golangci.yml                           # Production linter config (v2)
├── .goreleaser.yaml                        # Multi-binary release config (v2)
├── Dockerfile.user-service
├── Dockerfile.order-service
├── Dockerfile.notification-service
├── Makefile                                # build, test, lint, docker, e2e, deploy targets
├── go.mod                                  # Single module: github.com/kaio6fellipe/go-http-server
├── go.sum
├── .gitignore                              # Updated with bin/, dist/
├── LICENSE
└── README.md                               # Architecture docs, quick start, Makefile reference
```

---

## Implementation Phases

### Phase 1: Shared Packages (`pkg/`)

Create foundational packages that all services depend on.

| File | Description |
|---|---|
| `pkg/config/config.go` | `Config` struct + `Load()` from env vars (ServiceName, HTTPPort, OTLPEndpoint) |
| `pkg/telemetry/telemetry.go` | `Setup(ctx, serviceName)` - extracted from current `initTracer()`. Returns no-op provider when OTLP endpoint not set |
| `pkg/server/server.go` | `Run(ctx, port, registerRoutes)` - creates mux, wraps handlers with OTel, graceful shutdown on SIGINT/SIGTERM |
| `pkg/health/health.go` | `LivenessHandler()` returns `{"status":"ok"}`, `ReadinessHandler()` accepts optional check funcs |
| `pkg/health/health_test.go` | Basic tests for both handlers |

**Verify**: `go build ./pkg/...` and `go test ./pkg/...`

### Phase 2: User Service (template for others)

Full hexagonal architecture CRUD service.

**Domain** (`services/user-service/internal/core/domain/user.go`):
- `User` entity: ID, Name, Email, CreatedAt, UpdatedAt
- `CreateUserRequest`, `UpdateUserRequest` with `Validate()` methods
- Pure domain - zero infrastructure imports

**Ports** (`port/inbound.go`, `port/outbound.go`):
- `UserService` interface: Create, GetByID, List, Update, Delete
- `UserRepository` interface: Save, FindByID, FindAll, Update, Delete

**Service** (`service/user_service.go`):
- Implements `UserService` using `UserRepository` (dependency inversion)
- UUID generation, timestamp management, OTel spans
- `service/user_service_test.go` - table-driven tests with in-memory adapter

**Adapters**:
- `adapter/outbound/memory/user_repository.go` - thread-safe in-memory map with `sync.RWMutex`
- `adapter/inbound/http/dto.go` - request/response DTOs, conversion functions
- `adapter/inbound/http/handler.go` - Go 1.22+ routing: `POST /v1/users`, `GET /v1/users`, `GET /v1/users/{id}`, `PUT /v1/users/{id}`, `DELETE /v1/users/{id}`
- `adapter/inbound/http/handler_test.go` - httptest-based integration tests

**Entry point** (`cmd/main.go`):
- Composition root: wires memory repo -> service -> HTTP handler
- Registers `/healthz` (liveness) and `/readyz` (readiness) endpoints
- Uses `pkg/config`, `pkg/telemetry`, `pkg/server`, `pkg/health`

**Verify**: `go build ./services/user-service/cmd/`, `go test ./services/user-service/...`, curl test

### Phase 3: Order Service

Same hex arch structure with richer business logic.

**Domain**: `Order` (ID, UserID, Items, Status, TotalInCents), `OrderStatus` enum (Pending/Confirmed/Shipped/Delivered/Cancelled), `OrderItem`, `CalculateTotal()` business method

**Ports**: `OrderService`, `OrderRepository`, `NotificationSender` (outbound port for inter-service communication)

**Service**: Status transition validation (can only cancel Pending/Confirmed), auto-calculate total on create, send notification on confirmation

**Adapters**: In-memory repo, log-based notifier (demonstrates outbound port pattern), HTTP handler

**Routes**: `POST /v1/orders`, `GET /v1/orders/{id}`, `GET /v1/orders?user_id=X`, `PATCH /v1/orders/{id}/status`, `POST /v1/orders/{id}/cancel`

### Phase 4: Notification Service

Demonstrates event dispatch pattern.

**Domain**: `Notification` (ID, Recipient, Channel, Subject, Body, Status, SentAt), `Channel` enum (Email/SMS/Push), `NotificationStatus` enum (Queued/Sent/Failed)

**Ports**: `NotificationService`, `NotificationRepository`, `NotificationDispatcher` (outbound port)

**Adapters**: In-memory repo, log-based dispatcher, HTTP handler

**Routes**: `POST /v1/notifications`, `GET /v1/notifications/{id}`, `GET /v1/notifications?recipient=X`

### Phase 5: Cleanup & Module Update

- Delete old `cmd/main.go` and `Dockerfile`
- Rename module in `go.mod` to `github.com/kaio6fellipe/go-http-server`
- Promote `github.com/google/uuid` to direct dependency
- Run `go mod tidy`
- Update `.gitignore` (add `bin/`, `dist/`, `coverage.html`)

### Phase 6: Build & CI/CD Tooling

**Makefile** targets:
- `build SERVICE=<name>`, `build-all`, `test`, `test-cover`, `lint`, `fmt`, `vet`, `mod-tidy`
- `docker-build SERVICE=<name>`, `docker-build-all`, `clean`, `help`
- `e2e` - runs E2E tests via docker-compose + shell scripts
- `deploy-dry-run ENV=dev SERVICE=user-service` - runs `kustomize build` to preview manifests

**3 Dockerfiles** (Dockerfile.{user,order,notification}-service):
- Multi-stage: `golang:1.25-alpine` builder -> `gcr.io/distroless/static-debian12:nonroot`
- `CGO_ENABLED=0`, `-ldflags="-s -w"`, copies full source for shared `pkg/`

**.golangci.yml** (v2):
- Enable: errcheck, govet, staticcheck, unused, ineffassign, gosec, revive, gocyclo, misspell, unconvert, unparam, gosimple, gofmt, goimports, copyloopvar
- Relaxed rules for `_test.go` files

**.goreleaser.yaml** (v2):
- 3 builds (one per service), linux+darwin, amd64+arm64
- Archives, changelog with conventional commit filtering

**GitHub Actions**:
- `ci.yml`: lint (golangci-lint-action), test (go test -race -coverprofile), build - all parallel jobs on PR/push to main
- `release.yml`: goreleaser on tag push `v*`

### Phase 7: Kubernetes Manifests (Kustomize)

Each service gets a `deploy/<service>/` directory with base + overlays.

**Base manifests** (shared across environments):

`deployment.yaml`:
- 1 replica (overridden per overlay)
- Container image placeholder (replaced by kustomize `images` transformer)
- Port 8080 (internal standard)
- Liveness probe: `GET /healthz` (initialDelaySeconds: 10, periodSeconds: 10, failureThreshold: 5)
- Readiness probe: `GET /readyz` (initialDelaySeconds: 5, periodSeconds: 5, failureThreshold: 3)
- Resource requests: cpu 100m, memory 128Mi
- Resource limits: cpu 500m, memory 256Mi
- Environment variables from ConfigMap

`service.yaml`:
- ClusterIP service exposing port 80 -> target port 8080

`configmap.yaml`:
- `HTTP_PORT`, `LOG_LEVEL`, `OTEL_EXPORTER_OTLP_ENDPOINT`

`kustomization.yaml`:
- Lists resources, sets common labels (`app`, `part-of: go-http-server`)

**Overlay specifics**:

| Overlay | Replicas | Resources | Extras |
|---|---|---|---|
| `dev` | 1 | requests only (no limits) | LOG_LEVEL=debug |
| `staging` | 2 | same as base | LOG_LEVEL=info |
| `production` | 3 | higher limits (cpu 1, mem 512Mi) | HPA (min 3, max 10, target 70% CPU), LOG_LEVEL=warn |

**Usage**: `kustomize build deploy/user-service/overlays/dev | kubectl apply -f -`

### Phase 8: E2E Tests

Shell-based E2E smoke tests using Docker Compose.

**`test/e2e/docker-compose.yml`**:
- Builds all 3 services from their Dockerfiles
- Exposes ports: user-service:8081, order-service:8082, notification-service:8083
- Health checks on each container
- No external deps (services use in-memory storage)

**`test/e2e/run-tests.sh`** (main runner):
- Starts docker-compose in background
- Waits for all services to be healthy (polling `/healthz`)
- Runs each service test script sequentially
- Tears down docker-compose on exit (trap cleanup)
- Exits with non-zero code on any failure
- Colored output (green pass, red fail)

**`test/e2e/test-user-service.sh`**:
- Test health check: `GET /healthz` -> 200
- Test readiness: `GET /readyz` -> 200
- Create user: `POST /v1/users` with JSON body -> 201, captures returned ID
- Get user: `GET /v1/users/{id}` -> 200, validates response body
- List users: `GET /v1/users` -> 200, validates array contains created user
- Update user: `PUT /v1/users/{id}` -> 200
- Delete user: `DELETE /v1/users/{id}` -> 204
- Get deleted: `GET /v1/users/{id}` -> 404
- Invalid create: `POST /v1/users` with empty body -> 400

**`test/e2e/test-order-service.sh`**:
- Health + readiness checks
- Create order: `POST /v1/orders` -> 201
- Get order: `GET /v1/orders/{id}` -> 200, verify status=pending
- Update status: `PATCH /v1/orders/{id}/status` to confirmed -> 200
- Cancel order -> verify cancellation
- List by user: `GET /v1/orders?user_id=X` -> 200

**`test/e2e/test-notification-service.sh`**:
- Health + readiness checks
- Send notification: `POST /v1/notifications` -> 201
- Get notification: `GET /v1/notifications/{id}` -> 200, verify status
- List by recipient: `GET /v1/notifications?recipient=X` -> 200

### Phase 9: Claude Code Configuration

**`CLAUDE.md`** (root, ~120 lines):
- Project overview: Go hexagonal architecture monorepo with 3 example microservices
- Build commands: `make build-all`, `make test`, `make lint`, `make e2e`
- Architecture: hexagonal (ports & adapters), domain purity rules, dependency direction
- Coding standards: Go idioms, error handling with `%w`, small interfaces, table-driven tests
- Import ordering: stdlib -> external -> pkg/ -> internal/
- Git workflow: conventional commits (`feat(user-service): add email validation`)
- Service structure reference: domain -> ports -> service -> adapters -> cmd

**`.claude/rules/code-style.md`**:
- Go formatting and naming conventions
- File organization within packages
- Receiver naming (1-2 char abbreviations)
- Error handling patterns specific to this project

**`.claude/rules/testing.md`**:
- Table-driven test template
- Coverage target: 80%+
- Mock interfaces only (never concrete types)
- Integration test patterns with in-memory adapters

**`.claude/rules/api-design.md`** (path-scoped to `**/adapter/inbound/http/**`):
- HTTP handler patterns, JSON error response format
- Status code conventions (201 Created, 204 No Content, etc.)
- Input validation requirements
- DTO conventions (separate from domain types)

**`.claude/agents/code-reviewer.md`** (model: sonnet, read-only tools):
- Reviews Go code for idiomatic patterns, hex arch consistency, error handling, security
- Cannot edit files - only reads and reports
- Checks port/adapter boundary violations

**`.claude/agents/test-writer.md`** (model: haiku, lightweight):
- Generates table-driven Go tests
- Follows project testing conventions
- Targets 80%+ coverage

**`.claude/skills/test-service/SKILL.md`**:
- `/test-service <name>` - runs full test suite for a service (unit tests + lint + race detector + build)

**`.claude/skills/new-service/SKILL.md`**:
- `/new-service <name>` - scaffolds a new hexagonal architecture service with all layers (domain, ports, service, adapters, cmd)

### Phase 10: Documentation

**README.md** rewrite:
- Project description, hex arch diagram (ASCII), directory structure
- Prerequisites (Go 1.25+, Docker, golangci-lint, kustomize)
- Quick start: how to build and run each service
- Makefile targets reference
- Hexagonal architecture explanation mapping to this codebase
- Kubernetes deployment guide (kustomize overlays)
- E2E testing instructions
- Claude Code setup (agents, skills)
- Contributing section, License

---

## Key Files to Modify/Delete

| Action | File |
|---|---|
| DELETE | `cmd/main.go` (code moves to `pkg/` and `services/`) |
| DELETE | `Dockerfile` (replaced by 3 per-service Dockerfiles) |
| MODIFY | `go.mod` (rename module, promote uuid dep) |
| MODIFY | `.gitignore` (add bin/, dist/) |
| MODIFY | `README.md` (complete rewrite) |

## Existing Code to Reuse

| Current Code | New Location |
|---|---|
| `initTracer()` from `cmd/main.go:21-48` | `pkg/telemetry/telemetry.go` (parameterized by service name) |
| Signal handling from `cmd/main.go:66-68` | `pkg/server/server.go` (generalized) |
| Health handler from `cmd/main.go:60-62` | `pkg/health/health.go` (split into liveness + readiness, returns JSON) |
| OTel handler wrapping from `cmd/main.go:74-75` | `pkg/server/server.go` (automatic wrapping) |

---

## Verification Plan

1. **After each service**: `go build ./services/<name>/cmd/` compiles, `go test ./services/<name>/...` passes
2. **After all code**: `go test -race -count=1 ./...` passes
3. **Makefile**: `make lint` passes, `make test` passes, `make build-all` produces 3 binaries in `bin/`
4. **Docker**: `make docker-build-all` builds 3 images successfully
5. **GoReleaser**: `goreleaser check` validates config
6. **Kustomize**: `kustomize build deploy/user-service/overlays/dev` renders valid YAML for each service/overlay
7. **E2E**: `make e2e` runs docker-compose, executes all test scripts, all pass
8. **Claude Code**: CLAUDE.md loads correctly, agents/skills are discoverable
9. **Curl test**: Run each service locally and hit its endpoints to verify HTTP routing works
