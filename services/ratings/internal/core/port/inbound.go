// file: services/ratings/internal/core/port/inbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingService defines the inbound operations for the ratings domain.
type RatingService interface {
	// GetProductRatings returns all ratings for a given product ID.
	GetProductRatings(ctx context.Context, productID string) (*domain.ProductRatings, error)

	// SubmitRating creates and stores a new rating.
	SubmitRating(ctx context.Context, productID, reviewer string, stars int) (*domain.Rating, error)
}
