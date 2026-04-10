// file: services/productpage/internal/handler/handler_test.go
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/client"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/handler"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/pending"
)

// projectRoot walks up from the current working directory to find the go.mod file
// and returns the project root directory.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

func templateDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "services", "productpage", "templates")
}

func setupMockServers(t *testing.T) (detailsURL, reviewsURL, ratingsURL string) {
	t.Helper()

	detailsMux := http.NewServeMux()
	detailsMux.HandleFunc("GET /v1/details/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        r.PathValue("id"),
			"title":     "Test Book",
			"author":    "Test Author",
			"year":      2024,
			"type":      "paperback",
			"pages":     300,
			"publisher": "Test Press",
			"language":  "English",
			"isbn_10":   "1234567890",
			"isbn_13":   "1234567890123",
		})
	})
	detailsServer := httptest.NewServer(detailsMux)
	t.Cleanup(detailsServer.Close)

	reviewsMux := http.NewServeMux()
	reviewsMux.HandleFunc("GET /v1/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product_id": r.PathValue("id"),
			"reviews": []map[string]any{
				{
					"id":         "review-1",
					"product_id": r.PathValue("id"),
					"reviewer":   "alice",
					"text":       "Great book!",
					"rating":     map[string]any{"stars": 5, "average": 4.5, "count": 10},
				},
			},
		})
	})
	reviewsServer := httptest.NewServer(reviewsMux)
	t.Cleanup(reviewsServer.Close)

	ratingsMux := http.NewServeMux()
	ratingsMux.HandleFunc("POST /v1/ratings", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "rating-1",
			"product_id": "product-1",
			"reviewer":   "bob",
			"stars":      5,
		})
	})
	ratingsServer := httptest.NewServer(ratingsMux)
	t.Cleanup(ratingsServer.Close)

	return detailsServer.URL, reviewsServer.URL, ratingsServer.URL
}

func TestAPIGetProducts(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/products/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	detail, ok := body["detail"].(map[string]any)
	if !ok {
		t.Fatal("expected detail in response")
	}
	if detail["title"] != "Test Book" {
		t.Errorf("title = %v, want Test Book", detail["title"])
	}
}

func TestPartialDetails(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/partials/details/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Test Book") {
		t.Errorf("expected 'Test Book' in response, got:\n%s", body)
	}
	if !strings.Contains(body, "Test Author") {
		t.Errorf("expected 'Test Author' in response, got:\n%s", body)
	}
}

func TestPartialReviews(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/partials/reviews/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "alice") {
		t.Errorf("expected 'alice' in response, got:\n%s", body)
	}
	if !strings.Contains(body, "Great book!") {
		t.Errorf("expected 'Great book!' in response, got:\n%s", body)
	}
	// Should render 5 filled stars for alice's individual score
	filledStars := strings.Count(body, "star-filled")
	if filledStars != 5 {
		t.Errorf("expected 5 filled stars, got %d", filledStars)
	}
	// Should show "5/5" individual score
	if !strings.Contains(body, "5/5") {
		t.Errorf("expected '5/5' score display in response, got:\n%s", body)
	}
}

func TestPartialRatingSubmit(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	formData := "product_id=product-1&reviewer=bob&stars=5"
	req := httptest.NewRequest(http.MethodPost, "/partials/rating", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "successfully") {
		t.Errorf("expected success message, got:\n%s", body)
	}
}

func TestPartialRatingSubmitAsync(t *testing.T) {
	detailsURL, reviewsURL, _ := setupMockServers(t)

	// Simulate EventSource webhook: returns 200 OK with event ack (not 201 Created)
	asyncRatingsMux := http.NewServeMux()
	asyncRatingsMux.HandleFunc("POST /v1/ratings", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"eventID":     "evt-12345",
			"eventSource": "rating-submitted",
		})
	})
	asyncRatingsServer := httptest.NewServer(asyncRatingsMux)
	t.Cleanup(asyncRatingsServer.Close)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(asyncRatingsServer.URL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	formData := "product_id=product-1&reviewer=bob&stars=5"
	req := httptest.NewRequest(http.MethodPost, "/partials/rating", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "successfully") {
		t.Errorf("expected success message for async write, got:\n%s", body)
	}
}

func TestPartialRatingSubmitAsyncWithReview(t *testing.T) {
	detailsURL, _, _ := setupMockServers(t)

	// Simulate EventSource webhooks: return 200 OK
	asyncRatingsMux := http.NewServeMux()
	asyncRatingsMux.HandleFunc("POST /v1/ratings", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	asyncRatingsServer := httptest.NewServer(asyncRatingsMux)
	t.Cleanup(asyncRatingsServer.Close)

	asyncReviewsMux := http.NewServeMux()
	asyncReviewsMux.HandleFunc("GET /v1/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product_id": r.PathValue("id"),
			"reviews":    []any{},
		})
	})
	asyncReviewsMux.HandleFunc("POST /v1/reviews", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	asyncReviewsServer := httptest.NewServer(asyncReviewsMux)
	t.Cleanup(asyncReviewsServer.Close)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(asyncReviewsServer.URL)
	ratingsClient := client.NewRatingsClient(asyncRatingsServer.URL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	formData := "product_id=product-1&reviewer=bob&stars=5&text=Great+book"
	req := httptest.NewRequest(http.MethodPost, "/partials/rating", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "successfully") {
		t.Errorf("expected success message for async write with review, got:\n%s", body)
	}
}

func TestPartialReviewsWithPending(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	mr := miniredis.RunT(t)
	store, err := pending.NewRedisStore("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("failed to create redis store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Store a pending review
	ctx := context.Background()
	_ = store.StorePending(ctx, "product-1", pending.NewReview("bob", "Pending review text", 4))

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, store, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/partials/reviews/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()

	// Confirmed review from mock server
	if !strings.Contains(body, "alice") {
		t.Errorf("expected confirmed review 'alice' in response")
	}

	// Pending review
	if !strings.Contains(body, "bob") {
		t.Errorf("expected pending review 'bob' in response")
	}
	if !strings.Contains(body, "Pending review text") {
		t.Errorf("expected pending review text in response")
	}
	if !strings.Contains(body, "review-pending") {
		t.Errorf("expected 'review-pending' CSS class in response")
	}
	if !strings.Contains(body, "Processing") {
		t.Errorf("expected 'Processing' label in response")
	}

	// HTMX polling should be active
	if !strings.Contains(body, "every 2s") {
		t.Errorf("expected HTMX polling trigger 'every 2s' in response")
	}
}

func TestPartialReviewsNoPending(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/partials/reviews/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()

	// Should NOT have polling when no pending reviews
	if strings.Contains(body, "every 2s") {
		t.Errorf("expected NO HTMX polling when there are no pending reviews")
	}

	// Should NOT have pending CSS class
	if strings.Contains(body, "review-pending") {
		t.Errorf("expected NO review-pending CSS class when no pending reviews")
	}
}
