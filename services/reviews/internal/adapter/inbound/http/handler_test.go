// file: services/reviews/internal/adapter/inbound/http/handler_test.go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

type stubRatingsClient struct {
	rating *domain.ReviewRating
	err    error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.ReviewRating, error) {
	return s.rating, s.err
}

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		rating: &domain.ReviewRating{Average: 4.0, Count: 5},
	}
	svc := service.NewReviewService(repo, client)
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestSubmitReview_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitReviewRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Text:      "Great book!",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body handler.ReviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
	if body.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", body.ProductID, "product-1")
	}
}

func TestSubmitReview_InvalidBody(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubmitReview_EmptyText(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitReviewRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Text:      "",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetProductReviews_WithRatings(t *testing.T) {
	mux := setupHandler(t)

	// Submit a review
	reqBody := handler.SubmitReviewRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Text:      "Loved it!",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	// Get reviews
	getReq := httptest.NewRequest(http.MethodGet, "/v1/reviews/product-1", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var body handler.ProductReviewsResponse
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", body.ProductID, "product-1")
	}

	if len(body.Reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(body.Reviews))
	}

	review := body.Reviews[0]
	if review.Rating == nil {
		t.Fatal("expected non-nil rating")
	}
	if review.Rating.Average != 4.0 {
		t.Errorf("Rating.Average = %f, want 4.0", review.Rating.Average)
	}
	if review.Rating.Count != 5 {
		t.Errorf("Rating.Count = %d, want 5", review.Rating.Count)
	}
}

func TestGetProductReviews_Empty(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/reviews/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.ProductReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if len(body.Reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(body.Reviews))
	}
}
