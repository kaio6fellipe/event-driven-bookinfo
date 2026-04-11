# Reviews Pagination & Delete — Design Spec

**Date:** 2026-04-11
**Branch:** feat/pyroscope-profiling (or new branch TBD)
**Status:** Draft

## Overview

Two features for the reviews service:

1. **Pagination** — offset-based pagination for the reviews list endpoint
2. **Delete** — event-driven review deletion with optimistic UI via Redis cache

Both follow existing hex arch patterns and CQRS conventions (writes through EventSource/Kafka/Sensor, reads synchronous).

---

## Feature 1: Pagination

### API Contract

`GET /v1/reviews/{productID}?page=1&page_size=10`

**Query params:**
- `page` — page number, default 1, minimum 1
- `page_size` — items per page, default 10, minimum 1, maximum 100

**Response:**

```json
{
  "product_id": "123",
  "reviews": [
    {
      "id": "uuid",
      "product_id": "123",
      "reviewer": "alice",
      "text": "Great book",
      "rating": { "stars": 5, "average": 4.2, "count": 10 }
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 10,
    "total_items": 47,
    "total_pages": 5
  }
}
```

### Reviews Service Changes (Hex Arch)

**Outbound port** (`port/outbound.go`):
```go
type ReviewRepository interface {
    FindByProductID(ctx context.Context, productID string, offset, limit int) ([]domain.Review, int, error)
    Save(ctx context.Context, review *domain.Review) error
}
```

Changed signature: `FindByProductID` now takes `offset` and `limit`, returns `(reviews, totalCount, error)`.

**Inbound port** (`port/inbound.go`):
```go
type ReviewService interface {
    GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]domain.Review, int, error)
    SubmitReview(ctx context.Context, productID, reviewer, text string) (*domain.Review, error)
}
```

Changed signature: `GetProductReviews` now takes `page` and `pageSize`, returns total count.

**Memory adapter:**
- Filter reviews by productID (existing linear scan)
- Compute total count from filtered set
- Apply offset/limit via slice math: `filtered[offset:min(offset+limit, len(filtered))]`

**Postgres adapter:**
- Query: `SELECT id, product_id, reviewer, text FROM reviews WHERE product_id = $1 ORDER BY id LIMIT $2 OFFSET $3`
- Count: `SELECT COUNT(*) FROM reviews WHERE product_id = $1`
- Both queries use existing `idx_reviews_product_id` index

**Service layer:**
- Compute offset: `(page - 1) * pageSize`
- Pass offset/limit to repository
- Enrich with ratings (unchanged)
- Return reviews + total count

**Handler:**
- Parse `page` and `page_size` from query params with defaults
- Validate: page >= 1, 1 <= page_size <= 100
- Compute `total_pages = ceil(total_items / page_size)`
- Add `PaginationResponse` to DTO and include in response

**DTOs** (`dto.go`):
```go
type PaginationResponse struct {
    Page       int `json:"page"`
    PageSize   int `json:"page_size"`
    TotalItems int `json:"total_items"`
    TotalPages int `json:"total_pages"`
}

type ProductReviewsResponse struct {
    ProductID  string             `json:"product_id"`
    Reviews    []ReviewResponse   `json:"reviews"`
    Pagination PaginationResponse `json:"pagination"`
}
```

### Productpage Changes

**Reviews client** (`client/reviews.go`):
- `GetProductReviews` adds `page` and `page_size` query params
- Update `ProductReviewsResponse` DTO to include pagination metadata

**Handler** (`handler.go`):
- `GET /partials/reviews/{id}` parses page from query params (default page 1)
- Passes page/page_size to reviews client
- Pending reviews still append after confirmed reviews on page 1 only

**Template** (`partials/reviews.html`):
- Add pagination controls (prev/next buttons) below reviews list
- Buttons use HTMX: `hx-get="/partials/reviews/{id}?page=N"` with `hx-target="#reviews-section"`
- Show "Page X of Y" indicator
- Disable prev on page 1, disable next on last page

### Infrastructure

No HTTPRoute, EventSource, or Sensor changes needed — pagination only affects the synchronous read path, and query params pass through the existing `reviews-read` HTTPRoute.

---

## Feature 2: Delete Review

### API Contract

`DELETE /v1/reviews/{reviewID}` → 204 No Content (success), 404 Not Found

### Reviews Service Changes (Hex Arch)

**Outbound port** (`port/outbound.go`):
```go
type ReviewRepository interface {
    FindByProductID(ctx context.Context, productID string, offset, limit int) ([]domain.Review, int, error)
    Save(ctx context.Context, review *domain.Review) error
    DeleteByID(ctx context.Context, id string) error
}
```

