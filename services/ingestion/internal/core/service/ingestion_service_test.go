package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/service"
)

// mockFetcher implements port.BookFetcher for testing.
type mockFetcher struct {
	books map[string][]domain.Book
	err   error
}

func (m *mockFetcher) SearchBooks(_ context.Context, query string, _ int) ([]domain.Book, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.books[query], nil
}

// mockPublisher implements port.EventPublisher for testing.
type mockPublisher struct {
	published []domain.Book
	failOn    map[string]bool
}

func (m *mockPublisher) PublishBookAdded(_ context.Context, book domain.Book) error {
	if m.failOn[book.ISBN] {
		return fmt.Errorf("publish failed for %s", book.ISBN)
	}
	m.published = append(m.published, book)
	return nil
}

func validBook(title, isbn string) domain.Book {
	return domain.Book{
		Title:       title,
		Authors:     []string{"Author"},
		ISBN:        isbn,
		PublishYear: 2020,
	}
}

func TestTriggerScrape_Success(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"golang": {
				validBook("Go Book 1", "1111111111111"),
				validBook("Go Book 2", "2222222222222"),
			},
		},
	}
	pub := &mockPublisher{}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang"}, 10)
	result, err := svc.TriggerScrape(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BooksFound != 2 {
		t.Errorf("BooksFound = %d, want 2", result.BooksFound)
	}
	if result.EventsPublished != 2 {
		t.Errorf("EventsPublished = %d, want 2", result.EventsPublished)
	}
	if result.Errors != 0 {
		t.Errorf("Errors = %d, want 0", result.Errors)
	}
	if len(pub.published) != 2 {
		t.Errorf("published count = %d, want 2", len(pub.published))
	}
}

func TestTriggerScrape_OverrideQueries(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"rust": {validBook("Rust Book", "3333333333333")},
		},
	}
	pub := &mockPublisher{}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang"}, 10)
	result, err := svc.TriggerScrape(context.Background(), []string{"rust"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BooksFound != 1 {
		t.Errorf("BooksFound = %d, want 1", result.BooksFound)
	}
	if pub.published[0].Title != "Rust Book" {
		t.Errorf("published wrong book: %s", pub.published[0].Title)
	}
}

func TestTriggerScrape_FetchError(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{err: fmt.Errorf("network error")}
	pub := &mockPublisher{}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang"}, 10)
	result, err := svc.TriggerScrape(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Errors)
	}
	if result.BooksFound != 0 {
		t.Errorf("BooksFound = %d, want 0", result.BooksFound)
	}
}

func TestTriggerScrape_PublishError(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"golang": {
				validBook("Good Book", "1111111111111"),
				validBook("Bad Book", "2222222222222"),
			},
		},
	}
	pub := &mockPublisher{failOn: map[string]bool{"2222222222222": true}}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang"}, 10)
	result, err := svc.TriggerScrape(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EventsPublished != 1 {
		t.Errorf("EventsPublished = %d, want 1", result.EventsPublished)
	}
	if result.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Errors)
	}
}

func TestTriggerScrape_NoQueries(t *testing.T) {
	t.Parallel()

	svc := service.NewIngestionService(&mockFetcher{}, &mockPublisher{}, nil, 10)
	_, err := svc.TriggerScrape(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when no queries configured")
	}
}

func TestGetStatus_Initial(t *testing.T) {
	t.Parallel()

	svc := service.NewIngestionService(&mockFetcher{}, &mockPublisher{}, []string{"golang"}, 10)
	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "idle" {
		t.Errorf("State = %q, want %q", status.State, "idle")
	}
	if status.LastRunAt != nil {
		t.Error("expected nil LastRunAt for initial status")
	}
}

func TestGetStatus_AfterScrape(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"golang": {validBook("Book", "1234567890123")},
		},
	}
	svc := service.NewIngestionService(fetcher, &mockPublisher{}, []string{"golang"}, 10)

	_, _ = svc.TriggerScrape(context.Background(), nil)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "idle" {
		t.Errorf("State = %q, want %q after scrape", status.State, "idle")
	}
	if status.LastRunAt == nil {
		t.Error("expected non-nil LastRunAt after scrape")
	}
	if status.LastResult == nil {
		t.Fatal("expected non-nil LastResult after scrape")
	}
	if status.LastResult.BooksFound != 1 {
		t.Errorf("LastResult.BooksFound = %d, want 1", status.LastResult.BooksFound)
	}
}

func TestTriggerScrape_MultipleQueries(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"golang":     {validBook("Go Book", "1111111111111")},
			"kubernetes": {validBook("K8s Book", "2222222222222")},
		},
	}
	pub := &mockPublisher{}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang", "kubernetes"}, 10)
	result, err := svc.TriggerScrape(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BooksFound != 2 {
		t.Errorf("BooksFound = %d, want 2", result.BooksFound)
	}
	if result.EventsPublished != 2 {
		t.Errorf("EventsPublished = %d, want 2", result.EventsPublished)
	}
}
