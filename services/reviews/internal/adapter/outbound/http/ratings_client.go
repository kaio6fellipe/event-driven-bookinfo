// Package http provides an HTTP client for the ratings service used by reviews.
package http //nolint:revive // package name matches directory convention

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ratingsResponse mirrors the ratings service ProductRatingsResponse.
type ratingsResponse struct {
	ProductID string  `json:"product_id"`
	Average   float64 `json:"average"`
	Count     int     `json:"count"`
}

// RatingsClient is an HTTP client that fetches ratings from the ratings service.
type RatingsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRatingsClient creates a new RatingsClient pointing to the given base URL.
func NewRatingsClient(baseURL string) *RatingsClient {
	return &RatingsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// GetProductRatings fetches the aggregated ratings for a product from the ratings service.
func (c *RatingsClient) GetProductRatings(ctx context.Context, productID string) (*domain.ReviewRating, error) {
	url := fmt.Sprintf("%s/v1/ratings/%s", c.baseURL, productID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL comes from operator-controlled config, not user input
	if err != nil {
		return nil, fmt.Errorf("fetching ratings: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ratings service returned status %d", resp.StatusCode)
	}

	var body ratingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding ratings response: %w", err)
	}

	return &domain.ReviewRating{
		Average: body.Average,
		Count:   body.Count,
	}, nil
}
