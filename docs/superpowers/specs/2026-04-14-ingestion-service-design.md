# Ingestion Service Design Spec

**Date:** 2026-04-14
**Status:** Approved
**Service:** `ingestion`

## Overview

New service that emulates an external webhook ingestion source. Scrapes the Open Library API for coding books and publishes events to the existing event-driven pipeline via the Envoy Gateway. Fully stateless, follows all existing patterns (hexagonal architecture, dual-port server, observability, release).

**Phase 1:** Book catalog ingestion from Open Library API.
**Phase 2 (future):** Synthetic review generation for ingested books.

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Integration model | HTTP client to Gateway | Exercises the full CQRS pipeline (Gateway -> EventSource -> Kafka -> Sensor -> write service) |
| Data source | Open Library API (books) + synthetic reviews (phase 2) | Free, no auth, stable, real data. Synthetic reviews avoid needing a reviews API |
| Scheduling | Continuous polling + on-demand REST trigger | Fits existing service patterns (long-running, dual-port). REST trigger is good for demos |
| Deduplication | None in ingestion service | Downstream services handle idempotency via `pkg/idempotency`. Ingestion fires events freely |
| Storage | Stateless (no storage adapters) | No need to track ingested items. Downstream dedup handles duplicates |
| Configuration | Environment variables | Consistent with all other services via `pkg/config` |
| Concurrency | Single-threaded poll loop | Demo scale (50-100 books per query). Sequential is simple and sufficient |

## Service Structure

```
services/ingestion/
├── cmd/main.go                              # Composition root
├── .goreleaser.yaml                         # Release config
└── internal/
    ├── core/
    │   ├── domain/
    │   │   └── book.go                      # Book type + ScrapeResult + IngestionStatus
    │   ├── port/
    │   │   ├── inbound.go                   # IngestionService interface
    │   │   └── outbound.go                  # BookFetcher + EventPublisher interfaces
    │   └── service/
    │       ├── ingestion_service.go         # Orchestration: fetch -> transform -> publish
    │       └── ingestion_service_test.go    # Unit tests with mock ports
    └── adapter/
        ├── inbound/http/
        │   ├── handler.go                   # POST /v1/ingestion/trigger, GET /v1/ingestion/status
        │   ├── handler_test.go              # HTTP contract tests
        │   └── dto.go                       # Request/response types
        └── outbound/
            ├── openlibrary/
            │   ├── client.go               # Open Library API client (BookFetcher impl)
            │   └── client_test.go          # Canned JSON response tests
            └── gateway/
                ├── publisher.go            # HTTP client to Gateway (EventPublisher impl)
                └── publisher_test.go       # Request format verification tests
```

No storage adapters. No migrations. No memory/postgres outbound adapters.

## Port Interfaces

### Outbound Ports

```go
// BookFetcher retrieves books from an external catalog.
type BookFetcher interface {
    SearchBooks(ctx context.Context, query string, limit int) ([]domain.Book, error)
}

// EventPublisher sends events to the internal event pipeline.
// Returns nil when the EventSource webhook accepts the event (HTTP 200).
// Returns error on non-200 responses or connection failures.
type EventPublisher interface {
    PublishBookAdded(ctx context.Context, book domain.Book) error
}
```

### Inbound Port

```go
// IngestionService orchestrates fetching and publishing.
type IngestionService interface {
    TriggerScrape(ctx context.Context, queries []string) (*domain.ScrapeResult, error)
    GetStatus(ctx context.Context) (*domain.IngestionStatus, error)
}
```

## Domain Types

```go
type Book struct {
    Title       string
    Authors     []string
    ISBN        string
    PublishYear int
    Subjects    []string
}

type ScrapeResult struct {
    BooksFound      int
    EventsPublished int           // 200 responses from EventSource webhook
    Errors          int           // Non-200 responses or connection failures
    Duration        time.Duration
}

type IngestionStatus struct {
    State       string     // "idle" | "running"
    LastRunAt   *time.Time
    LastResult  *ScrapeResult
}
```

## Data Flow

```
Ticker (POLL_INTERVAL) or POST /v1/ingestion/trigger
  -> for each query in SEARCH_QUERIES:
      -> OpenLibrary.SearchBooks(ctx, query, MAX_RESULTS_PER_QUERY)
      -> for each book:
          -> transform domain.Book -> details CreateDetailRequest JSON
          -> POST http://<GATEWAY_URL>/v1/details
             - idempotency_key: "ingestion-isbn-{isbn}" (deterministic)
          -> classify response:
             - 200: accepted into pipeline (increment EventsPublished)
             - non-200 / error: log error, continue (increment Errors)
      -> aggregate stats into ScrapeResult
  -> update IngestionStatus
```

**Gateway pipeline after POST:**
```
POST /v1/details (Gateway)
  -> book-added EventSource (port 12000)
  -> Kafka EventBus (argo-events topic)
  -> details-sensor
  -> details-write service (creates book)
  -> notify-book-added trigger (notification service)
  -> dlqTrigger (if write fails after retries)
```

## Configuration

### Ingestion-Specific Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_URL` | `http://localhost:8080` | Target Gateway base URL for event publishing |
| `POLL_INTERVAL` | `5m` | Duration between automatic scrape cycles |
| `SEARCH_QUERIES` | `programming,golang` | Comma-separated Open Library search terms |
| `MAX_RESULTS_PER_QUERY` | `10` | Maximum books fetched per search query |

### Standard Environment Variables (from `pkg/config`)

