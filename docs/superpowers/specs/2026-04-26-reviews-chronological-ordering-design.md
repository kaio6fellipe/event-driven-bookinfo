# Reviews Chronological Ordering — Design

**Status:** Design approved · Implementation pending
**Date:** 2026-04-26
**Scope:** `reviews` service only (one repo + one migration + handful of adjacent edits)

## Problem

Reviews displayed on the productpage frontend (`http://localhost:8080/products/<id>`) appear in random-looking order. Investigation:

- `services/reviews/migrations/001_create_reviews.up.sql` — table has only `id, product_id, reviewer, text` (no creation timestamp)
- `services/reviews/internal/adapter/outbound/postgres/review_repository.go:35` — `... ORDER BY id LIMIT $2 OFFSET $3`. UUIDs sort lexicographically, so the result *looks* random
- `services/reviews/internal/adapter/outbound/memory/review_repository.go::FindByProductID` — returns insertion order, no sort, no timestamp tracked
- `services/reviews/internal/core/domain/review.go::Review` — struct has no `CreatedAt` field

The bug pre-dates the API spec generation work (PRs #55, #58, #59, #63, #64). Other services have similar patterns (`details.ORDER BY title`, `ratings` no sort) but only `reviews` is user-visibly broken because it's the only listing the productpage UI surfaces with implied chronology.

## Goals

- Reviews returned by `GET /v1/reviews/{productID}` are ordered **newest first** (DESC on creation time).
- Both `postgres` and `memory` adapters honor the order so dev (compose) and prod (k3d) match.
- The change is transparent to producers: `POST /v1/reviews` payload doesn't change. The client doesn't need to send `created_at`.
- Clients that want the timestamp see it in `ReviewResponse` for free.

## Non-goals

- Fixing the same pattern in `details` (uses `ORDER BY title` — semantically reasonable; no user complaint) or `ratings` (no ORDER BY but no user-visible chronology either). Each can get its own ticket if needed.
- Backfilling historical creation times for existing rows. Migration default `now()` is honest about the limitation: rows present at migration time get a current-ish timestamp; subsequent rows are correct.
- Soft-delete column. Reviews are hard-deleted via the existing `DeleteByID` flow.
- Cursor-based pagination. The current offset-pagination keeps working with the new ORDER BY.

## Schema change

New migration pair:

**`services/reviews/migrations/003_add_created_at.up.sql`:**

```sql
ALTER TABLE reviews ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Drop the existing single-column index in favor of a composite that
-- supports both per-product filtering AND chronological ordering.
DROP INDEX IF EXISTS idx_reviews_product_id;
CREATE INDEX idx_reviews_product_id_created_at ON reviews (product_id, created_at DESC, id);
```

**`services/reviews/migrations/003_add_created_at.down.sql`:**

```sql
DROP INDEX IF EXISTS idx_reviews_product_id_created_at;
CREATE INDEX idx_reviews_product_id ON reviews (product_id);
ALTER TABLE reviews DROP COLUMN IF EXISTS created_at;
```

Migration is automatically applied by `database.RunMigrations` at service startup (existing pattern from `services/reviews/cmd/main.go`).

The composite index `(product_id, created_at DESC, id)` supports the new `WHERE product_id = $1 ORDER BY created_at DESC, id` query path. The trailing `id` is a tiebreaker for rows submitted in the same millisecond — important for pagination stability.

## Domain change

`services/reviews/internal/core/domain/review.go`:

```go
import "time"

type Review struct {
    ID        string
    ProductID string
    Reviewer  string
    Text      string
    CreatedAt time.Time      // NEW
    Rating    *ReviewRating
}
```

`NewReview(productID, reviewer, text)` does NOT take `createdAt` as an argument — the service sets it. Keeps the constructor signature stable for existing callers.

## Service change

`services/reviews/internal/core/service/review_service.go::SubmitReview` (or wherever `repo.Save` is called for new reviews) sets:

```go
review.CreatedAt = time.Now().UTC()
```

right after `domain.NewReview(...)` returns and before `repo.Save(...)`. Use UTC so postgres `TIMESTAMPTZ` storage is unambiguous.

The idempotent-replay path (when `idempotencyKey` matches an existing record) does NOT update `CreatedAt` — first-write wins. This is consistent with the existing dedup logic in the service layer.

## Postgres adapter

`services/reviews/internal/adapter/outbound/postgres/review_repository.go`:

### INSERT

Existing INSERT statement (find via grep `INSERT INTO reviews`) gains the `created_at` column. Pass `review.CreatedAt` from the caller. The DB also has `DEFAULT now()` so omitting the column would also work — but explicit is better for tests that need deterministic timestamps.

### SELECT (the broken query)

```go
rows, err := r.pool.Query(ctx,
    "SELECT id, product_id, reviewer, text, created_at "+
        "FROM reviews WHERE product_id = $1 "+
        "ORDER BY created_at DESC, id LIMIT $2 OFFSET $3",
    productID, limit, offset,
)
```

The `id` tiebreaker makes pagination stable when multiple reviews share a microsecond.

### Scan

```go
if err := rows.Scan(&review.ID, &review.ProductID, &review.Reviewer, &review.Text, &review.CreatedAt); err != nil {
    return nil, 0, fmt.Errorf("scanning review row: %w", err)
}
```

`pgx.v5` natively scans `TIMESTAMPTZ` into `time.Time`.

### `FindByID` (for delete/individual gets)

Find existing `SELECT id, product_id, reviewer, text FROM reviews WHERE id = $1` and add `created_at` to it too. Otherwise `FindByID` would return `Review{CreatedAt: zero}` which is harmless but inconsistent.

## Memory adapter

`services/reviews/internal/adapter/outbound/memory/review_repository.go::FindByProductID`:

After the existing per-product filter loop, sort the filtered slice before pagination:

```go
sort.SliceStable(filtered, func(i, j int) bool {
    if !filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
        return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
    }
    return filtered[i].ID < filtered[j].ID
})
```

`Save` already copies the whole struct (no field renames), so `CreatedAt` flows through automatically once the service sets it.

## DTO change

`services/reviews/internal/adapter/inbound/http/dto.go`:

```go
type ReviewResponse struct {
    ID         string                  `json:"id"`
    ProductID  string                  `json:"product_id"`
    Reviewer   string                  `json:"reviewer"`
    Text       string                  `json:"text"`
    CreatedAt  time.Time               `json:"created_at"`         // NEW
    Rating     *ReviewRatingResponse   `json:"rating,omitempty"`
}
```

The `time.Time` JSON marshals as RFC3339 (e.g. `2026-04-26T19:30:00Z`). Productpage's existing template uses this for display if needed; clients can omit. Spec generator (PR #59) maps `time.Time` → `{type: string, format: date-time}` automatically — the `services/reviews/api/openapi.yaml` regenerates with that schema.

The `toReviewResponse(domain)` helper gains a single line: `CreatedAt: r.CreatedAt`.

## Tests

### Service layer test (memory adapter)

In `services/reviews/internal/core/service/review_service_test.go` (or a new file if none exists):

```go
func TestGetProductReviews_OrderedNewestFirst(t *testing.T) {
    repo := memory.NewReviewRepository()
    svc := service.NewReviewService(repo, /* ... other deps ... */)

    // Submit three reviews with monotonic timestamps via service (which sets CreatedAt).
    // Use t.Setenv or a fake clock if the service uses one; otherwise just submit
    // sequentially and rely on time.Now() ordering.
    for _, reviewer := range []string{"first", "second", "third"} {
        _, err := svc.SubmitReview(ctx, productID, reviewer, "text", uuid.NewString())
        if err != nil { t.Fatal(err) }
        time.Sleep(2 * time.Millisecond) // ensure distinct timestamps
    }

    reviews, total, err := svc.GetProductReviews(ctx, productID, 1, 10)
    if err != nil { t.Fatal(err) }
    if total != 3 { t.Errorf("total = %d, want 3", total) }

    // Newest first.
    if reviews[0].Reviewer != "third" {
        t.Errorf("reviews[0].Reviewer = %q, want third", reviews[0].Reviewer)
    }
    if reviews[2].Reviewer != "first" {
        t.Errorf("reviews[2].Reviewer = %q, want first", reviews[2].Reviewer)
    }
}
```

### Memory adapter unit test

In `services/reviews/internal/adapter/outbound/memory/review_repository_test.go`:

```go
func TestFindByProductID_OrdersNewestFirst(t *testing.T) {
    repo := memory.NewReviewRepository()
    now := time.Now().UTC()

    for i, reviewer := range []string{"oldest", "middle", "newest"} {
        rev := domain.Review{
            ID: fmt.Sprintf("id-%d", i),
            ProductID: "product-1",
            Reviewer: reviewer,
            Text: "text",
            CreatedAt: now.Add(time.Duration(i) * time.Minute),
        }
        if err := repo.Save(ctx, &rev); err != nil { t.Fatal(err) }
    }

    out, _, err := repo.FindByProductID(ctx, "product-1", 0, 10)
    if err != nil { t.Fatal(err) }
    want := []string{"newest", "middle", "oldest"}
    for i, w := range want {
        if out[i].Reviewer != w {
            t.Errorf("out[%d].Reviewer = %q, want %q", i, out[i].Reviewer, w)
        }
    }
}
```

### Postgres adapter test

The existing postgres tests (typically gated by a build tag `integration` and require a real Postgres) gain a parallel ordering test. If no postgres tests run by default, adding one is optional — the SQL is simple enough to verify by k3d smoke. Acceptable to skip in this PR if it requires new test infrastructure.

## Verification

1. `go test ./services/reviews/... -race -count=1` — all reviews tests pass, including the two new ordering tests
2. `golangci-lint run ./...` — 0 issues
3. `make e2e` — compose-mode regression check; existing reviews tests still pass
4. **Local k3d clean cycle** (`make stop-k8s && make run-k8s`):
   - Reviews-write rolls out with the new migration applied (database has `created_at` column + new index)
   - Submit 3 reviews to a fresh product via the gateway
   - `curl http://localhost:8080/v1/reviews/<id>` returns reviews newest-first
   - Open productpage in browser at `http://localhost:8080/products/<id>` and verify the reviews list ordering matches submission order (newest at top)
5. **Tempo trace** (per the standing rule from PR #63's brainstorm): one review-submitted chain spans through the gateway → reviews-events EventSource → notification-consumer-sensor; confirms no runtime-path regression

## Risks & open questions

- **Migration on a populated dev cluster**: existing reviews get `now()` as `created_at` at migration time. Their ordering relative to each other becomes "all clustered at migration time" which is honest and acceptable. Subsequent reviews ordered correctly.
- **Schema change is technically breaking** for any external consumer that reads the table directly (we have none — the repo is internal to the reviews service). For the REST contract, the additional `created_at` field on `ReviewResponse` is an additive change — existing clients ignore unknown fields.
- **Time skew across replicas**: postgres `now()` is server-side, so replicas write consistent timestamps. The service-layer `time.Now().UTC()` is set on the application node before INSERT — if there's clock skew between app replicas, ordering could appear slightly out-of-order across submissions from different replicas. In practice the local k3d cluster has a single replica per service; production multi-replica concerns are out of scope.
- **Productpage's HTMX poll for pending reviews** — the productpage caches "submitted" reviews in Redis until they're visible from the reviews service. The pending cache is best-effort and doesn't depend on chronology; no change there.

## File summary

```
services/reviews/migrations/003_add_created_at.up.sql      # NEW (~5 lines)
services/reviews/migrations/003_add_created_at.down.sql    # NEW (~3 lines)
services/reviews/internal/core/domain/review.go            # +1 field on Review struct
services/reviews/internal/core/service/review_service.go   # +1 line in SubmitReview
services/reviews/internal/adapter/outbound/postgres/review_repository.go   # SELECT/INSERT/Scan changes
services/reviews/internal/adapter/outbound/memory/review_repository.go     # +sort.SliceStable in FindByProductID
services/reviews/internal/adapter/inbound/http/dto.go      # +CreatedAt on ReviewResponse + toReviewResponse
services/reviews/internal/adapter/outbound/memory/review_repository_test.go  # +TestFindByProductID_OrdersNewestFirst
services/reviews/internal/core/service/review_service_test.go                # +TestGetProductReviews_OrderedNewestFirst (or appropriate test file)
services/reviews/api/openapi.yaml                          # regenerated (CreatedAt field appears as date-time)
```

Net: 2 new files, ~7 modified, 1 generated artifact regenerated. ~80–120 lines of net diff.
