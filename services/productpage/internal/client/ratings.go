// Package client provides HTTP clients for communicating with backend services.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// SubmitRatingRequest represents the request body for submitting a rating.
type SubmitRatingRequest struct {
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Stars     int    `json:"stars"`
}

// RatingResponse represents a single rating from the ratings service.
type RatingResponse struct {
	ID        string `json:"id"`
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Stars     int    `json:"stars"`
}

// RatingsClient submits ratings to the ratings service.
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

// SubmitRating submits a new rating to the ratings service.
func (c *RatingsClient) SubmitRating(ctx context.Context, productID, reviewer string, stars int) (*RatingResponse, error) {
	reqBody := SubmitRatingRequest{
		ProductID: productID,
		Reviewer:  reviewer,
		Stars:     stars,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/ratings", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL comes from operator-controlled config, not user input
	if err != nil {
		return nil, fmt.Errorf("submitting rating: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("ratings service returned status %d", resp.StatusCode)
	}

	var body RatingResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding rating response: %w", err)
	}

	return &body, nil
}
