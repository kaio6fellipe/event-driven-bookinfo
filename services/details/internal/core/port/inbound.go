// Package port defines the inbound and outbound interfaces for the details service.
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// DetailService defines the inbound operations for the details domain.
type DetailService interface {
	// GetDetail returns book details by ID.
	GetDetail(ctx context.Context, id string) (*domain.Detail, error)

	// AddDetail creates and stores a new book detail.
	AddDetail(ctx context.Context, title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13 string) (*domain.Detail, error)

	// ListDetails returns all book details.
	ListDetails(ctx context.Context) ([]*domain.Detail, error)
}
