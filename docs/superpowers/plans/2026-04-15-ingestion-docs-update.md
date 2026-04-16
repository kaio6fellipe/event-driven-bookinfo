# Ingestion Documentation Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Document the new `ingestion` service in `README.md` and `CLAUDE.md` so human readers and AI agents see the complete 7-service topology, updated diagrams, and ingestion-specific configuration.

**Architecture:** Edit only two top-level doc files. All changes follow the pattern established by the dlqueue docs-update (`docs/superpowers/specs/2026-04-13-dlqueue-docs-update-design.md`). Ship atomically on branch `feat/ingestion-service` in a single commit. Historical specs and plans stay untouched.

**Tech Stack:** Markdown, Mermaid diagrams (rendered by GitHub), GitHub-flavored tables. No code. No tests. Verification is `git diff` inspection plus Markdown/Mermaid preview.

**Spec reference:** `docs/superpowers/specs/2026-04-15-ingestion-docs-update-design.md`

---

## Task 1: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

All edits are small, independent, and stay inside CLAUDE.md. Do them in reading order so surrounding context is always visible.

- [ ] **Step 1: Append ingestion mention to Project Overview paragraph**

In `CLAUDE.md`, locate the paragraph ending at line 5:

```
...Failed events that exhaust sensor retries are captured by the `dlqueue` service for inspection and replay. Full observability: structured logging, distributed tracing, metrics, continuous profiling.
```

Replace with:

```
...Failed events that exhaust sensor retries are captured by the `dlqueue` service for inspection and replay. A standalone `ingestion` service polls the Open Library API and publishes `book-added` events through the Gateway, demonstrating the system as a self-hosted event broker. Full observability: structured logging, distributed tracing, metrics, continuous profiling.
```

- [ ] **Step 2: Add ingestion row to the Services table**

In the Services table, immediately after the `dlqueue` row (around line 18), insert:

```
| **ingestion** | Producer (hex arch) | :8080 / :9090 admin | Open Library scraper; polls on interval, publishes book-added events to the Gateway |
```

- [ ] **Step 3: Insert new Ingestion architecture bullet**

Immediately after the `Idempotency` bullet (line 31), insert a new bullet:

```
- **Ingestion**: `ingestion` service polls Open Library on `POLL_INTERVAL` → for each query in `SEARCH_QUERIES` → `POST {GATEWAY_URL}/v1/details` with `idempotency_key=ingestion-isbn-<ISBN>`. Stateless, single deployment, no CQRS split, no EventSource/Sensor of its own. Exercises the full write pipeline end-to-end.
```

- [ ] **Step 4: Update Build Commands counts**

In the Build Commands code block (lines 35-41):

Replace:
```
make build-all          # Build all 6 service binaries to bin/
```
with:
```
make build-all          # Build all 7 service binaries to bin/
```

Replace:
```
make docker-build-all   # Build all 6 Docker images
```
with:
```
make docker-build-all   # Build all 7 Docker images
```

- [ ] **Step 5: Update Deploy Structure "all 6 services" reference**

In the Deploy Structure code block (line 73):

Replace:
```
  bookinfo-service/          # Reusable Helm chart for all 6 services
```
with:
```
  bookinfo-service/          # Reusable Helm chart for all 7 services
```

- [ ] **Step 6: Insert ingestion block in Run Locally**

Locate the `**dlqueue**` block (lines 109-112) and the `**productpage**` block that follows it. Between them, insert:

````
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

- [ ] **Step 7: Append ingestion env vars to the optional-vars paragraph**

Locate the sentence at line 122:

```
Optional env vars (all services): `LOG_LEVEL` (debug/info/warn/error, default info), `OTEL_EXPORTER_OTLP_ENDPOINT`, `PYROSCOPE_SERVER_ADDRESS`, `STORAGE_BACKEND` (memory/postgres), `DATABASE_URL`. Productpage-specific: `REDIS_URL` (enables pending review cache; disabled when unset).
```

Replace with:

```
Optional env vars (all services): `LOG_LEVEL` (debug/info/warn/error, default info), `OTEL_EXPORTER_OTLP_ENDPOINT`, `PYROSCOPE_SERVER_ADDRESS`, `STORAGE_BACKEND` (memory/postgres), `DATABASE_URL`. Productpage-specific: `REDIS_URL` (enables pending review cache; disabled when unset). Ingestion-specific: `GATEWAY_URL`, `POLL_INTERVAL`, `SEARCH_QUERIES`, `MAX_RESULTS_PER_QUERY`.
```

