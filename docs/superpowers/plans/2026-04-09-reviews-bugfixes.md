# Reviews Bugfixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix two bugs: (1) review text field labeled "optional" but required by domain, (2) all reviews show product average instead of individual reviewer scores.

**Architecture:** Bug 1 is a UI-only fix (HTML templates). Bug 2 requires propagating individual ratings from the ratings API through the reviews service to the productpage — updating domain types, port interface, ratings client, service layer, handler DTOs, productpage client/model/handler, and template rendering.

**Tech Stack:** Go, HTML templates, HTMX

---

### Task 1: Fix UI — mark review text as required

**Files:**
- Modify: `services/productpage/templates/productpage.html:62-63`
- Modify: `services/productpage/templates/partials/rating-form.html:38-39`

- [ ] **Step 1: Update the main form template**

In `services/productpage/templates/productpage.html`, change the review textarea from optional to required:

```html
<!-- OLD (lines 62-63): -->
                <div class="form-group">
                    <label for="text">Review (optional)</label>
                    <textarea id="text" name="text" placeholder="Write your review..."></textarea>
                </div>

<!-- NEW: -->
                <div class="form-group">
                    <label for="text">Review</label>
                    <textarea id="text" name="text" required placeholder="Write your review..."></textarea>
                </div>
```

- [ ] **Step 2: Update the error-state partial template**

In `services/productpage/templates/partials/rating-form.html`, apply the same change:

```html
<!-- OLD (lines 38-39): -->
        <label for="text">Review (optional)</label>
        <textarea id="text" name="text" placeholder="Write your review..."></textarea>

<!-- NEW: -->
        <label for="text">Review</label>
        <textarea id="text" name="text" required placeholder="Write your review..."></textarea>
```

- [ ] **Step 3: Run existing tests to confirm no regressions**