New method: `DeleteByID`. Returns `domain.ErrNotFound` if review doesn't exist.

**Inbound port** (`port/inbound.go`):
```go
type ReviewService interface {
    GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]domain.Review, int, error)
    SubmitReview(ctx context.Context, productID, reviewer, text string) (*domain.Review, error)
    DeleteReview(ctx context.Context, id string) error
}
```

New method: `DeleteReview`.

**Memory adapter:**
- Linear scan for matching ID, remove from slice
- Return `domain.ErrNotFound` if not found

**Postgres adapter:**
- `DELETE FROM reviews WHERE id = $1`
- Check rows affected; return `domain.ErrNotFound` if 0

**Service layer:**
- Call `repo.DeleteByID(ctx, id)`
- Pass through errors (including ErrNotFound)

**Handler:**
- `DELETE /v1/reviews/{id}` route
- Call service `DeleteReview`
- 204 No Content on success
- 404 with error JSON if not found

**Domain:**
- Add `var ErrNotFound = errors.New("review not found")` sentinel error

### Event-Driven Write Path

**HTTPRoute** (`deploy/gateway/overlays/local/httproutes.yaml`):
- New route `reviews-delete`: matches `DELETE /v1/reviews/*`
- Routes to `review-deleted-eventsource-svc:12002`

**EventSource** (`deploy/reviews/`):
- New `review-deleted` EventSource
- Webhook on port 12002
- Local overlay: endpoint `/v1/reviews`
- Publishes to Kafka eventbus

**Sensor** (`deploy/reviews/base/sensor.yaml`):
- New trigger `delete-review` on `review-deleted` event
- Sends `DELETE http://reviews/v1/reviews/{id}` to reviews-write service
- The review ID needs to be extracted from the webhook URL path
- New `notify-delete` trigger: sends deletion notification to notification service

**Kustomization updates:**
- Add new EventSource to `deploy/reviews/base/kustomization.yaml`
- Add EventSource service overlay in `deploy/reviews/overlays/local/`

### Productpage Changes (Optimistic Delete UX)

**New endpoint: `DELETE /partials/reviews/{id}?product_id=xxx`**

Handler flow:
1. Extract review ID from path, product ID from query param
2. Call Gateway DELETE route via reviews client (async — goes through EventSource/Kafka/Sensor)
3. Store "deleting" state in Redis: `pendingStore.StoreDeleting(ctx, productID, reviewID)`
4. Return HTMX response that triggers reviews section refresh

**Reviews client** (`client/reviews.go`):
- Add `DeleteReview(ctx, reviewID string) error` — sends DELETE through the gateway

**Pending store interface** (`pending/pending.go`):
```go
type Store interface {
    StorePending(ctx context.Context, productID string, review Review) error
    StoreDeleting(ctx context.Context, productID string, reviewID string) error
    GetAndReconcile(ctx context.Context, productID string, confirmed []ConfirmedReview) ([]Review, []string, error)
}
```

Changes:
- New method `StoreDeleting` — stores review ID in Redis SET `deleting:reviews:{productID}`
- `GetAndReconcile` returns additional `[]string` — list of review IDs being deleted
- Reconciliation logic: if a review ID is in the deleting set but absent from confirmed reviews, remove from Redis (deletion confirmed)

**Redis store** (`pending/redis.go`):
- `StoreDeleting`: `SADD deleting:reviews:{productID} {reviewID}`
- `GetAndReconcile` enhanced:
  1. Fetch pending reviews (existing logic)
  2. Fetch deleting set: `SMEMBERS deleting:reviews:{productID}`
  3. For each deleting ID: if not in confirmed reviews, `SREM` from Redis (confirmed deleted)
  4. Return remaining deleting IDs for UI rendering

**No-op store** (`pending/noop.go`):
- `StoreDeleting` returns nil
- `GetAndReconcile` returns nil for deleting slice

**Template** (`partials/reviews.html`):
- Each review card gets a delete button with a browser confirmation dialog:
  ```html
  <button hx-delete="/partials/reviews/{{.ID}}?product_id={{.ProductID}}"
          hx-confirm="Are you sure you want to delete this review?"
          hx-target="#reviews-section"
          hx-swap="innerHTML">Delete</button>
  ```