- [ ] **Step 8: Verify CLAUDE.md diff**

Run:
```bash
git diff CLAUDE.md
```

Expected: 7 change hunks covering overview paragraph, services table, architecture bullet, build commands, deploy structure, run locally, optional env vars. No lines removed from sections other than the replaced text.

Do not commit yet — all changes ship in one commit at the end.

---

## Task 2: README.md — intro, services table, and top-of-file edits

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Append ingestion sentence to intro paragraph**

In `README.md`, locate the paragraph ending at line 66:

```
...This demonstrates using Argo Events not only for workflow automation, but as a real-time event-driven architecture platform — a self-hosted alternative to Google Eventarc or AWS EventBridge.
```

Replace with:

```
...This demonstrates using Argo Events not only for workflow automation, but as a real-time event-driven architecture platform — a self-hosted alternative to Google Eventarc or AWS EventBridge. A standalone `ingestion` service demonstrates the pipeline as a self-hosted event broker — it polls the Open Library API and publishes synthetic book-added events through the same Gateway → EventSource → Kafka → Sensor path used by the UI.
```

- [ ] **Step 2: Append ingestion row to Services table**

Locate the `dlqueue` row in the Services table (line 186). Immediately after it, insert this single-line row (no line breaks inside the row):

```
| **ingestion** | [![release](https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=ingestion-v*&sort=semver&display_name=tag&label=release)](https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=ingestion-v&expanded=true) | [![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/badges/coverage-ingestion.json)](https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain) | Producer (hex arch) | 8086 | 9096 | Polls Open Library for books on a configurable interval and publishes `book-added` events to the Gateway. Stateless; no storage adapters. |
```

- [ ] **Step 3: Insert ingestion block in Quick Start**

Locate the closing line of the `productpage` run block:

```bash
  ./bin/productpage

# Open in browser
open http://localhost:8080
```

Replace with:

````bash
  ./bin/productpage

# Optional standalone demo — polls Open Library and publishes to the Gateway
SERVICE_NAME=ingestion HTTP_PORT=8086 ADMIN_PORT=9096 \
  GATEWAY_URL=http://localhost:8080 \
  POLL_INTERVAL=5m \
  SEARCH_QUERIES=programming,golang \
  MAX_RESULTS_PER_QUERY=10 \
  ./bin/ingestion

# Open in browser
open http://localhost:8080
````

- [ ] **Step 4: Update Makefile targets table descriptions**

In the Makefile Targets table, replace:

```
| `make build-all` | Build all 6 service binaries |
```
with:
```
| `make build-all` | Build all 7 service binaries |
```

Replace:

```
| `make docker-build-all` | Build Docker images for all 6 services |
```
with:
```
| `make docker-build-all` | Build Docker images for all 7 services |
```

- [ ] **Step 5: Append ingestion to the Docker image list**

Locate the GHCR image list block ending with:

```
ghcr.io/kaio6fellipe/event-driven-bookinfo/dlqueue:<tag>
```

Insert one line immediately after:

```
ghcr.io/kaio6fellipe/event-driven-bookinfo/ingestion:<tag>
```

- [ ] **Step 6: Verify intermediate diff**

Run:
```bash
git diff README.md
```

Expected: 5 change hunks (intro, services table row, Quick Start addition, two Makefile-row replacements + a blank-line-free image-list append). Do not commit yet.

---

## Task 3: README.md — Event-Driven Write Flow diagram

**Files:**
- Modify: `README.md` (Event-Driven Write Flow mermaid block, lines 98-161)

- [ ] **Step 1: Add ingestion node declaration**

Inside the ```mermaid``` code block for "Event-Driven Write Flow", locate the existing declarations near the top:

```
    Browser["Browser POST /partials/rating"]
    PP["productpage (BFF)"]
    GW["Gateway (CQRS routing)"]
    WH["External POST /v1/*"]
```

Immediately after the `WH` line, add:

```
    ING["ingestion<br/>Open Library poller"]
```

- [ ] **Step 2: Add ingestion arrow to Gateway**

In the same mermaid block, locate the line:

```
    WH --> GW
```

Add a new line immediately after it:

```
    ING -->|"POST /v1/details"| GW
```

- [ ] **Step 3: Add ingestion style**

In the same mermaid block, locate the `style` declarations (they follow the arrow definitions, at the bottom of the block). The last existing style in the group is:

```
    style DLQW fill:#1a1d27,color:#e4e4e7,stroke:#a855f7
```

