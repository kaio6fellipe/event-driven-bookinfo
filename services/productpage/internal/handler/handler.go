// Package handler provides the HTTP handlers for the productpage BFF service.
package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/client"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/model"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/pending"
)

// Handler holds the HTTP handlers for the productpage BFF.
type Handler struct {
	detailsClient *client.DetailsClient
	reviewsClient *client.ReviewsClient
	ratingsClient *client.RatingsClient
	pendingStore  pending.Store
	templates     *template.Template
	templateDir   string
}

// NewHandler creates a new productpage handler.
func NewHandler(
	detailsClient *client.DetailsClient,
	reviewsClient *client.ReviewsClient,
	ratingsClient *client.RatingsClient,
	pendingStore pending.Store,
	templateDir string,
) *Handler {
	funcMap := template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
		"not":      func(v bool) bool { return !v },
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob(filepath.Join(templateDir, "*.html")))
	template.Must(tmpl.ParseGlob(filepath.Join(templateDir, "partials", "*.html")))

	return &Handler{
		detailsClient: detailsClient,
		reviewsClient: reviewsClient,
		ratingsClient: ratingsClient,
		pendingStore:  pendingStore,
		templates:     tmpl,
		templateDir:   templateDir,
	}
}

// RegisterRoutes registers all productpage routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// HTML pages
	mux.HandleFunc("GET /", h.homePage)
	mux.HandleFunc("GET /products/{id}", h.productPage)

	// JSON API
	mux.HandleFunc("GET /v1/products/{id}", h.apiGetProduct)

	// HTMX partials
	mux.HandleFunc("GET /partials/details/{id}", h.partialDetails)
	mux.HandleFunc("GET /partials/reviews/{id}", h.partialReviews)
	mux.HandleFunc("POST /partials/rating", h.partialRatingSubmit)
	mux.HandleFunc("DELETE /partials/reviews/{id}", h.partialDeleteReview)
}

