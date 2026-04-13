// Package port defines the inbound and outbound interfaces for the dlqueue service.
package port

import (
	"context"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
)

// IngestEventParams carries the fields captured by a sensor dlqTrigger.
// All fields originate from the CloudEvents context or static per-sensor values.
type IngestEventParams struct {
	EventID         string
	EventType       string
	EventSource     string
	EventSubject    string
	SensorName      string
	FailedTrigger   string
	EventSourceURL  string
	Namespace       string
	OriginalPayload []byte
	OriginalHeaders map[string][]string
	DataContentType string
	EventTimestamp  time.Time
}

// ListFilter constrains List queries.
type ListFilter struct {
	Status        string
	EventSource   string
	SensorName    string
	FailedTrigger string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Offset        int
	Limit         int
}

// BatchFilter is ListFilter used for BatchReplay.
type BatchFilter = ListFilter

// DLQService is the inbound port implemented by the service layer.
type DLQService interface {
	IngestEvent(ctx context.Context, params IngestEventParams) (*domain.DLQEvent, error)
	GetEvent(ctx context.Context, id string) (*domain.DLQEvent, error)
	ListEvents(ctx context.Context, filter ListFilter) ([]domain.DLQEvent, int, error)
	ReplayEvent(ctx context.Context, id string) (*domain.DLQEvent, error)
	ResolveEvent(ctx context.Context, id string, resolvedBy string) (*domain.DLQEvent, error)
	ResetPoisoned(ctx context.Context, id string) (*domain.DLQEvent, error)
	BatchReplay(ctx context.Context, filter BatchFilter) (int, error)
	BatchResolve(ctx context.Context, ids []string, resolvedBy string) (int, error)
}