Immediately after that line, add:

```
    style ING fill:#1a1d27,color:#e4e4e7,stroke:#10b981
```

- [ ] **Step 4: Verify diagram renders**

In VS Code (or any Markdown/Mermaid preview), open `README.md` and scroll to the "Event-Driven Write Flow" diagram. Expected: the new `ING` node appears with a dark grey fill and a green border, connected to `GW` by an arrow labeled "POST /v1/details". All existing nodes, edges, and styles remain unchanged.

Run `git diff README.md` and confirm the Event-Driven Write Flow block shows exactly 3 new lines added — no removals, no edits to existing lines.

Do not commit yet.

---

## Task 4: README.md — Cluster Architecture diagram

**Files:**
- Modify: `README.md` (Cluster Architecture mermaid block, lines 368-456)

- [ ] **Step 1: Declare ingestion node inside the bookinfo subgraph**

Inside the ```mermaid``` code block for "Cluster Architecture", locate the `subgraph bookinfo` block. Near the existing `DLQW["dlqueue-write"]` line, keep scanning to the node declarations. Immediately after:

```
        DLQR["dlqueue"]
        DLQW["dlqueue-write"]
```

add:

```
        ING["ingestion"]
```

- [ ] **Step 2: Add outbound arrow to Gateway**

In the same mermaid block, locate the existing arrow section (after the subgraphs):

```
    GW -->|"POST /v1/*"| ES
```

Immediately after that line, add:

```
    ING --> GW
```

- [ ] **Step 3: Add ingestion to observability fan-out arrows**

In the same mermaid block, locate the three observability fan-out lines. Two arrows list bookinfo workloads.

Replace:

```
    DR & DW & RR & RW & RTR & RTW & N & DLQR & DLQW --> PG
```
with:
```
    DR & DW & RR & RW & RTR & RTW & N & DLQR & DLQW --> PG
    ING -.->|"outbound HTTPS"| GW
```

(The `ING` line replaces an awkward `ING --> PG` since ingestion has no DB — it stays connected only to the Gateway, matching its stateless design. The existing `DR & DW & ... DLQW --> PG` line remains untouched above it.)

Now locate:

```
    PP & DR & DW & RR & RW & RTR & RTW & N & DLQR & DLQW -.->|push profiles| Pyro
```
Replace with:
```
    PP & DR & DW & RR & RW & RTR & RTW & N & DLQR & DLQW & ING -.->|push profiles| Pyro
```

- [ ] **Step 4: Add ingestion style**

In the same mermaid block, locate the `style` block at the bottom. The last existing style in that group is:

```
    style Redis fill:#ef4444,color:#fff,stroke:#dc2626
```

Immediately after that line, add:

```
    style ING fill:#1a1d27,color:#e4e4e7,stroke:#10b981
```

- [ ] **Step 5: Verify diagram renders**

Open `README.md` in a Markdown/Mermaid preview and scroll to "Cluster Architecture". Expected: the new `ING` node appears inside the `bookinfo` subgraph with a green border. Arrows: `ING --> GW` (solid, outbound publish) and `ING -.-> Pyro` (dotted, via the combined push-profiles fan-out line). Existing arrows and styles unchanged.

Run `git diff README.md` and confirm the Cluster Architecture block shows exactly 4 additions (node, outbound arrow, ING appended to Pyro fan-out, style), no removals.

Do not commit yet.

---

## Task 5: README.md — CQRS note, observability metrics row, project tree, releasing

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add note below CQRS Deployment Split table**

Locate the end of the CQRS Deployment Split table. The closing row is:

```
| `dlqueue` / `dlqueue-write` | Read / Write | operator/service API (GET) / `dlq-event-received` sensor (POST) |
```

Immediately after the table (before the next `###` heading), insert a blank line and a note paragraph:

```

> `ingestion` deploys as a single stateless deployment with no CQRS split — it is a pure event producer and is omitted from the table above.
```

- [ ] **Step 2: Append ingestion row to business metrics table**

In the Observability section, locate the "Business metrics (per service)" table:

```
  | Service | Metric |
  |---|---|
  | ratings | `ratings_submitted_total` |
  | details | `books_added_total` |
  | reviews | `reviews_submitted_total` |
  | notification | `notifications_dispatched_total`, `notifications_failed_total`, `notifications_by_status` |
```

Append one row:

```
  | ingestion | `ingestion_scrapes_total`, `ingestion_books_published_total`, `ingestion_errors_total` |
```

