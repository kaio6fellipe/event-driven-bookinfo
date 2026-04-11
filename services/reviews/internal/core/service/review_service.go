// Package service implements the business logic for the reviews service.
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

// GetProductReviews returns paginated reviews for a product, enriched with ratings data.
func (s *ReviewService) GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]domain.Review, int, error) {
	offset := (page - 1) * pageSize

	reviews, total, err := s.repo.FindByProductID(ctx, productID, offset, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("finding reviews for product %s: %w", productID, err)
	}

	ratingData, err := s.ratingsClient.GetProductRatings(ctx, productID)
	if err != nil {
		logger := logging.FromContext(ctx)
		logger.Warn("failed to fetch ratings, returning reviews without ratings",
			slog.String("product_id", productID),
			slog.String("error", err.Error()),
		)
		return reviews, total, nil
	}

	for i := range reviews {
		reviews[i].Rating = &domain.ReviewRating{
			Stars:   ratingData.IndividualRatings[reviews[i].Reviewer],
			Average: ratingData.Average,
			Count:   ratingData.Count,
		}
	}

	return reviews, total, nil
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

// DeleteReview removes a review by its ID.
func (s *ReviewService) DeleteReview(ctx context.Context, id string) error {
	if err := s.repo.DeleteByID(ctx, id); err != nil {
		return fmt.Errorf("deleting review %s: %w", id, err)
	}
	return nil
}
