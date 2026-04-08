// Package memory provides an in-memory implementation of the ratings repository.
package memory

import (
	"context"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingRepository is an in-memory implementation of port.RatingRepository.
type RatingRepository struct {
	mu      sync.RWMutex
	ratings []domain.Rating
}

// NewRatingRepository creates a new in-memory rating repository.
func NewRatingRepository() *RatingRepository {
	return &RatingRepository{
		ratings: make([]domain.Rating, 0),
	}
}

// FindByProductID returns all ratings for the given product ID.
func (r *RatingRepository) FindByProductID(_ context.Context, productID string) ([]domain.Rating, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []domain.Rating
	for _, rating := range r.ratings {
		if rating.ProductID == productID {
			result = append(result, rating)
		}
	}

	return result, nil
}

// Save persists a rating in memory.
func (r *RatingRepository) Save(_ context.Context, rating *domain.Rating) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ratings = append(r.ratings, *rating)
	return nil
}
