# Reviews Chronological Ordering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reviews returned by `GET /v1/reviews/{productID}` are ordered newest-first across both postgres and memory adapters.

**Architecture:** Add a `created_at TIMESTAMPTZ` column to the `reviews` table (with a composite index `(product_id, created_at DESC, id)` to support the new ORDER BY), thread a `CreatedAt time.Time` field through `domain.Review`, set it in `ReviewService.SubmitReview`, and update both adapters to honor the order. Surface the field in `ReviewResponse` for clients.

**Tech Stack:** Go 1.25, `github.com/jackc/pgx/v5`, `github.com/golang-migrate/migrate/v4`, in-memory `sort.SliceStable`.

**Spec:** `docs/superpowers/specs/2026-04-26-reviews-chronological-ordering-design.md`

---

## File Structure

| File | Status | Responsibility |
|---|---|---|
| `services/reviews/migrations/003_add_created_at.up.sql` | NEW | Add `created_at` column + composite index |
| `services/reviews/migrations/003_add_created_at.down.sql` | NEW | Reverse migration |
| `services/reviews/internal/core/domain/review.go` | MODIFY | Add `CreatedAt time.Time` to `Review` |
| `services/reviews/internal/core/service/review_service.go` | MODIFY | Set `review.CreatedAt` in `SubmitReview` |
| `services/reviews/internal/core/service/review_service_test.go` | MODIFY | Add ordering test against memory adapter |
| `services/reviews/internal/adapter/outbound/memory/review_repository.go` | MODIFY | Propagate `CreatedAt`, sort newest-first |
| `services/reviews/internal/adapter/outbound/memory/review_repository_test.go` | MODIFY | Add `TestFindByProductID_OrdersNewestFirst` |
| `services/reviews/internal/adapter/outbound/postgres/review_repository.go` | MODIFY | INSERT/SELECT/Scan changes |
| `services/reviews/internal/adapter/inbound/http/dto.go` | MODIFY | Add `CreatedAt` to `ReviewResponse` |
| `services/reviews/internal/adapter/inbound/http/handler.go` | MODIFY | Set `CreatedAt` in two inline `ReviewResponse` constructions |

---

## Task 1: Add migration files

**Files:**
- Create: `services/reviews/migrations/003_add_created_at.up.sql`
- Create: `services/reviews/migrations/003_add_created_at.down.sql`

The migrations are auto-applied at service startup via `database.RunMigrations`, which already discovers files via the `embed.FS` in `services/reviews/migrations/embed.go` (matches `*.sql`). No code changes needed there.

- [ ] **Step 1: Create the up migration**

Write `services/reviews/migrations/003_add_created_at.up.sql`:

```sql
ALTER TABLE reviews ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();

DROP INDEX IF EXISTS idx_reviews_product_id;
CREATE INDEX idx_reviews_product_id_created_at ON reviews (product_id, created_at DESC, id);
```

- [ ] **Step 2: Create the down migration**

Write `services/reviews/migrations/003_add_created_at.down.sql`:

```sql
DROP INDEX IF EXISTS idx_reviews_product_id_created_at;
CREATE INDEX idx_reviews_product_id ON reviews (product_id);
ALTER TABLE reviews DROP COLUMN IF EXISTS created_at;
```

- [ ] **Step 3: Verify the embed picks them up**

Run: `cd services/reviews && go build ./migrations/...`
Expected: build succeeds, no errors. (The `//go:embed *.sql` directive auto-includes the new files.)

- [ ] **Step 4: Commit**

```bash
git add services/reviews/migrations/003_add_created_at.up.sql services/reviews/migrations/003_add_created_at.down.sql
git commit -s -m "feat(reviews): add created_at column and composite index migration

Adds migration 003: created_at TIMESTAMPTZ NOT NULL DEFAULT now() plus
composite index (product_id, created_at DESC, id) to support
chronological ordering of reviews per product. Drops the old single-
column idx_reviews_product_id.
"
```

---

## Task 2: Add `CreatedAt` to `domain.Review`

