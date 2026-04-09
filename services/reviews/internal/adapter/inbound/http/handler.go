// Package http provides HTTP handlers and DTOs for the reviews service.
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

// Handler holds the HTTP handlers for the reviews service.
type Handler struct {
	svc port.ReviewService
}

// NewHandler creates a new HTTP handler with the given review service.
func NewHandler(svc port.ReviewService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the reviews routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/reviews/{id}", h.getProductReviews)
	mux.HandleFunc("POST /v1/reviews", h.submitReview)
}

func (h *Handler) getProductReviews(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	reviews, err := h.svc.GetProductReviews(r.Context(), productID)
	if err != nil {
		logger.Error("failed to get product reviews", "error", err, "product_id", productID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	reviewResponses := make([]ReviewResponse, 0, len(reviews))
	for _, review := range reviews {
		resp := ReviewResponse{
			ID:        review.ID,
			ProductID: review.ProductID,
			Reviewer:  review.Reviewer,
			Text:      review.Text,
		}
		if review.Rating != nil {
			resp.Rating = &ReviewRatingResponse{
				Stars:   review.Rating.Stars,
				Average: review.Rating.Average,
				Count:   review.Rating.Count,
			}
		}
		reviewResponses = append(reviewResponses, resp)
	}

	writeJSON(w, http.StatusOK, ProductReviewsResponse{
		ProductID: productID,
		Reviews:   reviewResponses,
	})
}

func (h *Handler) submitReview(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req SubmitReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	review, err := h.svc.SubmitReview(r.Context(), req.ProductID, req.Reviewer, req.Text)
	if err != nil {
		logger.Warn("failed to submit review", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("review submitted", "review_id", review.ID, "product_id", review.ProductID)

	writeJSON(w, http.StatusCreated, ReviewResponse{
		ID:        review.ID,
		ProductID: review.ProductID,
		Reviewer:  review.Reviewer,
		Text:      review.Text,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
