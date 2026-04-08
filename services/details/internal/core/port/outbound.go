// file: services/details/internal/core/port/outbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// DetailRepository defines the outbound persistence operations for details.
type DetailRepository interface {
	// FindByID returns a detail by its ID.
	FindByID(ctx context.Context, id string) (*domain.Detail, error)

	// Save persists a detail.
	Save(ctx context.Context, detail *domain.Detail) error
}
