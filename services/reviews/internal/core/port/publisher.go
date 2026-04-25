package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// EventPublisher is the outbound port for emitting review domain events.
type EventPublisher interface {
	PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error
	PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error
}