Run: `go test -race -count=1 ./services/productpage/...`
Expected: All tests PASS (template changes don't break existing tests).

- [ ] **Step 4: Commit**

```bash
git add services/productpage/templates/productpage.html services/productpage/templates/partials/rating-form.html
git commit -m "fix(productpage): mark review text field as required in UI"
```

---

### Task 2: Add RatingData domain type and update port interface

**Files:**
- Modify: `services/reviews/internal/core/domain/review.go`
- Modify: `services/reviews/internal/core/port/outbound.go`

- [ ] **Step 1: Add RatingData type to domain**

In `services/reviews/internal/core/domain/review.go`, add the new type after `ReviewRating`:

```go
// RatingData holds both product-level and per-reviewer rating data.
type RatingData struct {
	Average           float64
	Count             int
	IndividualRatings map[string]int // reviewer -> stars
}
```

- [ ] **Step 2: Update RatingsClient port interface**

In `services/reviews/internal/core/port/outbound.go`, change the return type of `GetProductRatings`:

```go
// RatingsClient defines the outbound operations for fetching ratings.
type RatingsClient interface {
	// GetProductRatings returns both product-level and per-reviewer rating data.
	GetProductRatings(ctx context.Context, productID string) (*domain.RatingData, error)
}
```

- [ ] **Step 3: Verify the code does NOT compile yet**

Run: `go build ./services/reviews/...`
Expected: FAIL — the ratings client adapter and test stubs still return `*domain.ReviewRating`, not `*domain.RatingData`.

This is expected. The next tasks will fix the compilation errors.

- [ ] **Step 4: Commit**

```bash
git add services/reviews/internal/core/domain/review.go services/reviews/internal/core/port/outbound.go
git commit -m "feat(reviews): add RatingData domain type and update port interface"
```

---

### Task 3: Update test stubs and write failing tests for individual ratings

**Files:**
- Modify: `services/reviews/internal/core/service/review_service_test.go`
- Modify: `services/reviews/internal/adapter/inbound/http/handler_test.go`

- [ ] **Step 1: Update the service test stub and add a failing test**

In `services/reviews/internal/core/service/review_service_test.go`, update the `stubRatingsClient` to match the new interface and add a test for individual ratings:

Replace the existing stub:

```go
// stubRatingsClient returns fixed rating data for testing.
type stubRatingsClient struct {
	rating *domain.ReviewRating
	err    error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.ReviewRating, error) {
	return s.rating, s.err
}
```

With:

```go
// stubRatingsClient returns fixed rating data for testing.
type stubRatingsClient struct {
	data *domain.RatingData
	err  error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.RatingData, error) {
	return s.data, s.err
}
```

Update `TestGetProductReviews_Empty` — change the stub setup:

```go
func TestGetProductReviews_Empty(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{Average: 0, Count: 0, IndividualRatings: map[string]int{}},
	}
	svc := service.NewReviewService(repo, client)

	reviews, err := svc.GetProductReviews(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}
}
```

Update `TestGetProductReviews_WithRatings` to verify individual stars per reviewer:

```go
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

	reviews, err := svc.GetProductReviews(context.Background(), "product-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviews) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(reviews))
	}

	// Each review should have its individual rating
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
```

Update `TestSubmitReview_Success` and `TestSubmitReview_ValidationError` — change the stub setup from `rating` to `data`:

```go
func TestSubmitReview_Success(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	// ... rest unchanged ...
}

func TestSubmitReview_ValidationError(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	// ... rest unchanged ...
}
```

These two functions don't change — they already use `&stubRatingsClient{}` with zero-value fields. The field name changed from `rating` to `data`, but these tests use the zero value so no update needed.

- [ ] **Step 2: Update the handler test stub**

In `services/reviews/internal/adapter/inbound/http/handler_test.go`, update the `stubRatingsClient` and `setupHandler`:

Replace the existing stub:

```go
type stubRatingsClient struct {
	rating *domain.ReviewRating
	err    error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.ReviewRating, error) {
	return s.rating, s.err
}

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		rating: &domain.ReviewRating{Average: 4.0, Count: 5},
	}
	svc := service.NewReviewService(repo, client)
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}
```

With:

```go
type stubRatingsClient struct {
	data *domain.RatingData
	err  error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.RatingData, error) {
	return s.data, s.err
}

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{
			Average: 4.0,
			Count:   5,
			IndividualRatings: map[string]int{
				"alice": 4,
			},
		},
	}
	svc := service.NewReviewService(repo, client)
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}
```

Update the assertion in `TestGetProductReviews_WithRatings` to also check `Stars`:

Find the assertions block (lines 145-154):

```go
	review := body.Reviews[0]
	if review.Rating == nil {
		t.Fatal("expected non-nil rating")
	}
	if review.Rating.Average != 4.0 {
		t.Errorf("Rating.Average = %f, want 4.0", review.Rating.Average)
	}
	if review.Rating.Count != 5 {
		t.Errorf("Rating.Count = %d, want 5", review.Rating.Count)
	}
```

Replace with:

```go
	review := body.Reviews[0]
	if review.Rating == nil {
		t.Fatal("expected non-nil rating")
	}
	if review.Rating.Stars != 4 {
		t.Errorf("Rating.Stars = %d, want 4", review.Rating.Stars)
	}
	if review.Rating.Average != 4.0 {
		t.Errorf("Rating.Average = %f, want 4.0", review.Rating.Average)
	}
	if review.Rating.Count != 5 {
		t.Errorf("Rating.Count = %d, want 5", review.Rating.Count)
	}
```

- [ ] **Step 3: Verify tests compile but the new assertions fail**

Run: `go test -race -count=1 ./services/reviews/...`
Expected: Tests compile. `TestGetProductReviews_WithRatings` FAILS on `Rating.Stars` assertions (service still assigns product average to all reviews, Stars is 0).

- [ ] **Step 4: Commit**

```bash
git add services/reviews/internal/core/service/review_service_test.go services/reviews/internal/adapter/inbound/http/handler_test.go
git commit -m "test(reviews): update stubs and add assertions for individual reviewer ratings"
```

---

### Task 4: Update ratings client adapter to parse individual ratings

**Files:**
- Modify: `services/reviews/internal/adapter/outbound/http/ratings_client.go`

- [ ] **Step 1: Update the ratings client to parse full response**

Replace the entire content of `services/reviews/internal/adapter/outbound/http/ratings_client.go`:

```go
// Package http provides an HTTP client for the ratings service used by reviews.
package http //nolint:revive // package name matches directory convention

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// individualRating mirrors a single rating entry from the ratings service.
type individualRating struct {
	ID        string `json:"id"`
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Stars     int    `json:"stars"`
}

// ratingsResponse mirrors the ratings service ProductRatingsResponse.
type ratingsResponse struct {
	ProductID string             `json:"product_id"`
	Average   float64            `json:"average"`
	Count     int                `json:"count"`
	Ratings   []individualRating `json:"ratings"`
}

// RatingsClient is an HTTP client that fetches ratings from the ratings service.
type RatingsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRatingsClient creates a new RatingsClient pointing to the given base URL.
func NewRatingsClient(baseURL string) *RatingsClient {
	return &RatingsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// GetProductRatings fetches both product-level and per-reviewer ratings from the ratings service.
func (c *RatingsClient) GetProductRatings(ctx context.Context, productID string) (*domain.RatingData, error) {
	url := fmt.Sprintf("%s/v1/ratings/%s", c.baseURL, productID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL comes from operator-controlled config, not user input
	if err != nil {
		return nil, fmt.Errorf("fetching ratings: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ratings service returned status %d", resp.StatusCode)
	}

	var body ratingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding ratings response: %w", err)
	}

	individual := make(map[string]int, len(body.Ratings))
	for _, r := range body.Ratings {
		individual[r.Reviewer] = r.Stars
	}

	return &domain.RatingData{
		Average:           body.Average,
		Count:             body.Count,
		IndividualRatings: individual,
	}, nil
}
```

- [ ] **Step 2: Verify the code compiles**

Run: `go build ./services/reviews/...`
Expected: FAIL — `review_service.go` still uses the old `*domain.ReviewRating` return type in its logic. This is expected.

- [ ] **Step 3: Commit**

```bash
git add services/reviews/internal/adapter/outbound/http/ratings_client.go
git commit -m "feat(reviews): parse individual ratings from ratings API response"
```

---

### Task 5: Update service layer to assign individual ratings per review

**Files:**
- Modify: `services/reviews/internal/core/service/review_service.go`

- [ ] **Step 1: Update GetProductReviews to use individual ratings**

Replace the `GetProductReviews` method in `services/reviews/internal/core/service/review_service.go`:

```go
// GetProductReviews returns all reviews for a product, enriched with ratings data.
func (s *ReviewService) GetProductReviews(ctx context.Context, productID string) ([]domain.Review, error) {
	reviews, err := s.repo.FindByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("finding reviews for product %s: %w", productID, err)
	}

	// Fetch ratings from the ratings service
	ratingData, err := s.ratingsClient.GetProductRatings(ctx, productID)
	if err != nil {
		logger := logging.FromContext(ctx)
		logger.Warn("failed to fetch ratings, returning reviews without ratings",
			slog.String("product_id", productID),
			slog.String("error", err.Error()),
		)
		return reviews, nil
	}

	// Enrich each review with product-level stats and individual reviewer score
	for i := range reviews {
		reviews[i].Rating = &domain.ReviewRating{
			Stars:   ratingData.IndividualRatings[reviews[i].Reviewer],
			Average: ratingData.Average,
			Count:   ratingData.Count,
		}
	}

	return reviews, nil
}
```

Key change: instead of assigning the same `*domain.ReviewRating` pointer to all reviews, we create a new `ReviewRating` per review with `Stars` looked up from `ratingData.IndividualRatings[reviewer]`. If the reviewer has no rating, the map returns 0.

- [ ] **Step 2: Run service tests**

Run: `go test -race -count=1 ./services/reviews/internal/core/service/...`
Expected: All tests PASS — including the new individual ratings assertions.

- [ ] **Step 3: Commit**

```bash
git add services/reviews/internal/core/service/review_service.go
git commit -m "fix(reviews): assign individual reviewer ratings instead of product average"
```

---

### Task 6: Update handler DTO to include Stars field

**Files:**
- Modify: `services/reviews/internal/adapter/inbound/http/dto.go`
- Modify: `services/reviews/internal/adapter/inbound/http/handler.go`

- [ ] **Step 1: Add Stars to ReviewRatingResponse DTO**

In `services/reviews/internal/adapter/inbound/http/dto.go`, update `ReviewRatingResponse`:

```go
// ReviewRatingResponse represents rating data embedded in a review response.
type ReviewRatingResponse struct {
	Stars   int     `json:"stars"`
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}
```

- [ ] **Step 2: Map Stars in the handler**

In `services/reviews/internal/adapter/inbound/http/handler.go`, update the rating mapping in `getProductReviews` (line 48-51):

```go
		if review.Rating != nil {
			resp.Rating = &ReviewRatingResponse{
				Stars:   review.Rating.Stars,
				Average: review.Rating.Average,
				Count:   review.Rating.Count,
			}
		}
```

- [ ] **Step 3: Run handler tests**

Run: `go test -race -count=1 ./services/reviews/internal/adapter/inbound/http/...`
Expected: All tests PASS — including the new `Rating.Stars` assertion.

- [ ] **Step 4: Run full reviews service tests**

Run: `go test -race -count=1 ./services/reviews/...`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/adapter/inbound/http/dto.go services/reviews/internal/adapter/inbound/http/handler.go
git commit -m "feat(reviews): expose individual stars in review API response"
```

---

### Task 7: Update productpage to render individual scores

**Files:**
- Modify: `services/productpage/internal/client/reviews.go`
- Modify: `services/productpage/internal/model/product.go`
- Modify: `services/productpage/internal/handler/handler.go:179-188`
- Modify: `services/productpage/templates/partials/reviews.html`
- Modify: `services/productpage/internal/handler/handler_test.go`

- [ ] **Step 1: Update the mock server in productpage tests**

In `services/productpage/internal/handler/handler_test.go`, update the reviews mock server (inside `setupMockServers`) to include `stars` in the rating:

Find (lines 69-76):

```go
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product_id": r.PathValue("id"),
			"reviews": []map[string]any{
				{
					"id":         "review-1",
					"product_id": r.PathValue("id"),
					"reviewer":   "alice",
					"text":       "Great book!",
					"rating":     map[string]any{"average": 4.5, "count": 10},
				},
			},
		})
