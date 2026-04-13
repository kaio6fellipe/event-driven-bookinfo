package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
)

// DLQRepository is the outbound port for DLQ event persistence.
type DLQRepository interface {
	Save(ctx context.Context, event *domain.DLQEvent) error
	FindByID(ctx context.Context, id string) (*domain.DLQEvent, error)
	// FindByNaturalKey locates an event by the dedup composite key.
	// Returns (nil, domain.ErrNotFound) when no match.
	FindByNaturalKey(ctx context.Context, sensorName, failedTrigger, payloadHash string) (*domain.DLQEvent, error)
	List(ctx context.Context, filter ListFilter) ([]domain.DLQEvent, int, error)
	Update(ctx context.Context, event *domain.DLQEvent) error
}

// EventReplayClient is the outbound port for replaying events back through
// the source EventSource webhook.
type EventReplayClient interface {
	// Replay POSTs payload (as-is) to url with the given HTTP headers.
	// Returns a non-nil error on network failure or non-2xx response.
	Replay(ctx context.Context, url string, payload []byte, headers map[string][]string) error
}
