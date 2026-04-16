package openlibrary_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/openlibrary"
)

func TestSearchBooks_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "golang" {
			t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("unexpected limit: %s", r.URL.Query().Get("limit"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"numFound": 2,
			"docs": [
				{
					"title": "The Go Programming Language",
					"author_name": ["Alan Donovan", "Brian Kernighan"],
					"isbn": ["0134190440", "9780134190440"],
					"first_publish_year": 2015,
					"subject": ["Go (Programming language)"],
					"number_of_pages_median": 380,
					"publisher": ["Addison-Wesley"],
					"language": ["eng"]
				},
				{
					"title": "Concurrency in Go",
					"author_name": ["Katherine Cox-Buday"],
					"isbn": ["9781491941195"],
					"first_publish_year": 2017,
					"number_of_pages_median": 238,
					"publisher": ["O'Reilly"],
					"language": ["eng"]
				}
			]
		}`))
	}))
	defer srv.Close()

	client := openlibrary.NewClientWithBaseURL(srv.Client(), srv.URL)
	books, err := client.SearchBooks(context.Background(), "golang", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(books) != 2 {
		t.Fatalf("expected 2 books, got %d", len(books))
	}

	// First book
	if books[0].Title != "The Go Programming Language" {
		t.Errorf("title = %q, want %q", books[0].Title, "The Go Programming Language")
	}
	if len(books[0].Authors) != 2 {
		t.Errorf("authors count = %d, want 2", len(books[0].Authors))
	}
	// Should prefer ISBN-13
	if books[0].ISBN != "9780134190440" {
		t.Errorf("ISBN = %q, want %q", books[0].ISBN, "9780134190440")
	}
	if books[0].PublishYear != 2015 {
		t.Errorf("year = %d, want 2015", books[0].PublishYear)
	}
	if books[0].Pages != 380 {
		t.Errorf("pages = %d, want 380", books[0].Pages)
	}
	if books[0].Language != "ENG" {
		t.Errorf("language = %q, want %q", books[0].Language, "ENG")
	}

	// Second book
	if books[1].Title != "Concurrency in Go" {
		t.Errorf("title = %q, want %q", books[1].Title, "Concurrency in Go")
	}
	if books[1].ISBN != "9781491941195" {
		t.Errorf("ISBN = %q, want %q", books[1].ISBN, "9781491941195")
	}
}

func TestSearchBooks_SkipsInvalidBooks(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Second book has no ISBN — should be skipped
		_, _ = w.Write([]byte(`{
			"numFound": 2,
			"docs": [
				{
					"title": "Valid Book",
					"author_name": ["Author"],
					"isbn": ["9781234567890"],
					"first_publish_year": 2020,
					"number_of_pages_median": 100
				},
				{
					"title": "No ISBN Book",
					"author_name": ["Author"],
					"isbn": [],
					"first_publish_year": 2020
				}
			]
		}`))
	}))
	defer srv.Close()

	client := openlibrary.NewClientWithBaseURL(srv.Client(), srv.URL)
	books, err := client.SearchBooks(context.Background(), "test", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(books) != 1 {
		t.Fatalf("expected 1 valid book, got %d", len(books))
	}
	if books[0].Title != "Valid Book" {
		t.Errorf("title = %q, want %q", books[0].Title, "Valid Book")
	}
}

func TestSearchBooks_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := openlibrary.NewClientWithBaseURL(srv.Client(), srv.URL)
	_, err := client.SearchBooks(context.Background(), "test", 10)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSearchBooks_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := openlibrary.NewClientWithBaseURL(srv.Client(), srv.URL)
	_, err := client.SearchBooks(context.Background(), "test", 10)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSearchBooks_EmptyResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"numFound": 0, "docs": []}`))
	}))
	defer srv.Close()

	client := openlibrary.NewClientWithBaseURL(srv.Client(), srv.URL)
	books, err := client.SearchBooks(context.Background(), "nonexistent", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("expected 0 books, got %d", len(books))
	}
}

func TestSearchBooks_PreferISBN13(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"numFound": 1,
			"docs": [{
				"title": "Test Book",
				"author_name": ["Author"],
				"isbn": ["0134190440", "9780134190440", "0987654321"],
				"first_publish_year": 2020
			}]
		}`))
	}))
	defer srv.Close()

	client := openlibrary.NewClientWithBaseURL(srv.Client(), srv.URL)
	books, err := client.SearchBooks(context.Background(), "test", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	if books[0].ISBN != "9780134190440" {
		t.Errorf("ISBN = %q, want ISBN-13 %q", books[0].ISBN, "9780134190440")
	}
}
