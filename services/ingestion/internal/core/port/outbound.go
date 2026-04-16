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

// EventPublisher sends events to the internal event pipeline.
// Returns nil when the EventSource webhook accepts the event (HTTP 200).
// Returns error on non-200 responses or connection failures.
type EventPublisher interface {
	// PublishBookAdded sends a book-added event to the Gateway.
	PublishBookAdded(ctx context.Context, book domain.Book) error
}
