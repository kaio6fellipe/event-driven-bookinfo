// Package port defines the inbound and outbound interfaces for the ratings service.
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingRepository defines the outbound persistence operations for ratings.
type RatingRepository interface {
	// FindByProductID returns all ratings for a given product ID.
	FindByProductID(ctx context.Context, productID string) ([]domain.Rating, error)

	// Save persists a rating.
	Save(ctx context.Context, rating *domain.Rating) error
}
