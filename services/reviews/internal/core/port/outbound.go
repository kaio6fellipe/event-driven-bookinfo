// file: services/reviews/internal/core/port/outbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewRepository defines the outbound persistence operations for reviews.
type ReviewRepository interface {
	// FindByProductID returns all reviews for a given product ID.
	FindByProductID(ctx context.Context, productID string) ([]domain.Review, error)

	// Save persists a review.
	Save(ctx context.Context, review *domain.Review) error
}

// RatingsClient defines the outbound operations for fetching ratings.
type RatingsClient interface {
	// GetProductRatings returns the aggregated rating info for a product.
	GetProductRatings(ctx context.Context, productID string) (*domain.ReviewRating, error)
}
