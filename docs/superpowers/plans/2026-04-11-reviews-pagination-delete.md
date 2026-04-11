# Reviews Pagination & Delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add offset-based pagination to the reviews list endpoint and an event-driven delete review feature with optimistic UI.

**Architecture:** Two independent features layered bottom-up through the hex arch stack. Pagination changes the read path (repo → service → handler → productpage client → template). Delete adds a new write path (domain → repo → service → handler → EventSource/Sensor → productpage handler → Redis cache → template). Both follow existing CQRS conventions.

**Tech Stack:** Go 1.24, net/http ServeMux, pgx v5, Redis (go-redis v9), Argo Events (EventSource + Sensor), Envoy Gateway HTTPRoute, HTMX 2.0, k6

**Spec:** `docs/superpowers/specs/2026-04-11-reviews-pagination-delete-design.md`

---

## File Map

### Reviews Service — Pagination + Delete

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `services/reviews/internal/core/domain/review.go` | Add `ErrNotFound` sentinel |
| Modify | `services/reviews/internal/core/domain/review_test.go` | (no changes needed — sentinel has no logic) |
| Modify | `services/reviews/internal/core/port/outbound.go` | Add pagination params to `FindByProductID`, add `DeleteByID` |
| Modify | `services/reviews/internal/core/port/inbound.go` | Add pagination params to `GetProductReviews`, add `DeleteReview` |
| Modify | `services/reviews/internal/adapter/outbound/memory/review_repository.go` | Paginated `FindByProductID`, `DeleteByID` |
| Create | `services/reviews/internal/adapter/outbound/memory/review_repository_test.go` | Memory adapter tests |
| Modify | `services/reviews/internal/adapter/outbound/postgres/review_repository.go` | Paginated query + count, `DeleteByID` |
| Modify | `services/reviews/internal/adapter/inbound/http/dto.go` | Add `PaginationResponse` |
| Modify | `services/reviews/internal/adapter/inbound/http/handler.go` | Parse pagination params, add DELETE route |
| Modify | `services/reviews/internal/adapter/inbound/http/handler_test.go` | Pagination + delete handler tests |
| Modify | `services/reviews/internal/core/service/review_service.go` | Pagination pass-through, `DeleteReview` |
| Modify | `services/reviews/internal/core/service/review_service_test.go` | Pagination + delete service tests |

### Productpage — Pagination + Optimistic Delete

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `services/productpage/internal/client/reviews.go` | Add pagination params, `DeleteReview` method, updated DTOs |
| Modify | `services/productpage/internal/model/product.go` | Add `Deleting` field to `ProductReview` |
| Modify | `services/productpage/internal/pending/pending.go` | Add `StoreDeleting` to interface, update `GetAndReconcile` return |
| Modify | `services/productpage/internal/pending/redis.go` | Implement `StoreDeleting`, updated reconciliation |
| Modify | `services/productpage/internal/pending/noop.go` | Add `StoreDeleting` + updated `GetAndReconcile` stubs |
| Modify | `services/productpage/internal/handler/handler.go` | Parse page param, add DELETE handler, merge deleting state |
| Modify | `services/productpage/internal/handler/handler_test.go` | Pagination + delete handler tests |
| Modify | `services/productpage/templates/partials/reviews.html` | Pagination controls, delete button, deleting badge |
| Modify | `services/productpage/templates/layout.html` | `.review-deleting` + `.deleting-badge` CSS |

### Infrastructure — Delete Event Path

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `deploy/gateway/overlays/local/httproutes.yaml` | Add `reviews-delete` HTTPRoute + `productpage-delete-partials` route |
| Create | `deploy/reviews/base/eventsource-delete.yaml` | `review-deleted` EventSource |
| Modify | `deploy/reviews/base/sensor.yaml` | Add `delete-review` + `notify-review-deleted` triggers |
| Modify | `deploy/reviews/base/kustomization.yaml` | Include new EventSource |
| Create | `deploy/reviews/overlays/local/eventsource-delete-patch.yaml` | Local overlay for delete EventSource |
| Create | `deploy/reviews/overlays/local/eventsource-delete-service.yaml` | ClusterIP service for delete EventSource |
| Modify | `deploy/reviews/overlays/local/kustomization.yaml` | Include new resources + patch |
| Modify | `deploy/reviews/overlays/local/sensor-patch.yaml` | Add local overlay for delete triggers |

### k6

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `test/k6/bookinfo-traffic.js` | Add teardown cleanup of k6-generated reviews |

---

## Task 1: Domain — Add ErrNotFound Sentinel

**Files:**
- Modify: `services/reviews/internal/core/domain/review.go`

- [ ] **Step 1: Add ErrNotFound sentinel error**

In `services/reviews/internal/core/domain/review.go`, add the sentinel error after the imports and before the type definitions:

```go
import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a review is not found.
var ErrNotFound = errors.New("review not found")
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/reviews/...`
Expected: no errors (existing tests will break later when we change interfaces — that's expected)

- [ ] **Step 3: Commit**

```bash
git add services/reviews/internal/core/domain/review.go
git commit -m "feat(reviews): add ErrNotFound sentinel error to domain"
```

---

## Task 2: Outbound Port — Add Pagination + Delete to Repository Interface

**Files:**
- Modify: `services/reviews/internal/core/port/outbound.go`

- [ ] **Step 1: Update ReviewRepository interface**

Replace the full `ReviewRepository` interface in `services/reviews/internal/core/port/outbound.go`:

```go
// ReviewRepository defines the outbound persistence operations for reviews.
type ReviewRepository interface {
	// FindByProductID returns paginated reviews for a given product ID.
	// offset is the number of reviews to skip, limit is the max to return.
	// Returns the matching reviews and the total count for the product.
	FindByProductID(ctx context.Context, productID string, offset, limit int) ([]domain.Review, int, error)

	// Save persists a review.
	Save(ctx context.Context, review *domain.Review) error

	// DeleteByID removes a review by its ID.
	// Returns domain.ErrNotFound if the review does not exist.
	DeleteByID(ctx context.Context, id string) error
}
```

- [ ] **Step 2: Verify the file is valid Go (will not compile yet — adapters need updating)**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go vet ./services/reviews/internal/core/port/...`
Expected: passes (interfaces only, no implementation to fail)

- [ ] **Step 3: Commit**

```bash
git add services/reviews/internal/core/port/outbound.go
git commit -m "feat(reviews): add pagination and delete to repository port"
```

---

## Task 3: Inbound Port — Add Pagination + Delete to Service Interface

**Files:**
- Modify: `services/reviews/internal/core/port/inbound.go`

- [ ] **Step 1: Update ReviewService interface**

Replace the full `ReviewService` interface in `services/reviews/internal/core/port/inbound.go`:

```go
// ReviewService defines the inbound operations for the reviews domain.
type ReviewService interface {
	// GetProductReviews returns paginated reviews for a product, enriched with ratings data.
	// Returns the matching reviews and the total count for the product.
	GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]domain.Review, int, error)

	// SubmitReview creates and stores a new review.
	SubmitReview(ctx context.Context, productID, reviewer, text string) (*domain.Review, error)

	// DeleteReview removes a review by its ID.
	// Returns domain.ErrNotFound if the review does not exist.
	DeleteReview(ctx context.Context, id string) error
}
```

- [ ] **Step 2: Commit**

```bash
git add services/reviews/internal/core/port/inbound.go
git commit -m "feat(reviews): add pagination and delete to service port"
```

---

## Task 4: Memory Adapter — Implement Pagination + Delete

**Files:**
- Modify: `services/reviews/internal/adapter/outbound/memory/review_repository.go`
- Create: `services/reviews/internal/adapter/outbound/memory/review_repository_test.go`

- [ ] **Step 1: Write failing tests for paginated FindByProductID and DeleteByID**

Create `services/reviews/internal/adapter/outbound/memory/review_repository_test.go`:

```go
package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

