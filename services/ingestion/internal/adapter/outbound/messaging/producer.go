// Package messaging implements the EventPublisher port using a backend
// chosen at startup (kafka or jetstream).
package messaging

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// BookEvent is the marshaled record value for a book-added CloudEvent.
// Its JSON shape matches details' AddDetailRequest, since the details
// Sensor uses passthrough payload. Exported so the events.Descriptor in
// exposed.go can reference it as a JSONSchema source for tools/specgen.
type BookEvent struct {
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
func (p *Producer) PublishBookAdded(ctx context.Context, book domain.Book) error {
	isbn10, isbn13 := classifyISBN(book.ISBN)
	idempotencyKey := fmt.Sprintf("ingestion-isbn-%s", book.ISBN)

	evt := BookEvent{
		Title:          book.Title,
		Author:         strings.Join(book.Authors, ", "),
		Year:           book.PublishYear,
		Type:           "paperback",
		Pages:          book.Pages,
		Publisher:      book.Publisher,
		Language:       book.Language,
		ISBN10:         isbn10,
		ISBN13:         isbn13,
		IdempotencyKey: idempotencyKey,
	}

	d := events.Find(Exposed, "book-added")
	return p.pub.Publish(ctx, d, evt, book.ISBN, idempotencyKey)
}

func classifyISBN(isbn string) (isbn10, isbn13 string) {
	if len(isbn) == 13 {
		return "", isbn
	}
	return isbn, ""
}
