# DLQ Documentation Update Design

## Overview

Update `README.md` and `CLAUDE.md` to reflect the new `dlqueue` service, the `dlqTrigger` infrastructure, and the `pkg/idempotency` shared package. Ships atomically with PR #22 (feat/dlqueue-service) as an amendment commit.

## Problem

The documentation describes a 5-service system with no dead-letter handling. PR #22 introduces:

- New `dlqueue` service (hex arch, CQRS, persistence, REST API, state machine)
- `dlqTrigger` on every existing sensor trigger (8 total)
- New `dlq-event-received` EventSource + sensor
- New `pkg/idempotency` shared package used by all write services
- `bookinfo_dlqueue` PostgreSQL database

Without doc updates, newcomers (human or AI agents via `CLAUDE.md`) will not see the DLQ service and will misunderstand the system topology.

## Scope

In scope:
- `README.md` — text sections, tables, 4 mermaid diagrams, intro, project structure
- `CLAUDE.md` — services table, architecture bullets, shared packages, run commands, build counts

Out of scope:
- `docs/superpowers/specs/*.md` — historical design records, point-in-time artifacts
- `docs/superpowers/plans/*.md` — historical plan records
- Code comments, inline doc comments

## README.md Changes

### Intro paragraph (after top CQRS diagram)

No change to the short description. Add one sentence to the paragraph at line 48 emphasizing reliability: "Failure recovery is built into the pipeline — a `dlqTrigger` on every sensor captures events that exhaust retries into a dead-letter queue service (`dlqueue`) where they can be inspected, replayed, or marked resolved."

### Top CQRS diagram (lines 3-44)

Add a DLQ subgraph representing the failure loop. The diagram currently shows the happy path only. Proposed addition:

```
    subgraph "Failure Recovery"
        DLQ[(DLQueue)]
    end

    WriteSvc -.->|on retry exhaustion| DLQ
    DLQ -.->|replay| Webhook
```

Styling: new `dlqStyle` class with `fill:#4a044e,stroke:#c026d3,color:#fff` to visually distinguish from write/query/infra.

### Service Topology diagram (lines 54-74)

