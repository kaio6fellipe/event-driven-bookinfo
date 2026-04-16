# Ingestion Documentation Update Design

**Date:** 2026-04-15
**Status:** Approved
**Scope:** `README.md` and `CLAUDE.md` only — ships atomically with the ingestion feature PR on `feat/ingestion-service`.

## Overview

Update `README.md` and `CLAUDE.md` to reflect the new `ingestion` service — a stateless, outbound-only producer that polls the Open Library API and publishes `book-added` events through the Envoy Gateway. Matches the pattern established by the dlqueue docs update (`docs/superpowers/specs/2026-04-13-dlqueue-docs-update-design.md`): doc changes ship with the feature PR, historical specs and plans remain unchanged.

## Problem

The ingestion service is fully implemented (core + adapters + cmd + tests + Helm values + Makefile + CI auto-tag integration) on branch `feat/ingestion-service`, but neither `README.md` nor `CLAUDE.md` mentions it. Readers — human or AI agents consuming `CLAUDE.md` — will see a 6-service topology and miss:

- A new service type (outbound producer, no inbound business traffic, no storage)
- A new demo capability (self-hosted event broker driven by real external data)
- Ingestion-specific env vars (`GATEWAY_URL`, `POLL_INTERVAL`, `SEARCH_QUERIES`, `MAX_RESULTS_PER_QUERY`)
- Updated service counts ("6 services" → "7 services") in build/release commentary

## Scope

**In scope:**
- `README.md` — services table, intro, 2 mermaid diagrams (Event-Driven Write Flow + Cluster Architecture), Quick Start, Makefile targets, Docker image list, Local Kubernetes CQRS note, Observability business metrics, new "Data Ingestion" subsection, Project Structure tree, service-count references
- `CLAUDE.md` — project overview sentence, services table, architecture bullets, build-command counts, Run Locally block, optional env-var list

**Out of scope:**
- `docs/superpowers/specs/*.md` and `docs/superpowers/plans/*.md` — point-in-time historical artifacts (matches dlqueue precedent)
- Per-service `services/ingestion/README.md` — no existing convention; deferred
- Top CQRS diagram (README lines 12-62) — abstract, unchanged (ingestion is another POST source, no new flow)
- Service Topology diagram (README lines 72-94) — shows sync service-to-service deps; ingestion has none
- `.github/SECURITY.md` — not service-scoped, unaffected
- Argo Events section — ingestion doesn't use Argo Events directly (publishes via Gateway)
- Shared Packages table — ingestion adds no new package under `pkg/`
- Service Structure (hex arch) section in CLAUDE.md — ingestion follows the same structure, just without memory/postgres outbound adapters

## README.md Changes

### Intro paragraph (append after line 66)

Append one sentence to the existing overview paragraph:

> "A standalone `ingestion` service demonstrates the pipeline as a self-hosted event broker — it polls the Open Library API and publishes synthetic book-added events through the same Gateway → EventSource → Kafka → Sensor path used by the UI."

### Services table (append one row after the dlqueue row at line 186)

```
| **ingestion** | [![release](https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=ingestion-v*&sort=semver&display_name=tag&label=release)](https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=ingestion-v&expanded=true) | [![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/badges/coverage-ingestion.json)](https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain) | Producer (hex arch) | 8086 | 9096 | Polls Open Library for books on a configurable interval and publishes `book-added` events to the Gateway. Stateless; no storage adapters. |
```

- Host ports `8086 / 9096` continue the sequential allocation pattern (productpage 8080 → dlqueue 8085 → ingestion 8086).
- Type column value `Producer (hex arch)` is a new label. Rationale: ingestion's architectural role (outbound-only event producer) differs from "Backend (hex arch)" (bidirectional REST service). Documented in the services-table legend via the Description column.
- Release and coverage badges follow the existing naming convention (`ingestion-v*` tag filter, `coverage-ingestion.json` file). The coverage JSON is produced by the existing CI workflow once ingestion is added to the services list (already done — see commit `bc49da6`).

### Event-Driven Write Flow diagram (lines 98-161)

Add one new node and one new arrow. All existing nodes, flows, and styles remain untouched.

```
ING["ingestion<br/>Open Library poller"]
ING -->|"POST /v1/details"| GW
style ING fill:#1a1d27,color:#e4e4e7,stroke:#10b981
```

