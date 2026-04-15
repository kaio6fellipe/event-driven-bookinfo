package gateway_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/gateway"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

func TestPublishBookAdded_Success(t *testing.T) {
	t.Parallel()

	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/details" {
			t.Errorf("path = %s, want /v1/details", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %s, want application/json", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pub := gateway.NewPublisher(srv.Client(), srv.URL)
	book := domain.Book{
		Title:       "The Go Programming Language",
		Authors:     []string{"Alan Donovan", "Brian Kernighan"},
		ISBN:        "9780134190440",
		PublishYear: 2015,
		Pages:       380,
		Publisher:   "Addison-Wesley",
		Language:    "ENG",
	}

	err := pub.PublishBookAdded(context.Background(), book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify request body matches details service DTO format
	if receivedBody["title"] != "The Go Programming Language" {
		t.Errorf("title = %v", receivedBody["title"])
	}
	if receivedBody["author"] != "Alan Donovan, Brian Kernighan" {
		t.Errorf("author = %v, want joined string", receivedBody["author"])
	}
	if receivedBody["isbn_13"] != "9780134190440" {
		t.Errorf("isbn_13 = %v", receivedBody["isbn_13"])
	}
	if receivedBody["isbn_10"] != "" {
		t.Errorf("isbn_10 = %v, want empty for ISBN-13", receivedBody["isbn_10"])
	}
	if receivedBody["idempotency_key"] != "ingestion-isbn-9780134190440" {
		t.Errorf("idempotency_key = %v", receivedBody["idempotency_key"])
	}
	if receivedBody["type"] != "paperback" {
		t.Errorf("type = %v, want paperback", receivedBody["type"])
	}
}

func TestPublishBookAdded_ISBN10(t *testing.T) {
	t.Parallel()

	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pub := gateway.NewPublisher(srv.Client(), srv.URL)
	book := domain.Book{
		Title:       "Short ISBN Book",
		Authors:     []string{"Author"},
		ISBN:        "0134190440",
		PublishYear: 2020,
	}

	err := pub.PublishBookAdded(context.Background(), book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["isbn_10"] != "0134190440" {
		t.Errorf("isbn_10 = %v, want the ISBN-10", receivedBody["isbn_10"])
	}
	if receivedBody["isbn_13"] != "" {
		t.Errorf("isbn_13 = %v, want empty for ISBN-10", receivedBody["isbn_13"])
	}
}

func TestPublishBookAdded_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	pub := gateway.NewPublisher(srv.Client(), srv.URL)
	book := domain.Book{
		Title:       "Test",
		Authors:     []string{"Author"},
		ISBN:        "1234567890",
		PublishYear: 2020,
	}

	err := pub.PublishBookAdded(context.Background(), book)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestPublishBookAdded_ConnectionError(t *testing.T) {
	t.Parallel()

	pub := gateway.NewPublisher(http.DefaultClient, "http://localhost:1")
	book := domain.Book{
		Title:       "Test",
		Authors:     []string{"Author"},
		ISBN:        "1234567890",
		PublishYear: 2020,
	}

	err := pub.PublishBookAdded(context.Background(), book)
	if err == nil {
		t.Fatal("expected error for connection failure")
	}
}
