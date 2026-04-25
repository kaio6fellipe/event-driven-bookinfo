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
	publisher     port.EventPublisher
}

// NewReviewService creates a new ReviewService.
func NewReviewService(repo port.ReviewRepository, ratingsClient port.RatingsClient, idem idempotency.Store, publisher port.EventPublisher) *ReviewService {
	return &ReviewService{
		repo:          repo,
		ratingsClient: ratingsClient,
		idempotency:   idem,
		publisher:     publisher,
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

// SubmitReview creates and persists a new review, then publishes a ReviewSubmittedEvent.
// Always publishes (even on idempotency dedup) for retry safety.
func (s *ReviewService) SubmitReview(ctx context.Context, productID, reviewer, text, idempotencyKey string) (*domain.Review, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, text)

	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}

	if !alreadyProcessed {
		if err := s.repo.Save(ctx, review); err != nil {
			return nil, fmt.Errorf("saving review: %w", err)
		}
	} else {
		logging.FromContext(ctx).Info("review submit skipped: already processed", slog.String("idempotency_key", key))
	}

	evt := domain.ReviewSubmittedEvent{
		ID:             review.ID,
		ProductID:      review.ProductID,
		Reviewer:       review.Reviewer,
		Text:           review.Text,
		IdempotencyKey: key,
	}
	if err := s.publisher.PublishReviewSubmitted(ctx, evt); err != nil {
		return nil, fmt.Errorf("publishing review-submitted event: %w", err)
	}

	if alreadyProcessed {
		return nil, ErrAlreadyProcessed
	}
	return review, nil
}

// DeleteReview removes a review by its ID and publishes a ReviewDeletedEvent.
// Idempotent: if the review does not exist, returns nil and still publishes.
// This is required for Option B retry semantics.
func (s *ReviewService) DeleteReview(ctx context.Context, id string) error {
	if err := s.repo.DeleteByID(ctx, id); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("deleting review %s: %w", id, err)
	}

	evt := domain.ReviewDeletedEvent{
		ReviewID:       id,
		IdempotencyKey: "review-deleted-" + id,
	}
	if err := s.publisher.PublishReviewDeleted(ctx, evt); err != nil {
		return fmt.Errorf("publishing review-deleted event: %w", err)
	}
	return nil
}
