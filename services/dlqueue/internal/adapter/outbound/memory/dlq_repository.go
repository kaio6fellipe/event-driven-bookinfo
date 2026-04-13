// Package memory provides an in-memory DLQRepository implementation.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/port"
)

// DLQRepository is an in-memory repository. Suitable for local dev and tests.
type DLQRepository struct {
	mu     sync.RWMutex
	events map[string]*domain.DLQEvent
}

// NewDLQRepository creates a new in-memory DLQRepository.
func NewDLQRepository() *DLQRepository {
	return &DLQRepository{events: make(map[string]*domain.DLQEvent)}
}

// Save implements port.DLQRepository.
func (r *DLQRepository) Save(_ context.Context, event *domain.DLQEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[event.ID] = cloneEvent(event)
	return nil
}

// FindByID implements port.DLQRepository.
func (r *DLQRepository) FindByID(_ context.Context, id string) (*domain.DLQEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.events[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneEvent(e), nil
}

// FindByNaturalKey implements port.DLQRepository.
func (r *DLQRepository) FindByNaturalKey(_ context.Context, sensorName, failedTrigger, payloadHash string) (*domain.DLQEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.events {
		if e.SensorName == sensorName && e.FailedTrigger == failedTrigger && e.PayloadHash == payloadHash {
			return cloneEvent(e), nil
		}
	}
	return nil, domain.ErrNotFound
}

// List implements port.DLQRepository. Supports filtering and pagination.
func (r *DLQRepository) List(_ context.Context, f port.ListFilter) ([]domain.DLQEvent, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matched := make([]domain.DLQEvent, 0, len(r.events))
	for _, e := range r.events {
		if !matchFilter(e, f) {
			continue
		}
		matched = append(matched, *cloneEvent(e))
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := len(matched)
	start := f.Offset
	if start > total {
		start = total
	}
	end := start + f.Limit
	if f.Limit == 0 || end > total {
		end = total
	}
	return matched[start:end], total, nil
}

// Update implements port.DLQRepository.
func (r *DLQRepository) Update(_ context.Context, event *domain.DLQEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.events[event.ID]; !ok {
		return domain.ErrNotFound
	}
	r.events[event.ID] = cloneEvent(event)
	return nil
}

func matchFilter(e *domain.DLQEvent, f port.ListFilter) bool {
	if f.Status != "" && string(e.Status) != f.Status {
		return false
	}
	if f.EventSource != "" && e.EventSource != f.EventSource {
		return false
	}
	if f.SensorName != "" && e.SensorName != f.SensorName {
		return false
	}
	if f.FailedTrigger != "" && e.FailedTrigger != f.FailedTrigger {
		return false
	}
	if f.CreatedAfter != nil && e.CreatedAt.Before(*f.CreatedAfter) {
		return false
	}
	if f.CreatedBefore != nil && e.CreatedAt.After(*f.CreatedBefore) {
		return false
	}
	return true
}

func cloneEvent(e *domain.DLQEvent) *domain.DLQEvent {
	c := *e
	if e.OriginalPayload != nil {
		c.OriginalPayload = make([]byte, len(e.OriginalPayload))
		copy(c.OriginalPayload, e.OriginalPayload)
	}
	if e.OriginalHeaders != nil {
		c.OriginalHeaders = make(map[string][]string, len(e.OriginalHeaders))
		for k, v := range e.OriginalHeaders {
			vc := make([]string, len(v))
			copy(vc, v)
			c.OriginalHeaders[k] = vc
		}
	}
	if e.LastReplayedAt != nil {
		t := *e.LastReplayedAt
		c.LastReplayedAt = &t
	}
	if e.ResolvedAt != nil {
		t := *e.ResolvedAt
		c.ResolvedAt = &t
	}
	return &c
}
