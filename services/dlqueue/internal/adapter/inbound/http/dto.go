package http //nolint:revive // package name matches directory convention

import (
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
)

// IngestEventRequest is the body of POST /v1/events.
type IngestEventRequest struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	EventSource     string `json:"event_source"`
	EventSubject    string `json:"event_subject"`
	SensorName      string `json:"sensor_name"`
	FailedTrigger   string `json:"failed_trigger"`
	EventSourceURL  string `json:"eventsource_url"`
	Namespace       string `json:"namespace"`
	OriginalPayload any    `json:"original_payload"` // JSON object, JSON-encoded string, or []byte
	OriginalHeaders any    `json:"original_headers"` // map[string][]string or JSON-encoded string
	DataContentType string `json:"datacontenttype"`
	EventTimestamp  string `json:"event_timestamp"` // ISO-8601
}

// DLQEventResponse is the REST representation of a DLQEvent.
type DLQEventResponse struct {
	ID              string              `json:"id"`
	EventID         string              `json:"event_id"`
	EventType       string              `json:"event_type"`
	EventSource     string              `json:"event_source"`
	EventSubject    string              `json:"event_subject"`
	SensorName      string              `json:"sensor_name"`
	FailedTrigger   string              `json:"failed_trigger"`
	EventSourceURL  string              `json:"eventsource_url"`
	Namespace       string              `json:"namespace"`
	OriginalPayload any                 `json:"original_payload"`
	PayloadHash     string              `json:"payload_hash"`
	OriginalHeaders map[string][]string `json:"original_headers"`
	DataContentType string              `json:"datacontenttype"`
	EventTimestamp  time.Time           `json:"event_timestamp"`
	Status          string              `json:"status"`
	RetryCount      int                 `json:"retry_count"`
	MaxRetries      int                 `json:"max_retries"`
	LastReplayedAt  *time.Time          `json:"last_replayed_at,omitempty"`
	ResolvedAt      *time.Time          `json:"resolved_at,omitempty"`
	ResolvedBy      string              `json:"resolved_by,omitempty"`
	Notes           string              `json:"notes,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// ListEventsResponse is the body of GET /v1/events.
type ListEventsResponse struct {
	Items      []DLQEventResponse `json:"items"`
	TotalItems int                `json:"total_items"`
	Offset     int                `json:"offset"`
	Limit      int                `json:"limit"`
}

// ResolveRequest is the body of POST /v1/events/{id}/resolve.
type ResolveRequest struct {
	ResolvedBy string `json:"resolved_by"`
}

// BatchResolveRequest is the body of POST /v1/events/batch/resolve.
type BatchResolveRequest struct {
	IDs        []string `json:"ids"`
	ResolvedBy string   `json:"resolved_by"`
}

// BatchReplayRequest is the body of POST /v1/events/batch/replay.
type BatchReplayRequest struct {
	Status        string `json:"status"`
	EventSource   string `json:"event_source"`
	SensorName    string `json:"sensor_name"`
	FailedTrigger string `json:"failed_trigger"`
	Limit         int    `json:"limit"`
}

// BatchActionResponse is the response body of batch endpoints.
type BatchActionResponse struct {
	AffectedCount int `json:"affected_count"`
}

// ErrorResponse is the standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}

// toResponse converts a domain event to its DTO.
func toResponse(e *domain.DLQEvent) DLQEventResponse {
	var payload any
	if len(e.OriginalPayload) > 0 {
		payload = e.OriginalPayload
	}
	return DLQEventResponse{
		ID:              e.ID,
		EventID:         e.EventID,
		EventType:       e.EventType,
		EventSource:     e.EventSource,
		EventSubject:    e.EventSubject,
		SensorName:      e.SensorName,
		FailedTrigger:   e.FailedTrigger,
		EventSourceURL:  e.EventSourceURL,
		Namespace:       e.Namespace,
		OriginalPayload: payload,
		PayloadHash:     e.PayloadHash,
		OriginalHeaders: e.OriginalHeaders,
		DataContentType: e.DataContentType,
		EventTimestamp:  e.EventTimestamp,
		Status:          string(e.Status),
		RetryCount:      e.RetryCount,
		MaxRetries:      e.MaxRetries,
		LastReplayedAt:  e.LastReplayedAt,
		ResolvedAt:      e.ResolvedAt,
		ResolvedBy:      e.ResolvedBy,
		Notes:           e.Notes,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}
