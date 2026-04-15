# Ingestion Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a stateless ingestion service that scrapes Open Library for coding books and publishes events to the Gateway, exercising the full CQRS pipeline.

**Architecture:** Hexagonal architecture with two outbound adapters (Open Library client, Gateway publisher) and one inbound HTTP adapter (trigger/status endpoints). A background goroutine runs a `time.Ticker` poll loop. No storage — fully stateless.

**Tech Stack:** Go 1.26, `net/http`, `otelhttp`, Open Library Search API, OTel metrics/tracing/profiling via existing `pkg/` packages.

---

### Task 1: Domain Types

**Files:**
- Create: `services/ingestion/internal/core/domain/book.go`

- [ ] **Step 1: Write the domain types file**

```go
// Package domain contains the core domain model for the ingestion service.
package domain

import (
	"fmt"
	"time"
)

// Book represents a book fetched from an external catalog.
type Book struct {
	Title       string
	Authors     []string
	ISBN        string
	PublishYear int
	Subjects    []string
	Pages       int
	Publisher   string
	Language    string
}

// Validate checks that the book has the minimum required fields for publishing.
func (b *Book) Validate() error {
	if b.Title == "" {
		return fmt.Errorf("title is required")
	}
	if b.ISBN == "" {
		return fmt.Errorf("ISBN is required")
	}
	if len(b.Authors) == 0 {
		return fmt.Errorf("at least one author is required")
	}
	if b.PublishYear <= 0 {
		return fmt.Errorf("publish year must be positive, got %d", b.PublishYear)
	}
	return nil
}

// ScrapeResult contains the outcome of a single ingestion cycle.
type ScrapeResult struct {
	BooksFound      int           `json:"books_found"`
	EventsPublished int           `json:"events_published"`
	Errors          int           `json:"errors"`
	Duration        time.Duration `json:"duration_ms"`
}

// IngestionStatus represents the current state of the ingestion service.
type IngestionStatus struct {
	State      string        `json:"state"`
	LastRunAt  *time.Time    `json:"last_run_at,omitempty"`
	LastResult *ScrapeResult `json:"last_result,omitempty"`
}
```

- [ ] **Step 2: Verify the file compiles**

Run: `go build ./services/ingestion/internal/core/domain/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/core/domain/book.go
git commit -m "feat(ingestion): add domain types for book, scrape result, and ingestion status"
```

---

### Task 2: Domain Validation Tests

**Files:**
- Create: `services/ingestion/internal/core/domain/book_test.go`

- [ ] **Step 1: Write the domain validation tests**

```go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

func TestBook_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		book    domain.Book
		wantErr bool
	}{
		{
			name: "valid book",
			book: domain.Book{
				Title:       "The Go Programming Language",
				Authors:     []string{"Alan Donovan"},
				ISBN:        "9780134190440",
				PublishYear: 2015,
			},
		},
		{
			name: "empty title",
			book: domain.Book{
				Authors:     []string{"Author"},
				ISBN:        "1234567890",
				PublishYear: 2020,
			},
			wantErr: true,
		},
		{
			name: "empty ISBN",
			book: domain.Book{
				Title:       "Some Book",
				Authors:     []string{"Author"},
				PublishYear: 2020,
			},
			wantErr: true,
		},
		{
			name: "no authors",
			book: domain.Book{
				Title:       "Some Book",
				Authors:     []string{},
				ISBN:        "1234567890",
				PublishYear: 2020,
			},
			wantErr: true,
		},
		{
			name: "nil authors",
			book: domain.Book{
				Title:       "Some Book",
				ISBN:        "1234567890",
				PublishYear: 2020,
			},
			wantErr: true,
		},
		{
			name: "zero publish year",
			book: domain.Book{
				Title:       "Some Book",
				Authors:     []string{"Author"},
				ISBN:        "1234567890",
				PublishYear: 0,
			},
			wantErr: true,
		},
		{
			name: "negative publish year",
			book: domain.Book{
				Title:       "Some Book",
				Authors:     []string{"Author"},
				ISBN:        "1234567890",
				PublishYear: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.book.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./services/ingestion/internal/core/domain/ -v`
Expected: all 7 subtests PASS

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/core/domain/book_test.go
git commit -m "test(ingestion): add table-driven validation tests for Book domain type"
```

---

### Task 3: Port Interfaces

**Files:**
- Create: `services/ingestion/internal/core/port/inbound.go`
- Create: `services/ingestion/internal/core/port/outbound.go`

- [ ] **Step 1: Write the inbound port**

```go
// Package port defines the inbound and outbound interfaces for the ingestion service.
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// IngestionService defines the inbound operations for the ingestion domain.
type IngestionService interface {
	// TriggerScrape runs an immediate ingestion cycle with the given search queries.
	// If queries is empty, the service uses its configured default queries.
	TriggerScrape(ctx context.Context, queries []string) (*domain.ScrapeResult, error)

	// GetStatus returns the current ingestion state.
	GetStatus(ctx context.Context) (*domain.IngestionStatus, error)
}
```

- [ ] **Step 2: Write the outbound ports**

```go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// BookFetcher retrieves books from an external catalog.
type BookFetcher interface {
	// SearchBooks searches for books matching the query, returning up to limit results.
	SearchBooks(ctx context.Context, query string, limit int) ([]domain.Book, error)
}

