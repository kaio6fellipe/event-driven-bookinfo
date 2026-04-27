// Package http provides HTTP handlers and DTOs for the reviews service.
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
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
	api.Register(mux, Endpoints, map[string]http.HandlerFunc{
		"GET /v1/reviews/{id}":    h.getProductReviews,
		"POST /v1/reviews":        h.submitReview,
		"POST /v1/reviews/delete": h.deleteReview,
	})
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
			CreatedAt: review.CreatedAt,
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

	review, err := h.svc.SubmitReview(r.Context(), req.ProductID, req.Reviewer, req.Text, req.IdempotencyKey)
	if err != nil {
		if errors.Is(err, service.ErrAlreadyProcessed) {
			logger.Info("duplicate submit skipped")
			writeJSON(w, http.StatusOK, ErrorResponse{Error: "already processed"})
			return
		}
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
		CreatedAt: review.CreatedAt,
	})
}

func (h *Handler) deleteReview(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req DeleteReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ReviewID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "review_id is required"})
		return
	}

	if err := h.svc.DeleteReview(r.Context(), req.ReviewID); err != nil {
		logger.Error("failed to delete review", "error", err, "review_id", req.ReviewID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	logger.Info("review deleted", "review_id", req.ReviewID)
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