func (h *Handler) homePage(w http.ResponseWriter, r *http.Request) {
	details, err := h.detailsClient.ListDetails(r.Context())

	var products []model.ProductDetail
	if err == nil {
		for _, d := range details {
			products = append(products, model.ProductDetail{
				ID:     d.ID,
				Title:  d.Title,
				Author: d.Author,
				Year:   d.Year,
				Type:   d.Type,
				Pages:  d.Pages,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "layout.html", struct {
		Products []model.ProductDetail
		Detail   *model.ProductDetail
	}{Products: products})
}

func (h *Handler) productPage(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")

	detail, err := h.detailsClient.GetDetail(r.Context(), productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	data := struct {
		Products []model.ProductDetail
		Detail   *model.ProductDetail
	}{
		Detail: &model.ProductDetail{
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
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "layout.html", data)
}

func (h *Handler) apiGetProduct(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	detail, err := h.detailsClient.GetDetail(r.Context(), productID)
	if err != nil {
		logger.Warn("failed to fetch detail", "product_id", productID, "error", err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "product not found"})
		return
	}

	reviews, err := h.reviewsClient.GetProductReviews(r.Context(), productID, 1, 100)
	if err != nil {
		logger.Warn("failed to fetch reviews", "product_id", productID, "error", err)
		reviews = &client.ProductReviewsResponse{ProductID: productID, Reviews: []client.ReviewResponse{}}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"detail":  detail,
		"reviews": reviews.Reviews,
	})
}

func (h *Handler) partialDetails(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	detail, err := h.detailsClient.GetDetail(r.Context(), productID)
	if err != nil {
		logger.Warn("failed to fetch detail for partial", "product_id", productID, "error", err)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<p class="error">Failed to load details.</p>`))
		return
	}

	data := model.ProductDetail{
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
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "details.html", data)
}

func (h *Handler) partialReviews(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p >= 1 {
			page = p
		}
	}

	reviews, err := h.reviewsClient.GetProductReviews(r.Context(), productID, page, 10)
	if err != nil {
		logger.Warn("failed to fetch reviews for partial", "product_id", productID, "error", err)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<p class="error">Failed to load reviews.</p>`))
		return
	}

	var confirmed []pending.ConfirmedReview
	var confirmedIDs []string
	var viewModels []model.ProductReview
	for _, review := range reviews.Reviews {
		confirmed = append(confirmed, pending.ConfirmedReview{
			Reviewer: review.Reviewer,
			Text:     review.Text,
		})
		confirmedIDs = append(confirmedIDs, review.ID)

		vm := model.ProductReview{
			ID:       review.ID,
			Reviewer: review.Reviewer,
			Text:     review.Text,
		}
		if review.Rating != nil {
			vm.Stars = review.Rating.Stars
			vm.Average = review.Rating.Average
			vm.Count = review.Rating.Count
		}
		viewModels = append(viewModels, vm)
	}

	// Merge pending reviews and deleting IDs from Redis
	pendingReviews, deletingIDs, err := h.pendingStore.GetAndReconcile(r.Context(), productID, confirmed, confirmedIDs)
	if err != nil {
		logger.Warn("failed to get pending reviews", "product_id", productID, "error", err)
	}

	// Only show pending reviews on page 1
	if page == 1 {
		for _, pr := range pendingReviews {
			viewModels = append(viewModels, model.ProductReview{
				Reviewer: pr.Reviewer,
				Text:     pr.Text,
				Stars:    pr.Stars,
				Pending:  true,
			})
		}
	}

	// Mark reviews being deleted
	deletingSet := make(map[string]struct{}, len(deletingIDs))
	for _, id := range deletingIDs {
		deletingSet[id] = struct{}{}
	}
	for i := range viewModels {
		if _, found := deletingSet[viewModels[i].ID]; found {
			viewModels[i].Deleting = true
		}
	}

	hasPendingState := len(pendingReviews) > 0 || len(deletingIDs) > 0

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "reviews.html", struct {
		Reviews    []model.ProductReview
		HasPending bool
		ProductID  string
		Page       int
		TotalPages int
	}{
		Reviews:    viewModels,
		HasPending: hasPendingState,
		ProductID:  productID,
		Page:       page,
		TotalPages: reviews.Pagination.TotalPages,
	})
}

func (h *Handler) partialRatingSubmit(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   "Invalid form data",
		})
		return
	}

	productID := r.FormValue("product_id")
	reviewer := r.FormValue("reviewer")
	starsStr := r.FormValue("stars")
	reviewText := r.FormValue("text")

	stars, err := strconv.Atoi(starsStr)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   "Invalid stars value",
		})
		return
	}

	_, err = h.ratingsClient.SubmitRating(r.Context(), productID, reviewer, stars)
	if err != nil {
		logger.Warn("failed to submit rating", "error", err)
		w.WriteHeader(http.StatusOK)
		_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   err.Error(),
		})
		return
	}

	if reviewText != "" {
		if err := h.reviewsClient.SubmitReview(r.Context(), productID, reviewer, reviewText); err != nil {
			logger.Warn("failed to submit review", "error", err)
		}

		if err := h.pendingStore.StorePending(r.Context(), productID, pending.NewReview(reviewer, reviewText, stars)); err != nil {
			logger.Warn("failed to store pending review", "error", err)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
		"Success":   true,
		"Stars":     stars,
		"ProductID": productID,
	})
}

func (h *Handler) partialDeleteReview(w http.ResponseWriter, r *http.Request) {
	reviewID := r.PathValue("id")
	productID := r.URL.Query().Get("product_id")
	logger := logging.FromContext(r.Context())

	if err := h.reviewsClient.DeleteReview(r.Context(), reviewID); err != nil {
		logger.Warn("failed to delete review", "error", err, "review_id", reviewID)
	}

	if productID != "" {
		if err := h.pendingStore.StoreDeleting(r.Context(), productID, reviewID); err != nil {
			logger.Warn("failed to store deleting state", "error", err)
		}
	}

	// Trigger a refresh of the reviews section
	if productID != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.templates.ExecuteTemplate(w, "delete-refresh.html", map[string]string{
			"ProductID": productID,
		})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