// EventPublisher sends events to the internal event pipeline.
// Returns nil when the EventSource webhook accepts the event (HTTP 200).
// Returns error on non-200 responses or connection failures.
type EventPublisher interface {
	// PublishBookAdded sends a book-added event to the Gateway.
	PublishBookAdded(ctx context.Context, book domain.Book) error
}
```

- [ ] **Step 3: Verify ports compile**

Run: `go build ./services/ingestion/internal/core/port/`
Expected: no output (success)

- [ ] **Step 4: Commit**

```bash
git add services/ingestion/internal/core/port/inbound.go services/ingestion/internal/core/port/outbound.go
git commit -m "feat(ingestion): define inbound and outbound port interfaces"
```

---

### Task 4: Open Library Client

**Files:**
- Create: `services/ingestion/internal/adapter/outbound/openlibrary/client.go`

- [ ] **Step 1: Write the Open Library client**

The Open Library Search API returns JSON like:
```json
{
  "numFound": 26,
  "docs": [
    {
      "title": "GOLANG Programming",
      "author_name": ["Ray Yao"],
      "isbn": ["9798457310421"],
      "first_publish_year": 2021,
      "subject": ["Go (Programming language)"],
      "number_of_pages_median": 200,
      "publisher": ["O'Reilly"],
      "language": ["eng"]
    }
  ]
}
```

```go
// Package openlibrary implements the BookFetcher port using the Open Library Search API.
package openlibrary

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

const (
	defaultBaseURL = "https://openlibrary.org"
	searchPath     = "/search.json"
	searchFields   = "title,author_name,isbn,first_publish_year,subject,number_of_pages_median,publisher,language"
)

// searchResponse is the top-level Open Library search response.
type searchResponse struct {
	NumFound int         `json:"numFound"`
	Docs     []searchDoc `json:"docs"`
}

// searchDoc is a single book entry from the Open Library search results.
type searchDoc struct {
	Title            string   `json:"title"`
	AuthorName       []string `json:"author_name"`
	ISBN             []string `json:"isbn"`
	FirstPublishYear int      `json:"first_publish_year"`
	Subject          []string `json:"subject"`
	Pages            int      `json:"number_of_pages_median"`
	Publisher        []string `json:"publisher"`
	Language         []string `json:"language"`
}

// Client implements port.BookFetcher using the Open Library Search API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Open Library client with the given HTTP client.
func NewClient(httpClient *http.Client) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    defaultBaseURL,
	}
}

// NewClientWithBaseURL creates a new Open Library client with a custom base URL (for testing).
func NewClientWithBaseURL(httpClient *http.Client, baseURL string) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// SearchBooks searches Open Library for books matching the query.
func (c *Client) SearchBooks(ctx context.Context, query string, limit int) ([]domain.Book, error) {
	logger := logging.FromContext(ctx)

	u, err := url.Parse(c.baseURL + searchPath)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	q := u.Query()
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("fields", searchFields)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	books := make([]domain.Book, 0, len(searchResp.Docs))
	for _, doc := range searchResp.Docs {
		book := docToBook(doc)
		if err := book.Validate(); err != nil {
			logger.Debug("skipping invalid book from Open Library", "title", doc.Title, "error", err)
			continue
		}
		books = append(books, book)
	}

	logger.Debug("fetched books from Open Library", "query", query, "found", searchResp.NumFound, "valid", len(books))
	return books, nil
}

