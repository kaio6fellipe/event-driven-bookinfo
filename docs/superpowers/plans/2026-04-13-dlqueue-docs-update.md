# DLQ Documentation Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update `README.md` and `CLAUDE.md` to reflect the new `dlqueue` service, `dlqTrigger` infrastructure, and `pkg/idempotency` shared package. Amend PR #22 (feat/dlqueue-service) with one docs commit before merge.

**Architecture:** Pure documentation changes. Two files. No code. Four mermaid diagrams get a failure-loop addition; tables and prose get dlqueue rows and references; a new "Dead Letter Queue" subsection is added under the Argo Events section.

**Tech Stack:** Markdown, Mermaid. GitHub renders both natively.

---

## File Structure

### Modified files

- `README.md` — Services table, Shared Packages table, Quick Start, Makefile table, Docker image list, CQRS Deployment Split table, Project Structure tree, Releasing section, Argo Events section (new DLQ subsection), 4 mermaid diagrams
- `CLAUDE.md` — Project Overview, Services table, Architecture bullets, Build Commands, Run Locally, Shared Packages table

### No new files. No deletions.

---

## Task 1: README.md — text/table updates (non-diagram)

**Files:**
- Modify: `README.md`

Work from: `/Users/kaio.fellipe/Documents/git/others/go-http-server`
Branch: `feat/dlqueue-service`

- [ ] **Step 1: Update intro paragraph**

Find the paragraph at line 46 (the one starting with "Go hexagonal architecture monorepo..."). It currently ends before the "## Architecture Overview" heading. Locate the second paragraph:

```
Services are plain REST APIs — all event-driven complexity (Kafka consumers, retries, dead-letter queues) is abstracted by Argo Events EventSources and Sensors. The write path flows through Kafka via Argo Events, ensuring every mutation is event-sourced, while the read path remains synchronous HTTP. This demonstrates using Argo Events not only for workflow automation, but as a real-time event-driven architecture platform — a self-hosted alternative to Google Eventarc or AWS EventBridge.
```

Replace with:

```
Services are plain REST APIs — all event-driven complexity (Kafka consumers, retries, dead-letter queues) is abstracted by Argo Events EventSources and Sensors. The write path flows through Kafka via Argo Events, ensuring every mutation is event-sourced, while the read path remains synchronous HTTP. Failure recovery is built into the pipeline — a `dlqTrigger` on every sensor captures events that exhaust retries into the `dlqueue` service, where they can be inspected, replayed, or marked resolved. This demonstrates using Argo Events not only for workflow automation, but as a real-time event-driven architecture platform — a self-hosted alternative to Google Eventarc or AWS EventBridge.
```

- [ ] **Step 2: Add dlqueue row to Services table**

Find the Services table (around line 144). After the `notification` row (last row in the table), add:

```
| **dlqueue** | Backend (hex arch) | 8085 | 9095 | Captures events failing sensor retry exhaustion; stores in PostgreSQL; supports replay via REST API |
```

- [ ] **Step 3: Add pkg/idempotency row to Shared Packages table**

Find the Shared Packages table (around line 158). After the `pkg/telemetry` row (last row), add:

```
| `pkg/idempotency` | `Store` interface (`CheckAndRecord`) with memory + postgres adapters; `NaturalKey` (SHA-256 with `0x1f` separator to prevent boundary collisions); `Resolve` picks explicit `idempotency_key` when present, otherwise derives a natural key from business fields. |
```

- [ ] **Step 4: Add dlqueue to Quick Start bash block**

Find the Quick Start section (around line 187). Locate the run commands block. After the `notification` command:

```bash
SERVICE_NAME=notification HTTP_PORT=8084 ADMIN_PORT=9094 ./bin/notification
```

Insert on the next line (before the `# Optional: REDIS_URL=...` comment):

```bash

SERVICE_NAME=dlqueue HTTP_PORT=8085 ADMIN_PORT=9095 ./bin/dlqueue
```

- [ ] **Step 5: Update Makefile table descriptions**

Find the Makefile targets table (around line 229). Locate these two rows:

```
| `make build-all` | Build all 5 service binaries |
...
| `make docker-build-all` | Build Docker images for all 5 services |
```

Change both to say "6 services" / "6 service binaries":

