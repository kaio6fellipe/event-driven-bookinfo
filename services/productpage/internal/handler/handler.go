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
)

// Handler holds the HTTP handlers for the productpage BFF.
type Handler struct {
	detailsClient *client.DetailsClient
	reviewsClient *client.ReviewsClient
	ratingsClient *client.RatingsClient
	templates     *template.Template
	templateDir   string
}

// NewHandler creates a new productpage handler.
func NewHandler(
	detailsClient *client.DetailsClient,
	reviewsClient *client.ReviewsClient,
	ratingsClient *client.RatingsClient,
	templateDir string,
) *Handler {
	tmpl := template.Must(template.ParseGlob(filepath.Join(templateDir, "*.html")))
	template.Must(tmpl.ParseGlob(filepath.Join(templateDir, "partials", "*.html")))

	return &Handler{
		detailsClient: detailsClient,
		reviewsClient: reviewsClient,
		ratingsClient: ratingsClient,
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

	reviews, err := h.reviewsClient.GetProductReviews(r.Context(), productID)
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

	reviews, err := h.reviewsClient.GetProductReviews(r.Context(), productID)
	if err != nil {
		logger.Warn("failed to fetch reviews for partial", "product_id", productID, "error", err)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<p class="error">Failed to load reviews.</p>`))
		return
	}

	var viewModels []model.ProductReview
	for _, review := range reviews.Reviews {
		vm := model.ProductReview{
			ID:       review.ID,
			Reviewer: review.Reviewer,
			Text:     review.Text,
		}
		if review.Rating != nil {
			vm.Average = review.Rating.Average
			vm.Count = review.Rating.Count
		}
		viewModels = append(viewModels, vm)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "reviews.html", viewModels)
}

func (h *Handler) partialRatingSubmit(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

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
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
		"Success":   true,
		"Stars":     stars,
		"ProductID": productID,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