// docToBook converts an Open Library search doc to a domain Book.
func docToBook(doc searchDoc) domain.Book {
	isbn := ""
	if len(doc.ISBN) > 0 {
		// Prefer ISBN-13 (13 digits) over ISBN-10 (10 digits)
		for _, i := range doc.ISBN {
			if len(i) == 13 {
				isbn = i
				break
			}
		}
		if isbn == "" {
			isbn = doc.ISBN[0]
		}
	}

	publisher := ""
	if len(doc.Publisher) > 0 {
		publisher = doc.Publisher[0]
	}

	language := ""
	if len(doc.Language) > 0 {
		language = strings.ToUpper(doc.Language[0])
	}

	return domain.Book{
		Title:       doc.Title,
		Authors:     doc.AuthorName,
		ISBN:        isbn,
		PublishYear: doc.FirstPublishYear,
		Subjects:    doc.Subject,
		Pages:       doc.Pages,
		Publisher:   publisher,
		Language:    language,
	}
}
```

- [ ] **Step 2: Verify client compiles**

Run: `go build ./services/ingestion/internal/adapter/outbound/openlibrary/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/adapter/outbound/openlibrary/client.go
git commit -m "feat(ingestion): implement Open Library API client adapter"
```

---

### Task 5: Open Library Client Tests

**Files:**
- Create: `services/ingestion/internal/adapter/outbound/openlibrary/client_test.go`

- [ ] **Step 1: Write client tests with canned JSON responses**

```go
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
```

- [ ] **Step 2: Run tests**

Run: `go test ./services/ingestion/internal/adapter/outbound/openlibrary/ -v`
Expected: all 6 tests PASS

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/adapter/outbound/openlibrary/client_test.go
git commit -m "test(ingestion): add Open Library client tests with canned JSON responses"
```

---

### Task 6: Gateway Publisher

**Files:**
- Create: `services/ingestion/internal/adapter/outbound/gateway/publisher.go`

The publisher must transform a `domain.Book` into the `AddDetailRequest` JSON format that the details service expects (see `services/details/internal/adapter/inbound/http/dto.go`).

- [ ] **Step 1: Write the gateway publisher**

```go
// Package gateway implements the EventPublisher port using HTTP calls to the Envoy Gateway.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// addDetailRequest matches the details service AddDetailRequest DTO.
type addDetailRequest struct {
	Title          string `json:"title"`
	Author         string `json:"author"`
	Year           int    `json:"year"`
	Type           string `json:"type"`
	Pages          int    `json:"pages"`
	Publisher      string `json:"publisher"`
	Language       string `json:"language"`
	ISBN10         string `json:"isbn_10"`
	ISBN13         string `json:"isbn_13"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Publisher implements port.EventPublisher by POSTing to the Gateway.
type Publisher struct {
	httpClient *http.Client
	gatewayURL string
}

// NewPublisher creates a new Gateway publisher.
func NewPublisher(httpClient *http.Client, gatewayURL string) *Publisher {
	return &Publisher{
		httpClient: httpClient,
		gatewayURL: strings.TrimRight(gatewayURL, "/"),
	}
}

// PublishBookAdded sends a book-added event to the Gateway via POST /v1/details.
func (p *Publisher) PublishBookAdded(ctx context.Context, book domain.Book) error {
	logger := logging.FromContext(ctx)

	isbn10, isbn13 := classifyISBN(book.ISBN)

	req := addDetailRequest{
		Title:          book.Title,
		Author:         strings.Join(book.Authors, ", "),
		Year:           book.PublishYear,
		Type:           "paperback",
		Pages:          book.Pages,
		Publisher:      book.Publisher,
		Language:       book.Language,
		ISBN10:         isbn10,
		ISBN13:         isbn13,
		IdempotencyKey: fmt.Sprintf("ingestion-isbn-%s", book.ISBN),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.gatewayURL+"/v1/details", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status from Gateway: %d", resp.StatusCode)
	}

	logger.Debug("published book-added event", "title", book.Title, "isbn", book.ISBN)
	return nil
}

// classifyISBN puts the ISBN into the correct field based on length.
func classifyISBN(isbn string) (isbn10, isbn13 string) {
	if len(isbn) == 13 {
		return "", isbn
	}
	return isbn, ""
}
```

- [ ] **Step 2: Verify publisher compiles**

Run: `go build ./services/ingestion/internal/adapter/outbound/gateway/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/adapter/outbound/gateway/publisher.go
git commit -m "feat(ingestion): implement Gateway event publisher adapter"
```

---

### Task 7: Gateway Publisher Tests

**Files:**
- Create: `services/ingestion/internal/adapter/outbound/gateway/publisher_test.go`

- [ ] **Step 1: Write publisher tests**

```go
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
```

- [ ] **Step 2: Run tests**

Run: `go test ./services/ingestion/internal/adapter/outbound/gateway/ -v`
Expected: all 4 tests PASS

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/adapter/outbound/gateway/publisher_test.go
git commit -m "test(ingestion): add Gateway publisher tests verifying DTO format and error handling"
```

---

### Task 8: Service Layer

**Files:**
- Create: `services/ingestion/internal/core/service/ingestion_service.go`

- [ ] **Step 1: Write the ingestion service**

