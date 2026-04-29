// Package messaging implements the EventPublisher port using a native Kafka producer.
package messaging

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/kafkapub"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// Client re-exports kafkapub.Client so tests in this package can use
// kafka.Client without importing pkg/eventsmessaging/kafkapub directly.
type Client = kafkapub.Client

// BookAddedPayload is the marshaled Kafka record value for a book-added
// CloudEvent. Exported because the events.Descriptor in exposed.go
// references it as a JSONSchema source for tools/specgen.
type BookAddedPayload struct {
	ID             string `json:"id,omitempty"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	Year           int    `json:"year"`
	Type           string `json:"type"`
	Pages          int    `json:"pages,omitempty"`
	Publisher      string `json:"publisher,omitempty"`
	Language       string `json:"language,omitempty"`
	ISBN10         string `json:"isbn_10,omitempty"`
	ISBN13         string `json:"isbn_13,omitempty"`
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

// PublishBookAdded sends a book-added CloudEvent to Kafka.
// Thin typed wrapper around Publish; the descriptor is the single source
// of truth for CE headers (exposed.go).
func (p *Producer) PublishBookAdded(ctx context.Context, evt domain.BookAddedEvent) error {
	body := BookAddedPayload{
		ID:             evt.ID,
		Title:          evt.Title,
		Author:         evt.Author,
		Year:           evt.Year,
		Type:           evt.Type,
		Pages:          evt.Pages,
		Publisher:      evt.Publisher,
		Language:       evt.Language,
		ISBN10:         evt.ISBN10,
		ISBN13:         evt.ISBN13,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.Publish(ctx, Exposed[0], body, evt.IdempotencyKey, evt.IdempotencyKey)
}
