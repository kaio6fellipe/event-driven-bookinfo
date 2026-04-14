# HTTPRoute Generation in bookinfo-service Chart — Design Specification

**Date:** 2026-04-14
**Status:** Draft
**Scope:** Add HTTPRoute generation to existing bookinfo-service Helm chart

## Overview

Move HTTPRoute resources from the monolithic `deploy/gateway/overlays/local/httproutes.yaml` into the `bookinfo-service` Helm chart. Routes are co-located with the service they belong to — auto-generated for CQRS services from `cqrs.endpoints`, explicitly defined for non-CQRS services via a `routes` list.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Write route derivation | Auto-generated from `cqrs.endpoints` | Method, endpoint path, eventsource-svc name, and port are already in the endpoint config — no duplication |
| Read route derivation | Auto-generated per CQRS endpoint | GET PathPrefix on the endpoint path → read service on port 80 |
| Non-CQRS routes | Explicit `routes` list in values | Only productpage needs this; generic enough to reuse |
| Gateway parentRef | Values-level config with defaults | Allows different environments to reference different gateways without chart changes |

## New values.yaml Fields

```yaml
# -- Gateway parentRef for HTTPRoutes
gateway:
  name: default-gw
  namespace: platform
  sectionName: web

# -- Custom HTTPRoutes (for non-CQRS services like productpage)
routes: []
  # - name: productpage        # HTTPRoute metadata.name
  #   path: /                  # match path
  #   pathType: PathPrefix     # PathPrefix or Exact
  #   method: GET              # HTTP method
```

## Route Generation Logic

### CQRS Services (auto-generated per `cqrs.endpoints` entry)

Each endpoint produces two HTTPRoutes:

1. **Write route** (`{fullname}-{endpointKey}-write`):
   - Match: `{endpoint.method} Exact {endpoint.endpoint}`
   - Backend: `{eventName}-eventsource-svc:{endpoint.port}`

2. **Read route** (`{fullname}-{endpointKey}-read`):
   - Match: `GET PathPrefix {endpoint.endpoint}`
   - Backend: `{fullname}:80` (read service)

### Non-CQRS Services (from `routes` list)

Each entry produces one HTTPRoute:
- Name: `{route.name}`
- Match: `{route.method} {route.pathType} {route.path}`
- Backend: `{fullname}:80`

## New Template

`charts/bookinfo-service/templates/httproute.yaml`

Renders:
1. One write + one read HTTPRoute per `cqrs.endpoints` entry
2. One HTTPRoute per `routes` list entry

All share the same `gateway` parentRef from values.

## Concrete Route Mapping

### Ratings (auto-generated)
| Route Name | Match | Backend |
|---|---|---|
| `ratings-rating-submitted-write` | `POST Exact /v1/ratings` | `rating-submitted-eventsource-svc:12002` |
| `ratings-rating-submitted-read` | `GET PathPrefix /v1/ratings` | `ratings:80` |

### Details (auto-generated)
| Route Name | Match | Backend |
|---|---|---|
| `details-book-added-write` | `POST Exact /v1/details` | `book-added-eventsource-svc:12000` |
| `details-book-added-read` | `GET PathPrefix /v1/details` | `details:80` |

### Reviews (auto-generated, 2 endpoints → 4 routes)
| Route Name | Match | Backend |
|---|---|---|
| `reviews-review-submitted-write` | `POST Exact /v1/reviews` | `review-submitted-eventsource-svc:12001` |
| `reviews-review-submitted-read` | `GET PathPrefix /v1/reviews` | `reviews:80` |
| `reviews-review-deleted-write` | `POST Exact /v1/reviews/delete` | `review-deleted-eventsource-svc:12003` |
| `reviews-review-deleted-read` | `GET PathPrefix /v1/reviews/delete` | `reviews:80` |

### DLQueue (auto-generated, no external read route needed)
| Route Name | Match | Backend |
|---|---|---|
| `dlqueue-dlq-event-received-write` | `POST Exact /dlq-event-received` | `dlq-event-received-eventsource-svc:12004` |
| `dlqueue-dlq-event-received-read` | `GET PathPrefix /dlq-event-received` | `dlqueue:80` |

Note: DLQueue's read route will produce a 405 (no GET handler), but this is harmless and consistent with the auto-generation pattern. If needed, read route generation can be disabled per-endpoint in the future.

### Productpage (explicit routes)
| Route Name | Match | Backend |
|---|---|---|
| `productpage` | `GET PathPrefix /` | `productpage:80` |
| `productpage-partials` | `POST PathPrefix /partials` | `productpage:80` |

### Notification (no routes)
Notification has no CQRS endpoints and no `routes` entries — no HTTPRoutes generated.

## Values File Changes

### Per-service values files — add `gateway` section

All services that produce routes need the gateway config. Add to each `values-local.yaml`:

```yaml
gateway:
  name: default-gw
  namespace: platform
  sectionName: web
```

### Productpage — add `routes` list

```yaml
routes:
  - name: productpage
    path: /
    pathType: PathPrefix
    method: GET
  - name: productpage-partials
    path: /partials
    pathType: PathPrefix
    method: POST
```

## Migration

### Removed
- `deploy/gateway/overlays/local/httproutes.yaml` — all 9 routes moved to chart
- `deploy/gateway/overlays/local/kustomization.yaml` — empty after removal
- `deploy/gateway/overlays/local/` directory
- Makefile: `kubectl apply -k deploy/gateway/overlays/local/` line from `k8s-deploy` and `k8s-rebuild`

### Kept Unchanged
- `deploy/gateway/base/` — Gateway, GatewayClass, EnvoyProxy, ReferenceGrant (platform-level, not per-service)

## Documentation Updates

### CLAUDE.md
- Update Deploy Structure to remove `gateway/overlays/local/` reference
- Note that HTTPRoutes are now generated by the Helm chart