```
| `make build-all` | Build all 6 service binaries |
...
| `make docker-build-all` | Build Docker images for all 6 services |
```

- [ ] **Step 6: Add dlqueue to Docker image list**

Find the Docker image list (around line 284). The block looks like:

```
ghcr.io/kaio6fellipe/event-driven-bookinfo/productpage:<tag>
ghcr.io/kaio6fellipe/event-driven-bookinfo/details:<tag>
ghcr.io/kaio6fellipe/event-driven-bookinfo/reviews:<tag>
ghcr.io/kaio6fellipe/event-driven-bookinfo/ratings:<tag>
ghcr.io/kaio6fellipe/event-driven-bookinfo/notification:<tag>
```

Add one line at the end:

```
ghcr.io/kaio6fellipe/event-driven-bookinfo/dlqueue:<tag>
```

- [ ] **Step 7: Add dlqueue row to CQRS Deployment Split table**

Find the CQRS Deployment Split table (around line 425). After the `notification` row:

```
| `notification` | Write-only | All 3 sensors |
```

Add:

```
| `dlqueue` / `dlqueue-write` | Read / Write | operator/service API (GET) / `dlq-event-received` sensor (POST) |
```

- [ ] **Step 8: Update Project Structure tree**

Find the Project Structure code block (around line 549). Locate these lines in the `services/` section:

```
│   ├── reviews/                # User reviews (hex arch, calls ratings)
│   ├── ratings/                # Star ratings (hex arch)
│   └── notification/           # Event consumer + audit log (hex arch)
```

Change the `└──` to `├──` on the notification line and add a dlqueue block:

```
│   ├── reviews/                # User reviews (hex arch, calls ratings)
│   ├── ratings/                # Star ratings (hex arch)
│   ├── notification/           # Event consumer + audit log (hex arch)
│   └── dlqueue/                # Dead letter queue (hex arch) — NEW
│       ├── cmd/main.go
│       ├── migrations/         # dlq_events + processed_events
│       └── internal/
│           ├── core/           # domain/, port/, service/
│           ├── adapter/
│           │   ├── inbound/http/
│           │   └── outbound/   # memory/, postgres/, http/ (replay client)
│           └── metrics/        # dlq_events_* counters
```

Then in the `pkg/` section, find these lines:

```
│   ├── config/                 # Env-based configuration
│   ├── health/                 # /healthz and /readyz handlers
```

Insert a new line after `health/`:

```
│   ├── idempotency/            # Store interface + adapters; natural-key hashing
```

So the full `pkg/` block becomes:

```
├── pkg/
│   ├── config/                 # Env-based configuration
│   ├── health/                 # /healthz and /readyz handlers
│   ├── idempotency/            # Store interface + adapters; natural-key hashing
│   ├── logging/                # slog + otelslog bridge + HTTP middleware
│   ├── metrics/                # OTel -> Prometheus + HTTP middleware + runtime
│   ├── profiling/              # Pyroscope SDK wrapper
│   ├── server/                 # Dual-port server + graceful shutdown
│   └── telemetry/              # OTel tracing setup
```

- [ ] **Step 9: Update Releasing section**

Find the Releasing section (around line 611). Locate step 2 of "How It Works":

```
2. **Detects changed services** — file paths under `services/<name>/`; changes to `pkg/`, `go.mod`, or `go.sum` trigger all 5 services
```

Change `5` to `6`:

```
2. **Detects changed services** — file paths under `services/<name>/`; changes to `pkg/`, `go.mod`, or `go.sum` trigger all 6 services
```

- [ ] **Step 10: Verify README still renders**

