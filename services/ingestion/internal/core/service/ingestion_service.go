// Package service implements the business logic for the ingestion service.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/port"
)

// IngestionService implements the port.IngestionService interface.
type IngestionService struct {
	fetcher        port.BookFetcher
	publisher      port.EventPublisher
	defaultQueries []string
	maxResultsPerQ int

	mu     sync.RWMutex
	status domain.IngestionStatus
}

// NewIngestionService creates a new IngestionService.
func NewIngestionService(
	fetcher port.BookFetcher,
	publisher port.EventPublisher,
	defaultQueries []string,
	maxResultsPerQuery int,
) *IngestionService {
	return &IngestionService{
		fetcher:        fetcher,
		publisher:      publisher,
		defaultQueries: defaultQueries,
		maxResultsPerQ: maxResultsPerQuery,
		status:         domain.IngestionStatus{State: "idle"},
	}
}

// TriggerScrape runs an ingestion cycle using the given queries (or defaults if empty).
func (s *IngestionService) TriggerScrape(ctx context.Context, queries []string) (*domain.ScrapeResult, error) {
	logger := logging.FromContext(ctx)

	if len(queries) == 0 {
		queries = s.defaultQueries
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("no search queries configured")
	}

	s.mu.Lock()
	if s.status.State == "running" {
		s.mu.Unlock()
		return nil, fmt.Errorf("ingestion cycle already running")
	}
	s.status.State = "running"
	s.mu.Unlock()

	start := time.Now()
	result := &domain.ScrapeResult{}

	for _, query := range queries {
		books, err := s.fetcher.SearchBooks(ctx, query, s.maxResultsPerQ)
		if err != nil {
			logger.Error("failed to fetch books", "query", query, "error", err)
			result.Errors++
			continue
		}

		result.BooksFound += len(books)

		for _, book := range books {
			if err := s.publisher.PublishBookAdded(ctx, book); err != nil {
				logger.Warn("failed to publish book", "title", book.Title, "isbn", book.ISBN, "error", err)
				result.Errors++
				continue
			}
			result.EventsPublished++
			logger.Debug("published book", "title", book.Title, "isbn", book.ISBN)
		}
	}

	result.Duration = time.Since(start)
	now := time.Now()

	s.mu.Lock()
	s.status.State = "idle"
	s.status.LastRunAt = &now
	s.status.LastResult = result
	s.mu.Unlock()

	logger.Info("ingestion cycle complete",
		"books_found", result.BooksFound,
		"events_published", result.EventsPublished,
		"errors", result.Errors,
		"duration", result.Duration,
	)

	return result, nil
}

// GetStatus returns the current ingestion status.
func (s *IngestionService) GetStatus(_ context.Context) (*domain.IngestionStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := domain.IngestionStatus{
		State:      s.status.State,
		LastRunAt:  s.status.LastRunAt,
		LastResult: s.status.LastResult,
	}
	return &status, nil
}
