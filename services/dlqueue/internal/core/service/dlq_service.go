// Package service implements the dlqueue business logic.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/port"
)

// DLQService implements port.DLQService.
type DLQService struct {
	repo       port.DLQRepository
	replayer   port.EventReplayClient
	maxRetries int
}

// NewDLQService constructs a DLQService.
func NewDLQService(repo port.DLQRepository, replayer port.EventReplayClient, maxRetries int) *DLQService {
	if maxRetries < 1 {
		maxRetries = 3
	}
	return &DLQService{repo: repo, replayer: replayer, maxRetries: maxRetries}
}

// IngestEvent persists a newly-received DLQ event or deduplicates against an
// existing record by natural key (sensor_name + failed_trigger + payload_hash).
// On dedup, increments retry_count and may transition to poisoned.
func (s *DLQService) IngestEvent(ctx context.Context, p port.IngestEventParams) (*domain.DLQEvent, error) {
	logger := logging.FromContext(ctx)
	hash := domain.HashPayload(p.OriginalPayload)

	existing, err := s.repo.FindByNaturalKey(ctx, p.SensorName, p.FailedTrigger, hash)
	switch {
	case err == nil:
		// Duplicate arrival — re-ingest.
		if err := existing.ReIngest(); err != nil {
			if errors.Is(err, domain.ErrAlreadyResolved) {
				logger.Info("dlq re-ingest on resolved event — ignoring", "dlq_event_id", existing.ID)
				return existing, nil
			}
			return nil, fmt.Errorf("re-ingesting: %w", err)
		}
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("updating dlq event: %w", err)
		}
		logger.Info("dlq event re-ingested",
			"dlq_event_id", existing.ID,
			"retry_count", existing.RetryCount,
			"status", existing.Status,
			"is_duplicate", true,
		)
		return existing, nil
	case errors.Is(err, domain.ErrNotFound):
		// New event.
		e, err := domain.NewDLQEvent(domain.NewDLQEventParams{
			EventID:         p.EventID,
			EventType:       p.EventType,
			EventSource:     p.EventSource,
			EventSubject:    p.EventSubject,
			SensorName:      p.SensorName,
			FailedTrigger:   p.FailedTrigger,
			EventSourceURL:  p.EventSourceURL,
			Namespace:       p.Namespace,
			OriginalPayload: p.OriginalPayload,
			OriginalHeaders: p.OriginalHeaders,
			DataContentType: p.DataContentType,
			EventTimestamp:  p.EventTimestamp,
			MaxRetries:      s.maxRetries,
		})
		if err != nil {
			return nil, fmt.Errorf("creating dlq event: %w", err)
		}
		if err := s.repo.Save(ctx, e); err != nil {
			return nil, fmt.Errorf("saving dlq event: %w", err)
		}
		logger.Info("dlq event ingested",
			"dlq_event_id", e.ID,
			"event_source", e.EventSource,
			"sensor_name", e.SensorName,
			"failed_trigger", e.FailedTrigger,
			"is_duplicate", false,
		)
		return e, nil
	default:
		return nil, fmt.Errorf("lookup by natural key: %w", err)
	}
}

// GetEvent returns a single DLQ event by ID.
func (s *DLQService) GetEvent(ctx context.Context, id string) (*domain.DLQEvent, error) {
	return s.repo.FindByID(ctx, id)
}

// ListEvents returns a filtered paginated list of DLQ events.
func (s *DLQService) ListEvents(ctx context.Context, f port.ListFilter) ([]domain.DLQEvent, int, error) {
	return s.repo.List(ctx, f)
}

// ReplayEvent transitions the event to replayed and POSTs the original payload
// + headers back through the source EventSource.
func (s *DLQService) ReplayEvent(ctx context.Context, id string) (*domain.DLQEvent, error) {
	logger := logging.FromContext(ctx)

	e, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := e.Replay(); err != nil {
		return nil, err
	}
	if err := s.replayer.Replay(ctx, e.EventSourceURL, e.OriginalPayload, e.OriginalHeaders); err != nil {
		return nil, fmt.Errorf("replaying event to %s: %w", e.EventSourceURL, err)
	}
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, fmt.Errorf("updating dlq event: %w", err)
	}
	logger.Info("dlq event replayed",
		"dlq_event_id", e.ID,
		"event_source", e.EventSource,
		"eventsource_url", e.EventSourceURL,
		"retry_count", e.RetryCount,
	)
	return e, nil
}

// ResolveEvent marks the event as resolved.
func (s *DLQService) ResolveEvent(ctx context.Context, id, resolvedBy string) (*domain.DLQEvent, error) {
	logger := logging.FromContext(ctx)

	e, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := e.Resolve(resolvedBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, fmt.Errorf("updating dlq event: %w", err)
	}
	logger.Info("dlq event resolved",
		"dlq_event_id", e.ID,
		"resolved_by", resolvedBy,
	)
	return e, nil
}

// ResetPoisoned transitions a poisoned event back to pending and clears
// retry_count. Operator-only action.
func (s *DLQService) ResetPoisoned(ctx context.Context, id string) (*domain.DLQEvent, error) {
	e, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := e.ResetPoison(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, fmt.Errorf("updating dlq event: %w", err)
	}
	return e, nil
}

// BatchReplay replays every event matching the filter. Stops at the first
// transport error and returns the count of successful replays so far.
func (s *DLQService) BatchReplay(ctx context.Context, f port.BatchFilter) (int, error) {
	events, _, err := s.repo.List(ctx, f)
	if err != nil {
		return 0, fmt.Errorf("listing events: %w", err)
	}
	var replayed int
	for i := range events {
		if _, err := s.ReplayEvent(ctx, events[i].ID); err != nil {
			return replayed, fmt.Errorf("replaying %s: %w", events[i].ID, err)
		}
		replayed++
	}
	return replayed, nil
}

// BatchResolve marks a batch of events as resolved.
func (s *DLQService) BatchResolve(ctx context.Context, ids []string, resolvedBy string) (int, error) {
	var resolved int
	for _, id := range ids {
		if _, err := s.ResolveEvent(ctx, id, resolvedBy); err != nil {
			return resolved, fmt.Errorf("resolving %s: %w", id, err)
		}
		resolved++
	}
	return resolved, nil
}