Run:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
grep -c "^|.*dlqueue" README.md
```

Expected output: `2` (one in Services table, one in CQRS Deployment Split) — does not count the `pkg/idempotency` row.

Run:

```bash
grep -n "5 services\|5 service binaries" README.md
```

Expected output: empty (no more "5 services" references).

- [ ] **Step 11: Commit**

```bash
git add README.md
git commit -m "docs(readme): add dlqueue to tables, project structure, and prose"
```

---

## Task 2: README.md — mermaid diagram updates

**Files:**
- Modify: `README.md` (4 mermaid blocks)

- [ ] **Step 1: Update top CQRS diagram (lines 3-44)**

Find the mermaid block that starts at line 3 (`flowchart LR`). The current structure:

```
flowchart LR
    User([User])
    Gateway[Envoy Gateway]

    subgraph "Query Side"
        ReadAPI[Read API]
        ReadDB[(Read DB)]
    end

    subgraph "Command Side"
        subgraph "Argo Events"
            Webhook[Webhook]
            Kafka[[Kafka EventBus]]
            Sensor[Sensor]
        end
        WriteSvc[Write Services]
        WriteDB[(Write DB)]
    end

    User -->|GET Query| Gateway
    User -->|POST Command| Gateway

    Gateway -->|method: GET| ReadAPI
    ReadAPI --> ReadDB

    Gateway -->|method: POST| Webhook
    Webhook --> Kafka
    Kafka --> Sensor
    Sensor -->|HTTP Trigger| WriteSvc
    WriteSvc --> WriteDB

    WriteDB -.->|replication| ReadDB

    classDef queryStyle fill:#1e3a8a,stroke:#3b82f6,color:#fff
    classDef writeStyle fill:#7c2d12,stroke:#f97316,color:#fff
    classDef infraStyle fill:#1f2937,stroke:#06b6d4,color:#fff

    class ReadAPI,ReadDB queryStyle
    class WriteSvc,WriteDB writeStyle
    class Gateway,Webhook,Kafka,Sensor infraStyle
```

Replace the entire block with:

```
flowchart LR
    User([User])
    Gateway[Envoy Gateway]

    subgraph "Query Side"
        ReadAPI[Read API]
        ReadDB[(Read DB)]
    end

    subgraph "Command Side"
        subgraph "Argo Events"
            Webhook[Webhook]
            Kafka[[Kafka EventBus]]
            Sensor[Sensor]
        end
        WriteSvc[Write Services]
        WriteDB[(Write DB)]
    end

    subgraph "Failure Recovery"
        DLQ[(DLQueue)]
    end

    User -->|GET Query| Gateway
    User -->|POST Command| Gateway

    Gateway -->|method: GET| ReadAPI
    ReadAPI --> ReadDB

    Gateway -->|method: POST| Webhook
    Webhook --> Kafka
    Kafka --> Sensor
    Sensor -->|HTTP Trigger| WriteSvc
    WriteSvc --> WriteDB

    WriteDB -.->|replication| ReadDB

    Sensor -.->|dlqTrigger<br/>retries exhausted| DLQ
    DLQ -.->|replay| Webhook

    classDef queryStyle fill:#1e3a8a,stroke:#3b82f6,color:#fff
    classDef writeStyle fill:#7c2d12,stroke:#f97316,color:#fff
    classDef infraStyle fill:#1f2937,stroke:#06b6d4,color:#fff
    classDef dlqStyle fill:#4a044e,stroke:#c026d3,color:#fff

    class ReadAPI,ReadDB queryStyle
    class WriteSvc,WriteDB writeStyle
    class Gateway,Webhook,Kafka,Sensor infraStyle
    class DLQ dlqStyle
```

- [ ] **Step 2: Update Service Topology diagram (lines 54-74)**

Find the mermaid block starting with `graph TD` that contains `PP["productpage (BFF)..."]`. The current structure:

```
graph TD
    PP["productpage (BFF)<br/>Go + html/template + HTMX<br/>:8080 / :9090"]
    D["details<br/>:8081 / :9091"]
    R["reviews<br/>:8082 / :9092"]
    RT["ratings<br/>:8083 / :9093"]
    N["notification<br/>:8084 / :9094<br/><i>event consumer only</i>"]
    Redis["Redis<br/><i>pending review cache</i>"]

    PP -->|sync GET| D
    PP -->|sync GET| R
    R -->|sync GET| RT
    PP -->|"pending cache"| Redis

    style PP fill:#6366f1,color:#fff,stroke:#818cf8
    style D fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style R fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style RT fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style N fill:#1a1d27,color:#e4e4e7,stroke:#f59e0b
    style Redis fill:#ef4444,color:#fff,stroke:#dc2626
