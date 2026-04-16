package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// BookFetcher retrieves books from an external catalog.
type BookFetcher interface {
	// SearchBooks searches for books matching the query, returning up to limit results.
	SearchBooks(ctx context.Context, query string, limit int) ([]domain.Book, error)
}

// EventPublisher sends events to Kafka.
// Returns nil when the event is successfully produced to the topic.
// Returns error on produce failures.
type EventPublisher interface {
	// PublishBookAdded sends a book-added event to Kafka.
	PublishBookAdded(ctx context.Context, book domain.Book) error
}