- [ ] **Step 3: Append ingestion block to Project Structure tree**

In the Project Structure tree, locate the `dlqueue/` block:

```
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

Replace the leading `└──` of `dlqueue/` with `├──` (to allow another entry below), and append the ingestion entry:

```
│   ├── dlqueue/                # Dead letter queue (hex arch) — NEW
│   │   ├── cmd/main.go
│   │   ├── migrations/         # dlq_events + processed_events
│   │   └── internal/
│   │       ├── core/           # domain/, port/, service/
│   │       ├── adapter/
│   │       │   ├── inbound/http/
│   │       │   └── outbound/   # memory/, postgres/, http/ (replay client)
│   │       └── metrics/        # dlq_events_* counters
│   └── ingestion/              # Producer (hex arch) — Open Library scraper, publishes to Gateway
│       ├── cmd/main.go
│       └── internal/
│           ├── core/           # domain/, port/, service/
│           └── adapter/
│               ├── inbound/http/
│               └── outbound/   # openlibrary/ (BookFetcher), gateway/ (EventPublisher)
```

- [ ] **Step 4: Update "all 6 services" in Releasing section**

In the Releasing > How It Works numbered list, locate:

```
2. **Detects changed services** — file paths under `services/<name>/`; changes to `pkg/`, `go.mod`, or `go.sum` trigger all 6 services
```

Replace with:

```
2. **Detects changed services** — file paths under `services/<name>/`; changes to `pkg/`, `go.mod`, or `go.sum` trigger all 7 services
```

- [ ] **Step 5: Verify intermediate diff**

Run:
```bash
git diff README.md
```

Expected: the prior task's hunks plus 4 new hunks (CQRS note, metrics row, project tree expansion, releasing count). No removals elsewhere. Do not commit yet.

---

## Task 6: README.md — new "Data Ingestion" subsection

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Insert new subsection after Dead Letter Queue**

Locate the end of the "Dead Letter Queue" subsection. The closing line is:

```
Replay is operator- or service-initiated via `POST /v1/events/{id}/replay`: dlqueue re-POSTs the original payload and headers to the source EventSource URL stored on the DLQ record, re-entering the full CQRS pipeline. All write services are idempotent (see `pkg/idempotency`) so replays are safe. For the full domain model, API surface, and metric definitions, see [docs/superpowers/specs/2026-04-13-dlqueue-service-design.md](docs/superpowers/specs/2026-04-13-dlqueue-service-design.md).
```

This is followed by a horizontal-rule line (`---`) and then the `## E2E Tests` heading.

Immediately after the horizontal-rule line (and before `## E2E Tests`), insert:

````markdown
## Data Ingestion

The `ingestion` service is a demo-scale synthetic-data generator that exercises the full event-driven write pipeline end-to-end (Gateway → EventSource → Kafka → Sensor → `details-write`). It runs as a single stateless deployment with no storage adapters, no CQRS split, and no EventSource/Sensor of its own — it is purely outbound.

A background poll loop ticks every `POLL_INTERVAL` (default 5m). For each query in `SEARCH_QUERIES`, the service calls `GET https://openlibrary.org/search.json`, validates each returned book (title, ISBN, authors, and publish year are required), and publishes accepted books via `POST {GATEWAY_URL}/v1/details`. Each publish carries a deterministic idempotency key `ingestion-isbn-<ISBN>` so the downstream `details-write` service deduplicates across cycles via `pkg/idempotency`. On-demand scrapes are also available via `POST /v1/ingestion/trigger` (optional body `{"queries": ["golang", "rust"]}`).

Configuration via `GATEWAY_URL`, `POLL_INTERVAL`, `SEARCH_QUERIES`, and `MAX_RESULTS_PER_QUERY` environment variables. Because idempotency is enforced at the write service, replays and overlapping cycles are safe. For the full design, port contracts, and metric definitions, see [docs/superpowers/specs/2026-04-14-ingestion-service-design.md](docs/superpowers/specs/2026-04-14-ingestion-service-design.md).

---

````

Note: the trailing `---` in the insert separates the new section from the next one, matching the section-separator style used elsewhere in README.md.

- [ ] **Step 2: Verify the new subsection renders**

Open `README.md` in a Markdown preview. Expected: a new `## Data Ingestion` heading appears between the DLQ "Replay" paragraph and the `## E2E Tests` heading. Three paragraphs. Both embedded paths (`docs/superpowers/specs/2026-04-14-ingestion-service-design.md`) render as clickable links.

