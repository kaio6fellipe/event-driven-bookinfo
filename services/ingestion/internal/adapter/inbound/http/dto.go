// Package http provides HTTP handlers and DTOs for the ingestion service.
package http //nolint:revive // package name matches directory convention

import "time"

// TriggerRequest is the optional JSON body for POST /v1/ingestion/trigger.
type TriggerRequest struct {
	Queries    []string `json:"queries,omitempty"`
	MaxResults int      `json:"max_results,omitempty"`
}

// ScrapeResultResponse is the JSON response for a completed scrape cycle.
type ScrapeResultResponse struct {
	BooksFound      int   `json:"books_found"`
	EventsPublished int   `json:"events_published"`
	Errors          int   `json:"errors"`
	DurationMs      int64 `json:"duration_ms"`
}

// StatusResponse is the JSON response for GET /v1/ingestion/status.
type StatusResponse struct {
	State      string                `json:"state"`
	LastRunAt  *time.Time            `json:"last_run_at,omitempty"`
	LastResult *ScrapeResultResponse `json:"last_result,omitempty"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
