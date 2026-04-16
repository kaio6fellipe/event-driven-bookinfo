package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/port"
)

// Handler holds the HTTP handlers for the ingestion service.
type Handler struct {
	svc port.IngestionService
}

// NewHandler creates a new HTTP handler with the given ingestion service.
func NewHandler(svc port.IngestionService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the ingestion routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/ingestion/trigger", h.triggerScrape)
	mux.HandleFunc("GET /v1/ingestion/status", h.getStatus)
}

func (h *Handler) triggerScrape(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req TriggerRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
			return
		}
	}

	result, err := h.svc.TriggerScrape(r.Context(), req.Queries)
	if err != nil {
		logger.Warn("trigger scrape failed", "error", err)
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, toScrapeResultResponse(result))
}

func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.GetStatus(r.Context())
	if err != nil {
		logger := logging.FromContext(r.Context())
		logger.Error("failed to get status", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, toStatusResponse(status))
}

func toScrapeResultResponse(r *domain.ScrapeResult) ScrapeResultResponse {
	return ScrapeResultResponse{
		BooksFound:      r.BooksFound,
		EventsPublished: r.EventsPublished,
		Errors:          r.Errors,
		DurationMs:      r.Duration.Milliseconds(),
	}
}

func toStatusResponse(s *domain.IngestionStatus) StatusResponse {
	resp := StatusResponse{
		State:     s.State,
		LastRunAt: s.LastRunAt,
	}
	if s.LastResult != nil {
		r := toScrapeResultResponse(s.LastResult)
		resp.LastResult = &r
	}
	return resp
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
