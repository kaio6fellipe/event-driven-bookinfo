// file: services/reviews/internal/adapter/inbound/http/handler_test.go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

type stubRatingsClient struct {
	data *domain.RatingData
	err  error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.RatingData, error) {
	return s.data, s.err
}

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{
			Average: 4.0,
			Count:   5,
			IndividualRatings: map[string]int{
				"alice": 4,
			},
		},
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
	if review.Rating.Stars != 4 {
		t.Errorf("Rating.Stars = %d, want 4", review.Rating.Stars)
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

func TestGetProductReviews_Pagination(t *testing.T) {
	mux := setupHandler(t)

	// Submit 15 reviews
	for i := 0; i < 15; i++ {
		reqBody := handler.SubmitReviewRequest{
			ProductID: "product-1",
			Reviewer:  "alice",
			Text:      fmt.Sprintf("Review %d", i),
		}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}

	tests := []struct {
		name           string
		url            string
		wantCount      int
		wantPage       int
		wantPageSize   int
		wantTotalItems int
		wantTotalPages int
	}{
		{
			name: "default pagination", url: "/v1/reviews/product-1",
			wantCount: 10, wantPage: 1, wantPageSize: 10, wantTotalItems: 15, wantTotalPages: 2,
		},
		{
			name: "page 2", url: "/v1/reviews/product-1?page=2",
			wantCount: 5, wantPage: 2, wantPageSize: 10, wantTotalItems: 15, wantTotalPages: 2,
		},
		{
			name: "custom page size", url: "/v1/reviews/product-1?page=1&page_size=5",
			wantCount: 5, wantPage: 1, wantPageSize: 5, wantTotalItems: 15, wantTotalPages: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			var body handler.ProductReviewsResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}

			if len(body.Reviews) != tt.wantCount {
				t.Errorf("got %d reviews, want %d", len(body.Reviews), tt.wantCount)
			}
			if body.Pagination.Page != tt.wantPage {
				t.Errorf("Page = %d, want %d", body.Pagination.Page, tt.wantPage)
			}
			if body.Pagination.PageSize != tt.wantPageSize {
				t.Errorf("PageSize = %d, want %d", body.Pagination.PageSize, tt.wantPageSize)
			}
			if body.Pagination.TotalItems != tt.wantTotalItems {
				t.Errorf("TotalItems = %d, want %d", body.Pagination.TotalItems, tt.wantTotalItems)
			}
			if body.Pagination.TotalPages != tt.wantTotalPages {
				t.Errorf("TotalPages = %d, want %d", body.Pagination.TotalPages, tt.wantTotalPages)
			}
		})
	}
}

func TestGetProductReviews_InvalidPagination(t *testing.T) {
	mux := setupHandler(t)

	tests := []struct {
		name string
		url  string
	}{
		{name: "page zero", url: "/v1/reviews/product-1?page=0"},
		{name: "negative page", url: "/v1/reviews/product-1?page=-1"},
		{name: "page_size zero", url: "/v1/reviews/product-1?page_size=0"},
		{name: "page_size over max", url: "/v1/reviews/product-1?page_size=101"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestDeleteReview_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitReviewRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Text:      "Great book!",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created handler.ReviewResponse
	_ = json.NewDecoder(createRec.Body).Decode(&created)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/reviews/"+created.ID, nil)
	deleteRec := httptest.NewRecorder()
	mux.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/reviews/product-1", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	var body handler.ProductReviewsResponse
	_ = json.NewDecoder(getRec.Body).Decode(&body)
	if len(body.Reviews) != 0 {
		t.Errorf("expected 0 reviews after delete, got %d", len(body.Reviews))
	}
}

func TestDeleteReview_NotFound(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/v1/reviews/nonexistent-id", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
