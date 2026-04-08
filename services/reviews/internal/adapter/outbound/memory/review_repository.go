// file: services/reviews/internal/adapter/outbound/memory/review_repository.go
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

// FindByProductID returns all reviews for the given product ID.
func (r *ReviewRepository) FindByProductID(_ context.Context, productID string) ([]domain.Review, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []domain.Review
	for _, review := range r.reviews {
		result = append(result, domain.Review{
			ID:        review.ID,
			ProductID: review.ProductID,
			Reviewer:  review.Reviewer,
			Text:      review.Text,
		})
	}

	filtered := make([]domain.Review, 0)
	for _, review := range result {
		if review.ProductID == productID {
			filtered = append(filtered, review)
		}
	}

	return filtered, nil
}

// Save persists a review in memory.
func (r *ReviewRepository) Save(_ context.Context, review *domain.Review) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reviews = append(r.reviews, *review)
	return nil
}