```go
// Package service implements the business logic for the ingestion service.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/port"
)

// IngestionService implements the port.IngestionService interface.
type IngestionService struct {
	fetcher          port.BookFetcher
	publisher        port.EventPublisher
	defaultQueries   []string
	maxResultsPerQ   int

	mu     sync.RWMutex
	status domain.IngestionStatus
}

// NewIngestionService creates a new IngestionService.
func NewIngestionService(
	fetcher port.BookFetcher,
	publisher port.EventPublisher,
	defaultQueries []string,
	maxResultsPerQuery int,
) *IngestionService {
	return &IngestionService{
		fetcher:        fetcher,
		publisher:      publisher,
		defaultQueries: defaultQueries,
		maxResultsPerQ: maxResultsPerQuery,
		status:         domain.IngestionStatus{State: "idle"},
	}
}

// TriggerScrape runs an ingestion cycle using the given queries (or defaults if empty).
func (s *IngestionService) TriggerScrape(ctx context.Context, queries []string) (*domain.ScrapeResult, error) {
	logger := logging.FromContext(ctx)

	if len(queries) == 0 {
		queries = s.defaultQueries
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("no search queries configured")
	}

	s.mu.Lock()
	if s.status.State == "running" {
		s.mu.Unlock()
		return nil, fmt.Errorf("ingestion cycle already running")
	}
	s.status.State = "running"
	s.mu.Unlock()

	start := time.Now()
	result := &domain.ScrapeResult{}

	for _, query := range queries {
		books, err := s.fetcher.SearchBooks(ctx, query, s.maxResultsPerQ)
		if err != nil {
			logger.Error("failed to fetch books", "query", query, "error", err)
			result.Errors++
			continue
		}

		result.BooksFound += len(books)

		for _, book := range books {
			if err := s.publisher.PublishBookAdded(ctx, book); err != nil {
				logger.Warn("failed to publish book", "title", book.Title, "isbn", book.ISBN, "error", err)
				result.Errors++
				continue
			}
			result.EventsPublished++
			logger.Debug("published book", "title", book.Title, "isbn", book.ISBN)
		}
	}

	result.Duration = time.Since(start)
	now := time.Now()

	s.mu.Lock()
	s.status.State = "idle"
	s.status.LastRunAt = &now
	s.status.LastResult = result
	s.mu.Unlock()

	logger.Info("ingestion cycle complete",
		"books_found", result.BooksFound,
		"events_published", result.EventsPublished,
		"errors", result.Errors,
		"duration", result.Duration,
	)

	return result, nil
}

// GetStatus returns the current ingestion status.
func (s *IngestionService) GetStatus(_ context.Context) (*domain.IngestionStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := domain.IngestionStatus{
		State:      s.status.State,
		LastRunAt:  s.status.LastRunAt,
		LastResult: s.status.LastResult,
	}
	return &status, nil
}
```

- [ ] **Step 2: Verify service compiles**

Run: `go build ./services/ingestion/internal/core/service/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/core/service/ingestion_service.go
git commit -m "feat(ingestion): implement ingestion service with fetch-transform-publish orchestration"
```

---

### Task 9: Service Layer Tests

**Files:**
- Create: `services/ingestion/internal/core/service/ingestion_service_test.go`

- [ ] **Step 1: Write service tests with mock ports**

