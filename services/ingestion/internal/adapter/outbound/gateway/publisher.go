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
	defer func() { _ = resp.Body.Close() }()

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