**Files:**
- Modify: `services/reviews/internal/core/domain/review.go`

The struct currently has `ID, ProductID, Reviewer, Text, Rating`. Adding `CreatedAt time.Time` is purely additive — `NewReview` does NOT take the timestamp (the service sets it; first-write wins on idempotent replay). No tests exist for this struct directly; the value is exercised by service and adapter tests in later tasks.

- [ ] **Step 1: Add `time` import and `CreatedAt` field**

Edit `services/reviews/internal/core/domain/review.go`:

Replace the `import` block:
```go
import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)
```

with:
```go
import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)
```

Replace the `Review` struct:
```go
// Review represents a user review for a product.
type Review struct {
	ID        string
	ProductID string
	Reviewer  string
	Text      string
	Rating    *ReviewRating
}
```

with:
```go
// Review represents a user review for a product.
type Review struct {
	ID        string
	ProductID string
	Reviewer  string
	Text      string
	CreatedAt time.Time
	Rating    *ReviewRating
}
```

`NewReview` is left unchanged; the service is responsible for setting `CreatedAt` after construction (Task 3).

- [ ] **Step 2: Verify the package builds**

Run: `cd services/reviews && go build ./internal/core/domain/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add services/reviews/internal/core/domain/review.go
git commit -s -m "feat(reviews): add CreatedAt to Review domain struct

Adds CreatedAt time.Time so the service can stamp creation time on
new reviews. NewReview does not take it as an argument — the service
sets it after construction so the constructor signature stays stable
for existing callers and idempotent replay paths.
"
```

---

## Task 3: Set `CreatedAt` in `SubmitReview` + service-layer ordering test

**Files:**
- Modify: `services/reviews/internal/core/service/review_service.go`
- Modify: `services/reviews/internal/core/service/review_service_test.go`

`SubmitReview` calls `domain.NewReview(...)` at line 72, then runs the idempotency check, then `s.repo.Save(ctx, review)` at line 83. The timestamp goes between `NewReview` and the idempotency check — set it before either branch so it's populated whether or not we end up saving (it's harmless to set on the dedup branch since we don't pass `review` anywhere observable).

- [ ] **Step 1: Read `services/reviews/internal/core/service/review_service.go`**

Confirm the structure around `SubmitReview`. The relevant snippet (lines 69-105):

```go
func (s *ReviewService) SubmitReview(ctx context.Context, productID, reviewer, text, idempotencyKey string) (*domain.Review, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, text)

	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	...
```

- [ ] **Step 2: Write the failing service-layer test**

Append to `services/reviews/internal/core/service/review_service_test.go`:

```go
func TestGetProductReviews_OrderedNewestFirst(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client, idempotency.NewMemoryStore(), &fakeReviewPublisher{})

	ctx := context.Background()
	for _, reviewer := range []string{"first", "second", "third"} {
		if _, err := svc.SubmitReview(ctx, "product-1", reviewer, "text", ""); err != nil {
			t.Fatalf("submitting %q: %v", reviewer, err)
		}
		// Ensure distinct CreatedAt values across submissions.
		time.Sleep(2 * time.Millisecond)
	}

	reviews, total, err := svc.GetProductReviews(ctx, "product-1", 1, 10)
	if err != nil {
		t.Fatalf("getting reviews: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	want := []string{"third", "second", "first"}
	for i, w := range want {
		if reviews[i].Reviewer != w {
			t.Errorf("reviews[%d].Reviewer = %q, want %q", i, reviews[i].Reviewer, w)
		}
	}
}
```

Add `"time"` to the import block of that test file (the rest of the imports — `context`, `fmt`, `testing`, etc. — already exist).

- [ ] **Step 3: Run the test — expect failure**

Run: `go test ./services/reviews/internal/core/service/... -run TestGetProductReviews_OrderedNewestFirst -count=1 -v`
Expected: FAIL — without the service setting `CreatedAt`, both the adapter sort (Task 4) and the service ordering are missing. The test will fail because `reviews[0].Reviewer` will be "first" (insertion order) not "third".