func seedReviews(t *testing.T, repo *memory.ReviewRepository, productID string, count int) []domain.Review {
	t.Helper()
	var reviews []domain.Review
	for i := 0; i < count; i++ {
		r, err := domain.NewReview(productID, "reviewer", "text")
		if err != nil {
			t.Fatalf("creating review: %v", err)
		}
		if err := repo.Save(context.Background(), r); err != nil {
			t.Fatalf("saving review: %v", err)
		}
		reviews = append(reviews, *r)
	}
	return reviews
}

func TestFindByProductID_Pagination(t *testing.T) {
	repo := memory.NewReviewRepository()
	seedReviews(t, repo, "product-1", 25)
	seedReviews(t, repo, "product-2", 3)

	tests := []struct {
		name       string
		productID  string
		offset     int
		limit      int
		wantCount  int
		wantTotal  int
	}{
		{name: "first page", productID: "product-1", offset: 0, limit: 10, wantCount: 10, wantTotal: 25},
		{name: "second page", productID: "product-1", offset: 10, limit: 10, wantCount: 10, wantTotal: 25},
		{name: "last page partial", productID: "product-1", offset: 20, limit: 10, wantCount: 5, wantTotal: 25},
		{name: "offset beyond total", productID: "product-1", offset: 30, limit: 10, wantCount: 0, wantTotal: 25},
		{name: "different product", productID: "product-2", offset: 0, limit: 10, wantCount: 3, wantTotal: 3},
		{name: "empty product", productID: "nonexistent", offset: 0, limit: 10, wantCount: 0, wantTotal: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviews, total, err := repo.FindByProductID(context.Background(), tt.productID, tt.offset, tt.limit)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(reviews) != tt.wantCount {
				t.Errorf("got %d reviews, want %d", len(reviews), tt.wantCount)
			}
			if total != tt.wantTotal {
				t.Errorf("got total %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestDeleteByID_Success(t *testing.T) {
	repo := memory.NewReviewRepository()
	reviews := seedReviews(t, repo, "product-1", 3)

	err := repo.DeleteByID(context.Background(), reviews[1].ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	remaining, total, err := repo.FindByProductID(context.Background(), "product-1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 remaining, got %d", total)
	}
	for _, r := range remaining {
		if r.ID == reviews[1].ID {
			t.Error("deleted review still present")
		}
	}
}

func TestDeleteByID_NotFound(t *testing.T) {
	repo := memory.NewReviewRepository()

	err := repo.DeleteByID(context.Background(), "nonexistent-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/internal/adapter/outbound/memory/... -v -run "TestFindByProductID_Pagination|TestDeleteByID" -count=1`
Expected: compilation failure — `FindByProductID` signature mismatch, `DeleteByID` not found

- [ ] **Step 3: Implement paginated FindByProductID and DeleteByID**

Replace the full content of `services/reviews/internal/adapter/outbound/memory/review_repository.go`:

```go
// Package memory provides an in-memory implementation of the reviews repository.
package memory

import (
	"context"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewRepository is an in-memory implementation of port.ReviewRepository.
type ReviewRepository struct {
	mu      sync.RWMutex
	reviews []domain.Review
}

// NewReviewRepository creates a new in-memory review repository.
func NewReviewRepository() *ReviewRepository {
	return &ReviewRepository{
		reviews: make([]domain.Review, 0),
	}
}

// FindByProductID returns paginated reviews for the given product ID.
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

// Save persists a review in memory.
func (r *ReviewRepository) Save(_ context.Context, review *domain.Review) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reviews = append(r.reviews, *review)
	return nil
}

// DeleteByID removes a review by its ID.
func (r *ReviewRepository) DeleteByID(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, review := range r.reviews {
		if review.ID == id {
			r.reviews = append(r.reviews[:i], r.reviews[i+1:]...)
			return nil
		}
	}

	return domain.ErrNotFound
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/internal/adapter/outbound/memory/... -v -count=1`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/adapter/outbound/memory/review_repository.go services/reviews/internal/adapter/outbound/memory/review_repository_test.go
git commit -m "feat(reviews): implement pagination and delete in memory adapter"
```

---

## Task 5: Postgres Adapter — Implement Pagination + Delete

**Files:**
- Modify: `services/reviews/internal/adapter/outbound/postgres/review_repository.go`

- [ ] **Step 1: Update FindByProductID with pagination and add DeleteByID**

Replace the full content of `services/reviews/internal/adapter/outbound/postgres/review_repository.go`:

```go
// Package postgres provides a PostgreSQL implementation of the reviews repository.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewRepository is a PostgreSQL implementation of port.ReviewRepository.
type ReviewRepository struct {
	pool *pgxpool.Pool
}

// NewReviewRepository creates a new PostgreSQL review repository.
func NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository {
	return &ReviewRepository{pool: pool}
}

// FindByProductID returns paginated reviews for the given product ID.
func (r *ReviewRepository) FindByProductID(ctx context.Context, productID string, offset, limit int) ([]domain.Review, int, error) {
	var total int
	err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM reviews WHERE product_id = $1",
		productID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting reviews for product %s: %w", productID, err)
	}

	rows, err := r.pool.Query(ctx,
		"SELECT id, product_id, reviewer, text FROM reviews WHERE product_id = $1 ORDER BY id LIMIT $2 OFFSET $3",
		productID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying reviews for product %s: %w", productID, err)
	}
	defer rows.Close()

	var reviews []domain.Review
	for rows.Next() {
		var review domain.Review
		if err := rows.Scan(&review.ID, &review.ProductID, &review.Reviewer, &review.Text); err != nil {
			return nil, 0, fmt.Errorf("scanning review row: %w", err)
		}
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating review rows: %w", err)
	}

	return reviews, total, nil
}

// Save persists a review in PostgreSQL.
func (r *ReviewRepository) Save(ctx context.Context, review *domain.Review) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO reviews (id, product_id, reviewer, text) VALUES ($1, $2, $3, $4)",
		review.ID, review.ProductID, review.Reviewer, review.Text,
	)
	if err != nil {
		return fmt.Errorf("inserting review %s: %w", review.ID, err)
	}

	return nil
}

// DeleteByID removes a review by its ID.
func (r *ReviewRepository) DeleteByID(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx,
		"DELETE FROM reviews WHERE id = $1",
		id,
	)
	if err != nil {
		return fmt.Errorf("deleting review %s: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/reviews/internal/adapter/outbound/postgres/...`
Expected: compiles cleanly

- [ ] **Step 3: Commit**

```bash
git add services/reviews/internal/adapter/outbound/postgres/review_repository.go
git commit -m "feat(reviews): implement pagination and delete in postgres adapter"
```

---

## Task 6: Service Layer — Implement Pagination + Delete

**Files:**
- Modify: `services/reviews/internal/core/service/review_service.go`
- Modify: `services/reviews/internal/core/service/review_service_test.go`

- [ ] **Step 1: Write failing tests for paginated GetProductReviews and DeleteReview**

Replace the full content of `services/reviews/internal/core/service/review_service_test.go`:

```go
// file: services/reviews/internal/core/service/review_service_test.go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

// stubRatingsClient returns fixed rating data for testing.
type stubRatingsClient struct {
	data *domain.RatingData
	err  error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.RatingData, error) {
	return s.data, s.err
}

func TestSubmitReview_Success(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	review, err := svc.SubmitReview(context.Background(), "product-1", "alice", "Great book!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if review.ID == "" {
		t.Error("expected non-empty ID")
	}
	if review.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", review.ProductID, "product-1")
	}
	if review.Reviewer != "alice" {
		t.Errorf("Reviewer = %q, want %q", review.Reviewer, "alice")
	}
}

func TestSubmitReview_ValidationError(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	tests := []struct {
		name      string
		productID string
		reviewer  string
		text      string
	}{
		{name: "empty product ID", productID: "", reviewer: "alice", text: "Great!"},
		{name: "empty reviewer", productID: "product-1", reviewer: "", text: "Great!"},
		{name: "empty text", productID: "product-1", reviewer: "alice", text: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SubmitReview(context.Background(), tt.productID, tt.reviewer, tt.text)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestGetProductReviews_Empty(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{Average: 0, Count: 0, IndividualRatings: map[string]int{}},
	}
	svc := service.NewReviewService(repo, client)

	reviews, total, err := svc.GetProductReviews(context.Background(), "nonexistent", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}

func TestGetProductReviews_WithRatings(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{
			Average: 3.5,
			Count:   2,
			IndividualRatings: map[string]int{
				"alice": 5,
				"bob":   2,
			},
		},
	}
	svc := service.NewReviewService(repo, client)

	_, err := svc.SubmitReview(context.Background(), "product-1", "alice", "Excellent!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.SubmitReview(context.Background(), "product-1", "bob", "Good read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviews, total, err := svc.GetProductReviews(context.Background(), "product-1", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviews) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(reviews))
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}

	for _, review := range reviews {
		if review.Rating == nil {
			t.Errorf("expected non-nil Rating on review by %s", review.Reviewer)
			continue
		}
		if review.Rating.Average != 3.5 {
			t.Errorf("Rating.Average = %f, want 3.5", review.Rating.Average)
		}
		if review.Rating.Count != 2 {
			t.Errorf("Rating.Count = %d, want 2", review.Rating.Count)
		}
		switch review.Reviewer {
		case "alice":
			if review.Rating.Stars != 5 {
				t.Errorf("alice Rating.Stars = %d, want 5", review.Rating.Stars)
			}
		case "bob":
			if review.Rating.Stars != 2 {
				t.Errorf("bob Rating.Stars = %d, want 2", review.Rating.Stars)
			}
		default:
			t.Errorf("unexpected reviewer: %s", review.Reviewer)
		}
	}
}

func TestGetProductReviews_Pagination(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{Average: 4.0, Count: 15, IndividualRatings: map[string]int{}},
	}
	svc := service.NewReviewService(repo, client)

	for i := 0; i < 15; i++ {
		_, err := svc.SubmitReview(context.Background(), "product-1", "reviewer", "text")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	tests := []struct {
		name      string
		page      int
		pageSize  int
		wantCount int
		wantTotal int
	}{
		{name: "first page", page: 1, pageSize: 10, wantCount: 10, wantTotal: 15},
		{name: "second page", page: 2, pageSize: 10, wantCount: 5, wantTotal: 15},
		{name: "page beyond total", page: 3, pageSize: 10, wantCount: 0, wantTotal: 15},
		{name: "custom page size", page: 1, pageSize: 5, wantCount: 5, wantTotal: 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviews, total, err := svc.GetProductReviews(context.Background(), "product-1", tt.page, tt.pageSize)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(reviews) != tt.wantCount {
				t.Errorf("got %d reviews, want %d", len(reviews), tt.wantCount)
			}
			if total != tt.wantTotal {
				t.Errorf("got total %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestDeleteReview_Success(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	review, err := svc.SubmitReview(context.Background(), "product-1", "alice", "Great book!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = svc.DeleteReview(context.Background(), review.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviews, total, err := svc.GetProductReviews(context.Background(), "product-1", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 reviews after delete, got %d", total)
	}
	if len(reviews) != 0 {
		t.Errorf("expected empty reviews after delete, got %d", len(reviews))
	}
}

func TestDeleteReview_NotFound(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	err := svc.DeleteReview(context.Background(), "nonexistent-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/internal/core/service/... -v -count=1`
Expected: compilation failure — `GetProductReviews` signature mismatch, `DeleteReview` not defined

- [ ] **Step 3: Update service implementation**

Replace the full content of `services/reviews/internal/core/service/review_service.go`:

```go
// Package service implements the business logic for the reviews service.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

// ReviewService implements the port.ReviewService interface.
type ReviewService struct {
	repo          port.ReviewRepository
	ratingsClient port.RatingsClient
}

// NewReviewService creates a new ReviewService.
func NewReviewService(repo port.ReviewRepository, ratingsClient port.RatingsClient) *ReviewService {
	return &ReviewService{
		repo:          repo,
		ratingsClient: ratingsClient,
	}
}

// GetProductReviews returns paginated reviews for a product, enriched with ratings data.
func (s *ReviewService) GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]domain.Review, int, error) {
	offset := (page - 1) * pageSize

	reviews, total, err := s.repo.FindByProductID(ctx, productID, offset, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("finding reviews for product %s: %w", productID, err)
	}

	// Fetch ratings from the ratings service
	ratingData, err := s.ratingsClient.GetProductRatings(ctx, productID)
	if err != nil {
		logger := logging.FromContext(ctx)
		logger.Warn("failed to fetch ratings, returning reviews without ratings",
			slog.String("product_id", productID),
			slog.String("error", err.Error()),
		)
		return reviews, total, nil
	}

	// Enrich each review with product-level stats and individual reviewer score
	for i := range reviews {
		reviews[i].Rating = &domain.ReviewRating{
			Stars:   ratingData.IndividualRatings[reviews[i].Reviewer],
			Average: ratingData.Average,
			Count:   ratingData.Count,
		}
	}

	return reviews, total, nil
}

// SubmitReview creates and persists a new review.
func (s *ReviewService) SubmitReview(ctx context.Context, productID, reviewer, text string) (*domain.Review, error) {
	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}

	if err := s.repo.Save(ctx, review); err != nil {
		return nil, fmt.Errorf("saving review: %w", err)
	}

	return review, nil
}

// DeleteReview removes a review by its ID.
func (s *ReviewService) DeleteReview(ctx context.Context, id string) error {
	if err := s.repo.DeleteByID(ctx, id); err != nil {
		return fmt.Errorf("deleting review %s: %w", id, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/internal/core/service/... -v -count=1`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/core/service/review_service.go services/reviews/internal/core/service/review_service_test.go
git commit -m "feat(reviews): implement pagination and delete in service layer"
```

---

## Task 7: DTOs — Add PaginationResponse

**Files:**
- Modify: `services/reviews/internal/adapter/inbound/http/dto.go`

- [ ] **Step 1: Add PaginationResponse and update ProductReviewsResponse**

Replace the full content of `services/reviews/internal/adapter/inbound/http/dto.go`:

```go
// Package http provides HTTP handlers and DTOs for the reviews service.
package http //nolint:revive // package name matches directory convention

// SubmitReviewRequest is the JSON body for POST /v1/reviews.
type SubmitReviewRequest struct {
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Text      string `json:"text"`
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

// PaginationResponse contains pagination metadata.
type PaginationResponse struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// ProductReviewsResponse wraps multiple reviews for a product.
type ProductReviewsResponse struct {
	ProductID  string             `json:"product_id"`
	Reviews    []ReviewResponse   `json:"reviews"`
	Pagination PaginationResponse `json:"pagination"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/reviews/internal/adapter/inbound/http/...`
Expected: compilation failure — handler still uses old interface. That's expected; we fix it next.

- [ ] **Step 3: Commit**

```bash
git add services/reviews/internal/adapter/inbound/http/dto.go
git commit -m "feat(reviews): add PaginationResponse DTO"
```

---

## Task 8: Handler — Add Pagination + DELETE Route

**Files:**
- Modify: `services/reviews/internal/adapter/inbound/http/handler.go`
- Modify: `services/reviews/internal/adapter/inbound/http/handler_test.go`

- [ ] **Step 1: Write failing tests for pagination and DELETE**

Add these tests to the end of `services/reviews/internal/adapter/inbound/http/handler_test.go`:

```go
func TestGetProductReviews_Pagination(t *testing.T) {
	mux := setupHandler(t)

	// Submit 15 reviews
	for i := 0; i < 15; i++ {
		reqBody := handler.SubmitReviewRequest{
			ProductID: "product-1",
			Reviewer:  "alice",
			Text:      fmt.Sprintf("Review %d", i),
		}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}

	tests := []struct {
		name           string
		url            string
		wantCount      int
		wantPage       int
		wantPageSize   int
		wantTotalItems int
		wantTotalPages int
	}{
		{
			name: "default pagination", url: "/v1/reviews/product-1",
			wantCount: 10, wantPage: 1, wantPageSize: 10, wantTotalItems: 15, wantTotalPages: 2,
		},
		{
			name: "page 2", url: "/v1/reviews/product-1?page=2",
			wantCount: 5, wantPage: 2, wantPageSize: 10, wantTotalItems: 15, wantTotalPages: 2,
		},
		{
			name: "custom page size", url: "/v1/reviews/product-1?page=1&page_size=5",
			wantCount: 5, wantPage: 1, wantPageSize: 5, wantTotalItems: 15, wantTotalPages: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			var body handler.ProductReviewsResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}

			if len(body.Reviews) != tt.wantCount {
				t.Errorf("got %d reviews, want %d", len(body.Reviews), tt.wantCount)
			}
			if body.Pagination.Page != tt.wantPage {
				t.Errorf("Page = %d, want %d", body.Pagination.Page, tt.wantPage)
			}
			if body.Pagination.PageSize != tt.wantPageSize {
				t.Errorf("PageSize = %d, want %d", body.Pagination.PageSize, tt.wantPageSize)
			}
			if body.Pagination.TotalItems != tt.wantTotalItems {
				t.Errorf("TotalItems = %d, want %d", body.Pagination.TotalItems, tt.wantTotalItems)
			}
			if body.Pagination.TotalPages != tt.wantTotalPages {
				t.Errorf("TotalPages = %d, want %d", body.Pagination.TotalPages, tt.wantTotalPages)
			}
		})
	}
}

func TestGetProductReviews_InvalidPagination(t *testing.T) {
	mux := setupHandler(t)

	tests := []struct {
		name string
		url  string
	}{
		{name: "page zero", url: "/v1/reviews/product-1?page=0"},
		{name: "negative page", url: "/v1/reviews/product-1?page=-1"},
		{name: "page_size zero", url: "/v1/reviews/product-1?page_size=0"},
		{name: "page_size over max", url: "/v1/reviews/product-1?page_size=101"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestDeleteReview_Success(t *testing.T) {
	mux := setupHandler(t)

	// Submit a review
	reqBody := handler.SubmitReviewRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Text:      "Great book!",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created handler.ReviewResponse
	_ = json.NewDecoder(createRec.Body).Decode(&created)

	// Delete it
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/reviews/"+created.ID, nil)
	deleteRec := httptest.NewRecorder()
	mux.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	// Verify it's gone
	getReq := httptest.NewRequest(http.MethodGet, "/v1/reviews/product-1", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	var body handler.ProductReviewsResponse
	_ = json.NewDecoder(getRec.Body).Decode(&body)
	if len(body.Reviews) != 0 {
		t.Errorf("expected 0 reviews after delete, got %d", len(body.Reviews))
	}
}

func TestDeleteReview_NotFound(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/v1/reviews/nonexistent-id", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
```

Also add `"fmt"` to the imports in the test file if not already present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/internal/adapter/inbound/http/... -v -run "TestGetProductReviews_Pagination|TestGetProductReviews_InvalidPagination|TestDeleteReview" -count=1`
Expected: compilation failure — handler uses old interface

- [ ] **Step 3: Update handler implementation**

Replace the full content of `services/reviews/internal/adapter/inbound/http/handler.go`:

```go
// Package http provides HTTP handlers and DTOs for the reviews service.
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

const (
	defaultPage     = 1
	defaultPageSize = 10
	maxPageSize     = 100
)

// Handler holds the HTTP handlers for the reviews service.
type Handler struct {
	svc port.ReviewService
}

// NewHandler creates a new HTTP handler with the given review service.
func NewHandler(svc port.ReviewService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the reviews routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/reviews/{id}", h.getProductReviews)
	mux.HandleFunc("POST /v1/reviews", h.submitReview)
	mux.HandleFunc("DELETE /v1/reviews/{id}", h.deleteReview)
}

func (h *Handler) getProductReviews(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	page, pageSize, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	reviews, total, err := h.svc.GetProductReviews(r.Context(), productID, page, pageSize)
	if err != nil {
		logger.Error("failed to get product reviews", "error", err, "product_id", productID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	reviewResponses := make([]ReviewResponse, 0, len(reviews))
	for _, review := range reviews {
		resp := ReviewResponse{
			ID:        review.ID,
			ProductID: review.ProductID,
			Reviewer:  review.Reviewer,
			Text:      review.Text,
		}
		if review.Rating != nil {
			resp.Rating = &ReviewRatingResponse{
				Stars:   review.Rating.Stars,
				Average: review.Rating.Average,
				Count:   review.Rating.Count,
			}
		}
		reviewResponses = append(reviewResponses, resp)
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	writeJSON(w, http.StatusOK, ProductReviewsResponse{
		ProductID: productID,
		Reviews:   reviewResponses,
		Pagination: PaginationResponse{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

func (h *Handler) submitReview(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req SubmitReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	review, err := h.svc.SubmitReview(r.Context(), req.ProductID, req.Reviewer, req.Text)
	if err != nil {
		logger.Warn("failed to submit review", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("review submitted", "review_id", review.ID, "product_id", review.ProductID)

	writeJSON(w, http.StatusCreated, ReviewResponse{
		ID:        review.ID,
		ProductID: review.ProductID,
		Reviewer:  review.Reviewer,
		Text:      review.Text,
	})
}

func (h *Handler) deleteReview(w http.ResponseWriter, r *http.Request) {
	reviewID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	// The Sensor forwards the review ID in the JSON body (event-driven path),
	// while direct API calls use the URL path param. Support both.
	if reviewID == "" {
		var body struct {
			ReviewID string `json:"review_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.ReviewID != "" {
			reviewID = body.ReviewID
		}
	}

	if reviewID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "review ID is required"})
		return
	}

	err := h.svc.DeleteReview(r.Context(), reviewID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "review not found"})
			return
		}
		logger.Error("failed to delete review", "error", err, "review_id", reviewID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	logger.Info("review deleted", "review_id", reviewID)
	w.WriteHeader(http.StatusNoContent)
}

func parsePagination(r *http.Request) (page, pageSize int, err error) {
	page = defaultPage
	pageSize = defaultPageSize

	if v := r.URL.Query().Get("page"); v != "" {
		page, err = strconv.Atoi(v)
		if err != nil || page < 1 {
			return 0, 0, errors.New("page must be a positive integer")
		}
	}

	if v := r.URL.Query().Get("page_size"); v != "" {
		pageSize, err = strconv.Atoi(v)
		if err != nil || pageSize < 1 || pageSize > maxPageSize {
			return 0, 0, errors.New("page_size must be between 1 and 100")
		}
	}

	return page, pageSize, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Run ALL reviews service tests**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/... -v -count=1`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/adapter/inbound/http/handler.go services/reviews/internal/adapter/inbound/http/handler_test.go
git commit -m "feat(reviews): add pagination and DELETE endpoint to handler"
```

---

## Task 9: Productpage — Update Reviews Client + Model

**Files:**
- Modify: `services/productpage/internal/client/reviews.go`
- Modify: `services/productpage/internal/model/product.go`

- [ ] **Step 1: Update ProductReviewsResponse DTO and add pagination + delete to client**

In `services/productpage/internal/client/reviews.go`, add the `PaginationResponse` type after `ProductReviewsResponse`, update `ProductReviewsResponse` to include it, update `GetProductReviews` to accept page/pageSize params, and add `DeleteReview`:

```go
// PaginationResponse contains pagination metadata from the reviews service.
type PaginationResponse struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// ProductReviewsResponse represents the reviews service aggregated response.
type ProductReviewsResponse struct {
	ProductID  string             `json:"product_id"`
	Reviews    []ReviewResponse   `json:"reviews"`
	Pagination PaginationResponse `json:"pagination"`
}
```

Update `GetProductReviews` to accept `page` and `pageSize`:

```go
// GetProductReviews fetches paginated reviews for a product.
func (c *ReviewsClient) GetProductReviews(ctx context.Context, productID string, page, pageSize int) (*ProductReviewsResponse, error) {
	url := fmt.Sprintf("%s/v1/reviews/%s?page=%d&page_size=%d", c.baseURL, productID, page, pageSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL comes from operator-controlled config, not user input
	if err != nil {
		return nil, fmt.Errorf("fetching reviews: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reviews service returned status %d", resp.StatusCode)
	}

	var body ProductReviewsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding reviews response: %w", err)
	}

	return &body, nil
}
```

Add `DeleteReview` method:

```go
// DeleteReview sends a delete request for a review.
func (c *ReviewsClient) DeleteReview(ctx context.Context, reviewID string) error {
	url := fmt.Sprintf("%s/v1/reviews/%s", c.baseURL, reviewID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL comes from operator-controlled config, not user input
	if err != nil {
		return fmt.Errorf("deleting review: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reviews service returned status %d", resp.StatusCode)
	}

	return nil
}
```

- [ ] **Step 2: Add Deleting field to ProductReview model**

In `services/productpage/internal/model/product.go`, add `Deleting bool` to `ProductReview`:

```go
// ProductReview is the view model for a single review.
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

- [ ] **Step 3: Verify it compiles (will fail — callers need updating)**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/productpage/internal/client/... ./services/productpage/internal/model/...`
Expected: compiles (client and model are leaf packages)

- [ ] **Step 4: Commit**

```bash
git add services/productpage/internal/client/reviews.go services/productpage/internal/model/product.go
git commit -m "feat(productpage): add pagination and delete to reviews client and model"
```

---

## Task 10: Pending Store — Add StoreDeleting + Update GetAndReconcile

**Files:**
- Modify: `services/productpage/internal/pending/pending.go`
- Modify: `services/productpage/internal/pending/redis.go`
- Modify: `services/productpage/internal/pending/noop.go`

- [ ] **Step 1: Update Store interface in pending.go**

Replace the full content of `services/productpage/internal/pending/pending.go`:

```go
// Package pending provides a pending review store for the productpage BFF.
// Pending reviews are stored in Redis and merged into read responses
// until the async CQRS write pipeline confirms them.
package pending

import (
	"context"
	"time"
)

// Review represents a review that has been submitted but not yet confirmed
// by the read path.
type Review struct {
	Reviewer  string `json:"reviewer"`
	Text      string `json:"text"`
	Stars     int    `json:"stars"`
	Timestamp int64  `json:"timestamp"`
}

// ConfirmedReview contains the fields used to match a pending review
// against a confirmed review from the read service.
type ConfirmedReview struct {
	Reviewer string
	Text     string
}

// Store defines operations for managing pending reviews.
type Store interface {
	// StorePending appends a pending review for the given product.
	StorePending(ctx context.Context, productID string, review Review) error

	// StoreDeleting marks a review as being deleted for the given product.
	StoreDeleting(ctx context.Context, productID string, reviewID string) error

	// GetAndReconcile returns pending reviews and deleting review IDs for a product
	// after reconciling against the confirmed reviews.
	// Pending reviews that match confirmed reviews are removed.
	// Deleting IDs that no longer appear in confirmed reviews are removed.
	GetAndReconcile(ctx context.Context, productID string, confirmed []ConfirmedReview, confirmedIDs []string) ([]Review, []string, error)
}

// NewReview creates a Review with the current timestamp.
func NewReview(reviewer, text string, stars int) Review {
	return Review{
		Reviewer:  reviewer,
		Text:      text,
		Stars:     stars,
		Timestamp: time.Now().Unix(),
	}
}
```

- [ ] **Step 2: Update RedisStore implementation**

Replace the full content of `services/productpage/internal/pending/redis.go`:

```go
package pending

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

const (
	pendingKeyPrefix  = "pending:reviews:"
	deletingKeyPrefix = "deleting:reviews:"
)

// RedisStore implements Store backed by Redis lists.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a RedisStore from a Redis URL (e.g. "redis://localhost:6379").
// The client is instrumented with OpenTelemetry tracing — every command
// produces a child span with operation name and key.
func NewRedisStore(redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	if err := redisotel.InstrumentTracing(client,
		redisotel.WithDBStatement(false),
	); err != nil {
		return nil, fmt.Errorf("instrumenting redis tracing: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// Ping verifies the connection to Redis.
func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// Close closes the Redis connection.
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// StorePending appends a pending review to the Redis list for the given product.
func (s *RedisStore) StorePending(ctx context.Context, productID string, review Review) error {
	data, err := json.Marshal(review)
	if err != nil {
		return fmt.Errorf("marshaling pending review: %w", err)
	}
	return s.client.RPush(ctx, pendingKeyPrefix+productID, data).Err()
}

// StoreDeleting marks a review as being deleted by adding its ID to a Redis set.
func (s *RedisStore) StoreDeleting(ctx context.Context, productID string, reviewID string) error {
	return s.client.SAdd(ctx, deletingKeyPrefix+productID, reviewID).Err()
}

// GetAndReconcile returns pending reviews and deleting review IDs after reconciliation.
func (s *RedisStore) GetAndReconcile(ctx context.Context, productID string, confirmed []ConfirmedReview, confirmedIDs []string) ([]Review, []string, error) {
	pendingReviews, err := s.reconcilePending(ctx, productID, confirmed)
	if err != nil {
		return nil, nil, fmt.Errorf("reconciling pending reviews: %w", err)
	}

	deletingIDs, err := s.reconcileDeleting(ctx, productID, confirmedIDs)
	if err != nil {
		return pendingReviews, nil, fmt.Errorf("reconciling deleting reviews: %w", err)
	}

	return pendingReviews, deletingIDs, nil
}

func (s *RedisStore) reconcilePending(ctx context.Context, productID string, confirmed []ConfirmedReview) ([]Review, error) {
	key := pendingKeyPrefix + productID

	vals, err := s.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("fetching pending reviews: %w", err)
	}

	if len(vals) == 0 {
		return nil, nil
	}

	confirmedSet := make(map[string]struct{}, len(confirmed))
	for _, c := range confirmed {
		confirmedSet[c.Reviewer+"\x00"+c.Text] = struct{}{}
	}

	var remaining []Review
	for _, raw := range vals {
		var r Review
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			continue // skip corrupted entries
		}

		matchKey := r.Reviewer + "\x00" + r.Text
		if _, found := confirmedSet[matchKey]; found {
			// Review confirmed — remove from Redis
			s.client.LRem(ctx, key, 1, raw)
			delete(confirmedSet, matchKey) // only remove first match
		} else {
			remaining = append(remaining, r)
		}
	}

	return remaining, nil
}

func (s *RedisStore) reconcileDeleting(ctx context.Context, productID string, confirmedIDs []string) ([]string, error) {
	key := deletingKeyPrefix + productID

	deletingIDs, err := s.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("fetching deleting review IDs: %w", err)
	}

	if len(deletingIDs) == 0 {
		return nil, nil
	}

	// Build set of confirmed IDs for fast lookup
	confirmedSet := make(map[string]struct{}, len(confirmedIDs))
	for _, id := range confirmedIDs {
		confirmedSet[id] = struct{}{}
	}

	var stillDeleting []string
	for _, id := range deletingIDs {
		if _, found := confirmedSet[id]; !found {
			// Review no longer in confirmed list — deletion confirmed
			s.client.SRem(ctx, key, id)
		} else {
			// Review still exists — still deleting
			stillDeleting = append(stillDeleting, id)
		}
	}

	return stillDeleting, nil
}
```

- [ ] **Step 3: Update NoopStore**

Replace the full content of `services/productpage/internal/pending/noop.go`:

```go
package pending

import "context"

// NoopStore is a PendingStore that does nothing. Used when REDIS_URL is unset.
type NoopStore struct{}

// StorePending is a no-op.
func (NoopStore) StorePending(_ context.Context, _ string, _ Review) error {
	return nil
}

// StoreDeleting is a no-op.
func (NoopStore) StoreDeleting(_ context.Context, _ string, _ string) error {
	return nil
}

// GetAndReconcile is a no-op that always returns nil.
func (NoopStore) GetAndReconcile(_ context.Context, _ string, _ []ConfirmedReview, _ []string) ([]Review, []string, error) {
	return nil, nil, nil
}
```

- [ ] **Step 4: Verify pending package compiles**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/productpage/internal/pending/...`
Expected: compiles

- [ ] **Step 5: Commit**

```bash
git add services/productpage/internal/pending/pending.go services/productpage/internal/pending/redis.go services/productpage/internal/pending/noop.go
git commit -m "feat(productpage): add StoreDeleting and updated reconciliation to pending store"
```

---

## Task 11: Productpage Handler — Pagination + DELETE + Deleting State

**Files:**
- Modify: `services/productpage/internal/handler/handler.go`
- Modify: `services/productpage/internal/handler/handler_test.go`

- [ ] **Step 1: Update handler — partialReviews with pagination + deleting, add partialDeleteReview**

In `services/productpage/internal/handler/handler.go`:

Register the new DELETE route in `RegisterRoutes`:

```go
// RegisterRoutes registers all productpage routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// HTML pages
	mux.HandleFunc("GET /", h.homePage)
	mux.HandleFunc("GET /products/{id}", h.productPage)

	// JSON API
	mux.HandleFunc("GET /v1/products/{id}", h.apiGetProduct)

	// HTMX partials
	mux.HandleFunc("GET /partials/details/{id}", h.partialDetails)
	mux.HandleFunc("GET /partials/reviews/{id}", h.partialReviews)
	mux.HandleFunc("POST /partials/rating", h.partialRatingSubmit)
	mux.HandleFunc("DELETE /partials/reviews/{id}", h.partialDeleteReview)
}
```

Update `apiGetProduct` to pass pagination params:

```go
	reviews, err := h.reviewsClient.GetProductReviews(r.Context(), productID, 1, 100)
```

Update `partialReviews` to handle pagination and deleting state:

```go
func (h *Handler) partialReviews(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p >= 1 {
			page = p
		}
	}

	reviews, err := h.reviewsClient.GetProductReviews(r.Context(), productID, page, 10)
	if err != nil {
		logger.Warn("failed to fetch reviews for partial", "product_id", productID, "error", err)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<p class="error">Failed to load reviews.</p>`))
		return
	}

	var confirmed []pending.ConfirmedReview
	var confirmedIDs []string
	var viewModels []model.ProductReview
	for _, review := range reviews.Reviews {
		confirmed = append(confirmed, pending.ConfirmedReview{
			Reviewer: review.Reviewer,
			Text:     review.Text,
		})
		confirmedIDs = append(confirmedIDs, review.ID)

		vm := model.ProductReview{
			ID:       review.ID,
			Reviewer: review.Reviewer,
			Text:     review.Text,
		}
		if review.Rating != nil {
			vm.Stars = review.Rating.Stars
			vm.Average = review.Rating.Average
			vm.Count = review.Rating.Count
		}
		viewModels = append(viewModels, vm)
	}

	// Merge pending reviews from Redis
	pendingReviews, deletingIDs, err := h.pendingStore.GetAndReconcile(r.Context(), productID, confirmed, confirmedIDs)
	if err != nil {
		logger.Warn("failed to get pending reviews", "product_id", productID, "error", err)
	}

	// Only show pending reviews on page 1
	if page == 1 {
		for _, pr := range pendingReviews {
			viewModels = append(viewModels, model.ProductReview{
				Reviewer: pr.Reviewer,
				Text:     pr.Text,
				Stars:    pr.Stars,
				Pending:  true,
			})
		}
	}

	// Mark reviews being deleted
	deletingSet := make(map[string]struct{}, len(deletingIDs))
	for _, id := range deletingIDs {
		deletingSet[id] = struct{}{}
	}
	for i := range viewModels {
		if _, found := deletingSet[viewModels[i].ID]; found {
			viewModels[i].Deleting = true
		}
	}

	hasPendingState := len(pendingReviews) > 0 || len(deletingIDs) > 0

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "reviews.html", struct {
		Reviews         []model.ProductReview
		HasPending      bool
		ProductID       string
		Page            int
		TotalPages      int
	}{
		Reviews:         viewModels,
		HasPending:      hasPendingState,
		ProductID:       productID,
		Page:            page,
		TotalPages:      reviews.Pagination.TotalPages,
	})
}
```

Add the new delete handler:

```go
func (h *Handler) partialDeleteReview(w http.ResponseWriter, r *http.Request) {
	reviewID := r.PathValue("id")
	productID := r.URL.Query().Get("product_id")
	logger := logging.FromContext(r.Context())

	if err := h.reviewsClient.DeleteReview(r.Context(), reviewID); err != nil {
		logger.Warn("failed to delete review", "error", err, "review_id", reviewID)
	}

	if productID != "" {
		if err := h.pendingStore.StoreDeleting(r.Context(), productID, reviewID); err != nil {
			logger.Warn("failed to store deleting state", "error", err)
		}
	}

	// Trigger a refresh of the reviews section
	if productID != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<div hx-get="/partials/reviews/%s" hx-trigger="load" hx-target="#reviews-section" hx-swap="innerHTML"></div>`, productID)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}
```

Add `"fmt"` to the handler imports.

- [ ] **Step 2: Add tests for delete handler and deleting state**

Add to `services/productpage/internal/handler/handler_test.go`:

```go
func TestPartialDeleteReview(t *testing.T) {
	detailsURL, _, ratingsURL := setupMockServers(t)

	// Mock reviews server that accepts DELETE
	deleteReceived := false
	reviewsMux := http.NewServeMux()
	reviewsMux.HandleFunc("GET /v1/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product_id": r.PathValue("id"),
			"reviews":    []any{},
			"pagination": map[string]any{"page": 1, "page_size": 10, "total_items": 0, "total_pages": 0},
		})
	})
	reviewsMux.HandleFunc("DELETE /v1/reviews/{id}", func(w http.ResponseWriter, _ *http.Request) {
		deleteReceived = true
		w.WriteHeader(http.StatusNoContent)
	})
	reviewsServer := httptest.NewServer(reviewsMux)
	t.Cleanup(reviewsServer.Close)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsServer.URL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	mr := miniredis.RunT(t)
	store, err := pending.NewRedisStore("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("failed to create redis store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, store, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/partials/reviews/review-1?product_id=product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if !deleteReceived {
		t.Error("expected DELETE request to be sent to reviews service")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "hx-get") {
		t.Error("expected HTMX refresh trigger in response")
	}
}

func TestPartialReviewsWithDeleting(t *testing.T) {
	detailsURL, _, ratingsURL := setupMockServers(t)

	// Mock reviews server that returns a review being deleted
	reviewsMux := http.NewServeMux()
	reviewsMux.HandleFunc("GET /v1/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product_id": r.PathValue("id"),
			"reviews": []map[string]any{
				{
					"id":         "review-1",
					"product_id": r.PathValue("id"),
					"reviewer":   "alice",
					"text":       "Great book!",
					"rating":     map[string]any{"stars": 5, "average": 4.5, "count": 10},
				},
			},
			"pagination": map[string]any{"page": 1, "page_size": 10, "total_items": 1, "total_pages": 1},
		})
	})
	reviewsServer := httptest.NewServer(reviewsMux)
	t.Cleanup(reviewsServer.Close)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsServer.URL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	mr := miniredis.RunT(t)
	store, err := pending.NewRedisStore("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("failed to create redis store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Mark review-1 as deleting
	ctx := context.Background()
	_ = store.StoreDeleting(ctx, "product-1", "review-1")

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, store, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/partials/reviews/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()

	if !strings.Contains(body, "review-deleting") {
		t.Errorf("expected 'review-deleting' CSS class in response")
	}
	if !strings.Contains(body, "Deleting") {
		t.Errorf("expected 'Deleting' label in response")
	}
	// HTMX polling should be active
	if !strings.Contains(body, "every 2s") {
		t.Errorf("expected HTMX polling trigger 'every 2s' in response")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail (template not yet updated)**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/productpage/internal/handler/... -v -run "TestPartialDeleteReview|TestPartialReviewsWithDeleting" -count=1`
Expected: compilation errors or test failures — template lacks deleting badge, handler not yet updated

- [ ] **Step 4: Apply all handler changes**

Apply the handler changes described in Step 1 to the actual file.

- [ ] **Step 5: Also fix the mock servers in existing tests to include pagination**

Update the reviews mock in `setupMockServers` to include pagination in the response:

```go
	reviewsMux.HandleFunc("GET /v1/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product_id": r.PathValue("id"),
			"reviews": []map[string]any{
				{
					"id":         "review-1",
					"product_id": r.PathValue("id"),
					"reviewer":   "alice",
					"text":       "Great book!",
					"rating":     map[string]any{"stars": 5, "average": 4.5, "count": 10},
				},
			},
			"pagination": map[string]any{"page": 1, "page_size": 10, "total_items": 1, "total_pages": 1},
		})
	})
```

- [ ] **Step 6: Commit handler changes (template changes in next task)**

```bash
git add services/productpage/internal/handler/handler.go services/productpage/internal/handler/handler_test.go
git commit -m "feat(productpage): add pagination, delete handler, and deleting state merge"
```

---

## Task 12: Templates — Pagination Controls + Delete Button + Deleting Badge

**Files:**
- Modify: `services/productpage/templates/partials/reviews.html`
- Modify: `services/productpage/templates/layout.html`

- [ ] **Step 1: Update reviews.html template**

Replace the full content of `services/productpage/templates/partials/reviews.html`:

```html
{{if .HasPending}}
<div hx-get="/partials/reviews/{{.ProductID}}"
     hx-trigger="every 2s"
     hx-target="#reviews-section"
     hx-swap="innerHTML">
{{end}}
{{if .Reviews}}
{{range .Reviews}}
<div class="review{{if .Pending}} review-pending{{end}}{{if .Deleting}} review-deleting{{end}}">
    <div class="review-header">
        <div class="reviewer">
            <div class="reviewer-avatar">{{slice .Reviewer 0 1}}</div>
            <div>
                <div class="reviewer-name">
                    {{.Reviewer}}
                    {{if .Pending}}
                    <span class="pending-badge">
                        <span class="pending-dot"></span>
                        Processing
                    </span>
                    {{end}}
                    {{if .Deleting}}
                    <span class="deleting-badge">
                        <span class="deleting-dot"></span>
                        Deleting...
                    </span>
                    {{end}}
                </div>
            </div>
        </div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
            {{if gt .Stars 0}}
            <div class="stars">
                {{$s := .Stars}}
                {{if ge $s 1}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
                {{if ge $s 2}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
                {{if ge $s 3}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
                {{if ge $s 4}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
                {{if ge $s 5}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
            </div>
            <span class="rating-count">{{.Stars}}/5</span>
            {{end}}
            {{if and .ID (not .Pending) (not .Deleting)}}
            <button class="btn-delete-review"
                    hx-delete="/partials/reviews/{{.ID}}?product_id={{$.ProductID}}"
                    hx-confirm="Are you sure you want to delete this review?"
                    hx-target="#reviews-section"
                    hx-swap="innerHTML">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg>
            </button>
            {{end}}
        </div>
    </div>
    <div class="review-text">{{.Text}}</div>
</div>
{{end}}
{{else}}
<p style="color: var(--text-muted); font-size: 0.9rem; padding: 0.5rem 0;">No reviews yet.</p>
{{end}}
{{if gt .TotalPages 1}}
<div class="pagination-controls">
    {{if gt .Page 1}}
    <button class="btn btn-sm"
            hx-get="/partials/reviews/{{.ProductID}}?page={{subtract .Page 1}}"
            hx-target="#reviews-section"
            hx-swap="innerHTML">&laquo; Prev</button>
    {{end}}
    <span class="pagination-info">Page {{.Page}} of {{.TotalPages}}</span>
    {{if lt .Page .TotalPages}}
    <button class="btn btn-sm"
            hx-get="/partials/reviews/{{.ProductID}}?page={{add .Page 1}}"
            hx-target="#reviews-section"
            hx-swap="innerHTML">Next &raquo;</button>
    {{end}}
</div>
{{end}}
{{if .HasPending}}
</div>
{{end}}
```

Note: The template uses `add` and `subtract` template functions. These need to be registered in the handler's template initialization. Update the `NewHandler` function in `handler.go` to include template functions:

```go
func NewHandler(
	detailsClient *client.DetailsClient,
	reviewsClient *client.ReviewsClient,
	ratingsClient *client.RatingsClient,
	pendingStore pending.Store,
	templateDir string,
) *Handler {
	funcMap := template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob(filepath.Join(templateDir, "*.html")))
	template.Must(tmpl.ParseGlob(filepath.Join(templateDir, "partials", "*.html")))

	return &Handler{
		detailsClient: detailsClient,
		reviewsClient: reviewsClient,
		ratingsClient: ratingsClient,
		pendingStore:  pendingStore,
		templates:     tmpl,
		templateDir:   templateDir,
	}
}
```

- [ ] **Step 2: Add CSS for deleting state and delete button**

In `services/productpage/templates/layout.html`, add after the `.pending-dot` animation block (after line 505):

```css
        /* Deleting review */
        .review-deleting {
            border: 1px dashed rgba(239, 68, 68, 0.4);
            border-radius: 6px;
            background: rgba(239, 68, 68, 0.03);
            padding: 0.75rem;
            margin-top: 0.5rem;
            opacity: 0.6;
        }

        .deleting-badge {
            display: inline-flex;
            align-items: center;
            gap: 0.3rem;
            color: #ef4444;
            font-size: 0.65rem;
            font-weight: 500;
            margin-left: 0.5rem;
        }

        .deleting-dot {
            width: 6px;
            height: 6px;
            border-radius: 50%;
            background: #ef4444;
            display: inline-block;
            animation: pending-pulse 1.5s ease-in-out infinite;
        }

        .btn-delete-review {
            background: none;
            border: 1px solid rgba(239, 68, 68, 0.3);
            border-radius: 4px;
            color: #ef4444;
            cursor: pointer;
            padding: 0.25rem 0.4rem;
            display: inline-flex;
            align-items: center;
            opacity: 0.5;
            transition: opacity 0.2s;
        }

        .btn-delete-review:hover {
            opacity: 1;
            background: rgba(239, 68, 68, 0.1);
        }

        .pagination-controls {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 1rem;
            padding: 1rem 0 0.5rem;
        }

        .pagination-info {
            color: var(--text-muted);
            font-size: 0.85rem;
        }

        .btn-sm {
            padding: 0.3rem 0.8rem;
            font-size: 0.8rem;
        }
```

- [ ] **Step 3: Run all productpage tests**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/productpage/... -v -count=1`
Expected: all tests PASS

- [ ] **Step 4: Run full test suite**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./... -count=1`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add services/productpage/templates/partials/reviews.html services/productpage/templates/layout.html services/productpage/internal/handler/handler.go
git commit -m "feat(productpage): add pagination controls, delete button, and deleting badge to templates"
```

---

## Task 13: Infrastructure — Delete EventSource + Sensor + HTTPRoute

**Files:**
- Create: `deploy/reviews/base/eventsource-delete.yaml`
- Modify: `deploy/reviews/base/sensor.yaml`
- Modify: `deploy/reviews/base/kustomization.yaml`
- Create: `deploy/reviews/overlays/local/eventsource-delete-patch.yaml`
- Create: `deploy/reviews/overlays/local/eventsource-delete-service.yaml`
- Modify: `deploy/reviews/overlays/local/kustomization.yaml`
- Modify: `deploy/reviews/overlays/local/sensor-patch.yaml`
- Modify: `deploy/gateway/overlays/local/httproutes.yaml`

- [ ] **Step 1: Create base EventSource for review-deleted**

Create `deploy/reviews/base/eventsource-delete.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: review-deleted
spec:
  eventBusName: kafka
  webhook:
    review-deleted:
      port: "12003"
      endpoint: /review-deleted
      method: DELETE
```

- [ ] **Step 2: Add delete triggers to base sensor**

In `deploy/reviews/base/sensor.yaml`, add a new dependency and two triggers. Replace the full content:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: review-submitted-sensor
spec:
  eventBusName: kafka
  dependencies:
    - name: review-submitted-dep
      eventSourceName: review-submitted
      eventName: review-submitted
    - name: review-deleted-dep
      eventSourceName: review-deleted
      eventName: review-deleted
  triggers:
    - template:
        name: create-review
        conditions: review-submitted-dep
        http:
          url: http://reviews/v1/reviews
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body
              dest: ""
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-review-submitted
        conditions: review-submitted-dep
        http:
          url: http://notification/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                dependencyName: review-submitted-dep
                value: "New review submitted"
              dest: body
            - src:
                dependencyName: review-submitted-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: review-submitted-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: delete-review
        conditions: review-deleted-dep
        http:
          url: http://reviews/v1/reviews
          method: DELETE
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: review_id
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-review-deleted
        conditions: review-deleted-dep
        http:
          url: http://notification/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: subject
            - src:
                dependencyName: review-deleted-dep
                value: "Review deleted"
              dest: body
            - src:
                dependencyName: review-deleted-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: review-deleted-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
```

- [ ] **Step 3: Update base kustomization to include new EventSource**

In `deploy/reviews/base/kustomization.yaml`, add the new EventSource:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - deployment.yaml
  - service.yaml
  - configmap.yaml
  - eventsource.yaml
  - eventsource-delete.yaml
  - sensor.yaml

commonLabels:
  app: reviews
  part-of: event-driven-bookinfo
```

- [ ] **Step 4: Create local overlay for delete EventSource**

Create `deploy/reviews/overlays/local/eventsource-delete-patch.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: review-deleted
spec:
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "review-deleted-eventsource"
  webhook:
    review-deleted:
      endpoint: /v1/reviews
      method: DELETE
```

- [ ] **Step 5: Create ClusterIP service for delete EventSource**

Create `deploy/reviews/overlays/local/eventsource-delete-service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: review-deleted-eventsource-svc
  namespace: bookinfo
spec:
  selector:
    eventsource-name: review-deleted
  ports:
    - port: 12003
      targetPort: 12003
```

- [ ] **Step 6: Update local overlay sensor patch to include delete triggers**

Replace the full content of `deploy/reviews/overlays/local/sensor-patch.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: review-submitted-sensor
spec:
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "review-submitted-sensor"
  triggers:
    - template:
        name: create-review
        conditions: review-submitted-dep
        http:
          url: http://reviews-write.bookinfo.svc.cluster.local/v1/reviews
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body
              dest: ""
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-review-submitted
        conditions: review-submitted-dep
        http:
          url: http://notification.bookinfo.svc.cluster.local/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                dependencyName: review-submitted-dep
                value: "New review submitted"
              dest: body
            - src:
                dependencyName: review-submitted-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: review-submitted-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: delete-review
        conditions: review-deleted-dep
        http:
          url: http://reviews-write.bookinfo.svc.cluster.local/v1/reviews
          method: DELETE
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: review_id
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-review-deleted
        conditions: review-deleted-dep
        http:
          url: http://notification.bookinfo.svc.cluster.local/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: subject
            - src:
                dependencyName: review-deleted-dep
                value: "Review deleted"
              dest: body
            - src:
                dependencyName: review-deleted-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: review-deleted-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
```

- [ ] **Step 7: Update local overlay kustomization**

Replace the full content of `deploy/reviews/overlays/local/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - ../../base
  - deployment-write.yaml
  - service-write.yaml
  - eventsource-service.yaml
  - eventsource-delete-service.yaml

patches:
  - path: deployment-read-patch.yaml
    target:
      kind: Deployment
      name: reviews
  - path: configmap-patch.yaml
    target:
      kind: ConfigMap
      name: reviews
  - path: eventsource-patch.yaml
    target:
      kind: EventSource
      name: review-submitted
  - path: eventsource-delete-patch.yaml
    target:
      kind: EventSource
      name: review-deleted
  - path: sensor-patch.yaml
    target:
      kind: Sensor
      name: review-submitted-sensor

images:
  - name: event-driven-bookinfo/reviews
    newTag: local
```

- [ ] **Step 8: Add HTTPRoutes for DELETE**

In `deploy/gateway/overlays/local/httproutes.yaml`, add two new routes. Append before the `productpage` catch-all route (before the last `---`):

After the `productpage-partials` POST route, add:

```yaml
---
# DELETE /partials/* → productpage (HTMX delete actions)
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: productpage-delete-partials
  namespace: bookinfo
spec:
  parentRefs:
    - name: default-gw
      namespace: platform
      sectionName: web
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /partials
          method: DELETE
      backendRefs:
        - name: productpage
          port: 80
---
# DELETE /v1/reviews/* → review-deleted EventSource (write path)
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: reviews-delete
  namespace: bookinfo
spec:
  parentRefs:
    - name: default-gw
      namespace: platform
      sectionName: web
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /v1/reviews
          method: DELETE
      backendRefs:
        - name: review-deleted-eventsource-svc
          port: 12003
```

- [ ] **Step 9: Verify kustomize build**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && kubectl kustomize deploy/reviews/overlays/local/ > /dev/null`
Expected: no errors

- [ ] **Step 10: Commit**

```bash
git add deploy/reviews/base/eventsource-delete.yaml deploy/reviews/base/sensor.yaml deploy/reviews/base/kustomization.yaml deploy/reviews/overlays/local/eventsource-delete-patch.yaml deploy/reviews/overlays/local/eventsource-delete-service.yaml deploy/reviews/overlays/local/kustomization.yaml deploy/reviews/overlays/local/sensor-patch.yaml deploy/gateway/overlays/local/httproutes.yaml
git commit -m "feat(deploy): add review-deleted EventSource, Sensor triggers, and HTTPRoutes"
```

---

## Task 14: k6 — Add Teardown Cleanup

**Files:**
- Modify: `test/k6/bookinfo-traffic.js`

- [ ] **Step 1: Add teardown function to clean up k6-generated reviews**

Add this `teardown` function at the end of `test/k6/bookinfo-traffic.js`, after the default function:

```javascript
export function teardown(data) {
  const productId = data.productId;
  if (!productId) return;

  console.log('Cleaning up k6-generated reviews...');

  // Paginate through all reviews and delete k6 ones
  let page = 1;
  let totalPages = 1;
  let deleted = 0;

  while (page <= totalPages) {
    const res = http.get(`${BASE_URL}/v1/reviews/${productId}?page=${page}&page_size=100`,
      { tags: { name: 'teardown: GET reviews' } });

    if (res.status !== 200) {
      console.log(`Failed to fetch reviews page ${page}: status ${res.status}`);
      break;
    }

    const body = JSON.parse(res.body);
    totalPages = body.pagination.total_pages;

    for (const review of body.reviews) {
      if (review.text && review.text.startsWith('k6 load test review')) {
        const delRes = http.del(`${BASE_URL}/v1/reviews/${review.id}`,
          null, { tags: { name: 'teardown: DELETE review' } });
        if (delRes.status === 204 || delRes.status === 200) {
          deleted++;
        }
      }
    }

    page++;
  }

  console.log(`Cleanup complete: deleted ${deleted} k6-generated reviews.`);
}
```

- [ ] **Step 2: Commit**

```bash
git add test/k6/bookinfo-traffic.js
git commit -m "feat(k6): add teardown cleanup for k6-generated reviews"
```

---

## Task 15: Final Verification

- [ ] **Step 1: Run full test suite with race detection**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -race -count=1 ./...`
Expected: all tests PASS, no race conditions

- [ ] **Step 2: Run linter**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && make lint`
Expected: no lint errors

- [ ] **Step 3: Build all services**

Run: `cd /Users/kaio.fellipe/Documents/git/others/go-http-server && make build-all`
Expected: all 5 binaries build successfully

- [ ] **Step 4: Verify kustomize builds for all affected overlays**

Run:
```bash
kubectl kustomize deploy/reviews/overlays/local/ > /dev/null && echo "reviews OK"
kubectl kustomize deploy/gateway/overlays/local/ > /dev/null && echo "gateway OK"
```
Expected: both OK

- [ ] **Step 5: Final commit if any fixes were needed**

If any fixes were needed during verification, commit them with an appropriate message.
