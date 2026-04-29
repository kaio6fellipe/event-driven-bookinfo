// Package messaging implements the EventPublisher port using a backend
// chosen at startup (kafka or jetstream).
package messaging

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingSubmittedPayload is the marshaled record value for a
// rating-submitted CloudEvent. Exported because the events.Descriptor in
// exposed.go references it as a JSONSchema source for tools/specgen.
type RatingSubmittedPayload struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Stars          int    `json:"stars"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Producer wraps an eventsmessaging.Publisher with service-specific
// typed methods. The Publisher impl is chosen by cmd/main.go.
type Producer struct {
	pub eventsmessaging.Publisher
}

// NewProducer builds a Producer from a Publisher. main.go decides which
// concrete impl to pass.
func NewProducer(pub eventsmessaging.Publisher) *Producer {
	return &Producer{pub: pub}
}

// Close releases the underlying publisher.
func (p *Producer) Close() { p.pub.Close() }

// PublishRatingSubmitted sends a rating-submitted CloudEvent to the configured backend.
func (p *Producer) PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error {
	body := RatingSubmittedPayload{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Stars:          evt.Stars,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.pub.Publish(ctx, Exposed[0], body, evt.ProductID, evt.IdempotencyKey)
}
