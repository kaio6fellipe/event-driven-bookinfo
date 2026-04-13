// Package http provides HTTP handlers and DTOs for the ratings service.
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
)

// Handler holds the HTTP handlers for the ratings service.
type Handler struct {
	svc port.RatingService
}

// NewHandler creates a new HTTP handler with the given rating service.
func NewHandler(svc port.RatingService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the ratings routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/ratings/{id}", h.getProductRatings)
	mux.HandleFunc("POST /v1/ratings", h.submitRating)
}

func (h *Handler) getProductRatings(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	pr, err := h.svc.GetProductRatings(r.Context(), productID)
	if err != nil {
		logger.Error("failed to get product ratings", "error", err, "product_id", productID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	ratings := make([]RatingResponse, 0, len(pr.Ratings))
	for _, rating := range pr.Ratings {
		ratings = append(ratings, RatingResponse{
			ID:        rating.ID,
			ProductID: rating.ProductID,
			Reviewer:  rating.Reviewer,
			Stars:     rating.Stars,
		})
	}

	writeJSON(w, http.StatusOK, ProductRatingsResponse{
		ProductID: pr.ProductID,
		Average:   pr.Average(),
		Count:     len(pr.Ratings),
		Ratings:   ratings,
	})
}

func (h *Handler) submitRating(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req SubmitRatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	rating, err := h.svc.SubmitRating(r.Context(), req.ProductID, req.Reviewer, req.Stars, req.IdempotencyKey)
	if err != nil {
		if errors.Is(err, service.ErrAlreadyProcessed) {
			logger.Info("duplicate submit skipped")
			writeJSON(w, http.StatusOK, ErrorResponse{Error: "already processed"})
			return
		}
		logger.Warn("failed to submit rating", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("rating submitted", "rating_id", rating.ID, "product_id", rating.ProductID)

	writeJSON(w, http.StatusCreated, RatingResponse{
		ID:        rating.ID,
		ProductID: rating.ProductID,
		Reviewer:  rating.Reviewer,
		Stars:     rating.Stars,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
