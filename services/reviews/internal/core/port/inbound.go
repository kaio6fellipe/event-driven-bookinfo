// Package port defines the inbound and outbound interfaces for the reviews service.
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewService defines the inbound operations for the reviews domain.
type ReviewService interface {
	// GetProductReviews returns all reviews for a product, enriched with ratings data.
	GetProductReviews(ctx context.Context, productID string) ([]domain.Review, error)

	// SubmitReview creates and stores a new review.
	SubmitReview(ctx context.Context, productID, reviewer, text string) (*domain.Review, error)
}