(If Task 4 has not yet been done, the failure mode is "memory adapter returns insertion order" — same outcome: `reviews[0].Reviewer == "first"`. Test still fails until both Task 3 and Task 4 land.)

- [ ] **Step 4: Set `CreatedAt` in `SubmitReview`**

Edit `services/reviews/internal/core/service/review_service.go`:

Add `"time"` to the existing import block (it currently has `context`, `errors`, `fmt`, `log/slog`, plus repo packages). Insert `"time"` in the standard-library group:

```go
import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)
```

Replace:

```go
	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
```

with:

```go
	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}
	review.CreatedAt = time.Now().UTC()

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
```

- [ ] **Step 5: Run the test — still expects to fail until Task 4 sorts**

Run: `go test ./services/reviews/internal/core/service/... -run TestGetProductReviews_OrderedNewestFirst -count=1 -v`
Expected: still FAIL — the memory adapter does not yet sort. Reviews come back in insertion order ("first" then "second" then "third"). Service is now stamping `CreatedAt`, but the order is enforced by the adapter (Task 4).

If you want this task to be self-contained green, defer the commit until Task 4. Otherwise commit now and let Task 4 close the loop. Plan recommends: defer the commit to bundle Task 3 + Task 4 once the test passes.

- [ ] **Step 6: Hold the diff (no commit yet)**

Do not commit yet. Move to Task 4. Both will be committed together once the test passes.

---

## Task 4: Memory adapter — propagate `CreatedAt` and sort newest-first

**Files:**
- Modify: `services/reviews/internal/adapter/outbound/memory/review_repository.go`
- Modify: `services/reviews/internal/adapter/outbound/memory/review_repository_test.go`

The current adapter has two issues to fix in this task:

1. `FindByProductID` builds a per-product `filtered` slice by copying field-by-field — `ID, ProductID, Reviewer, Text` — which silently drops `CreatedAt`. Need to add `CreatedAt`.
2. The slice has no sort — insertion order leaks out. Need a `sort.SliceStable` on `CreatedAt` descending with `id` as tiebreaker.

`Save` already does `r.reviews = append(r.reviews, *review)` which copies the full struct, so once the service stamps `CreatedAt` (Task 3) it's stored. Only the per-call copy in `FindByProductID` needs adjustment.

- [ ] **Step 1: Write the failing memory-adapter test**

Append to `services/reviews/internal/adapter/outbound/memory/review_repository_test.go`:

```go
func TestFindByProductID_OrdersNewestFirst(t *testing.T) {
	repo := memory.NewReviewRepository()
	now := time.Now().UTC()

	// Save in arbitrary order; the adapter must return newest first.
	saves := []struct {
		id        string
		reviewer  string
		createdAt time.Time
	}{
		{id: "id-oldest", reviewer: "oldest", createdAt: now},
		{id: "id-newest", reviewer: "newest", createdAt: now.Add(2 * time.Minute)},
		{id: "id-middle", reviewer: "middle", createdAt: now.Add(1 * time.Minute)},
	}
	for _, s := range saves {
		rev := domain.Review{
			ID:        s.id,
			ProductID: "product-1",
			Reviewer:  s.reviewer,
			Text:      "text",
			CreatedAt: s.createdAt,
		}
		if err := repo.Save(context.Background(), &rev); err != nil {
			t.Fatalf("saving %q: %v", s.id, err)
		}
	}

	out, total, err := repo.FindByProductID(context.Background(), "product-1", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	want := []string{"newest", "middle", "oldest"}
	for i, w := range want {
		if out[i].Reviewer != w {
			t.Errorf("out[%d].Reviewer = %q, want %q", i, out[i].Reviewer, w)
		}
	}
}
```

Add `"time"` to that file's import block (currently `context`, `errors`, `testing` plus the local packages).

- [ ] **Step 2: Run the test — expect failure**

Run: `go test ./services/reviews/internal/adapter/outbound/memory/... -run TestFindByProductID_OrdersNewestFirst -count=1 -v`
Expected: FAIL. Without the sort, `out[0].Reviewer == "oldest"` (insertion order) — wanted "newest".