Green stroke (`#10b981`) distinguishes ingestion from backend nodes (grey stroke), EventSources (green fill), and dlqueue nodes (purple stroke). The node sits upstream of `GW` (Gateway) alongside `Browser`, `PP`, and `WH` as one of the POST sources.

### Cluster Architecture diagram (lines 368-456)

Add `ING["ingestion"]` node inside the `bookinfo` subgraph. Add arrows:

- `ING --> GW` (outbound publish to Gateway)
- Add `ING` to the existing Alloy scrape line, OTLP traces line, and push-profiles line so the observability arrows enumerate all bookinfo workloads

Same styling as the Write Flow diagram (`stroke:#10b981`).

### Quick Start (after the productpage block at line 252)

Add:

````markdown
# Optional standalone demo — polls Open Library and publishes to the Gateway
SERVICE_NAME=ingestion HTTP_PORT=8086 ADMIN_PORT=9096 \
  GATEWAY_URL=http://localhost:8080 \
  POLL_INTERVAL=5m \
  SEARCH_QUERIES=programming,golang \
  MAX_RESULTS_PER_QUERY=10 \
  ./bin/ingestion
````

Positioned after productpage because ingestion depends on the Gateway/productpage port being reachable at `GATEWAY_URL`.

### Makefile targets table (lines 270-296)

- `make build-all` description → "Build all 7 service binaries"
- `make docker-build-all` description → "Build Docker images for all 7 services"

### Docker image list (lines 323-329)

Add one line:

```
ghcr.io/kaio6fellipe/event-driven-bookinfo/ingestion:<tag>
```

### Local Kubernetes — CQRS Deployment Split note (after the table at lines 471-479)

Append a one-line note below the table:

> "`ingestion` deploys as a single stateless deployment with no CQRS split — it is a pure event producer and is omitted from the table above."

### Observability — business metrics table (after line 572)

Append one row:

```
| ingestion | `ingestion_scrapes_total`, `ingestion_books_published_total`, `ingestion_errors_total` |
```

### NEW subsection `## Data Ingestion`

Insert after the "Dead Letter Queue" subsection and before "E2E Tests". Three paragraphs:

**Paragraph 1 — Purpose:**

> "The `ingestion` service is a demo-scale synthetic-data generator that exercises the full event-driven write pipeline end-to-end (Gateway → EventSource → Kafka → Sensor → `details-write`). It runs as a single stateless deployment with no storage adapters, no CQRS split, and no EventSource/Sensor of its own — it is purely outbound."

**Paragraph 2 — Flow:**

> "A background poll loop ticks every `POLL_INTERVAL` (default 5m). For each query in `SEARCH_QUERIES`, the service calls `GET https://openlibrary.org/search.json`, validates each returned book (title, ISBN, authors, and publish year are required), and publishes accepted books via `POST {GATEWAY_URL}/v1/details`. Each publish carries a deterministic idempotency key `ingestion-isbn-<ISBN>` so the downstream `details-write` service deduplicates across cycles via `pkg/idempotency`. On-demand scrapes are also available via `POST /v1/ingestion/trigger` (optional body `{"queries": ["golang", "rust"]}`)."

**Paragraph 3 — Config + link:**

> "Configuration via `GATEWAY_URL`, `POLL_INTERVAL`, `SEARCH_QUERIES`, and `MAX_RESULTS_PER_QUERY` environment variables. Because idempotency is enforced at the write service, replays and overlapping cycles are safe. For the full design, port contracts, and metric definitions, see [docs/superpowers/specs/2026-04-14-ingestion-service-design.md](docs/superpowers/specs/2026-04-14-ingestion-service-design.md)."

### Project Structure tree (services/ block at lines 608-634)

Insert after the dlqueue entry:

```
│   └── ingestion/              # Producer (hex arch) — Open Library scraper, publishes to Gateway
│       ├── cmd/main.go
│       └── internal/
│           ├── core/           # domain/, port/, service/
│           └── adapter/
│               ├── inbound/http/
│               └── outbound/   # openlibrary/ (BookFetcher), gateway/ (EventPublisher)
```

### Releasing section (lines 676-711)

Update any "6 services" references to "7 services" — specifically the bullet "changes to `pkg/`, `go.mod`, or `go.sum` trigger all 6 services" at line 682 becomes "trigger all 7 services".

