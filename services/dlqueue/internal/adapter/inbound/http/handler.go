// Package http provides HTTP handlers for the dlqueue service.
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"strconv"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/port"
)

const (
	defaultLimit = 20
	maxLimit     = 200
)

// Handler serves the dlqueue REST API.
type Handler struct {
	svc port.DLQService
}

// NewHandler constructs a Handler.
func NewHandler(svc port.DLQService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all dlqueue routes on the given mux.
func (h *Handler) RegisterRoutes(mux *stdhttp.ServeMux) {
	mux.HandleFunc("POST /v1/events", h.ingestEvent)
	mux.HandleFunc("GET /v1/events", h.listEvents)
	mux.HandleFunc("GET /v1/events/{id}", h.getEvent)
	mux.HandleFunc("POST /v1/events/{id}/replay", h.replayEvent)
	mux.HandleFunc("POST /v1/events/{id}/resolve", h.resolveEvent)
	mux.HandleFunc("POST /v1/events/{id}/reset", h.resetPoisoned)
	mux.HandleFunc("POST /v1/events/batch/replay", h.batchReplay)
	mux.HandleFunc("POST /v1/events/batch/resolve", h.batchResolve)
}

func (h *Handler) ingestEvent(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	logger := logging.FromContext(r.Context())

	var req IngestEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	payloadBytes, err := marshalPayload(req.OriginalPayload)
	if err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, ErrorResponse{Error: "invalid original_payload"})
		return
	}

	var ts time.Time
	if req.EventTimestamp != "" {
		ts, _ = time.Parse(time.RFC3339, req.EventTimestamp)
	}

	params := port.IngestEventParams{
		EventID:         req.EventID,
		EventType:       req.EventType,
		EventSource:     req.EventSource,
		EventSubject:    req.EventSubject,
		SensorName:      req.SensorName,
		FailedTrigger:   req.FailedTrigger,
		EventSourceURL:  req.EventSourceURL,
		Namespace:       req.Namespace,
		OriginalPayload: payloadBytes,
		OriginalHeaders: req.OriginalHeaders,
		DataContentType: req.DataContentType,
		EventTimestamp:  ts,
	}
	e, err := h.svc.IngestEvent(r.Context(), params)
	if err != nil {
		logger.Error("failed to ingest dlq event", "error", err)
		writeJSON(w, stdhttp.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}
	writeJSON(w, stdhttp.StatusCreated, toResponse(e))
}

func (h *Handler) listEvents(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	filter := port.ListFilter{
		Status:        q.Get("status"),
		EventSource:   q.Get("event_source"),
		SensorName:    q.Get("sensor_name"),
		FailedTrigger: q.Get("failed_trigger"),
		Offset:        offset,
		Limit:         limit,
	}
	if v := q.Get("created_after"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.CreatedAfter = &t
		}
	}
	if v := q.Get("created_before"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.CreatedBefore = &t
		}
	}

	events, total, err := h.svc.ListEvents(r.Context(), filter)
	if err != nil {
		writeJSON(w, stdhttp.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	items := make([]DLQEventResponse, 0, len(events))
	for i := range events {
		items = append(items, toResponse(&events[i]))
	}
	writeJSON(w, stdhttp.StatusOK, ListEventsResponse{
		Items:      items,
		TotalItems: total,
		Offset:     offset,
		Limit:      limit,
	})
}

func (h *Handler) getEvent(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	e, err := h.svc.GetEvent(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, stdhttp.StatusNotFound, ErrorResponse{Error: "event not found"})
			return
		}
		writeJSON(w, stdhttp.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}
	writeJSON(w, stdhttp.StatusOK, toResponse(e))
}

func (h *Handler) replayEvent(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	e, err := h.svc.ReplayEvent(r.Context(), id)
	if err != nil {
		writeTransitionError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toResponse(e))
}

func (h *Handler) resolveEvent(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	var req ResolveRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ResolvedBy == "" {
		writeJSON(w, stdhttp.StatusBadRequest, ErrorResponse{Error: "resolved_by is required"})
		return
	}
	e, err := h.svc.ResolveEvent(r.Context(), id, req.ResolvedBy)
	if err != nil {
		writeTransitionError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toResponse(e))
}

func (h *Handler) resetPoisoned(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	e, err := h.svc.ResetPoisoned(r.Context(), id)
	if err != nil {
		writeTransitionError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toResponse(e))
}

func (h *Handler) batchReplay(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var req BatchReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = defaultLimit
	}
	n, err := h.svc.BatchReplay(r.Context(), port.BatchFilter{
		Status:        req.Status,
		EventSource:   req.EventSource,
		SensorName:    req.SensorName,
		FailedTrigger: req.FailedTrigger,
		Limit:         req.Limit,
	})
	if err != nil {
		writeJSON(w, stdhttp.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, stdhttp.StatusOK, BatchActionResponse{AffectedCount: n})
}

func (h *Handler) batchResolve(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var req BatchResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}
	if req.ResolvedBy == "" || len(req.IDs) == 0 {
		writeJSON(w, stdhttp.StatusBadRequest, ErrorResponse{Error: "ids and resolved_by required"})
		return
	}
	n, err := h.svc.BatchResolve(r.Context(), req.IDs, req.ResolvedBy)
	if err != nil {
		writeJSON(w, stdhttp.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, stdhttp.StatusOK, BatchActionResponse{AffectedCount: n})
}

func writeTransitionError(w stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, stdhttp.StatusNotFound, ErrorResponse{Error: "event not found"})
	case errors.Is(err, domain.ErrAlreadyResolved):
		writeJSON(w, stdhttp.StatusConflict, ErrorResponse{Error: "event already resolved"})
	case errors.Is(err, domain.ErrInvalidTransition):
		writeJSON(w, stdhttp.StatusConflict, ErrorResponse{Error: "invalid status transition"})
	default:
		writeJSON(w, stdhttp.StatusInternalServerError, ErrorResponse{Error: "internal error"})
	}
}

func marshalPayload(p any) ([]byte, error) {
	if p == nil {
		return []byte(`null`), nil
	}
	// If already []byte or json.RawMessage, pass through.
	if b, ok := p.([]byte); ok {
		return b, nil
	}
	return json.Marshal(p)
}

func writeJSON(w stdhttp.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