```

Replace with (adds DLQ node + style):

```
graph TD
    PP["productpage (BFF)<br/>Go + html/template + HTMX<br/>:8080 / :9090"]
    D["details<br/>:8081 / :9091"]
    R["reviews<br/>:8082 / :9092"]
    RT["ratings<br/>:8083 / :9093"]
    N["notification<br/>:8084 / :9094<br/><i>event consumer only</i>"]
    DLQ["dlqueue<br/>:8085 / :9095<br/><i>failure capture + replay</i>"]
    Redis["Redis<br/><i>pending review cache</i>"]

    PP -->|sync GET| D
    PP -->|sync GET| R
    R -->|sync GET| RT
    PP -->|"pending cache"| Redis

    style PP fill:#6366f1,color:#fff,stroke:#818cf8
    style D fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style R fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style RT fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style N fill:#1a1d27,color:#e4e4e7,stroke:#f59e0b
    style DLQ fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
    style Redis fill:#ef4444,color:#fff,stroke:#dc2626
```

- [ ] **Step 3: Update Event-Driven Write Flow diagram (lines 78-126)**

Find the mermaid block starting with `graph TD` that contains `Browser["Browser POST /partials/rating"]`. The current structure:

```
graph TD
    Browser["Browser POST /partials/rating"]
    PP["productpage (BFF)"]
    GW["Gateway (CQRS routing)"]
    WH["External POST /v1/*"]
    ES["Argo Events<br/>EventSource (webhook)"]
    K["Kafka EventBus<br/>CloudEvent + traceparent"]

    S1["Sensor: book-added"]
    S2["Sensor: review-submitted"]
    S3["Sensor: rating-submitted"]

    D["details-write<br/>POST /v1/details"]
    R["reviews-write<br/>POST /v1/reviews"]
    RT["ratings-write<br/>POST /v1/ratings"]
    N1["notification<br/>POST /v1/notifications"]
    N2["notification<br/>POST /v1/notifications"]
    N3["notification<br/>POST /v1/notifications"]

    Browser --> PP
    PP -->|"POST /v1/* via gateway"| GW
    WH --> GW
    GW -->|"method: POST"| ES
    ES --> K

    K --> S1
    K --> S2
    K --> S3

    S1 -->|HTTP Trigger| D
    S1 -->|HTTP Trigger| N1

    S2 -->|HTTP Trigger| R
    S2 -->|HTTP Trigger| N2

    S3 -->|HTTP Trigger| RT
    S3 -->|HTTP Trigger| N3

    style Browser fill:#6366f1,color:#fff,stroke:#818cf8
    style PP fill:#6366f1,color:#fff,stroke:#818cf8
    style GW fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style WH fill:#6366f1,color:#fff,stroke:#818cf8
    style ES fill:#22c55e,color:#fff,stroke:#16a34a
    style K fill:#f59e0b,color:#000,stroke:#d97706
    style S1 fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style S2 fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style S3 fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
```

Replace with (adds DLQ EventSource, Sensor, write service, dlqTrigger arrows, and replay arrow):

```
graph TD
    Browser["Browser POST /partials/rating"]
    PP["productpage (BFF)"]
    GW["Gateway (CQRS routing)"]
    WH["External POST /v1/*"]
    ES["Argo Events<br/>EventSource (webhook)"]
    K["Kafka EventBus<br/>CloudEvent + traceparent"]

    S1["Sensor: book-added"]
    S2["Sensor: review-submitted"]
    S3["Sensor: rating-submitted"]

    D["details-write<br/>POST /v1/details"]
    R["reviews-write<br/>POST /v1/reviews"]
    RT["ratings-write<br/>POST /v1/ratings"]
    N1["notification<br/>POST /v1/notifications"]
    N2["notification<br/>POST /v1/notifications"]
    N3["notification<br/>POST /v1/notifications"]

    DLQES["dlq-event-received<br/>EventSource"]
    DLQS["Sensor: dlq-event-received"]
    DLQW["dlqueue-write<br/>POST /v1/events"]

    Browser --> PP
    PP -->|"POST /v1/* via gateway"| GW
    WH --> GW
    GW -->|"method: POST"| ES
    ES --> K

    K --> S1
    K --> S2
    K --> S3

    S1 -->|HTTP Trigger| D
    S1 -->|HTTP Trigger| N1

    S2 -->|HTTP Trigger| R
    S2 -->|HTTP Trigger| N2

    S3 -->|HTTP Trigger| RT
    S3 -->|HTTP Trigger| N3

    S1 -.->|"dlqTrigger (retries exhausted)"| DLQES
    S2 -.->|"dlqTrigger (retries exhausted)"| DLQES
    S3 -.->|"dlqTrigger (retries exhausted)"| DLQES
    DLQES --> K
    K --> DLQS
    DLQS -->|HTTP Trigger| DLQW
    DLQW -.->|replay via POST to eventsource_url| WH

    style Browser fill:#6366f1,color:#fff,stroke:#818cf8
    style PP fill:#6366f1,color:#fff,stroke:#818cf8
    style GW fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style WH fill:#6366f1,color:#fff,stroke:#818cf8
    style ES fill:#22c55e,color:#fff,stroke:#16a34a
    style K fill:#f59e0b,color:#000,stroke:#d97706
    style S1 fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style S2 fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style S3 fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style DLQES fill:#22c55e,color:#fff,stroke:#16a34a
    style DLQS fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
    style DLQW fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
