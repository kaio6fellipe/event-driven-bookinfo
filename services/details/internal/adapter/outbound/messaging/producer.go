// Package messaging implements the EventPublisher port using a backend
// chosen at startup (kafka or jetstream).
package messaging

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// BookAddedPayload is the marshaled record value for a book-added
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

// PublishBookAdded sends a book-added CloudEvent to the configured backend.
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
	return p.pub.Publish(ctx, Exposed[0], body, evt.IdempotencyKey, evt.IdempotencyKey)
}