```go
package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/service"
)

// mockFetcher implements port.BookFetcher for testing.
type mockFetcher struct {
	books map[string][]domain.Book
	err   error
}

func (m *mockFetcher) SearchBooks(_ context.Context, query string, _ int) ([]domain.Book, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.books[query], nil
}

// mockPublisher implements port.EventPublisher for testing.
type mockPublisher struct {
	published []domain.Book
	failOn    map[string]bool
}

func (m *mockPublisher) PublishBookAdded(_ context.Context, book domain.Book) error {
	if m.failOn[book.ISBN] {
		return fmt.Errorf("publish failed for %s", book.ISBN)
	}
	m.published = append(m.published, book)
	return nil
}

func validBook(title, isbn string) domain.Book {
	return domain.Book{
		Title:       title,
		Authors:     []string{"Author"},
		ISBN:        isbn,
		PublishYear: 2020,
	}
}

func TestTriggerScrape_Success(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"golang": {
				validBook("Go Book 1", "1111111111111"),
				validBook("Go Book 2", "2222222222222"),
			},
		},
	}
	pub := &mockPublisher{}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang"}, 10)
	result, err := svc.TriggerScrape(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BooksFound != 2 {
		t.Errorf("BooksFound = %d, want 2", result.BooksFound)
	}
	if result.EventsPublished != 2 {
		t.Errorf("EventsPublished = %d, want 2", result.EventsPublished)
	}
	if result.Errors != 0 {
		t.Errorf("Errors = %d, want 0", result.Errors)
	}
	if len(pub.published) != 2 {
		t.Errorf("published count = %d, want 2", len(pub.published))
	}
}

func TestTriggerScrape_OverrideQueries(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"rust": {validBook("Rust Book", "3333333333333")},
		},
	}
	pub := &mockPublisher{}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang"}, 10)
	result, err := svc.TriggerScrape(context.Background(), []string{"rust"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BooksFound != 1 {
		t.Errorf("BooksFound = %d, want 1", result.BooksFound)
	}
	if pub.published[0].Title != "Rust Book" {
		t.Errorf("published wrong book: %s", pub.published[0].Title)
	}
}

func TestTriggerScrape_FetchError(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{err: fmt.Errorf("network error")}
	pub := &mockPublisher{}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang"}, 10)
	result, err := svc.TriggerScrape(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Errors)
	}
	if result.BooksFound != 0 {
		t.Errorf("BooksFound = %d, want 0", result.BooksFound)
	}
}

func TestTriggerScrape_PublishError(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"golang": {
				validBook("Good Book", "1111111111111"),
				validBook("Bad Book", "2222222222222"),
			},
		},
	}
	pub := &mockPublisher{failOn: map[string]bool{"2222222222222": true}}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang"}, 10)
	result, err := svc.TriggerScrape(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EventsPublished != 1 {
		t.Errorf("EventsPublished = %d, want 1", result.EventsPublished)
	}
	if result.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Errors)
	}
}

func TestTriggerScrape_NoQueries(t *testing.T) {
	t.Parallel()

	svc := service.NewIngestionService(&mockFetcher{}, &mockPublisher{}, nil, 10)
	_, err := svc.TriggerScrape(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when no queries configured")
	}
}

func TestGetStatus_Initial(t *testing.T) {
	t.Parallel()

	svc := service.NewIngestionService(&mockFetcher{}, &mockPublisher{}, []string{"golang"}, 10)
	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "idle" {
		t.Errorf("State = %q, want %q", status.State, "idle")
	}
	if status.LastRunAt != nil {
		t.Error("expected nil LastRunAt for initial status")
	}
}

func TestGetStatus_AfterScrape(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"golang": {validBook("Book", "1234567890123")},
		},
	}
	svc := service.NewIngestionService(fetcher, &mockPublisher{}, []string{"golang"}, 10)

	_, _ = svc.TriggerScrape(context.Background(), nil)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "idle" {
		t.Errorf("State = %q, want %q after scrape", status.State, "idle")
	}
	if status.LastRunAt == nil {
		t.Error("expected non-nil LastRunAt after scrape")
	}
	if status.LastResult == nil {
		t.Fatal("expected non-nil LastResult after scrape")
	}
	if status.LastResult.BooksFound != 1 {
		t.Errorf("LastResult.BooksFound = %d, want 1", status.LastResult.BooksFound)
	}
}

func TestTriggerScrape_MultipleQueries(t *testing.T) {
	t.Parallel()

	fetcher := &mockFetcher{
		books: map[string][]domain.Book{
			"golang":     {validBook("Go Book", "1111111111111")},
			"kubernetes": {validBook("K8s Book", "2222222222222")},
		},
	}
	pub := &mockPublisher{}

	svc := service.NewIngestionService(fetcher, pub, []string{"golang", "kubernetes"}, 10)
	result, err := svc.TriggerScrape(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BooksFound != 2 {
		t.Errorf("BooksFound = %d, want 2", result.BooksFound)
	}
	if result.EventsPublished != 2 {
		t.Errorf("EventsPublished = %d, want 2", result.EventsPublished)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./services/ingestion/internal/core/service/ -v`
Expected: all 8 tests PASS

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/core/service/ingestion_service_test.go
git commit -m "test(ingestion): add service layer tests with mock fetcher and publisher"
```

---

### Task 10: HTTP DTOs

**Files:**
- Create: `services/ingestion/internal/adapter/inbound/http/dto.go`

- [ ] **Step 1: Write the request/response DTOs**

```go
// Package http provides HTTP handlers and DTOs for the ingestion service.
package http //nolint:revive // package name matches directory convention

import "time"

// TriggerRequest is the optional JSON body for POST /v1/ingestion/trigger.
type TriggerRequest struct {
	Queries    []string `json:"queries,omitempty"`
	MaxResults int      `json:"max_results,omitempty"`
}

// ScrapeResultResponse is the JSON response for a completed scrape cycle.
type ScrapeResultResponse struct {
	BooksFound      int   `json:"books_found"`
	EventsPublished int   `json:"events_published"`
	Errors          int   `json:"errors"`
	DurationMs      int64 `json:"duration_ms"`
}

