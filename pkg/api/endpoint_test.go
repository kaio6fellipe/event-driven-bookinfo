package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
)

func TestRegister_DispatchesByMethodAndPath(t *testing.T) {
	endpoints := []api.Endpoint{
		{Method: "GET", Path: "/v1/things/{id}"},
		{Method: "POST", Path: "/v1/things"},
	}

	hits := map[string]int{}
	handlers := map[string]http.HandlerFunc{
		"GET /v1/things/{id}": func(w http.ResponseWriter, r *http.Request) {
			hits["get"]++
			w.WriteHeader(http.StatusOK)
		},
		"POST /v1/things": func(w http.ResponseWriter, r *http.Request) {
			hits["post"]++
			w.WriteHeader(http.StatusCreated)
		},
	}

	mux := http.NewServeMux()
	api.Register(mux, endpoints, handlers)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/things/abc", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/things", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201", rec.Code)
	}

	if hits["get"] != 1 || hits["post"] != 1 {
		t.Errorf("hits = %v, want each = 1", hits)
	}
}

func TestRegister_PanicsOnMissingHandler(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing handler, got none")
		}
	}()

	api.Register(http.NewServeMux(),
		[]api.Endpoint{{Method: "GET", Path: "/v1/missing"}},
		map[string]http.HandlerFunc{})
}