Also, even after sort is added, `out[i].Reviewer` would round-trip but `out[i].CreatedAt` would be zero-value if the field-copy isn't fixed. The reviewer-string assertions don't catch that, but the next step does both fixes together.

- [ ] **Step 3: Update the adapter — copy `CreatedAt` and sort**

Edit `services/reviews/internal/adapter/outbound/memory/review_repository.go`:

Add `"sort"` to the import block:

```go
import (
	"context"
	"sort"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)
```

Replace the body of `FindByProductID`:

```go
func (r *ReviewRepository) FindByProductID(_ context.Context, productID string, offset, limit int) ([]domain.Review, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []domain.Review
	for _, review := range r.reviews {
		if review.ProductID == productID {
			filtered = append(filtered, domain.Review{
				ID:        review.ID,
				ProductID: review.ProductID,
				Reviewer:  review.Reviewer,
				Text:      review.Text,
			})
		}
	}

	total := len(filtered)

	if offset >= total {
		return []domain.Review{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return filtered[offset:end], total, nil
}
```

with:

```go
func (r *ReviewRepository) FindByProductID(_ context.Context, productID string, offset, limit int) ([]domain.Review, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []domain.Review
	for _, review := range r.reviews {
		if review.ProductID == productID {
			filtered = append(filtered, domain.Review{
				ID:        review.ID,
				ProductID: review.ProductID,
				Reviewer:  review.Reviewer,
				Text:      review.Text,
				CreatedAt: review.CreatedAt,
			})
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if !filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
		}
		return filtered[i].ID < filtered[j].ID
	})

	total := len(filtered)

	if offset >= total {
		return []domain.Review{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return filtered[offset:end], total, nil
}
```

- [ ] **Step 4: Run the new memory-adapter test — expect pass**

Run: `go test ./services/reviews/internal/adapter/outbound/memory/... -run TestFindByProductID_OrdersNewestFirst -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Run the service-layer test from Task 3 — expect pass**

Run: `go test ./services/reviews/internal/core/service/... -run TestGetProductReviews_OrderedNewestFirst -count=1 -v`
Expected: PASS.

- [ ] **Step 6: Run the full reviews suite — make sure nothing regressed**

Run: `go test ./services/reviews/... -race -count=1`
Expected: all tests pass, including the existing `TestFindByProductID_Pagination` which doesn't depend on order.

- [ ] **Step 7: Commit Tasks 3 + 4 together**

```bash
git add services/reviews/internal/core/domain/review.go \
        services/reviews/internal/core/service/review_service.go \
        services/reviews/internal/core/service/review_service_test.go \
        services/reviews/internal/adapter/outbound/memory/review_repository.go \
        services/reviews/internal/adapter/outbound/memory/review_repository_test.go
git commit -s -m "fix(reviews): order product reviews newest-first

Adds CreatedAt to domain.Review and stamps it in ReviewService.
SubmitReview as time.Now().UTC() right after domain.NewReview returns.

Memory adapter now propagates CreatedAt across the per-product copy
loop and sorts the filtered slice newest-first via sort.SliceStable
with the review id as the deterministic tiebreaker.

New tests:
- TestGetProductReviews_OrderedNewestFirst (service layer, memory)
- TestFindByProductID_OrdersNewestFirst (memory adapter unit)