- `hx-confirm` is a built-in HTMX attribute that shows a native browser confirm dialog before issuing the request. No custom modal or JS needed.
- Reviews in "deleting" state render with "Deleting..." badge (red/gray tone)
- New `.review-deleting` CSS class — dashed border, semi-transparent background (red/gray variant of `.review-pending`)
- `.deleting-badge` — same structure as `.pending-badge` with different color
- Polling activates when there are pending OR deleting reviews

**Handler merge logic update:**
- `GET /partials/reviews/{id}` receives both pending reviews and deleting IDs from `GetAndReconcile`
- Confirmed reviews matching a deleting ID get `Deleting: true` flag in the view model
- `HasPending` flag becomes true if there are pending reviews OR deleting reviews

**View model update** (`model/product.go`):
```go
type ProductReview struct {
    ID       string
    Reviewer string
    Text     string
    Stars    int
    Average  float64
    Count    int
    Pending  bool
    Deleting bool
}
```

### k6 Load Test Changes

**File:** `test/k6/bookinfo-traffic.js`

- In the teardown function, use the paginated `GET /v1/reviews/{productID}` API to list all reviews
- Filter by text pattern matching `"k6 load test review"` to identify k6-generated reviews
- Call `DELETE /v1/reviews/{id}` for each matched review
- The delete calls go through the Gateway (event-driven path)
- Note: review creation goes through productpage HTML form, which returns HTML not JSON, so extracting IDs at creation time is impractical — querying at teardown is the clean approach

---

## Testing Strategy

### Reviews Service

- **Handler tests:** table-driven tests for GET with pagination params (default, custom, out-of-bounds, max page_size). DELETE with valid ID, missing ID, not-found ID.
- **Service tests:** pagination offset/limit calculation, delete with in-memory adapter.
- **Memory adapter tests:** FindByProductID with offset/limit, DeleteByID found/not-found.
- **Postgres adapter tests:** (if integration test infrastructure exists) same coverage.

### Productpage

- **Handler tests:** extend existing `TestPartialReviewsWithPending` pattern for deleting state. New test for DELETE endpoint. Pagination controls rendering.
- **Pending store tests:** `StoreDeleting`, reconciliation of deleting IDs.

---

## Files Affected

### Reviews Service
- `services/reviews/internal/core/domain/review.go` — ErrNotFound sentinel
- `services/reviews/internal/core/port/inbound.go` — updated interface
- `services/reviews/internal/core/port/outbound.go` — updated interface
- `services/reviews/internal/core/service/review_service.go` — pagination + delete logic
- `services/reviews/internal/core/service/review_service_test.go` — new tests
- `services/reviews/internal/adapter/inbound/http/handler.go` — pagination params, DELETE route
- `services/reviews/internal/adapter/inbound/http/handler_test.go` — new tests
- `services/reviews/internal/adapter/inbound/http/dto.go` — PaginationResponse
- `services/reviews/internal/adapter/outbound/memory/review_repository.go` — pagination + delete
- `services/reviews/internal/adapter/outbound/memory/review_repository_test.go` — new tests
- `services/reviews/internal/adapter/outbound/postgres/review_repository.go` — pagination + delete

### Productpage
- `services/productpage/internal/client/reviews.go` — pagination params, DeleteReview method, updated DTOs
- `services/productpage/internal/handler/handler.go` — pagination, DELETE endpoint, deleting state merge
- `services/productpage/internal/handler/handler_test.go` — new tests
- `services/productpage/internal/model/product.go` — Deleting field
- `services/productpage/internal/pending/pending.go` — StoreDeleting, updated interface
- `services/productpage/internal/pending/redis.go` — StoreDeleting, updated GetAndReconcile
- `services/productpage/internal/pending/noop.go` — StoreDeleting stub
- `services/productpage/templates/partials/reviews.html` — pagination controls, delete button, deleting badge
- `services/productpage/templates/layout.html` — .review-deleting, .deleting-badge CSS

### Infrastructure (k8s)
- `deploy/gateway/overlays/local/httproutes.yaml` — reviews-delete route
- `deploy/reviews/base/eventsource-delete.yaml` — new review-deleted EventSource
- `deploy/reviews/base/sensor.yaml` — new delete-review trigger
- `deploy/reviews/base/kustomization.yaml` — include new EventSource
- `deploy/reviews/overlays/local/eventsource-delete-patch.yaml` — local endpoint override
- `deploy/reviews/overlays/local/eventsource-delete-service.yaml` — ClusterIP service for EventSource
- `deploy/reviews/overlays/local/kustomization.yaml` — include new resources

### k6
- `test/k6/bookinfo-traffic.js` — store review IDs, delete in teardown