```

- [ ] **Step 4: Update Cluster Architecture diagram (lines 328-410)**

Find the mermaid block starting with `graph TD` inside the Cluster Architecture section. It contains `Browser["Browser :8080"]` and `subgraph envoy-gateway-system`. The current `bookinfo` subgraph contains:

```
    subgraph bookinfo
        PP["productpage"]

        DR["details"]
        RR["reviews"]
        RTR["ratings"]

        DW["details-write"]
        RW["reviews-write"]
        RTW["ratings-write"]
        N["notification"]

        ES["EventSources"]
        EB["EventBus"]
        S["Sensors"]

        PG["PostgreSQL"]
        Redis["Redis"]
    end
```

Change it to add dlqueue read/write deployments:

```
    subgraph bookinfo
        PP["productpage"]

        DR["details"]
        RR["reviews"]
        RTR["ratings"]

        DW["details-write"]
        RW["reviews-write"]
        RTW["ratings-write"]
        N["notification"]

        DLQR["dlqueue"]
        DLQW["dlqueue-write"]

        ES["EventSources"]
        EB["EventBus"]
        S["Sensors"]

        PG["PostgreSQL"]
        Redis["Redis"]
    end
```

Then find the arrow block:

```
    ES --> EB
    EB --> Kafka
    Kafka --> S
    S -->|trigger| DW
    S -->|trigger| RW
    S -->|trigger| RTW
    S -->|trigger| N

    DR & DW & RR & RW & RTR & RTW & N --> PG
    PP --> Redis
```

Change it to add dlqueue trigger and database arrows:

```
    ES --> EB
    EB --> Kafka
    Kafka --> S
    S -->|trigger| DW
    S -->|trigger| RW
    S -->|trigger| RTW
    S -->|trigger| N
    S -->|"dlqTrigger<br/>(on failure)"| DLQW

    DR & DW & RR & RW & RTR & RTW & N & DLQR & DLQW --> PG
    PP --> Redis
```

Then find the Alloy/Pyroscope scrape line:

```
    PP & DR & DW & RR & RW & RTR & RTW & N -.->|push profiles| Pyro
```

Change to include dlqueue:

```
    PP & DR & DW & RR & RW & RTR & RTW & N & DLQR & DLQW -.->|push profiles| Pyro
```

Then find the style block at the bottom of the diagram. Add two style lines for DLQR and DLQW before the `style Grafana` line:

```
    style DLQR fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
    style DLQW fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
```

The final diagram style block should look like:

```
    style Browser fill:#6366f1,color:#fff,stroke:#818cf8
    style EG fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style GWSvc fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style GW fill:#1a1d27,color:#e4e4e7,stroke:#2a2d3a
    style Kafka fill:#f59e0b,color:#000,stroke:#d97706
    style EB fill:#f59e0b,color:#000,stroke:#d97706
    style ES fill:#22c55e,color:#fff,stroke:#16a34a
    style S fill:#22c55e,color:#fff,stroke:#16a34a
    style ArgoCtrl fill:#22c55e,color:#fff,stroke:#16a34a
    style DLQR fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
    style DLQW fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
    style Grafana fill:#e879f9,color:#000,stroke:#c026d3
    style PG fill:#3b82f6,color:#fff,stroke:#2563eb
    style Redis fill:#ef4444,color:#fff,stroke:#dc2626