Run `git diff README.md` and confirm the new hunk is purely additive (no existing content removed).

Do not commit yet.

---

## Task 7: Final verification and single commit

**Files:**
- Modify: `README.md` (review only)
- Modify: `CLAUDE.md` (review only)
- Commit: both files plus the two spec/plan docs

- [ ] **Step 1: Check for missed service-count references**

Run:
```bash
grep -n "6 services\|6 service \|all 6 " README.md CLAUDE.md
```

Expected: no matches. Any remaining hits must be fixed before committing.

- [ ] **Step 2: Confirm ingestion appears in every place it should**

Run:
```bash
grep -c "ingestion" README.md
grep -c "ingestion" CLAUDE.md
```

Expected: README.md returns ≥ 9, CLAUDE.md returns ≥ 6. Lower counts mean an expected insertion was missed — re-run prior tasks.

- [ ] **Step 3: Full diff inspection**

Run:
```bash
git diff README.md CLAUDE.md
```

Walk the entire diff. Verify:
- Only additions and one sentence-level replacement per section described in Task 1-6 appear
- No stray trailing whitespace or CRLF changes
- No accidental edits to lines outside the described hunks
- Mermaid code blocks remain syntactically valid (fences balanced, styles referencing the correct node IDs)

- [ ] **Step 4: Render both diagrams**

Open `README.md` in VS Code with a Mermaid preview extension, or paste each mermaid block into https://mermaid.live. Verify the two updated diagrams (Event-Driven Write Flow, Cluster Architecture) render without errors and the new `ING` node and arrows are visible.

- [ ] **Step 5: Stage and commit**

Stage the two modified doc files plus the brainstorming spec and plan:

```bash
git add README.md CLAUDE.md \
  docs/superpowers/specs/2026-04-15-ingestion-docs-update-design.md \
  docs/superpowers/plans/2026-04-15-ingestion-docs-update.md
```

Commit with the pre-approved message:

```bash
git commit -m "$(cat <<'EOF'
docs: update README and CLAUDE.md for ingestion service

Add ingestion service row to services tables, project structure, Quick
Start, Run Locally, business metrics, and Docker image list. Update
Event-Driven Write Flow and Cluster Architecture diagrams with the new
producer node. Add new "Data Ingestion" subsection after Dead Letter
Queue describing purpose, flow, and config. Bump "6 services" references
to "7". Note ingestion's omission from the CQRS Deployment Split table.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 6: Confirm commit landed cleanly**

Run:
```bash
git log -1 --stat
```

Expected: single new commit on `feat/ingestion-service`, four files changed: `README.md`, `CLAUDE.md`, the new spec under `docs/superpowers/specs/`, and the plan under `docs/superpowers/plans/`. No untracked or modified files remain (`git status` should be clean).

---

## Self-review summary

Spec coverage verified against every section of `docs/superpowers/specs/2026-04-15-ingestion-docs-update-design.md`:

| Spec section | Plan task |
|---|---|
| Intro sentence (README) | Task 2 Step 1 |
| Services table (README) | Task 2 Step 2 |
| Event-Driven Write Flow diagram | Task 3 |
| Cluster Architecture diagram | Task 4 |
| Quick Start | Task 2 Step 3 |
| Makefile + Docker image list | Task 2 Steps 4-5 |
| CQRS Deployment Split note | Task 5 Step 1 |
| Observability business metrics row | Task 5 Step 2 |
| Data Ingestion subsection | Task 6 |
| Project Structure tree | Task 5 Step 3 |
| Releasing "6 services" bump | Task 5 Step 4 |
| CLAUDE.md Project Overview | Task 1 Step 1 |
| CLAUDE.md Services table | Task 1 Step 2 |
| CLAUDE.md architecture bullet | Task 1 Step 3 |
| CLAUDE.md Build Commands counts | Task 1 Step 4 |
| CLAUDE.md Deploy Structure "6 services" | Task 1 Step 5 (bonus — caught during plan writing) |
| CLAUDE.md Run Locally block | Task 1 Step 6 |
| CLAUDE.md optional env-vars sentence | Task 1 Step 7 |
| Single commit on feat/ingestion-service | Task 7 |

No placeholders, no TODOs, every edit shows before/after text. File paths are absolute references inside the repo root. Step counts: Task 1 has 8 steps (7 edits + verify), Tasks 2-5 have 5-6 steps each, Task 6 has 2 steps, Task 7 has 6 steps. Total: ≈32 discrete steps, each 2-5 minutes.