Bug surface: GET /v1/reviews/{id} previously returned reviews in
insertion order (memory) or UUID-lexical order (postgres ORDER BY id),
which looks random in the productpage UI.
"
```

(Domain change from Task 2 was already committed; this commit reuses the existing domain field. Re-add domain/review.go only if you skipped Task 2's commit.)

---

## Task 5: Postgres adapter — `created_at` in INSERT, SELECT order, Scan

**Files:**
- Modify: `services/reviews/internal/adapter/outbound/postgres/review_repository.go`

The postgres adapter has three statements to update:

- INSERT (line 62): add `created_at` column + parameter — explicit timestamp wins over the column default for deterministic timestamps.
- SELECT (line 35): add `created_at` to the projection AND change `ORDER BY id` to `ORDER BY created_at DESC, id`.
- Scan (line 46): add `&review.CreatedAt` to the field list.

There is no `FindByID` in this adapter (the file has `FindByProductID`, `Save`, `DeleteByID` only — no individual GET path). Skip the spec's `FindByID` note.

No unit tests are added in this task (postgres tests would need real DB infra). The change is verified by the service-layer test (already passing against the memory adapter, which mirrors the contract) and the k3d smoke test (Task 7).

- [ ] **Step 1: Update SELECT — projection and ORDER BY**

Edit `services/reviews/internal/adapter/outbound/postgres/review_repository.go`:

Replace the SELECT in `FindByProductID`:

```go
	rows, err := r.pool.Query(ctx,
		"SELECT id, product_id, reviewer, text FROM reviews WHERE product_id = $1 ORDER BY id LIMIT $2 OFFSET $3",
		productID, limit, offset,
	)
```

with:

```go
	rows, err := r.pool.Query(ctx,
		"SELECT id, product_id, reviewer, text, created_at "+
			"FROM reviews WHERE product_id = $1 "+
			"ORDER BY created_at DESC, id LIMIT $2 OFFSET $3",
		productID, limit, offset,
	)
```

- [ ] **Step 2: Update Scan to include `CreatedAt`**

Replace:

```go
		if err := rows.Scan(&review.ID, &review.ProductID, &review.Reviewer, &review.Text); err != nil {
			return nil, 0, fmt.Errorf("scanning review row: %w", err)
		}
```

with:

```go
		if err := rows.Scan(&review.ID, &review.ProductID, &review.Reviewer, &review.Text, &review.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning review row: %w", err)
		}
```

`pgx/v5` natively scans `TIMESTAMPTZ` into `time.Time`.

- [ ] **Step 3: Update INSERT to include `created_at`**

Replace:

```go
	_, err := r.pool.Exec(ctx,
		"INSERT INTO reviews (id, product_id, reviewer, text) VALUES ($1, $2, $3, $4)",
		review.ID, review.ProductID, review.Reviewer, review.Text,
	)
```

with:

```go
	_, err := r.pool.Exec(ctx,
		"INSERT INTO reviews (id, product_id, reviewer, text, created_at) VALUES ($1, $2, $3, $4, $5)",
		review.ID, review.ProductID, review.Reviewer, review.Text, review.CreatedAt,
	)
```

- [ ] **Step 4: Build and lint**

Run: `cd services/reviews && go build ./... && golangci-lint run ./internal/adapter/outbound/postgres/...`
Expected: success, 0 lint issues.

- [ ] **Step 5: Run the full reviews suite again to make sure builds still test-clean**

Run: `go test ./services/reviews/... -race -count=1`
Expected: all tests pass (postgres adapter has no unit tests but must compile cleanly).

- [ ] **Step 6: Commit**

```bash
git add services/reviews/internal/adapter/outbound/postgres/review_repository.go
git commit -s -m "fix(reviews): postgres adapter orders reviews by created_at desc

Updates ReviewRepository.FindByProductID's projection to select
created_at and changes ORDER BY id to ORDER BY created_at DESC, id —
the trailing id is a tiebreaker for rows submitted in the same
microsecond, which keeps offset pagination stable.

INSERT statement now binds review.CreatedAt explicitly so the timestamp
the service stamped in SubmitReview survives round-tripping through
the database (the column default of now() is the floor; explicit value
wins for deterministic ordering across replicas).
"
```

---

## Task 6: DTO + handler — surface `CreatedAt` in `ReviewResponse`

**Files:**
- Modify: `services/reviews/internal/adapter/inbound/http/dto.go`
- Modify: `services/reviews/internal/adapter/inbound/http/handler.go`

`ReviewResponse` is constructed inline in two places in `handler.go`:

1. Line 60-65 — inside the `getProductReviews` loop.
2. Line 116-121 — inside `submitReview` for the 201 response.

Both need `CreatedAt` added.

- [ ] **Step 1: Add `time` import + `CreatedAt` field to DTO**

Edit `services/reviews/internal/adapter/inbound/http/dto.go`:

Replace the file's start (it currently has no imports — package directive only) and the `ReviewResponse` struct:

```go
// Package http provides HTTP handlers and DTOs for the reviews service.
package http //nolint:revive // package name matches directory convention

