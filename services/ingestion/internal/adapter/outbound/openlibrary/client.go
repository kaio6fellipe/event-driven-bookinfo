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

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL is built from trusted baseURL constant, not user input
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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
