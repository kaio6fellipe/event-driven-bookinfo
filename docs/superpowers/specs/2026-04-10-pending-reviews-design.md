# Pending Reviews — Async UX Design Spec

## Problem

When a user submits a review, the CQRS write path is asynchronous: the POST hits an Argo Events EventSource which returns HTTP 200 immediately, then the event flows through Kafka → Sensor → write service. The productpage shows "Review submitted successfully!" and auto-refreshes the reviews list via HTMX, but the review hasn't been written to the read store yet. The result is that the review only appears after one or two manual page refreshes.

Even when the pipeline is fast (~100ms), we cannot assume it will always complete before the read refresh. The write service could be offline or degraded.

## Solution Overview

Server-side pending review cache backed by Redis. The productpage stores submitted reviews in Redis immediately after the async POST, then merges them into the reviews list on every read. Pending reviews are visually distinct (dashed border, pulsing dot, "Processing" label) and appear at the bottom of the reviews list. HTMX auto-polling removes the pending state once the review is confirmed in the read path.

## Architecture Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Pending state storage | Redis (server-side) | Survives page navigation, shared across users, production-ready for multi-replica productpage |
| Lifecycle | Confirmation on read (no TTL) | Pending reviews persist until the read path confirms them. No silent expiry. |
| Visibility scope | Per-product, all users | Any visitor sees pending reviews for a product. No session tracking needed. |
| List position | Bottom | Avoids visual reordering when the review transitions from pending to confirmed. |
| Auto-refresh | HTMX polling (every 2s) | Only active when pending reviews exist. Stops when all are confirmed. Natural fit with existing HTMX stack. |
| UI treatment | Dashed border + pulsing dot + "Processing" label | Clearly communicates async state without being alarming. |

## Infrastructure: Redis in Local k8s

- **Helm chart:** Bitnami Redis, standalone mode (no sentinel/cluster)
- **Namespace:** `bookinfo` (application-level cache, not shared platform infrastructure)
- **Persistence:** PVC-backed so pending reviews survive pod restarts
- **Auth:** Disabled for local dev
- **Deployed in:** `k8s-apps` Makefile target (same phase as PostgreSQL)
- **Service address:** `redis.bookinfo.svc.cluster.local:6379`

Deploy structure:
```
deploy/redis/local/
└── redis-values.yaml    # Bitnami Helm values (standalone, no auth, PVC)
```

## Redis Data Model

**Key pattern:** `pending:reviews:{product_id}` — Redis List

**Entry format (JSON):**
```json
{
  "reviewer": "Bob",
  "text": "Great book...",
  "stars": 4,
  "timestamp": 1712700000
}
```

Timestamp is Unix epoch at submission time, used for ordering within pending reviews.

**Operations:**
- `RPUSH` — append new pending review on submit
- `LRANGE 0 -1` — fetch all pending reviews for a product on read
- `LREM` — remove a pending review when it matches a confirmed review

**Matching/deduplication:** A pending review is considered confirmed when the read path returns a review matching on `(product_id, reviewer, text)`. On match, the pending entry is removed from Redis.

## Productpage Changes

### New Outbound Adapter: Redis Client

Lives in `services/productpage/internal/adapter/outbound/redis/` (productpage-specific, not in `pkg/`).

Two operations:
- `StorePending(ctx, productID, review)` — RPUSH to the pending list
- `GetAndReconcile(ctx, productID, confirmedReviews)` — LRANGE all pending, LREM any that match confirmed reviews, return remaining pending reviews

### Handler Flow Changes

**`POST /partials/rating` (submit):**
1. Parse form data (unchanged)
2. POST rating to gateway async endpoint (unchanged)
3. POST review to gateway async endpoint (unchanged)
4. **New:** Call `StorePending()` with the submitted review data
5. Return success partial with HTMX refresh trigger (unchanged)

**`GET /partials/reviews/{id}` (read):**
1. Fetch reviews from reviews read service (unchanged)
2. **New:** Call `GetAndReconcile()` with the product ID and confirmed reviews
3. **New:** Append remaining pending reviews to the bottom of the list with `Pending: true`
4. **New:** If pending reviews exist, set `hx-trigger="every 2s"` on the container div
5. Render template (updated to handle pending flag)

### Configuration

New env var: `REDIS_URL` (e.g., `redis://redis.bookinfo.svc.cluster.local:6379`)

When `REDIS_URL` is unset, the pending review feature is disabled — productpage behaves exactly as today. This keeps local `go run` development simple (no Redis dependency required).

Added to `pkg/config` struct alongside existing env vars.

## Template and CSS Changes

### Review View Model

Add `Pending bool` field to the review view model. Confirmed reviews: `false`. Pending reviews: `true`.

### reviews.html Template

Pending reviews render with distinct styling:
- Dashed amber border (`1px dashed` with warning color)
- Subtle amber background tint
- Pulsing dot animation next to reviewer name
- "Processing" text label

New CSS class `.review-pending` added to `layout.html` styles.

### HTMX Auto-Poll

When the `GET /partials/reviews/{id}` response contains pending reviews, the reviews container div includes `hx-trigger="every 2s"` to auto-poll. When no pending reviews remain, the attribute is omitted — polling stops naturally because the re-rendered HTML doesn't include it.

## Deployment Changes

### New: Redis Helm Install

Added to `k8s-apps` Makefile target before app service deployment.

### Updated: Productpage ConfigMap

Productpage read deployment ConfigMap updated with `REDIS_URL` env var pointing to `redis.bookinfo.svc.cluster.local:6379`.

### Unchanged

- CQRS write path (EventSource → Kafka → Sensor → write service)
- Reviews, ratings, details, notification backend services
- Sensors, EventSources, HTTPRoutes
- Form submission HTML structure
- Success banner message

## Request Lifecycle (Updated)

```
Submit:
1. Browser → POST /partials/rating (HTMX)
2. Productpage handler:
   ├─ POST to gateway /v1/ratings (async, returns 200)
   ├─ POST to gateway /v1/reviews (async, returns 200)
   ├─ RPUSH review to Redis pending list         ← NEW
   └─ Return success partial + HTMX refresh trigger
3. HTMX auto-triggers GET /partials/reviews/{id}

Read (with reconciliation):
4. Productpage handler:
   ├─ GET reviews from read service
   ├─ LRANGE pending reviews from Redis           ← NEW
   ├─ Match & LREM confirmed pending reviews      ← NEW
   ├─ Append remaining pending to bottom           ← NEW
   ├─ Set hx-trigger="every 2s" if pending exist  ← NEW
   └─ Render template (pending reviews styled)

Auto-poll (2s interval while pending exist):
5. Repeat step 4 until all pending are confirmed
6. Polling stops (no hx-trigger in response)
```
