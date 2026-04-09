# Gateway CQRS Routing Design

**Date:** 2026-04-09
**Status:** Draft
**Scope:** Infrastructure routing + minimal client fix (async UX deferred)

## Problem

Productpage currently POSTs directly to backend services (`http://details`, `http://reviews`, `http://ratings`), bypassing the Argo Events event pipeline entirely. Writes should flow through EventSource webhooks for event-driven processing (Kafka, sensors, write services). The webhook EventSources exist but are only reachable on a separate Gateway listener (port 443 / localhost:8443), and there is no mechanism for services to route writes through the event pipeline using their existing `SERVICE_URL` configuration.

## Solution

Consolidate the Gateway to a single `web` listener (port 80) with method-based routing. The Envoy Gateway becomes the CQRS routing boundary:

- **GET** requests route to read services (existing behavior)
- **POST** requests route to EventSource webhooks (event pipeline)

Services reuse their existing `SERVICE_URL` env vars, pointed at a stable gateway service instead of backend services directly. Zero new env vars.

## Architecture

### Request Flows

**Browser read:**
```
Browser --> GET localhost:8080/products/1
  --> Gateway (web listener :80)
    --> productpage
      --> GET http://gateway.envoy-gateway-system.svc.cluster.local/v1/details/1
        --> Gateway --> details service (read)
```

**Browser write (HTMX form):**
```
Browser --> POST localhost:8080/partials/rating
  --> Gateway (web listener :80)
    --> productpage
      --> POST http://gateway.envoy-gateway-system.svc.cluster.local/v1/ratings
        --> Gateway --> rating-submitted EventSource
          --> Kafka --> rating-submitted-sensor
            --> POST ratings-write.bookinfo.svc.cluster.local/v1/ratings
```

**External write (curl):**
```
curl -X POST localhost:8080/v1/ratings -d '{...}'
  --> Gateway (web listener :80)
    --> rating-submitted EventSource
      --> Kafka --> rating-submitted-sensor
        --> POST ratings-write.bookinfo.svc.cluster.local/v1/ratings
```

### HTTPRoute Routing Table

All routes on the `web` listener (port 80). Ordered by specificity (Gateway API spec guarantees more specific matches win):

| Method | Path | Match Type | Backend |
|--------|------|------------|---------|
| POST | `/v1/details` | Exact | book-added-eventsource-svc:12000 |
| POST | `/v1/reviews` | Exact | review-submitted-eventsource-svc:12001 |
| POST | `/v1/ratings` | Exact | rating-submitted-eventsource-svc:12002 |
| GET | `/v1/details` | PathPrefix | details:80 |
| GET | `/v1/reviews` | PathPrefix | reviews:80 |
| GET | `/v1/ratings` | PathPrefix | ratings:80 |
| POST | `/partials` | PathPrefix | productpage:80 |
| GET | `/` | PathPrefix | productpage:80 |

### EventSource Endpoint Alignment

EventSource webhook endpoints must mirror the service API paths they represent. Every POST endpoint on a service is mirrored by its corresponding EventSource webhook.

| EventSource | Current endpoint | New endpoint | Mirrors |
|---|---|---|---|
| book-added | `/book-added` (port 12000) | `/v1/details` (port 12000) | `POST /v1/details` on details service |
| review-submitted | `/review-submitted` (port 12001) | `/v1/reviews` (port 12001) | `POST /v1/reviews` on reviews service |
| rating-submitted | `/rating-submitted` (port 12002) | `/v1/ratings` (port 12002) | `POST /v1/ratings` on ratings service |

**Convention:** When new POST, PATCH, or DELETE endpoints are added to services, a corresponding EventSource webhook must be created with a matching path.

## Changes

### 1. Stable Gateway Service

Create a ClusterIP Service in `envoy-gateway-system` with stable label selectors (no hash-suffixed names):

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: envoy-gateway-system
spec:
  type: ClusterIP
  selector:
    gateway.envoyproxy.io/owning-gateway-name: default-gw
    gateway.envoyproxy.io/owning-gateway-namespace: platform
  ports:
    - name: http
      port: 80
      targetPort: 10080
```

Services reference: `http://gateway.envoy-gateway-system.svc.cluster.local`

### 2. Gateway Listener

Remove the `webhooks` listener (port 443). Keep only the `web` listener (port 80).

**Before:**
```yaml
listeners:
  - name: web
    protocol: HTTP
    port: 80
    allowedRoutes:
      namespaces:
        from: All
  - name: webhooks      # REMOVE
    protocol: HTTP
    port: 443
    allowedRoutes:
      namespaces:
        from: All
```

