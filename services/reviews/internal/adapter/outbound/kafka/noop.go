package kafka

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// NoopPublisher implements port.EventPublisher without side effects.
type NoopPublisher struct{}

// NewNoopPublisher returns a no-op publisher.
func NewNoopPublisher() *NoopPublisher { return &NoopPublisher{} }

// PublishReviewSubmitted logs at debug and returns nil.
func (n *NoopPublisher) PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error {
	logging.FromContext(ctx).Debug("noop publisher: review-submitted discarded", "idempotency_key", evt.IdempotencyKey)
	return nil
}

// PublishReviewDeleted logs at debug and returns nil.
func (n *NoopPublisher) PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error {
	logging.FromContext(ctx).Debug("noop publisher: review-deleted discarded", "idempotency_key", evt.IdempotencyKey)
	return nil
}
