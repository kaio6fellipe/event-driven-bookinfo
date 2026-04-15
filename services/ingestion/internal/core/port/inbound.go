// Package port defines the inbound and outbound interfaces for the ingestion service.
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// IngestionService defines the inbound operations for the ingestion domain.
type IngestionService interface {
	// TriggerScrape runs an immediate ingestion cycle with the given search queries.
	// If queries is empty, the service uses its configured default queries.
	TriggerScrape(ctx context.Context, queries []string) (*domain.ScrapeResult, error)

	// GetStatus returns the current ingestion state.
	GetStatus(ctx context.Context) (*domain.IngestionStatus, error)
}