`SERVICE_NAME`, `HTTP_PORT` (8080), `ADMIN_PORT` (9090), `LOG_LEVEL` (info), `OTEL_EXPORTER_OTLP_ENDPOINT`, `PYROSCOPE_SERVER_ADDRESS`.

`STORAGE_BACKEND` and `DATABASE_URL` are not used (stateless service).

## REST API

### Business Port (:8080)

| Method | Path | Status | Description |
|--------|------|--------|-------------|
| `POST` | `/v1/ingestion/trigger` | `200 OK` | Trigger immediate scrape cycle. Optional body: `{"queries": ["golang","rust"], "max_results": 5}`. Returns `ScrapeResult`. |
| `GET` | `/v1/ingestion/status` | `200 OK` | Returns current `IngestionStatus` (state, last run time, last run stats). |

### Admin Port (:9090)

Standard: `/healthz`, `/readyz`, `/metrics`, `/debug/pprof/*`.

### No CQRS / No Gateway Exposure

This service is a **producer**, not a consumer of events. It has:
- No EventSource
- No Sensor
- No write Deployment split
- No HTTPRoutes in the Gateway

It is deployed as a single Deployment and makes outbound HTTP calls to the Gateway.

## Observability

### Logging
- Structured JSON via `pkg/logging`
- Request-scoped logging with trace IDs on inbound HTTP
- Log each book publish result (created/duplicate/error) at debug level
- Log scrape cycle summary (found/published/errors) at info level

### Metrics
Standard `pkg/metrics` HTTP middleware on both ports, plus custom metrics:

**Scrape cycle metrics:**
- `ingestion_scrapes_total` (counter) — completed scrape cycles
- `ingestion_books_published_total` (counter) — events accepted by EventSource webhook (200)
- `ingestion_errors_total` (counter) — publish failures (non-200 or connection errors)

**Open Library client metrics:**
- `ingestion_openlibrary_request_duration_seconds` (histogram) — latency per API call
- `ingestion_openlibrary_requests_total` (counter) — labeled by `status_code`, `query`
- `ingestion_openlibrary_errors_total` (counter) — timeouts, connection failures, non-2xx

**Gateway publisher metrics:**
- `ingestion_gateway_request_duration_seconds` (histogram) — latency per publish call
- `ingestion_gateway_requests_total` (counter) — labeled by `status_code`

### Tracing
- `otelhttp.Transport` wrapping `http.Client` in both outbound adapters
- Automatic spans per outgoing request (URL, method, status code, duration)
- Parent span from scrape cycle — each Open Library fetch and Gateway publish is a child span
- Full trace: poll tick -> N Open Library fetches -> M Gateway publishes

### Profiling
- Pyroscope integration via `pkg/profiling` (no-op when `PYROSCOPE_SERVER_ADDRESS` unset)

## Deployment

### Docker
- `build/Dockerfile.ingestion` — multi-stage (golang:1.26.2-alpine -> scratch)
- Same pattern as all other services
- Added to `make docker-build-all`

### GoReleaser
- `services/ingestion/.goreleaser.yaml` — multi-arch (linux/darwin, amd64/arm64), GHCR
- Same template as ratings service

### Helm
- `deploy/ingestion/values-local.yaml` using existing `bookinfo-service` chart
- `cqrs.enabled: false`
- No EventSource, no Sensor, no write Deployment, no HTTPRoutes
- Config values: `GATEWAY_URL`, `POLL_INTERVAL`, `SEARCH_QUERIES`, `MAX_RESULTS_PER_QUERY`
- `GATEWAY_URL` in k8s: `http://default-gw-envoy.envoy-gateway-system.svc.cluster.local`

### Makefile
- Add `ingestion` to `SERVICES` list
- Included in `make build-all`, `make docker-build-all`, `make k8s-deploy`

### Release
- Auto-tag: changes in `services/ingestion/` trigger `ingestion-v*` tags
- `pkg/` changes trigger all services including ingestion
- GoReleaser OSS with `GORELEASER_CURRENT_TAG`

## Testing

| Layer | Target | Approach |
|-------|--------|----------|
| Domain | `Book` validation (empty ISBN, empty title, invalid year) | Table-driven unit tests |
| Service | Fetch -> publish orchestration, error aggregation, status tracking | Mock `BookFetcher` + `EventPublisher` interfaces |
| Handler | HTTP contract (trigger request parsing, status response format) | `httptest.NewRequest` + `httptest.NewRecorder` |
| Open Library client | Response parsing, error handling, pagination | Table-driven tests with canned JSON, `httptest.NewServer` |
| Gateway publisher | Request body/headers format, status code classification | `httptest.NewServer` verifying payload matches details DTO |
| E2E | `test/e2e/test-ingestion.sh` | Start ingestion + details (memory backend), trigger scrape, verify books created via GET |

## Phase 2: Synthetic Reviews (Future)

When phase 2 is implemented:
- Add `ReviewGenerator` outbound port — generates synthetic reviews for known ISBNs
- Add `PublishReviewSubmitted` method to `EventPublisher` — POSTs to `/v1/reviews`
- Coordination: only generate reviews for books where the POST returned 201 or 409 (confirmed registered downstream)
- New env vars: `ENABLE_REVIEWS` (bool), `REVIEWS_PER_BOOK` (int)
- New domain type: `Review` (product_id, reviewer, text, rating)

## Out of Scope

- Amazon scraping (legal/technical complexity not worth it for a demo)
- Real review data from any external source
- Deduplication within the ingestion service
- Persistent state / storage adapters
- Worker pool / concurrent publishing
- Authentication to Open Library (not required)
