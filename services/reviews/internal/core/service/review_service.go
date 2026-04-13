// Package service implements the business logic for the reviews service.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

// ErrAlreadyProcessed signals that an idempotent request was previously processed.
var ErrAlreadyProcessed = errors.New("request already processed")

// ReviewService implements the port.ReviewService interface.
type ReviewService struct {
	repo          port.ReviewRepository
	ratingsClient port.RatingsClient
	idempotency   idempotency.Store
}

// NewReviewService creates a new ReviewService.
func NewReviewService(repo port.ReviewRepository, ratingsClient port.RatingsClient, idem idempotency.Store) *ReviewService {
	return &ReviewService{
		repo:          repo,
		ratingsClient: ratingsClient,
		idempotency:   idem,
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

// SubmitReview creates and persists a new review. Deduplicates on idempotencyKey
// (falls back to a natural key derived from productID+reviewer+text).
func (s *ReviewService) SubmitReview(ctx context.Context, productID, reviewer, text, idempotencyKey string) (*domain.Review, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, text)

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}
	if alreadyProcessed {
		logger := logging.FromContext(ctx)
		logger.Info("review submit skipped: already processed", slog.String("idempotency_key", key))
		return nil, ErrAlreadyProcessed
	}

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
