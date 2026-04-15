// Package domain contains the core domain model for the ingestion service.
package domain

import (
	"fmt"
	"time"
)

// Book represents a book fetched from an external catalog.
type Book struct {
	Title       string
	Authors     []string
	ISBN        string
	PublishYear int
	Subjects    []string
	Pages       int
	Publisher   string
	Language    string
}

// Validate checks that the book has the minimum required fields for publishing.
func (b *Book) Validate() error {
	if b.Title == "" {
		return fmt.Errorf("title is required")
	}
	if b.ISBN == "" {
		return fmt.Errorf("ISBN is required")
	}
	if len(b.Authors) == 0 {
		return fmt.Errorf("at least one author is required")
	}
	if b.PublishYear <= 0 {
		return fmt.Errorf("publish year must be positive, got %d", b.PublishYear)
	}
	return nil
}

// ScrapeResult contains the outcome of a single ingestion cycle.
type ScrapeResult struct {
	BooksFound      int           `json:"books_found"`
	EventsPublished int           `json:"events_published"`
	Errors          int           `json:"errors"`
	Duration        time.Duration `json:"duration_ms"`
}

// IngestionStatus represents the current state of the ingestion service.
type IngestionStatus struct {
	State      string        `json:"state"`
	LastRunAt  *time.Time    `json:"last_run_at,omitempty"`
	LastResult *ScrapeResult `json:"last_result,omitempty"`
}
