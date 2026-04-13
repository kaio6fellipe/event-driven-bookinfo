package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore())
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestAddDetail_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.AddDetailRequest{
		Title:     "The Art of Go",
		Author:    "Jane Doe",
		Year:      2024,
		Type:      "paperback",
		Pages:     350,
		Publisher: "Go Press",
		Language:  "English",
		ISBN10:    "1234567890",
		ISBN13:    "1234567890123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/details", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body handler.DetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
	if body.Title != "The Art of Go" {
		t.Errorf("Title = %q, want %q", body.Title, "The Art of Go")
	}
	if body.Author != "Jane Doe" {
		t.Errorf("Author = %q, want %q", body.Author, "Jane Doe")
	}
}

func TestAddDetail_InvalidBody(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/details", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAddDetail_MissingTitle(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.AddDetailRequest{
		Author: "Jane Doe",
		Year:   2024,
		Pages:  350,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/details", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetDetail_Found(t *testing.T) {
	mux := setupHandler(t)

	// Create a detail first
	reqBody := handler.AddDetailRequest{
		Title:     "Concurrency in Go",
		Author:    "John Smith",
		Year:      2023,
		Type:      "hardcover",
		Pages:     400,
		Publisher: "Tech Books",
		Language:  "English",
		ISBN10:    "0987654321",
		ISBN13:    "0987654321098",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/details", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created handler.DetailResponse
	_ = json.NewDecoder(createRec.Body).Decode(&created)

	// Get the detail
	getReq := httptest.NewRequest(http.MethodGet, "/v1/details/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var body handler.DetailResponse
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID != created.ID {
		t.Errorf("ID = %q, want %q", body.ID, created.ID)
	}
	if body.Title != "Concurrency in Go" {
		t.Errorf("Title = %q, want %q", body.Title, "Concurrency in Go")
	}
}

func TestGetDetail_NotFound(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/details/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