// StatusResponse is the JSON response for GET /v1/ingestion/status.
type StatusResponse struct {
	State      string                `json:"state"`
	LastRunAt  *time.Time            `json:"last_run_at,omitempty"`
	LastResult *ScrapeResultResponse `json:"last_result,omitempty"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
```

- [ ] **Step 2: Verify DTOs compile**

Run: `go build ./services/ingestion/internal/adapter/inbound/http/`
Expected: no output (success) — note: this will fail because handler.go doesn't exist yet; that's expected, proceed to next task.

Actually, since there's no handler yet, just verify the file is syntactically correct:

Run: `go vet ./services/ingestion/internal/adapter/inbound/http/`
Expected: may report unused types (OK at this stage)

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/adapter/inbound/http/dto.go
git commit -m "feat(ingestion): add HTTP request/response DTOs"
```

---

### Task 11: HTTP Handler

**Files:**
- Create: `services/ingestion/internal/adapter/inbound/http/handler.go`

- [ ] **Step 1: Write the HTTP handler**

```go
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/port"
)

// Handler holds the HTTP handlers for the ingestion service.
type Handler struct {
	svc port.IngestionService
}

// NewHandler creates a new HTTP handler with the given ingestion service.
func NewHandler(svc port.IngestionService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the ingestion routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/ingestion/trigger", h.triggerScrape)
	mux.HandleFunc("GET /v1/ingestion/status", h.getStatus)
}

func (h *Handler) triggerScrape(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req TriggerRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
			return
		}
	}

	result, err := h.svc.TriggerScrape(r.Context(), req.Queries)
	if err != nil {
		logger.Warn("trigger scrape failed", "error", err)
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, toScrapeResultResponse(result))
}

func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.GetStatus(r.Context())
	if err != nil {
		logger := logging.FromContext(r.Context())
		logger.Error("failed to get status", "error", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, toStatusResponse(status))
}

func toScrapeResultResponse(r *domain.ScrapeResult) ScrapeResultResponse {
	return ScrapeResultResponse{
		BooksFound:      r.BooksFound,
		EventsPublished: r.EventsPublished,
		Errors:          r.Errors,
		DurationMs:      r.Duration.Milliseconds(),
	}
}

func toStatusResponse(s *domain.IngestionStatus) StatusResponse {
	resp := StatusResponse{
		State:     s.State,
		LastRunAt: s.LastRunAt,
	}
	if s.LastResult != nil {
		r := toScrapeResultResponse(s.LastResult)
		resp.LastResult = &r
	}
	return resp
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 2: Verify handler compiles**

Run: `go build ./services/ingestion/internal/adapter/inbound/http/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/adapter/inbound/http/handler.go
git commit -m "feat(ingestion): implement HTTP handler with trigger and status endpoints"
```

---

### Task 12: HTTP Handler Tests

**Files:**
- Create: `services/ingestion/internal/adapter/inbound/http/handler_test.go`

- [ ] **Step 1: Write handler tests**

```go
// file: services/ingestion/internal/adapter/inbound/http/handler_test.go
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
```

- [ ] **Step 2: Run tests**

Run: `go test ./services/ingestion/internal/adapter/inbound/http/ -v`
Expected: all 6 tests PASS

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/adapter/inbound/http/handler_test.go
git commit -m "test(ingestion): add HTTP handler tests for trigger and status endpoints"
```

---

### Task 13: Ingestion Config Extension

**Files:**
- Modify: `pkg/config/config.go`

The ingestion service needs 4 additional config fields. Add them to the shared `Config` struct so they follow the same env-based pattern as all other services.

- [ ] **Step 1: Add ingestion-specific fields to Config**

Add these fields to the `Config` struct in `pkg/config/config.go`:

```go
// Ingestion service configuration
GatewayURL         string
PollInterval        time.Duration
SearchQueries       []string
MaxResultsPerQuery  int
```

Add these lines to the `Load()` function, after the existing fields:

```go
GatewayURL:         envOrDefault("GATEWAY_URL", "http://localhost:8080"),
PollInterval:       parseDuration(envOrDefault("POLL_INTERVAL", "5m")),
SearchQueries:      parseCSV(envOrDefault("SEARCH_QUERIES", "programming,golang")),
MaxResultsPerQuery: parseInt(envOrDefault("MAX_RESULTS_PER_QUERY", "10")),
```

Add these helper functions:

```go
func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 10
	}
	return n
}
```

Add `"strconv"`, `"strings"`, and `"time"` to the import block.

- [ ] **Step 2: Verify config compiles and existing tests pass**

Run: `go build ./pkg/config/ && go test ./...`
Expected: all tests pass, no compilation errors

- [ ] **Step 3: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat(pkg/config): add ingestion service config fields (GatewayURL, PollInterval, SearchQueries, MaxResultsPerQuery)"
```

---

### Task 14: Composition Root (cmd/main.go)

**Files:**
- Create: `services/ingestion/cmd/main.go`

- [ ] **Step 1: Write the composition root**

