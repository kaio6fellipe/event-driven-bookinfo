// Package service implements the business logic for the ratings service.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
)

// ErrAlreadyProcessed signals that an idempotent request was previously processed.
var ErrAlreadyProcessed = errors.New("request already processed")

// RatingService implements the port.RatingService interface.
type RatingService struct {
	repo        port.RatingRepository
	idempotency idempotency.Store
	publisher   port.EventPublisher
}

// NewRatingService creates a new RatingService with the given repository, idempotency store, and event publisher.
func NewRatingService(repo port.RatingRepository, idem idempotency.Store, publisher port.EventPublisher) *RatingService {
	return &RatingService{repo: repo, idempotency: idem, publisher: publisher}
}

// GetProductRatings returns all ratings aggregated for a product.
func (s *RatingService) GetProductRatings(ctx context.Context, productID string) (*domain.ProductRatings, error) {
	ratings, err := s.repo.FindByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("finding ratings for product %s: %w", productID, err)
	}
	return &domain.ProductRatings{ProductID: productID, Ratings: ratings}, nil
}

// SubmitRating creates and persists a new rating, then publishes a RatingSubmittedEvent.
// Always publishes (even on idempotency dedup) for retry safety.
func (s *RatingService) SubmitRating(ctx context.Context, productID, reviewer string, stars int, idempotencyKey string) (*domain.Rating, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, strconv.Itoa(stars))

	rating, err := domain.NewRating(productID, reviewer, stars)
	if err != nil {
		return nil, fmt.Errorf("creating rating: %w", err)
	}

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}

	if !alreadyProcessed {
		if err := s.repo.Save(ctx, rating); err != nil {
			return nil, fmt.Errorf("saving rating: %w", err)
		}
	} else {
		logging.FromContext(ctx).Info("rating submit skipped: already processed", slog.String("idempotency_key", key))
	}

	evt := domain.RatingSubmittedEvent{
		ID:             rating.ID,
		ProductID:      rating.ProductID,
		Reviewer:       rating.Reviewer,
		Stars:          rating.Stars,
		IdempotencyKey: key,
	}
	if err := s.publisher.PublishRatingSubmitted(ctx, evt); err != nil {
		return nil, fmt.Errorf("publishing rating-submitted event: %w", err)
	}

	if alreadyProcessed {
		return nil, ErrAlreadyProcessed
	}
	return rating, nil
}