```

Also update the namespaces table just below the diagram (around line 419). The current `bookinfo` row says:

```
| `bookinfo` | 8 app deployments (CQRS split), PostgreSQL, 3 EventSources, 3 Sensors, method-based HTTPRoutes |
```

Change to:

```
| `bookinfo` | 10 app deployments (CQRS split incl. dlqueue read/write), PostgreSQL, 4 EventSources (incl. `dlq-event-received`), 4 Sensors (incl. DLQ), method-based HTTPRoutes |
```

- [ ] **Step 5: Render-check the diagrams**

Open `README.md` in a Markdown previewer that supports mermaid (GitHub, VS Code with Mermaid Preview, or https://mermaid.live). Each of the 4 diagrams should render without syntax errors.

Quick sanity check via grep:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
grep -c "^\`\`\`mermaid" README.md
```

Expected: `4`

```bash
grep -c "dlqTrigger\|DLQ\|dlqueue" README.md
```

Expected: at least `15` (multiple mentions across diagrams + tables + prose).

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "docs(readme): update mermaid diagrams to show DLQ pipeline"
```

---

## Task 3: README.md — Argo Events DLQ subsection

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Locate insertion point**

Find the Argo Events section (around line 454). The section ends with a bash block showing `kubectl apply --dry-run=client -f deploy/argo-events/...` and is followed by `---` and then `## E2E Tests`. Insert the new subsection BEFORE the trailing `---`.

- [ ] **Step 2: Insert Dead Letter Queue subsection**

Between the closing ``` of the last bash block in the Argo Events section and the `---` separator, add:

```markdown
### Dead Letter Queue

Every primary sensor trigger carries `atLeastOnce: true` + exponential backoff and a `dlqTrigger` that fires after retry exhaustion. The dlqTrigger captures the full CloudEvents context (`id`, `type`, `source`, `subject`, `time`, `datacontenttype`) via `contextKey`, plus the original body and HTTP headers (preserving `traceparent` for distributed trace correlation), and POSTs the structured payload to a dedicated `dlq-event-received` EventSource. The DLQ event then flows through the standard Argo Events pipeline: EventSource → Kafka → DLQ sensor → `dlqueue-write` service → PostgreSQL.

The dlqueue service deduplicates arrivals by a natural composite key (`sensor_name + failed_trigger + SHA-256(original_payload)`). The CloudEvents `id` cannot be used as a dedup key because Argo Events regenerates it on every EventSource pass — per the CNCF CloudEvents spec, `id` is a hop-level identifier, not an end-to-end correlation key. Events are tracked through a state machine (`pending → replayed → resolved` on success; `poisoned` after `max_retries` failed replays).

Replay is operator- or service-initiated via `POST /v1/events/{id}/replay`: dlqueue re-POSTs the original payload and headers to the source EventSource URL stored on the DLQ record, re-entering the full CQRS pipeline. All write services are idempotent (see `pkg/idempotency`) so replays are safe. For the full domain model, API surface, and metric definitions, see [docs/superpowers/specs/2026-04-13-dlqueue-service-design.md](docs/superpowers/specs/2026-04-13-dlqueue-service-design.md).
```

- [ ] **Step 3: Verify insertion**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
grep -A2 "^### Dead Letter Queue" README.md | head -5
```

