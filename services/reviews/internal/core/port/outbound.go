// Package port defines the inbound and outbound interfaces for the reviews service.
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

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

// RatingsClient defines the outbound operations for fetching ratings.
type RatingsClient interface {
	// GetProductRatings returns both product-level and per-reviewer rating data.
	GetProductRatings(ctx context.Context, productID string) (*domain.RatingData, error)
}
