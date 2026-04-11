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
