// Package kafka implements the EventPublisher port using a native Kafka producer.
package kafka

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/kafkapub"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// Client re-exports kafkapub.Client so tests in this package can use
// kafka.Client without importing pkg/eventsmessaging/kafkapub directly.
type Client = kafkapub.Client

// RatingSubmittedPayload is the marshaled Kafka record value for a
// rating-submitted CloudEvent. Exported because the events.Descriptor in
// exposed.go references it as a JSONSchema source for tools/specgen.
type RatingSubmittedPayload struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Stars          int    `json:"stars"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Producer wraps kafkapub.Producer with service-specific typed
// methods. The shared Publish, Close, and constructors come from the
// embedded type.
type Producer struct {
	*kafkapub.Producer
}

// NewProducer connects to the brokers and ensures the topic exists.
func NewProducer(ctx context.Context, brokers, topic string) (*Producer, error) {
	inner, err := kafkapub.NewProducer(ctx, brokers, topic)
	if err != nil {
		return nil, err
	}
	return &Producer{Producer: inner}, nil
}

// NewProducerWithClient creates a Producer with an injected client (for tests).
func NewProducerWithClient(client Client, topic string) *Producer {
	return &Producer{Producer: kafkapub.NewProducerWithClient(client, topic)}
}

// PublishRatingSubmitted sends a rating-submitted CloudEvent to Kafka.
// Thin typed wrapper around Publish; the descriptor is the single source
// of truth for CE headers (exposed.go).
func (p *Producer) PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error {
	body := RatingSubmittedPayload{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Stars:          evt.Stars,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.Publish(ctx, Exposed[0], body, evt.ProductID, evt.IdempotencyKey)
}