// SubmitReviewRequest is the JSON body for POST /v1/reviews.
type SubmitReviewRequest struct {
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key,omitempty"` // optional; falls back to natural key
}

// ReviewRatingResponse represents rating data embedded in a review response.
type ReviewRatingResponse struct {
	Stars   int     `json:"stars"`
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// ReviewResponse represents a single review in API responses.
type ReviewResponse struct {
	ID        string                `json:"id"`
	ProductID string                `json:"product_id"`
	Reviewer  string                `json:"reviewer"`
	Text      string                `json:"text"`
	Rating    *ReviewRatingResponse `json:"rating,omitempty"`
}
```

with:

```go
// Package http provides HTTP handlers and DTOs for the reviews service.
package http //nolint:revive // package name matches directory convention

import "time"

// SubmitReviewRequest is the JSON body for POST /v1/reviews.
type SubmitReviewRequest struct {
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key,omitempty"` // optional; falls back to natural key
}

// ReviewRatingResponse represents rating data embedded in a review response.
type ReviewRatingResponse struct {
	Stars   int     `json:"stars"`
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// ReviewResponse represents a single review in API responses.
type ReviewResponse struct {
	ID        string                `json:"id"`
	ProductID string                `json:"product_id"`
	Reviewer  string                `json:"reviewer"`
	Text      string                `json:"text"`
	CreatedAt time.Time             `json:"created_at"`
	Rating    *ReviewRatingResponse `json:"rating,omitempty"`
}
```

- [ ] **Step 2: Update both inline `ReviewResponse` constructions in handler.go**

Edit `services/reviews/internal/adapter/inbound/http/handler.go`:

Replace (in `getProductReviews`):

```go
		resp := ReviewResponse{
			ID:        review.ID,
			ProductID: review.ProductID,
			Reviewer:  review.Reviewer,
			Text:      review.Text,
		}
```

with:

```go
		resp := ReviewResponse{
			ID:        review.ID,
			ProductID: review.ProductID,
			Reviewer:  review.Reviewer,
			Text:      review.Text,
			CreatedAt: review.CreatedAt,
		}
```

Replace (in `submitReview`):

```go
	writeJSON(w, http.StatusCreated, ReviewResponse{
		ID:        review.ID,
		ProductID: review.ProductID,
		Reviewer:  review.Reviewer,
		Text:      review.Text,
	})
```

with:

```go
	writeJSON(w, http.StatusCreated, ReviewResponse{
		ID:        review.ID,
		ProductID: review.ProductID,
		Reviewer:  review.Reviewer,
		Text:      review.Text,
		CreatedAt: review.CreatedAt,
	})
```

- [ ] **Step 3: Run the full reviews suite**

Run: `go test ./services/reviews/... -race -count=1`
Expected: all tests pass.

- [ ] **Step 4: Lint the package**

Run: `golangci-lint run ./services/reviews/...`
Expected: 0 issues.

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/adapter/inbound/http/dto.go \
        services/reviews/internal/adapter/inbound/http/handler.go
git commit -s -m "feat(reviews): expose CreatedAt on ReviewResponse

Adds CreatedAt time.Time to the ReviewResponse DTO and threads
review.CreatedAt through both inline constructions in
getProductReviews and submitReview.

JSON marshal of time.Time produces RFC3339 (e.g.
2026-04-26T19:30:00Z); existing clients that ignore unknown fields
are unaffected. Spec generator (PR #59) maps time.Time to
{type: string, format: date-time} automatically when
services/reviews/api/openapi.yaml regenerates.
"
```

---

## Task 7: End-to-end k3d verification

**Files:**
- No code changes. Verification only.

This is the standing rule from PR #63's brainstorm: the user expects k3d clean-cycle verification on every PR with a runtime impact. Migrations + ordering changes count.

- [ ] **Step 1: Run `make lint` and `make test` from repo root**

Run: `make lint && make test`
Expected: 0 lint issues, all tests pass with `-race`.

- [ ] **Step 2: Clean k3d cycle**

Run: `make stop-k8s && make run-k8s`
Expected: cluster comes up; all 11 deployments report `Available`. Reviews-write applies migration 003 at startup. Watch with `kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/reviews-write | grep migration` — expect a line confirming `003_add_created_at` applied.

- [ ] **Step 3: Verify schema in postgres**

Run:
```bash
kubectl --context=k3d-bookinfo-local -n bookinfo exec -it reviews-postgresql-0 -- \
  psql -U reviews -d reviews -c '\d+ reviews'
```
Expected: column `created_at | timestamp with time zone | not null | now()` and index `idx_reviews_product_id_created_at btree (product_id, created_at DESC, id)`.

- [ ] **Step 4: Submit three reviews to a fresh product**

```bash
PID="ord-test-$(date +%s)"
for r in alice bob carol; do
  curl -fsS -X POST http://localhost:8080/v1/reviews \
    -H 'Content-Type: application/json' \
    -d "{\"product_id\":\"$PID\",\"reviewer\":\"$r\",\"text\":\"review by $r\"}"
  sleep 1
done
```

Expected: three 200-or-201 responses with `created_at` populated in the body.

- [ ] **Step 5: Verify newest-first via the gateway**

```bash
curl -s "http://localhost:8080/v1/reviews/$PID" | jq '.reviews[].reviewer'
```
Expected output:
```
"carol"
"bob"
"alice"
```

- [ ] **Step 6: Verify in the productpage UI**

Open `http://localhost:8080/products/$PID` in a browser. The reviews list should show carol on top, bob in the middle, alice at the bottom — newest first.

- [ ] **Step 7: Confirm Tempo trace integrity**

Open Grafana at `http://localhost:3000`, navigate to Tempo, search for traces tagged `service.name=reviews-write` in the last 5 minutes. Pick one — confirm the chain: `gateway → reviews-events EventSource → notification-consumer-sensor → notification-write` is intact (no new spans broken by the migration).

- [ ] **Step 8: Done — no commit**

This task is verification-only. Move on to the finishing-a-development-branch flow.

---

## Self-review

After writing this plan, verify:

1. **Spec coverage**: Each spec section maps to a task —
   - Schema change → Task 1
   - Domain change → Task 2
   - Service change → Task 3
   - Postgres adapter → Task 5
   - Memory adapter → Task 4
   - DTO change → Task 6
   - Tests → integrated into Tasks 3 + 4
   - Verification → Task 7

2. **Spec gaps**: The spec mentions `FindByID` for postgres — that method does not exist in the current adapter. Plan correctly skips it (called out in Task 5).

3. **Spec gap on `services/reviews/api/openapi.yaml` regeneration**: Listed in spec file summary as a generated artifact. Skipped in this plan because the project's spec generator is invoked as a separate step (the `make` target / CI pipeline regenerates it on commit). If regeneration is wanted in this PR, run `cd tools/specgen && go run . generate ../../services/reviews` after Task 6 and add the regenerated file to the Task 6 commit.

4. **Productpage client struct**: `services/productpage/internal/client/reviews.go::ReviewResponse` currently mirrors the reviews DTO without `CreatedAt`. JSON decode will tolerate the unknown field. Plan does not add `CreatedAt` to the client struct — out of scope per spec ("transparent to producers"); UI does not currently render the timestamp. If a follow-up wants timestamp display in the UI, it adds the field to the client struct and the template.

5. **Type consistency**: `time.Time` used consistently across domain, service, adapters, DTO. `CreatedAt` capitalization is consistent.

6. **No placeholders**: Every task has runnable commands and full code blocks. No "implement appropriately" or "add error handling" stubs.
