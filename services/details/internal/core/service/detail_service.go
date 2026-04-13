// Package service implements the business logic for the details service.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"
)

// ErrAlreadyProcessed signals that an idempotent request was previously processed.
var ErrAlreadyProcessed = errors.New("request already processed")

// DetailService implements the port.DetailService interface.
type DetailService struct {
	repo        port.DetailRepository
	idempotency idempotency.Store
}

// NewDetailService creates a new DetailService with the given repository.
func NewDetailService(repo port.DetailRepository, idem idempotency.Store) *DetailService {
	return &DetailService{repo: repo, idempotency: idem}
}

// GetDetail returns a book detail by ID.
func (s *DetailService) GetDetail(ctx context.Context, id string) (*domain.Detail, error) {
	detail, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding detail %s: %w", id, err)
	}
	return detail, nil
}

// AddDetail creates and persists a new book detail. Deduplicates on idempotencyKey
// (falls back to a natural key derived from title+author+year+bookType+pages+publisher+language+isbn10+isbn13).
func (s *DetailService) AddDetail(ctx context.Context, title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13, idempotencyKey string) (*domain.Detail, error) {
	key := idempotency.Resolve(idempotencyKey, title, author, strconv.Itoa(year), bookType, strconv.Itoa(pages), publisher, language, isbn10, isbn13)

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}
	if alreadyProcessed {
		logger := logging.FromContext(ctx)
		logger.Info("detail add skipped: already processed", slog.String("idempotency_key", key))
		return nil, ErrAlreadyProcessed
	}

	detail, err := domain.NewDetail(title, author, year, bookType, pages, publisher, language, isbn10, isbn13)
	if err != nil {
		return nil, fmt.Errorf("creating detail: %w", err)
	}

	if err := s.repo.Save(ctx, detail); err != nil {
		return nil, fmt.Errorf("saving detail: %w", err)
	}

	return detail, nil
}

// ListDetails returns all stored book details.
func (s *DetailService) ListDetails(ctx context.Context) ([]*domain.Detail, error) {
	return s.repo.FindAll(ctx)
}