Expected: shows the new heading followed by the first paragraph's first sentence.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs(readme): add Dead Letter Queue subsection under Argo Events"
```

---

## Task 4: CLAUDE.md — all updates

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update Project Overview sentence**

Find the Project Overview paragraph (line 5):

```
Go hexagonal architecture monorepo adapting Istio's Bookinfo as a **book review system with event-driven architecture**. Services are plain REST APIs — all event-driven complexity (Kafka consumers, retries, DLQ) is abstracted by Argo Events EventSources and Sensors. Full observability: structured logging, distributed tracing, metrics, continuous profiling.
```

Replace with:

```
Go hexagonal architecture monorepo adapting Istio's Bookinfo as a **book review system with event-driven architecture**. Services are plain REST APIs — all event-driven complexity (Kafka consumers, retries) is abstracted by Argo Events EventSources and Sensors. Failed events that exhaust sensor retries are captured by the `dlqueue` service for inspection and replay. Full observability: structured logging, distributed tracing, metrics, continuous profiling.
```

- [ ] **Step 2: Add dlqueue row to Services table**

Find the Services table (line 11-17). After the `notification` row:

```
| **notification** | Backend (hex arch) | :8080 / :9090 admin | Event consumer audit log |
```

Add:

```
| **dlqueue** | Backend (hex arch) | :8080 / :9090 admin | Dead letter queue for failed sensor deliveries; REST API for inspection, replay, and resolution |
```

- [ ] **Step 3: Add DLQ and Idempotency architecture bullets**

Find the Architecture bullet list (line 19-28). After the last bullet (the `**Local k8s**` one):

```
- **Local k8s** (`make run-k8s`): k3d cluster with Envoy Gateway API, Strimzi Kafka (KRaft), full observability stack (Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy)
```

Add two new bullets:

```
- **DLQ**: sensor `dlqTrigger` → `dlq-event-received` EventSource → `dlqueue-write` → PostgreSQL. Dedup by natural key (`sensor_name + failed_trigger + SHA-256(payload)`). State machine: `pending → replayed → resolved / poisoned`. REST API at `/v1/events` supports list/get/replay/resolve/reset plus batch operations.
- **Idempotency**: all write services (reviews, ratings, details, notification, dlqueue) dedupe on client-supplied `idempotency_key` or derived natural key (SHA-256 of business fields). Prerequisite for safe DLQ replay — CloudEvents `id` cannot be used because Argo Events regenerates it per EventSource pass.
```

- [ ] **Step 4: Update Build Commands counts**

Find the Build Commands block (line 32-38):

```
make build-all          # Build all 5 service binaries to bin/
make test               # go test -race -count=1 ./...
make lint               # golangci-lint run
make e2e                # Docker Compose + shell smoke tests (memory backend)
make docker-build-all   # Build all 5 Docker images
```

Change `5` to `6` on the two relevant lines:

```
make build-all          # Build all 6 service binaries to bin/
make test               # go test -race -count=1 ./...
make lint               # golangci-lint run
make e2e                # Docker Compose + shell smoke tests (memory backend)
make docker-build-all   # Build all 6 Docker images
```

- [ ] **Step 5: Add dlqueue to Run Locally section**

Find the Run Locally section (line 72-100). After the `notification` block:

```
**notification** (no dependencies):
\`\`\`bash
SERVICE_NAME=notification HTTP_PORT=8084 ADMIN_PORT=9094 go run ./services/notification/cmd/
\`\`\`
```

Insert before the `**productpage**` block:

```
**dlqueue** (no dependencies):
\`\`\`bash
SERVICE_NAME=dlqueue HTTP_PORT=8085 ADMIN_PORT=9095 go run ./services/dlqueue/cmd/
\`\`\`

```

Note: the bash fences in the plan file are escaped; use real backticks in the actual edit.

- [ ] **Step 6: Add pkg/idempotency row to Shared Packages table**

Find the Shared Packages table (line 106-114). After the `pkg/health` row:

```
| `pkg/health` | `/healthz` (liveness) and `/readyz` (readiness with optional check functions) |
```

Insert:

```
| `pkg/idempotency` | `Store` interface (`CheckAndRecord`) with memory + postgres adapters; `NaturalKey(fields...)` (SHA-256 with `0x1f` separator); `Resolve(explicitKey, fields...)` picks explicit when present, natural key otherwise |
```

- [ ] **Step 7: Verify**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
grep -c "dlqueue\|DLQ\|idempotency" CLAUDE.md
```

Expected: at least `10`.

```bash
grep -n "5 service binaries\|5 Docker images" CLAUDE.md
```

Expected: empty output.

- [ ] **Step 8: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): add dlqueue, DLQ architecture, and idempotency to project instructions"
```

---

## Task 5: Push and update PR #22

**Files:** none (git operations only)