Add `DLQ` node representing the dlqueue service. No sync arrows (it's event-driven and doesn't participate in productpage's GET fanout). Styled similar to `N` (notification, event-only) but distinct color.

```
    DLQ["dlqueue<br/>:8085 / :9095<br/><i>failure capture + replay</i>"]

    style DLQ fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
```

### Event-Driven Write Flow diagram (lines 78-126)

This is where DLQ is most informative. Add:

- `DLQES["dlq-event-received<br/>EventSource"]`
- `DLQS["Sensor: dlq-event-received"]`
- `DLQW["dlqueue-write<br/>POST /v1/events"]`
- Three dashed arrows from `S1`, `S2`, `S3` → `DLQES` labeled "dlqTrigger (retries exhausted)"
- `DLQES --> K` (reuses existing Kafka EventBus)
- `K --> DLQS --> DLQW`
- Replay arrow: `DLQW -.->|replay| WH` (loops back through the external webhook entry)

Styling: reuse `fill:#22c55e` for DLQES (same as other EventSources), `fill:#1a1d27,stroke:#a855f7` for DLQW (dlqueue accent).

### Cluster Architecture diagram (lines 328-410)

In the `bookinfo` subgraph add:

- `DLQR["dlqueue"]` (read Deployment)
- `DLQW["dlqueue-write"]` (write Deployment)
- Include DLQ EventSource + Sensor via the existing `ES` / `S` labels (they're grouped abstractly); add a comment noting the DLQ EventSource and Sensor are part of those abstractions.

Add arrows:
- `DLQR --> PG`
- `DLQW --> PG`
- Alloy scrape + Pyroscope push lines include `DLQR` and `DLQW`

No new subgraph needed — dlqueue slots cleanly into the existing bookinfo namespace structure.

### Services table (lines 144-150)

Add one row:

```
| **dlqueue** | Backend (hex arch) | 8085 | 9095 | Captures events failing sensor retry exhaustion; stores in PostgreSQL; supports replay via REST API |
```

### Shared Packages table (lines 158-166)

Add one row:

```
| `pkg/idempotency` | `Store` interface (`CheckAndRecord`) with memory + postgres adapters; `NaturalKey` (SHA-256 with `0x1f` separator to prevent boundary collisions); `Resolve` picks explicit idempotency_key when present, otherwise derives a natural key from business fields. |
```

### Quick Start (lines 187-213)

Add between notification and productpage blocks:

```bash
SERVICE_NAME=dlqueue HTTP_PORT=8085 ADMIN_PORT=9095 ./bin/dlqueue
```

### Makefile targets table (lines 229-257)

Update `build-all` description: "Build all 6 service binaries" (from 5).
Update `docker-build-all` description: "Build Docker images for all 6 services" (from 5).

### Docker image list (lines 284-289)

Add one line:

```
ghcr.io/kaio6fellipe/event-driven-bookinfo/dlqueue:<tag>
```

### Kubernetes Deployment — CQRS Deployment Split (lines 425-432)

Add one row:

```
| `dlqueue` / `dlqueue-write` | Read / Write | operator/service API / dlq-event-received sensor |
```

### Argo Events section (after line 478)

Add a new subsection titled **Dead Letter Queue** with three paragraphs:

1. dlqTrigger contract: every primary trigger has `atLeastOnce: true` + exponential backoff + a `dlqTrigger` that fires after retries exhaust. The dlqTrigger captures the CloudEvents context (via `contextKey`) + body + headers into a structured DLQ payload.

2. DLQ pipeline: the dlqTrigger POSTs to `dlq-event-received` EventSource → Kafka → dlq sensor → `dlqueue-write` service → Postgres. The dlqueue service deduplicates by natural key (`sensor_name + failed_trigger + SHA-256(payload)`) because Argo Events regenerates the CloudEvents `id` on every EventSource pass.

3. Lifecycle + replay: events are tracked through a state machine (`pending → replayed → resolved / poisoned`). Operators or affected services can replay via `POST /v1/events/{id}/replay` — the original payload + headers (preserving `traceparent`) are POSTed back through the source EventSource URL, re-entering the full CQRS pipeline. All write services are idempotent (see `pkg/idempotency`), so replays are safe.

Link to the design spec: "For the full design and REST API, see [docs/superpowers/specs/2026-04-13-dlqueue-service-design.md](docs/superpowers/specs/2026-04-13-dlqueue-service-design.md)."

### Project Structure tree (lines 549-605)

Under `services/`, add:

```
│   ├── dlqueue/                # Dead letter queue (hex arch, new)
│   │   ├── cmd/main.go
│   │   ├── migrations/         # dlq_events + processed_events
│   │   └── internal/
│   │       ├── core/           # domain/, port/, service/
│   │       ├── adapter/
│   │       │   ├── inbound/http/
│   │       │   └── outbound/   # memory/, postgres/, http/ (replay client)
│   │       └── metrics/        # dlq_events_* counters
```

Under `pkg/`, add:

```
│   ├── idempotency/            # Store interface + adapters; natural-key hashing
```

### Releasing section (lines 609-647)

Update:
- "all 5 services" → "all 6 services" (line 616 or wherever applicable)

## CLAUDE.md Changes

### Project Overview (line 5)

Replace the sentence "all event-driven complexity (Kafka consumers, retries, DLQ) is abstracted by Argo Events EventSources and Sensors" with:

"all event-driven complexity (Kafka consumers, retries) is abstracted by Argo Events EventSources and Sensors. Failed events that exhaust sensor retries are captured by the `dlqueue` service for inspection and replay."

### Services table (after line 17)

Add (matching existing rows which use `:8080 / :9090 admin` as the in-container port convention regardless of host port):

```
| **dlqueue** | Backend (hex arch) | :8080 / :9090 admin | Dead letter queue for failed sensor deliveries; REST API for inspection, replay, and resolution |
```

Note: README uses host-facing ports (`8085 / 9095` for dlqueue), while CLAUDE.md uses in-container ports (`:8080 / :9090`). This mirrors the existing style in both files and is intentional — CLAUDE.md describes the standardized service shape, README documents the dev/local port mapping.

### Architecture bullets (after line 28)

Insert two new bullets:

- **DLQ**: sensor `dlqTrigger` → `dlq-event-received` EventSource → `dlqueue-write` → PostgreSQL. Dedup by natural key (`sensor_name + failed_trigger + payload_hash`). State machine: `pending → replayed → resolved / poisoned`. REST API at `/v1/events` supports list/get/replay/resolve/reset plus batch operations.
- **Idempotency**: all write services (reviews, ratings, details, notification, dlqueue) dedupe on client-supplied `idempotency_key` or derived natural key (SHA-256 hash of business fields). Prerequisite for safe DLQ replay — CloudEvents `id` cannot be used because Argo Events regenerates it per EventSource pass.

### Build Commands (line 32-38)

```
make build-all          # Build all 6 service binaries to bin/
```

```
make docker-build-all   # Build all 6 Docker images
```

### Run Locally (line 72-100)

Add after the notification block:

```
**dlqueue** (no dependencies):

\`\`\`bash
SERVICE_NAME=dlqueue HTTP_PORT=8085 ADMIN_PORT=9095 go run ./services/dlqueue/cmd/
\`\`\`
```

### Shared Packages table (line 106-114)

Add row:

```
| `pkg/idempotency` | `Store` interface (`CheckAndRecord`) with memory + postgres adapters; `NaturalKey(fields...)` (SHA-256 with `0x1f` separator); `Resolve(explicitKey, fields...)` picks explicit when present, natural key otherwise |
```

## Style conventions

- Preserve user's `<h1 align="center">` at top of README
- Preserve existing mermaid node styling (reuse classes where possible; add one new class for DLQ)
- Preserve existing table column order and formatting
- Preserve conventional commit style for commit message
- Use backticks for file paths, code identifiers, env vars

## Deliverable

One commit on `feat/dlqueue-service` branch:

```
docs: update README and CLAUDE.md for dlqueue service

Add dlqueue service to services tables, CQRS split table, and project
structure. Update all four mermaid diagrams to show the dlqTrigger
failure loop. Add new "Dead Letter Queue" subsection under Argo Events
describing the pipeline and replay semantics. Add pkg/idempotency row
to shared packages. Update "5 services" references to "6".
```

Re-trigger CI on PR #22 (automatic on push).

## Out of Scope (v1)

- Standalone `docs/dlqueue.md` deep-dive (spec doc already exists at `docs/superpowers/specs/2026-04-13-dlqueue-service-design.md`)
- Admin UI documentation (not implemented yet)
- Runbook / troubleshooting guide (can be added later if operational issues surface)
- Updates to `docs/superpowers/` spec and plan files (historical artifacts)
