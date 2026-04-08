// file: services/details/internal/adapter/outbound/memory/detail_repository.go
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// DetailRepository is an in-memory implementation of port.DetailRepository.
type DetailRepository struct {
	mu      sync.RWMutex
	details map[string]domain.Detail
}

// NewDetailRepository creates a new in-memory detail repository.
func NewDetailRepository() *DetailRepository {
	return &DetailRepository{
		details: make(map[string]domain.Detail),
	}
}

// FindByID returns a detail by its ID.
func (r *DetailRepository) FindByID(_ context.Context, id string) (*domain.Detail, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	detail, ok := r.details[id]
	if !ok {
		return nil, fmt.Errorf("detail not found: %s", id)
	}

	return &detail, nil
}

// Save persists a detail in memory.
func (r *DetailRepository) Save(_ context.Context, detail *domain.Detail) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.details[detail.ID] = *detail
	return nil
}