- [ ] **Step 1: Verify all commits are on the branch**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
git log --oneline origin/feat/dlqueue-service..HEAD
```

Expected: 4 new commits on top of what origin has:
- `docs(readme): add dlqueue to tables, project structure, and prose`
- `docs(readme): update mermaid diagrams to show DLQ pipeline`
- `docs(readme): add Dead Letter Queue subsection under Argo Events`
- `docs(claude): add dlqueue, DLQ architecture, and idempotency to project instructions`

- [ ] **Step 2: Push**

```bash
git push origin feat/dlqueue-service
```

Expected: fast-forward push succeeds. PR #22 updates automatically with the new commits. GitHub Actions re-run CI.

- [ ] **Step 3: Wait for CI to pass**

```bash
gh pr checks 22 --repo kaio6fellipe/event-driven-bookinfo --watch --fail-fast --interval 20
```

Expected: all 9 checks pass (Lint, Secret Scan, Test, Vet, Vulnerability Scan, Build, Docker Build + Scan, E2E Tests, E2E Tests (PostgreSQL)). Docs-only changes should not break any of these.

- [ ] **Step 4: Done**

No commit in this step. Confirm PR is ready for merge.

---

## Self-Review

### Spec coverage

Walking through each spec section and mapping it to a task:

| Spec section | Task(s) |
|---|---|
| Intro paragraph update | Task 1, Step 1 |
| Top CQRS diagram (DLQ subgraph) | Task 2, Step 1 |
| Service Topology (DLQ node) | Task 2, Step 2 |
| Event-Driven Write Flow (full DLQ pipeline) | Task 2, Step 3 |
| Cluster Architecture (DLQ deployments + arrows + namespaces table) | Task 2, Step 4 |
| Services table — dlqueue row | Task 1, Step 2 |
| Shared Packages table — pkg/idempotency row | Task 1, Step 3 |
| Quick Start — dlqueue run command | Task 1, Step 4 |
| Makefile table — "6 services" counts | Task 1, Step 5 |
| Docker image list | Task 1, Step 6 |
| CQRS Deployment Split table | Task 1, Step 7 |
| Project Structure tree — services + pkg | Task 1, Step 8 |
| Releasing section — "6 services" | Task 1, Step 9 |
| Argo Events DLQ subsection | Task 3 |
| CLAUDE.md Project Overview | Task 4, Step 1 |
| CLAUDE.md Services table | Task 4, Step 2 |
| CLAUDE.md Architecture bullets | Task 4, Step 3 |
| CLAUDE.md Build Commands | Task 4, Step 4 |
| CLAUDE.md Run Locally | Task 4, Step 5 |
| CLAUDE.md Shared Packages table | Task 4, Step 6 |
| PR update + CI | Task 5 |

All spec items covered. No gaps.

### Placeholder scan

No "TBD", "TODO", "fill in later", or similar. Every step contains the exact text or diff to apply.

### Type consistency

- `dlqueue` is used consistently (not "DLQ service" in one place and "dlqueue-service" in another)
- `dlq-event-received` is the consistent EventSource name
- `dlq-event-received-sensor` is the consistent Sensor name
- `dlqueue-write` is the consistent write-deployment name
- `pkg/idempotency` path is consistent everywhere
- Port pairs: README uses `8085 / 9095` (host-facing), CLAUDE.md uses `:8080 / :9090 admin` (in-container) — documented as intentional in the spec

### Notes for the implementing engineer

- **Backtick escaping in step content**: where the plan shows escaped backticks (` \`\`\` `) around bash blocks inside prose inserts, use real triple-backticks in the actual Markdown. This is a plan-rendering artifact only.
- **Mermaid render check**: if any diagram fails to render after editing, copy the diagram source to https://mermaid.live to isolate the syntax error. The most common cause is a missing `style` line for a newly-added node, or a label containing `|` or `()` without quoting.
- **Line numbers are approximate**: the spec references line numbers based on the current README/CLAUDE state. If the file has been edited between tasks, adjust — anchor on unique strings (e.g., the specific row contents) rather than line numbers.
- **CI re-run is expected**: all 9 CI checks were green on the last code push; docs-only changes should keep them green. If a check fails, it is almost certainly a flake or infrastructure issue, not a docs problem — re-run with `gh workflow run` or push an empty commit.