## CLAUDE.md Changes

### Project Overview (line 5)

Append one sentence to the existing overview paragraph:

> "A standalone `ingestion` service polls the Open Library API and publishes `book-added` events through the Gateway, demonstrating the system as a self-hosted event broker."

### Services table (append one row after the dlqueue row at line 18)

```
| **ingestion** | Producer (hex arch) | :8080 / :9090 admin | Open Library scraper; polls on interval, publishes book-added events to the Gateway |
```

Uses in-container port convention (`:8080 / :9090`) matching existing CLAUDE.md rows. Host-facing ports (`8086 / 9096`) are documented only in README.md — this matches the existing convention also followed in the dlqueue docs-update spec.

### Architecture bullets (after the DLQ + Idempotency bullets)

Insert one new bullet:

> - **Ingestion**: `ingestion` service polls Open Library on `POLL_INTERVAL` → for each query in `SEARCH_QUERIES` → `POST {GATEWAY_URL}/v1/details` with `idempotency_key=ingestion-isbn-<ISBN>`. Stateless, single deployment, no CQRS split, no EventSource/Sensor of its own. Exercises the full write pipeline end-to-end.

### Build Commands (lines 32-38)

- `make build-all` description → "Build all 7 service binaries to bin/"
- `make docker-build-all` description → "Build all 7 Docker images"

### Run Locally (after the dlqueue block, before the productpage block)

Add:

````markdown
**ingestion** (publishes to Gateway; requires Gateway/productpage reachable):

```bash
SERVICE_NAME=ingestion HTTP_PORT=8086 ADMIN_PORT=9096 \
  GATEWAY_URL=http://localhost:8080 \
  POLL_INTERVAL=5m \
  SEARCH_QUERIES=programming,golang \
  MAX_RESULTS_PER_QUERY=10 \
  go run ./services/ingestion/cmd/
```
````

### Optional env-var list (bottom of "Run Locally" section)

Append one sentence to the paragraph listing optional env vars:

> "Ingestion-specific: `GATEWAY_URL`, `POLL_INTERVAL`, `SEARCH_QUERIES`, `MAX_RESULTS_PER_QUERY`."

### Shared Packages table

**No change.** Ingestion adds no new package under `pkg/`.

### Service Structure (hex arch) section

**No change.** Ingestion follows the same structure, with the difference (no memory/postgres outbound adapters) captured in the new architecture bullet above.

## Style Conventions

- Preserve the `<h1 align="center">` header block at the top of README.md
- Reuse existing mermaid node styling classes where possible; introduce one new stroke color (`#10b981` green) to visually distinguish ingestion from backend (grey), EventSource (green fill), and dlqueue (purple) nodes
- Preserve existing table column order and formatting
- Use backticks for file paths, code identifiers, and environment variables
- Conventional commit style for the commit message

## Deliverable

One commit on the `feat/ingestion-service` branch (docs changes ship atomically with the implementation PR, mirroring the dlqueue-docs-update precedent):

```
docs: update README and CLAUDE.md for ingestion service

Add ingestion service row to services tables, project structure, Quick
Start, Run Locally, business metrics, and Docker image list. Update
Event-Driven Write Flow and Cluster Architecture diagrams with the new
producer node. Add new "Data Ingestion" subsection after Dead Letter
Queue describing purpose, flow, and config. Bump "6 services" references
to "7". Note ingestion's omission from the CQRS Deployment Split table.
```

## Validation

- `git diff` review — verify no stray edits and that the 7-service count is consistent wherever it appears
- Render both modified mermaid diagrams (Event-Driven Write Flow + Cluster Architecture) in a Markdown preview to confirm the new `ING` node and arrows render correctly
- Spot-check every "N services" count reference in README.md and CLAUDE.md is consistent with 7
- CI re-runs automatically on push; linters and link checks gate the merge

## Out of Scope (v1)

- Per-service `services/ingestion/README.md` — no existing convention in the repo
- Updates to historical `docs/superpowers/specs/*` and `docs/superpowers/plans/*` — they are point-in-time artifacts
- Admin UI documentation (no admin UI exists)
- Runbook / troubleshooting guide (can be added later if operational issues surface)
- Phase 2 synthetic review generation — already captured as a "Phase 2" section in the ingestion design spec itself
