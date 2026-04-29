package messaging

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// NoopPublisher implements port.EventPublisher but performs no work.
// Used when KAFKA_BROKERS is not set (e.g. docker-compose, unit tests).
type NoopPublisher struct{}

// NewNoopPublisher returns a publisher that silently succeeds.
func NewNoopPublisher() *NoopPublisher { return &NoopPublisher{} }

// PublishBookAdded logs at debug and returns nil.
func (n *NoopPublisher) PublishBookAdded(ctx context.Context, evt domain.BookAddedEvent) error {
	logging.FromContext(ctx).Debug("noop publisher: book-added event discarded", "idempotency_key", evt.IdempotencyKey)
	return nil
}
