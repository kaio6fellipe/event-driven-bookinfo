// Package port defines the inbound and outbound interfaces for the reviews service.
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewService defines the inbound operations for the reviews domain.
type ReviewService interface {
	// GetProductReviews returns paginated reviews for a product, enriched with ratings data.
	// Returns the matching reviews and the total count for the product.
	GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]domain.Review, int, error)

	// SubmitReview creates and stores a new review. idempotencyKey deduplicates
	// repeated requests; if empty, a natural key is derived from productID, reviewer, and text.
	SubmitReview(ctx context.Context, productID, reviewer, text, idempotencyKey string) (*domain.Review, error)

	// DeleteReview removes a review by its ID.
	// Returns domain.ErrNotFound if the review does not exist.
	DeleteReview(ctx context.Context, id string) error
}