```

Replace with:

```go
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
		})
```

Update `TestPartialReviews` to assert on the individual star count rendered in the template. Add after the existing assertions (after line 189):

```go
	// Should render 5 filled stars for alice's individual score
	filledStars := strings.Count(body, "star-filled")
	if filledStars != 5 {
		t.Errorf("expected 5 filled stars, got %d", filledStars)
	}
	// Should show "5/5" individual score
	if !strings.Contains(body, "5/5") {
		t.Errorf("expected '5/5' score display in response, got:\n%s", body)
	}
```

- [ ] **Step 2: Verify test fails**

Run: `go test -race -count=1 -run TestPartialReviews ./services/productpage/...`
Expected: FAIL — the `Stars` field isn't parsed yet, template still uses `Average`.

- [ ] **Step 3: Add Stars to productpage client DTO**

In `services/productpage/internal/client/reviews.go`, update `ReviewRatingResponse`:

```go
// ReviewRatingResponse represents rating data from the reviews service.
type ReviewRatingResponse struct {
	Stars   int     `json:"stars"`
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}
```

- [ ] **Step 4: Add Stars to productpage view model**

In `services/productpage/internal/model/product.go`, update `ProductReview`:

```go
// ProductReview is the view model for a single review.
type ProductReview struct {
	ID       string
	Reviewer string
	Text     string
	Stars    int
	Average  float64
	Count    int
}
```

- [ ] **Step 5: Map Stars in productpage handler**

In `services/productpage/internal/handler/handler.go`, update the `partialReviews` function (lines 179-189):

```go
	var viewModels []model.ProductReview
	for _, review := range reviews.Reviews {
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
```

- [ ] **Step 6: Update reviews.html template to render individual Stars**

Replace the entire content of `services/productpage/templates/partials/reviews.html`:

```html
{{if .}}
{{range .}}
<div class="review">
    <div class="review-header">
        <div class="reviewer">
            <div class="reviewer-avatar">{{slice .Reviewer 0 1}}</div>
            <div>
                <div class="reviewer-name">{{.Reviewer}}</div>
            </div>
        </div>
        {{if gt .Stars 0}}
        <div style="display: flex; align-items: center; gap: 0.5rem;">
            <div class="stars">
                {{$s := .Stars}}
                {{if ge $s 1}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
                {{if ge $s 2}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
                {{if ge $s 3}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
                {{if ge $s 4}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
                {{if ge $s 5}}<svg class="star star-filled" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{else}}<svg class="star star-empty" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>{{end}}
            </div>
            <span class="rating-count">{{.Stars}}/5</span>
        </div>
        {{end}}
    </div>
    <div class="review-text">{{.Text}}</div>
</div>
{{end}}
{{else}}
<p style="color: var(--text-muted); font-size: 0.9rem; padding: 0.5rem 0;">No reviews yet.</p>
{{end}}
```

Key changes:
- `{{if gt .Count 0}}` → `{{if gt .Stars 0}}` — show stars only if reviewer has a rating
- `{{$a := .Average}}` → `{{$s := .Stars}}` — use individual integer stars
- `{{if ge $a 1.0}}` → `{{if ge $s 1}}` — integer comparison instead of float
- `{{printf "%.1f" .Average}}` → `{{.Stars}}/5` — show individual score

- [ ] **Step 7: Run productpage tests**

Run: `go test -race -count=1 ./services/productpage/...`
Expected: All tests PASS — including the new filled-stars and "5/5" assertions.

- [ ] **Step 8: Commit**

```bash
git add services/productpage/internal/client/reviews.go services/productpage/internal/model/product.go services/productpage/internal/handler/handler.go services/productpage/templates/partials/reviews.html services/productpage/internal/handler/handler_test.go
git commit -m "fix(productpage): render individual reviewer scores instead of product average"
```

---

### Task 8: Run full test suite and verify

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite**

Run: `go test -race -count=1 ./...`
Expected: All tests PASS across all services.

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: No lint errors.

- [ ] **Step 3: Build all binaries**

Run: `make build-all`
Expected: All 5 service binaries build successfully.
