// file: services/ratings/internal/adapter/inbound/http/handler_test.go
package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/inbound/http"
	messagingadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/messaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
)

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo, idempotency.NewMemoryStore(), messagingadapter.NewNoopPublisher())
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestGetProductRatings_Empty(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/ratings/product-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.ProductRatingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", body.ProductID, "product-1")
	}
	if body.Count != 0 {
		t.Errorf("Count = %d, want 0", body.Count)
	}
	if body.Average != 0 {
		t.Errorf("Average = %f, want 0", body.Average)
	}
}

func TestSubmitRating_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitRatingRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Stars:     5,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body handler.RatingResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
	if body.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", body.ProductID, "product-1")
	}
	if body.Stars != 5 {
		t.Errorf("Stars = %d, want 5", body.Stars)
	}
}

func TestSubmitRating_InvalidStars(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitRatingRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Stars:     6,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubmitRating_EmptyBody(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubmitAndGet_RoundTrip(t *testing.T) {
	mux := setupHandler(t)

	// Submit two ratings for the same product
	for _, reviewer := range []string{"alice", "bob"} {
		reqBody := handler.SubmitRatingRequest{
			ProductID: "product-1",
			Reviewer:  reviewer,
			Stars:     4,
		}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("submit status = %d, want %d", rec.Code, http.StatusCreated)
		}
	}

	// Get ratings
	req := httptest.NewRequest(http.MethodGet, "/v1/ratings/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.ProductRatingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.Count != 2 {
		t.Errorf("Count = %d, want 2", body.Count)
	}
	if body.Average != 4.0 {
		t.Errorf("Average = %f, want 4.0", body.Average)
	}
	if len(body.Ratings) != 2 {
		t.Errorf("len(Ratings) = %d, want 2", len(body.Ratings))
	}
}

func TestSubmitRating_InvalidJSON(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