**After:**
```yaml
listeners:
  - name: web
    protocol: HTTP
    port: 80
    allowedRoutes:
      namespaces:
        from: All
```

### 3. k3d Cluster

Remove the 8443 port mapping from cluster creation:

**Before:** `-p "8443:443@loadbalancer"`
**After:** removed

### 4. HTTPRoutes

Replace the current `httproutes.yaml` with method-based routing. All routes attach to `sectionName: web`.

Webhook routes use `method: POST` matching. Service read routes use `method: GET` matching. Productpage gets GET (catch-all) and POST `/partials` (HTMX forms).

### 5. EventSource Webhooks

Update endpoint paths to match service API paths:

- `book-added.yaml`: endpoint `/book-added` -> `/v1/details`
- `review-submitted.yaml`: endpoint `/review-submitted` -> `/v1/reviews`
- `rating-submitted.yaml`: endpoint `/rating-submitted` -> `/v1/ratings`

### 6. Configmaps

**Productpage** (`deploy/productpage/overlays/local/configmap-patch.yaml`):
```yaml
DETAILS_SERVICE_URL: "http://gateway.envoy-gateway-system.svc.cluster.local"
REVIEWS_SERVICE_URL: "http://gateway.envoy-gateway-system.svc.cluster.local"
RATINGS_SERVICE_URL: "http://gateway.envoy-gateway-system.svc.cluster.local"
```

**Reviews** (`deploy/reviews/overlays/local/configmap-patch.yaml`):
```yaml
RATINGS_SERVICE_URL: "http://gateway.envoy-gateway-system.svc.cluster.local"
```

### 7. Productpage Client Code

Minimal fix for async writes. `SubmitRating()` and `SubmitReview()` currently expect `201 Created` with the created resource in the response body. EventSource webhooks return `200 OK` with an event acknowledgment.

**Change:** Accept both 200 and 201 as success. When receiving 200 (async via EventSource), return a "submitted" acknowledgment to the browser instead of the created resource. Full async UX improvements (loading states, polling, optimistic UI) are deferred to a follow-up spec.

### 8. Makefile

- Remove `-p "8443:443@loadbalancer"` from `k8s-cluster` target
- Update `k8s-status` target: remove webhook curl examples on port 8443, add examples on port 8080

### 9. Documentation Updates

All docs and diagrams referencing the dual-listener architecture, port 8443, or old EventSource paths must be updated.

**`README.md`:**
- Update infrastructure Mermaid diagram: remove `:8443` webhook listener, show single `:8080` entry point with method-based routing
- Update write-flow Mermaid diagram: reflect that POST goes through the same port as GET, routed by method
- Update access URLs table: remove `localhost:8443` webhook row, update webhook examples to use `localhost:8080`
- Update EventSource descriptions: new endpoint paths (`/v1/details`, `/v1/reviews`, `/v1/ratings`)
- Update deploy structure comments if needed

**`CLAUDE.md`:**
- Remove `Webhooks http://localhost:8443/v1/*` from Access line
- Update to reflect single-port access: `Productpage + Webhooks http://localhost:8080`
- Update event-driven writes description to mention gateway CQRS routing
- Update argo-events overlay description

**Historical specs/plans** (update only if they serve as active reference; otherwise leave as historical record):
- `docs/superpowers/specs/2026-04-08-k8s-local-environment-design.md`
- `docs/superpowers/plans/2026-04-08-k8s-local-environment.md`
- `docs/superpowers/specs/2026-04-08-argo-events-otel-realtime-design.md`
- `docs/superpowers/plans/2026-04-08-argo-events-otel-realtime.md`

## What Doesn't Change

- **Sensors** continue POSTing directly to write services (`details-write`, `reviews-write`, `ratings-write`, `notification`). Routing sensor traffic through the gateway would create a loop (POST -> EventSource -> Kafka -> sensor -> POST -> EventSource...).
- **Notification** service receives POSTs from sensors directly. No EventSource needed.
- **EventBus** (Kafka) configuration unchanged.
- **EventSource service objects** (`eventsource-services.yaml`) unchanged (same ports, same selectors).
- **Sensor trigger URLs** unchanged (still target `-write` services directly).

## Out of Scope

- **Async write UX**: Loading states, polling/SSE for write completion, optimistic UI, error handling for failed async writes. Separate follow-up spec.
- **New write endpoints**: No new POST/PATCH/DELETE endpoints on services in this change. The EventSource mirroring convention applies to future additions.
- **Authentication/authorization**: No auth on gateway routes in local dev.
