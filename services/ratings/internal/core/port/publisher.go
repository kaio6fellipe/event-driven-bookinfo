package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// EventPublisher is the outbound port for emitting rating domain events.
type EventPublisher interface {
	PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error
}
