# Reviews Bugfixes: Required Text Label + Individual Scores

**Date:** 2026-04-09
**Status:** Approved

## Problem

Two bugs in the reviews/ratings display flow:

1. **Review text labeled "optional" but required by domain** — The HTML form labels the review textarea as "Review (optional)", but `domain.NewReview()` rejects empty text with `"review text is required"`. In the k8s CQRS flow, the POST goes through the Argo Events webhook (returns 200 OK immediately), then the Sensor triggers the reviews write service which silently rejects the review.

2. **All reviews show the product average instead of individual scores** — The ratings service returns individual `ratings[]` per reviewer, but the reviews service `RatingsClient` only extracts the product-level `average` and `count`, then assigns the same average to every review.

## Approach

### Bug 1: Fix UI to match domain (text is required)

The domain validation is correct — reviews should have text. Fix the UI labels and add HTML validation.

**Files changed:**
- `services/productpage/templates/productpage.html` — Remove "(optional)" from label, add `required` attribute to textarea
- `services/productpage/templates/partials/rating-form.html` — Same changes (error-state re-render of the form)

### Bug 2: Show individual reviewer scores (Approach A — match by reviewer)

The ratings API already returns individual ratings in `ratings[]`. The reviews service just needs to parse and use them.

**Data flow (current, broken):**
1. Reviews service calls `GET /v1/ratings/{product_id}`
2. Ratings API returns `{average: 3.5, count: 4, ratings: [{reviewer: "A", stars: 5}, ...]}`
3. Reviews `RatingsClient` extracts only `average` and `count`, ignores `ratings[]`
4. Same average assigned to every review

**Data flow (fixed):**
1. Reviews service calls `GET /v1/ratings/{product_id}` (no API change)
2. Ratings API returns same response (no change)
3. Reviews `RatingsClient` parses full `ratings[]` array, builds `map[reviewer]stars`
4. Returns both product-level stats AND per-reviewer ratings
5. Service layer matches each review's `Reviewer` to their individual rating
6. Each review gets its own `Stars` value
7. Handler serializes `stars` field per review
8. Productpage parses `stars` and renders individual scores

**Files changed (reviews service):**

| File | Change |
|------|--------|
| `services/reviews/internal/adapter/outbound/http/ratings_client.go` | Parse `ratings[]` from response. Add `individualRating` struct. Return `ProductRatingData` containing both product stats and `map[string]int` (reviewer -> stars). |
| `services/reviews/internal/core/port/outbound.go` | Update `RatingsClient` interface: `GetProductRatings` returns a new `RatingData` type with product average + individual ratings map. |
| `services/reviews/internal/core/domain/review.go` | No structural change. `ReviewRating.Stars` field already exists — it will now be populated. |
| `services/reviews/internal/core/service/review_service.go` | In `GetProductReviews()`, use individual ratings map to set `Stars` per review. Fall back to 0 if reviewer has no rating. Keep `Average`/`Count` as product-level. |
| `services/reviews/internal/adapter/inbound/http/dto.go` | Add `Stars int` field to `ReviewRatingResponse`. |
| `services/reviews/internal/adapter/inbound/http/handler.go` | Map `review.Rating.Stars` into response DTO. |

**Files changed (productpage service):**

| File | Change |
|------|--------|
| `services/productpage/internal/client/reviews.go` | Add `Stars int` to `ReviewRatingResponse`. |
| `services/productpage/internal/model/product.go` | Add `Stars int` to `ProductReview`. |
| `services/productpage/internal/handler/handler.go` | Map `review.Rating.Stars` into view model in `partialReviews()`. |
| `services/productpage/templates/partials/reviews.html` | Render stars based on individual `.Stars` integer instead of `.Average` float. |

## Interface Changes

### Reviews service outbound port (new return type)

The current `RatingsClient` interface returns `*domain.ReviewRating`. This needs to change to carry both product-level stats and individual ratings:

```go
// RatingData holds both product-level and per-reviewer rating data.
type RatingData struct {
    Average           float64
    Count             int
    IndividualRatings map[string]int // reviewer -> stars
}

type RatingsClient interface {
    GetProductRatings(ctx context.Context, productID string) (*RatingData, error)
}
```

`RatingData` lives in the domain package since the port depends on domain types.

### Reviews API response (additive change)

```json
{
  "product_id": "1",
  "reviews": [
    {
      "id": "abc",
      "product_id": "1",
      "reviewer": "Alice",
      "text": "Great book",
      "rating": {
        "stars": 5,
        "average": 3.5,
        "count": 4
      }
    }
  ]
}
```

The `stars` field is new. `average` and `count` remain for backward compatibility (product-level stats).

## Template Rendering Change

Current `reviews.html` uses `{{$a := .Average}}` and `{{if ge $a 1.0}}` for float comparison. The fix switches to integer comparison using `.Stars`:

```
{{$s := .Stars}}
{{if ge $s 1}}...filled...{{else}}...empty...{{end}}
{{if ge $s 2}}...filled...{{else}}...empty...{{end}}
...
```

The numeric display changes from `{{printf "%.1f" .Average}}` to `{{.Stars}}/5`.

## Testing

- **Reviews domain**: existing tests still pass (no domain changes)
- **Reviews handler**: update `getProductReviews` test to verify `stars` field in response
- **Reviews service**: update `GetProductReviews` test to verify per-reviewer rating matching
- **Productpage handler**: update `partialReviews` test to verify `Stars` field mapping
- **E2E (k8s)**: submit rating + review for a product, verify the review page shows the individual score

## Out of Scope

- Changing the ratings service API
- Adding a per-reviewer ratings endpoint
- Storing ratings within the reviews domain
- Half-star or fractional individual ratings (ratings are always integer 1-5)
