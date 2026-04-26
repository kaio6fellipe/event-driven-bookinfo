// Package kafka implements the EventPublisher port using a native Kafka producer.
package kafka

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventskafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// Client re-exports eventskafka.Client so tests in this package can use
// kafka.Client without importing pkg/eventskafka directly.
type Client = eventskafka.Client

// ReviewSubmittedPayload is the marshaled Kafka record value for a
// review-submitted CloudEvent.
type ReviewSubmittedPayload struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key"`
}

// ReviewDeletedPayload is the marshaled Kafka record value for a
// review-deleted CloudEvent.
type ReviewDeletedPayload struct {
	ReviewID       string `json:"review_id"`
	ProductID      string `json:"product_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Producer wraps eventskafka.Producer with service-specific typed
// methods. The shared Publish, Close, and constructors come from the
// embedded type.
type Producer struct {
	*eventskafka.Producer
}

// NewProducer connects to the brokers and ensures the topic exists.
func NewProducer(ctx context.Context, brokers, topic string) (*Producer, error) {
	inner, err := eventskafka.NewProducer(ctx, brokers, topic)
	if err != nil {
		return nil, err
	}
	return &Producer{Producer: inner}, nil
}

// NewProducerWithClient creates a Producer with an injected client (for tests).
func NewProducerWithClient(client Client, topic string) *Producer {
	return &Producer{Producer: eventskafka.NewProducerWithClient(client, topic)}
}

// PublishReviewSubmitted sends a review-submitted CloudEvent to Kafka.
func (p *Producer) PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error {
	body := ReviewSubmittedPayload{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Text:           evt.Text,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.Publish(ctx, events.Find(Exposed, "review-submitted"), body, evt.ProductID, evt.IdempotencyKey)
}

// PublishReviewDeleted sends a review-deleted CloudEvent to Kafka.
func (p *Producer) PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error {
	body := ReviewDeletedPayload{
		ReviewID:       evt.ReviewID,
		ProductID:      evt.ProductID,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.Publish(ctx, events.Find(Exposed, "review-deleted"), body, evt.ProductID, evt.IdempotencyKey)
}
