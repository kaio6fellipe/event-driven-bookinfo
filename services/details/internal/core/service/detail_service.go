// file: services/details/internal/core/service/detail_service.go
package service

import (
	"context"
	"fmt"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"
)

// DetailService implements the port.DetailService interface.
type DetailService struct {
	repo port.DetailRepository
}

// NewDetailService creates a new DetailService with the given repository.
func NewDetailService(repo port.DetailRepository) *DetailService {
	return &DetailService{repo: repo}
}

// GetDetail returns a book detail by ID.
func (s *DetailService) GetDetail(ctx context.Context, id string) (*domain.Detail, error) {
	detail, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding detail %s: %w", id, err)
	}
	return detail, nil
}

// AddDetail creates and persists a new book detail.
func (s *DetailService) AddDetail(ctx context.Context, title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13 string) (*domain.Detail, error) {
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
