package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// stubService implements port.IngestionService for handler tests.
type stubService struct {
	triggerResult *domain.ScrapeResult
	triggerErr    error
	statusResult  *domain.IngestionStatus
	lastQueries   []string
}

func (s *stubService) TriggerScrape(_ context.Context, queries []string) (*domain.ScrapeResult, error) {
	s.lastQueries = queries
	return s.triggerResult, s.triggerErr
}

func (s *stubService) GetStatus(_ context.Context) (*domain.IngestionStatus, error) {
	return s.statusResult, nil
}

func setupHandler(t *testing.T, svc *stubService) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestTriggerScrape_Success_EmptyBody(t *testing.T) {
	svc := &stubService{
		triggerResult: &domain.ScrapeResult{
			BooksFound:      5,
			EventsPublished: 4,
			Errors:          1,
			Duration:        2 * time.Second,
		},
	}
	mux := setupHandler(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingestion/trigger", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.ScrapeResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if body.BooksFound != 5 {
		t.Errorf("BooksFound = %d, want 5", body.BooksFound)
	}
	if body.EventsPublished != 4 {
		t.Errorf("EventsPublished = %d, want 4", body.EventsPublished)
	}
	if body.DurationMs != 2000 {
		t.Errorf("DurationMs = %d, want 2000", body.DurationMs)
	}
}

func TestTriggerScrape_WithQueryOverrides(t *testing.T) {
	svc := &stubService{
		triggerResult: &domain.ScrapeResult{},
	}
	mux := setupHandler(t, svc)

	reqBody := handler.TriggerRequest{Queries: []string{"rust", "python"}}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingestion/trigger", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if len(svc.lastQueries) != 2 || svc.lastQueries[0] != "rust" {
		t.Errorf("queries = %v, want [rust python]", svc.lastQueries)
	}
}

func TestTriggerScrape_AlreadyRunning(t *testing.T) {
	svc := &stubService{
		triggerErr: fmt.Errorf("ingestion cycle already running"),
	}
	mux := setupHandler(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingestion/trigger", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestTriggerScrape_InvalidJSON(t *testing.T) {
	svc := &stubService{}
	mux := setupHandler(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingestion/trigger", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	// Set content length to trigger body parsing
	req.ContentLength = 8
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetStatus_Idle(t *testing.T) {
	svc := &stubService{
		statusResult: &domain.IngestionStatus{State: "idle"},
	}
	mux := setupHandler(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/v1/ingestion/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if body.State != "idle" {
		t.Errorf("State = %q, want %q", body.State, "idle")
	}
	if body.LastRunAt != nil {
		t.Error("expected nil LastRunAt for idle status")
	}
}

func TestGetStatus_AfterRun(t *testing.T) {
	now := time.Now()
	svc := &stubService{
		statusResult: &domain.IngestionStatus{
			State:     "idle",
			LastRunAt: &now,
			LastResult: &domain.ScrapeResult{
				BooksFound:      3,
				EventsPublished: 3,
				Duration:        time.Second,
			},
		},
	}
	mux := setupHandler(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/v1/ingestion/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if body.LastResult == nil {
		t.Fatal("expected non-nil LastResult")
	}
	if body.LastResult.BooksFound != 3 {
		t.Errorf("BooksFound = %d, want 3", body.LastResult.BooksFound)
	}
}
