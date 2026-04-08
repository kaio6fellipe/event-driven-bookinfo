// file: services/productpage/internal/client/details.go
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DetailResponse represents the details service API response.
type DetailResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	Pages     int    `json:"pages"`
	Publisher string `json:"publisher"`
	Language  string `json:"language"`
	ISBN10    string `json:"isbn_10"`
	ISBN13    string `json:"isbn_13"`
}

// DetailsClient fetches book details from the details service.
type DetailsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewDetailsClient creates a new DetailsClient pointing to the given base URL.
func NewDetailsClient(baseURL string) *DetailsClient {
	return &DetailsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetDetail fetches a book detail by ID.
func (c *DetailsClient) GetDetail(ctx context.Context, id string) (*DetailResponse, error) {
	url := fmt.Sprintf("%s/v1/details/%s", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching detail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("detail not found: %s", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("details service returned status %d", resp.StatusCode)
	}

	var body DetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding detail response: %w", err)
	}

	return &body, nil
}
