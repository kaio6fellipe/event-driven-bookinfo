// Package http provides HTTP handlers and DTOs for the details service.
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)

// Handler holds the HTTP handlers for the details service.
type Handler struct {
	svc port.DetailService
}

// NewHandler creates a new HTTP handler with the given detail service.
func NewHandler(svc port.DetailService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the details routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/details", h.listDetails)
	mux.HandleFunc("GET /v1/details/{id}", h.getDetail)
	mux.HandleFunc("POST /v1/details", h.addDetail)
}

func (h *Handler) listDetails(w http.ResponseWriter, r *http.Request) {
	details, err := h.svc.ListDetails(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to list details"})
		return
	}

	resp := make([]DetailResponse, 0, len(details))
	for _, d := range details {
		resp = append(resp, DetailResponse{
			ID:        d.ID,
			Title:     d.Title,
			Author:    d.Author,
			Year:      d.Year,
			Type:      d.Type,
			Pages:     d.Pages,
			Publisher: d.Publisher,
			Language:  d.Language,
			ISBN10:    d.ISBN10,
			ISBN13:    d.ISBN13,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	detail, err := h.svc.GetDetail(r.Context(), id)
	if err != nil {
		logger.Warn("detail not found", "id", id, "error", err)
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "detail not found"})
		return
	}

	writeJSON(w, http.StatusOK, DetailResponse{
		ID:        detail.ID,
		Title:     detail.Title,
		Author:    detail.Author,
		Year:      detail.Year,
		Type:      detail.Type,
		Pages:     detail.Pages,
		Publisher: detail.Publisher,
		Language:  detail.Language,
		ISBN10:    detail.ISBN10,
		ISBN13:    detail.ISBN13,
	})
}

func (h *Handler) addDetail(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req AddDetailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	detail, err := h.svc.AddDetail(r.Context(),
		req.Title, req.Author, req.Year, req.Type,
		req.Pages, req.Publisher, req.Language, req.ISBN10, req.ISBN13, req.IdempotencyKey,
	)
	if err != nil {
		if errors.Is(err, service.ErrAlreadyProcessed) {
			logger.Info("duplicate add skipped")
			writeJSON(w, http.StatusOK, ErrorResponse{Error: "already processed"})
			return
		}
		logger.Warn("failed to add detail", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("detail added", "detail_id", detail.ID, "title", detail.Title)

	writeJSON(w, http.StatusCreated, DetailResponse{
		ID:        detail.ID,
		Title:     detail.Title,
		Author:    detail.Author,
		Year:      detail.Year,
		Type:      detail.Type,
		Pages:     detail.Pages,
		Publisher: detail.Publisher,
		Language:  detail.Language,
		ISBN10:    detail.ISBN10,
		ISBN13:    detail.ISBN13,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
