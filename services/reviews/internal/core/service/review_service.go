// file: services/reviews/internal/core/service/review_service.go
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

// ReviewService implements the port.ReviewService interface.
type ReviewService struct {
	repo          port.ReviewRepository
	ratingsClient port.RatingsClient
}

// NewReviewService creates a new ReviewService.
func NewReviewService(repo port.ReviewRepository, ratingsClient port.RatingsClient) *ReviewService {
	return &ReviewService{
		repo:          repo,
		ratingsClient: ratingsClient,
	}
}

// GetProductReviews returns all reviews for a product, enriched with ratings data.
func (s *ReviewService) GetProductReviews(ctx context.Context, productID string) ([]domain.Review, error) {
	reviews, err := s.repo.FindByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("finding reviews for product %s: %w", productID, err)
	}

	// Fetch ratings from the ratings service
	rating, err := s.ratingsClient.GetProductRatings(ctx, productID)
	if err != nil {
		logger := logging.FromContext(ctx)
		logger.Warn("failed to fetch ratings, returning reviews without ratings",
			slog.String("product_id", productID),
			slog.String("error", err.Error()),
		)
		return reviews, nil
	}

	// Enrich each review with the product-level rating
	for i := range reviews {
		reviews[i].Rating = rating
	}

	return reviews, nil
}

// SubmitReview creates and persists a new review.
func (s *ReviewService) SubmitReview(ctx context.Context, productID, reviewer, text string) (*domain.Review, error) {
	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}

	if err := s.repo.Save(ctx, review); err != nil {
		return nil, fmt.Errorf("saving review: %w", err)
	}

	return review, nil
}
