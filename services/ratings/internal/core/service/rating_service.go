// file: services/ratings/internal/core/service/rating_service.go
package service

import (
	"context"
	"fmt"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
)

// RatingService implements the port.RatingService interface.
type RatingService struct {
	repo port.RatingRepository
}

// NewRatingService creates a new RatingService with the given repository.
func NewRatingService(repo port.RatingRepository) *RatingService {
	return &RatingService{repo: repo}
}

// GetProductRatings returns all ratings aggregated for a product.
func (s *RatingService) GetProductRatings(ctx context.Context, productID string) (*domain.ProductRatings, error) {
	ratings, err := s.repo.FindByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("finding ratings for product %s: %w", productID, err)
	}

	return &domain.ProductRatings{
		ProductID: productID,
		Ratings:   ratings,
	}, nil
}

// SubmitRating creates and persists a new rating.
func (s *RatingService) SubmitRating(ctx context.Context, productID, reviewer string, stars int) (*domain.Rating, error) {
	rating, err := domain.NewRating(productID, reviewer, stars)
	if err != nil {
		return nil, fmt.Errorf("creating rating: %w", err)
	}

	if err := s.repo.Save(ctx, rating); err != nil {
		return nil, fmt.Errorf("saving rating: %w", err)
	}

	return rating, nil
}