```go
// Package main is the entry point for the ingestion service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/gateway"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/openlibrary"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.PyroscopeServerAddress != "")
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(ctx) }()

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metrics
	meter := otel.Meter(cfg.ServiceName)
	scrapesTotal, _ := meter.Int64Counter(
		"ingestion_scrapes_total",
		metric.WithDescription("Total number of completed scrape cycles"),
	)
	booksPublished, _ := meter.Int64Counter(
		"ingestion_books_published_total",
		metric.WithDescription("Total number of events accepted by EventSource webhook"),
	)
	errorsTotal, _ := meter.Int64Counter(
		"ingestion_errors_total",
		metric.WithDescription("Total number of publish failures"),
	)
	_, _ = scrapesTotal, booksPublished // Will be used in future metric decorator
	_ = errorsTotal

	// Outbound HTTP client with OTel transport for tracing
	outboundTransport := otelhttp.NewTransport(http.DefaultTransport)
	outboundClient := &http.Client{
		Transport: outboundTransport,
		Timeout:   30 * time.Second,
	}

	// Wire hex arch
	fetcher := openlibrary.NewClient(outboundClient)
	publisher := gateway.NewPublisher(outboundClient, cfg.GatewayURL)
	svc := service.NewIngestionService(fetcher, publisher, cfg.SearchQueries, cfg.MaxResultsPerQuery)
	h := handler.NewHandler(svc)

	// Start background poll loop
	go pollLoop(ctx, logger, svc, cfg.PollInterval)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func pollLoop(ctx context.Context, logger *slog.Logger, svc *service.IngestionService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("ingestion poll loop started", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			logger.Info("ingestion poll loop stopped")
			return
		case <-ticker.C:
			logger.Info("poll loop: starting ingestion cycle")
			if _, err := svc.TriggerScrape(ctx, nil); err != nil {
				logger.Error("poll loop: ingestion cycle failed", "error", err)
			}
		}
	}
}
```

- [ ] **Step 2: Verify the service builds**

Run: `go build ./services/ingestion/cmd/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/cmd/main.go
git commit -m "feat(ingestion): add composition root with poll loop and full observability wiring"
```

---

### Task 15: Run All Ingestion Tests

- [ ] **Step 1: Run the full test suite for the ingestion service**

Run: `go test -race -count=1 ./services/ingestion/...`
Expected: all tests PASS with race detector enabled

- [ ] **Step 2: Run the full repo test suite to ensure no regressions**

Run: `go test ./...`
Expected: all tests PASS (existing + new ingestion tests)

- [ ] **Step 3: Run vet and build**

Run: `go vet ./... && go build ./...`
Expected: no errors

---

### Task 16: Dockerfile

**Files:**
- Create: `build/Dockerfile.ingestion`
- Create: `build/Dockerfile.goreleaser.ingestion`

- [ ] **Step 1: Write the local development Dockerfile**

```dockerfile
FROM golang:1.26.2-alpine@sha256:c2a1f7b2095d046ae14b286b18413a05bb82c9bca9b25fe7ff5efef0f0826166 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY pkg/ ./pkg/
COPY services/ingestion/ ./services/ingestion/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/ingestion ./services/ingestion/cmd/

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/ingestion /bin/ingestion
USER 65534:65534
ENTRYPOINT ["/bin/ingestion"]
```

- [ ] **Step 2: Write the GoReleaser Dockerfile**

```dockerfile
FROM scratch
COPY ingestion /bin/ingestion
USER 65534:65534
ENTRYPOINT ["/bin/ingestion"]
```

- [ ] **Step 3: Build the Docker image to verify**

Run: `docker build -f build/Dockerfile.ingestion -t event-driven-bookinfo/ingestion:latest .`
Expected: image builds successfully

- [ ] **Step 4: Commit**

```bash
git add build/Dockerfile.ingestion build/Dockerfile.goreleaser.ingestion
git commit -m "feat(ingestion): add Dockerfiles for local dev and GoReleaser builds"
```

---

### Task 17: GoReleaser Config

**Files:**
- Create: `services/ingestion/.goreleaser.yaml`

- [ ] **Step 1: Write the GoReleaser config**

```yaml
version: 2
project_name: ingestion

builds:
  - id: ingestion
    binary: ingestion
    main: ./services/ingestion/cmd/
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w

archives:
  - id: ingestion
    builds:
      - ingestion
    name_template: "ingestion_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format: tar.gz

dockers:
  - id: ingestion-amd64
    ids:
      - ingestion
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/ingestion:{{ .Tag }}-amd64"
    dockerfile: build/Dockerfile.goreleaser.ingestion
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=ingestion"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

  - id: ingestion-arm64
    ids:
      - ingestion
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/ingestion:{{ .Tag }}-arm64"
    dockerfile: build/Dockerfile.goreleaser.ingestion
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=ingestion"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

docker_manifests:
  - name_template: "ghcr.io/kaio6fellipe/event-driven-bookinfo/ingestion:{{ .Tag }}"
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/ingestion:{{ .Tag }}-amd64"
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/ingestion:{{ .Tag }}-arm64"

release:
  disable: true

changelog:
  disable: true
```

