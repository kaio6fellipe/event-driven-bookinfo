# Compose Lite-Mode Documentation — Design

**Date:** 2026-04-25
**Status:** Design — pending implementation

## Problem

The recent event-driven notifications work introduced producer-side Kafka publishing in details/reviews/ratings/ingestion services and notification consumption via Argo Events sensors. The `make run-k8s` (k3d) path covers the full chain. The `make run` (docker-compose) path does not — `docker-compose.yml` has no Kafka broker, no ingestion service, no Argo Events components.

In compose, services start with `KAFKA_BROKERS=""`; producers fall back to `NoopPublisher`. Notifications are exercised via direct HTTP POST in e2e tests. None of the recently-added event-chain code is meaningfully exercised under compose.

Today this is undocumented. A developer running `make run` or reading `make e2e` could reasonably assume compose covers the full event-driven flow. It does not.

## Goal

Document compose as a "lite" development environment with explicit scope boundaries, so:

1. Developers understand which path exercises which code.
2. The choice of compose vs `make run-k8s` matches the work being tested (HTTP CRUD vs full event chain).
3. The decision to NOT extend compose with Kafka + a sensor bridge is captured for future reference.

This is a docs-only change. No new compose file overlays, no new make targets, no code changes.

## Approach

Three coordinated edits to existing files. Same wording reused across each so the message is consistent:

- `docker-compose.yml` — top-of-file header comment, ~6 lines
- `README.md` — extend the existing `make run` and `make e2e` sections, ~12 lines
- `CLAUDE.md` — add a short paragraph under build/run instructions explaining the compose-vs-k8s boundary, ~5 lines

## Decisions

| # | Decision | Rationale |
| --- | --- | --- |
| 1 | Document, do not extend compose | k3s-in-compose duplicates `make run-k8s` with worse ergonomics; argo-events is too k8s-coupled for standalone running; a Redpanda-Connect-style bridge introduces dev/prod drift. The simplest correct choice is to scope compose narrowly. |
| 2 | Same wording across all three files | Consistency. A reader hitting any one of the three docs gets the same accurate picture. |
| 3 | Mention NoopPublisher behavior explicitly | Producers don't error on missing `KAFKA_BROKERS`; they silently no-op. This is non-obvious and a likely source of "where are my events?" confusion under compose. |
| 4 | Reference `make run-k8s` as the full-chain path | Points the reader at the existing solution rather than implying a gap to be filled. |

## Components

### `docker-compose.yml`

Current header (lines 1-3):

```yaml
# Local development compose file.
# Usage: make run | make run-logs | make stop | make seed
#   or:  docker compose up --build
```

Replace with:

```yaml
# Local development compose file — lite mode.
#
# Scope: postgres + redis + 5 backend services + productpage.
# NOT included: Kafka, ingestion service, Argo Events, observability.
# Producers detect missing KAFKA_BROKERS and fall back to a no-op
# publisher; events are dropped silently.
#
# The full event-driven path (ingestion → Kafka → Argo Events sensor
# → notification HTTP trigger) requires `make run-k8s` (k3d cluster).
#
# Usage: make run | make run-logs | make stop | make seed
#   or:  docker compose up --build
```

### `README.md`

Find the existing `make run` section (around the "Or use Docker Compose" paragraph). Add a callout block immediately after the existing fenced code listing:

```markdown
> **Lite mode.** Docker Compose runs the synchronous service mesh
> only: postgres + redis + 5 backend services + productpage. Kafka,
> the ingestion service, Argo Events, and the observability stack
> are NOT included. Producers detect missing `KAFKA_BROKERS` and
> fall back to a no-op publisher; events are dropped silently.
>
> The full event-driven path (ingestion polling Open Library →
> Kafka → Argo Events EventSource → Sensor → notification HTTP
> trigger) is exercised only via `make run-k8s` (k3d-based local
> cluster).
```

Find the `make e2e` section (or the row in the make-target table). Append:

```markdown
> `make e2e` covers HTTP-level acceptance tests (idempotency,
> validation, CRUD round-trips) under the lite-mode compose stack.
> The event chain is verified end-to-end via trace inspection in
> Tempo after `make run-k8s`.
```

### `CLAUDE.md`

Find the existing `## Build Commands` or `## Local Kubernetes` section. Add a new paragraph between them (or in an appropriate adjacent location):

```markdown
**Compose vs k8s scope:** `make run` (docker-compose) is a lite
development environment — postgres + redis + 5 backend services +
productpage. Compose does NOT include Kafka, the ingestion service,
Argo Events, or the observability stack. Producers fall back to a
no-op publisher when `KAFKA_BROKERS` is unset; events are dropped.
The full event-driven flow (ingestion + Kafka + Argo Events sensors
driving notifications) and observability stack are exercised only
via `make run-k8s`. Use `make e2e` for HTTP-level acceptance tests;
the event chain is verified via Tempo traces after `make run-k8s`.
```

## Files modified

```text
docker-compose.yml        # ~6 lines change — header comment block
README.md                 # ~12 lines added — under existing run/e2e sections
CLAUDE.md                 # ~5 lines added — under build/run instructions
```

Total diff: ~25 lines. No code change.

## Out of scope

- New compose overlay (`docker-compose.full.yml`) with Kafka + bridge container
- New make target
- New e2e test scope
- Changes to argo-events, helm charts, or service code
- Adding ingestion service to compose
- Refactoring existing compose layout

## Acceptance criteria

1. `docker-compose.yml` opens with the lite-mode header comment.
2. `README.md` `make run` and `make e2e` sections each carry the lite-mode disclaimer.
3. `CLAUDE.md` has the compose-vs-k8s scope paragraph adjacent to existing build/run instructions.
4. Wording is consistent across all three (one canonical message, repeated where appropriate, not three drift-prone variants).
5. No code, helm, or compose-service changes.
