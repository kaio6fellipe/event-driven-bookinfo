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
}

// NewRatingService creates a new RatingService with the given repository.
func NewRatingService(repo port.RatingRepository, idem idempotency.Store) *RatingService {
	return &RatingService{repo: repo, idempotency: idem}
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

// SubmitRating creates and persists a new rating. Deduplicates on idempotencyKey
// (falls back to a natural key derived from productID+reviewer+stars).
func (s *RatingService) SubmitRating(ctx context.Context, productID, reviewer string, stars int, idempotencyKey string) (*domain.Rating, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, strconv.Itoa(stars))

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}
	if alreadyProcessed {
		logger := logging.FromContext(ctx)
		logger.Info("rating submit skipped: already processed", slog.String("idempotency_key", key))
		return nil, ErrAlreadyProcessed
	}

	rating, err := domain.NewRating(productID, reviewer, stars)
	if err != nil {
		return nil, fmt.Errorf("creating rating: %w", err)
	}

	if err := s.repo.Save(ctx, rating); err != nil {
		return nil, fmt.Errorf("saving rating: %w", err)
	}

	return rating, nil
}