- [ ] **Step 2: Commit**

```bash
git add services/ingestion/.goreleaser.yaml
git commit -m "feat(ingestion): add GoReleaser config for multi-arch binary and Docker builds"
```

---

### Task 18: Helm Values

**Files:**
- Create: `deploy/ingestion/values-local.yaml`

- [ ] **Step 1: Write the Helm values for local k8s deployment**

```yaml
# deploy/ingestion/values-local.yaml
serviceName: ingestion
fullnameOverride: ingestion
image:
  repository: event-driven-bookinfo/ingestion
  tag: local

config:
  LOG_LEVEL: "debug"
  GATEWAY_URL: "http://default-gw-envoy.envoy-gateway-system.svc.cluster.local"
  POLL_INTERVAL: "5m"
  SEARCH_QUERIES: "programming,golang,kubernetes,python,devops"
  MAX_RESULTS_PER_QUERY: "10"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"
```

- [ ] **Step 2: Verify the chart renders with these values**

Run: `helm template ingestion charts/bookinfo-service -f deploy/ingestion/values-local.yaml --namespace bookinfo`
Expected: renders Deployment, Service, ServiceAccount, ConfigMap. No EventSource, Sensor, or write Deployment (cqrs.enabled defaults to false).

- [ ] **Step 3: Commit**

```bash
git add deploy/ingestion/values-local.yaml
git commit -m "feat(ingestion): add Helm values for local Kubernetes deployment"
```

---

### Task 19: Makefile and CI Updates

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/auto-tag.yml`

- [ ] **Step 1: Add `ingestion` to the SERVICES list in Makefile**

Change line 4 of `Makefile`:

```
SERVICES := productpage details reviews ratings notification dlqueue
```

to:

```
SERVICES := productpage details reviews ratings notification dlqueue ingestion
```

- [ ] **Step 2: Add `ingestion` to the ALL_SERVICES array in auto-tag.yml**

In `.github/workflows/auto-tag.yml`, update the `ALL_SERVICES` array (line 33):

```
ALL_SERVICES=(productpage details reviews ratings notification dlqueue)
```

to:

```
ALL_SERVICES=(productpage details reviews ratings notification dlqueue ingestion)
```

- [ ] **Step 3: Add `ingestion` to the k8s-deploy wait list in Makefile**

In the `k8s-deploy` target (line 389), update the deployment wait loop:

```
@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification dlqueue dlqueue-write; do \
```

to:

```
@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification dlqueue dlqueue-write ingestion; do \
```

Also update the same list in `k8s-rebuild` target (lines 437 and 441) — both the `rollout restart` and `rollout status` loops:

```
@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification dlqueue dlqueue-write ingestion; do \
```

- [ ] **Step 4: Verify Makefile works**

Run: `make build SERVICE=ingestion`
Expected: builds `bin/ingestion` successfully

Run: `make helm-lint`
Expected: lints pass for all services including ingestion

- [ ] **Step 5: Commit**

```bash
git add Makefile .github/workflows/auto-tag.yml
git commit -m "chore: add ingestion service to Makefile SERVICES list and CI auto-tag workflow"
```

---

### Task 20: Verification and Final Build

- [ ] **Step 1: Run the complete test suite with race detection**

Run: `go test -race -count=1 ./...`
Expected: all tests pass

- [ ] **Step 2: Build all services**

Run: `make build-all`
Expected: all 7 services build successfully (including ingestion)

- [ ] **Step 3: Build Docker image**

Run: `make docker-build SERVICE=ingestion`
Expected: Docker image builds successfully

- [ ] **Step 4: Run lint**

Run: `make lint`
Expected: no lint errors (or only pre-existing ones)

- [ ] **Step 5: Verify the service starts locally**

Run: `SERVICE_NAME=ingestion HTTP_PORT=8086 ADMIN_PORT=9096 POLL_INTERVAL=24h go run ./services/ingestion/cmd/ &`
(Poll interval set to 24h to avoid immediate scrape during verification)

Then verify endpoints:
Run: `curl -sf http://localhost:9096/healthz && echo "healthz OK"`
Run: `curl -sf http://localhost:8086/v1/ingestion/status | python3 -m json.tool`

Expected: healthz returns 200, status returns `{"state":"idle","last_run_at":null,"last_result":null}`

Kill the background process after verification.

- [ ] **Step 6: Commit any fixes if needed, then create final commit**

If all checks pass with no changes needed, this step is complete.
