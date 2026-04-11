// Package http provides HTTP handlers and DTOs for the reviews service.
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

const (
	defaultPage     = 1
	defaultPageSize = 10
	maxPageSize     = 100
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
	mux.HandleFunc("DELETE /v1/reviews/{id}", h.deleteReview)
	mux.HandleFunc("DELETE /v1/reviews", h.deleteReview)
}

func (h *Handler) getProductReviews(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	page, pageSize, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	reviews, total, err := h.svc.GetProductReviews(r.Context(), productID, page, pageSize)
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

	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	writeJSON(w, http.StatusOK, ProductReviewsResponse{
		ProductID: productID,
		Reviews:   reviewResponses,
		Pagination: PaginationResponse{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
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

func (h *Handler) deleteReview(w http.ResponseWriter, r *http.Request) {
	reviewID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	// The Sensor forwards the review ID in the JSON body (event-driven path),
	// while direct API calls use the URL path param. Support both.
	if reviewID == "" {
		var body struct {
			ReviewID string `json:"review_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.ReviewID != "" {
			reviewID = body.ReviewID
		}
	}

	if reviewID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "review ID is required"})
		return
	}

	err := h.svc.DeleteReview(r.Context(), reviewID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "review not found"})
			return
		}
		logger.Error("failed to delete review", "error", err, "review_id", reviewID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	logger.Info("review deleted", "review_id", reviewID)
	w.WriteHeader(http.StatusNoContent)
}

func parsePagination(r *http.Request) (page, pageSize int, err error) {
	page = defaultPage
	pageSize = defaultPageSize

	if v := r.URL.Query().Get("page"); v != "" {
		page, err = strconv.Atoi(v)
		if err != nil || page < 1 {
			return 0, 0, errors.New("page must be a positive integer")
		}
	}

	if v := r.URL.Query().Get("page_size"); v != "" {
		pageSize, err = strconv.Atoi(v)
		if err != nil || pageSize < 1 || pageSize > maxPageSize {
			return 0, 0, errors.New("page_size must be between 1 and 100")
		}
	}

	return page, pageSize, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
