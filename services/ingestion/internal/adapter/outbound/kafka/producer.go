// Package kafka implements the EventPublisher port using a native Kafka producer.
package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventskafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// Client re-exports eventskafka.Client so tests in this package can use
// kafka.Client without importing pkg/eventskafka directly.
type Client = eventskafka.Client

// BookEvent is the marshaled Kafka record value for a book-added
// CloudEvent. Its JSON shape matches details' AddDetailRequest, since
// the details Sensor uses passthrough payload. Exported so the
// events.Descriptor in exposed.go can reference it as a JSONSchema
// source for tools/specgen.
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

// PublishBookAdded sends a book-added CloudEvent to Kafka. Thin typed
// wrapper around Publish; the descriptor is the single source of truth
// for CE headers (exposed.go).
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

	return p.Publish(ctx, Exposed[0], evt, book.ISBN, idempotencyKey)
}

func classifyISBN(isbn string) (isbn10, isbn13 string) {
	if len(isbn) == 13 {
		return "", isbn
	}
	return isbn, ""
}
