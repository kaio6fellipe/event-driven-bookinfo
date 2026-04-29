package messaging

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// NoopPublisher implements port.EventPublisher without side effects.
type NoopPublisher struct{}

// NewNoopPublisher returns a no-op publisher.
func NewNoopPublisher() *NoopPublisher { return &NoopPublisher{} }

// PublishRatingSubmitted logs at debug and returns nil.
func (n *NoopPublisher) PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error {
	logging.FromContext(ctx).Debug("noop publisher: rating-submitted discarded", "idempotency_key", evt.IdempotencyKey)
	return nil
}
