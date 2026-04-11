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

// ReviewRatingResponse represents rating data from the reviews service.
type ReviewRatingResponse struct {
	Stars   int     `json:"stars"`
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// ReviewResponse represents a single review from the reviews service.
type ReviewResponse struct {
	ID        string                `json:"id"`
	ProductID string                `json:"product_id"`
	Reviewer  string                `json:"reviewer"`
	Text      string                `json:"text"`
	Rating    *ReviewRatingResponse `json:"rating,omitempty"`
}

// PaginationResponse contains pagination metadata from the reviews service.
type PaginationResponse struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// ProductReviewsResponse represents the reviews service aggregated response.
type ProductReviewsResponse struct {
	ProductID  string             `json:"product_id"`
	Reviews    []ReviewResponse   `json:"reviews"`
	Pagination PaginationResponse `json:"pagination"`
}

// ReviewsClient fetches reviews from the reviews service.
type ReviewsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewReviewsClient creates a new ReviewsClient pointing to the given base URL.
func NewReviewsClient(baseURL string) *ReviewsClient {
	return &ReviewsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// GetProductReviews fetches paginated reviews for a product.
func (c *ReviewsClient) GetProductReviews(ctx context.Context, productID string, page, pageSize int) (*ProductReviewsResponse, error) {
	url := fmt.Sprintf("%s/v1/reviews/%s?page=%d&page_size=%d", c.baseURL, productID, page, pageSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL comes from operator-controlled config, not user input
	if err != nil {
		return nil, fmt.Errorf("fetching reviews: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reviews service returned status %d", resp.StatusCode)
	}

	var body ProductReviewsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding reviews response: %w", err)
	}

	return &body, nil
}

// DeleteReview sends a delete request for a review.
func (c *ReviewsClient) DeleteReview(ctx context.Context, reviewID string) error {
	body := map[string]string{
		"review_id": reviewID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling delete request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/reviews/delete", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL comes from operator-controlled config, not user input
	if err != nil {
		return fmt.Errorf("deleting review: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reviews service returned status %d", resp.StatusCode)
	}

	return nil
}

// SubmitReview creates a new review for a product.
func (c *ReviewsClient) SubmitReview(ctx context.Context, productID, reviewer, text string) error {
	body := map[string]string{
		"product_id": productID,
		"reviewer":   reviewer,
		"text":       text,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling review: %w", err)
	}

	url := fmt.Sprintf("%s/v1/reviews", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // URL comes from operator-controlled config, not user input
	if err != nil {
		return fmt.Errorf("submitting review: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reviews service returned status %d", resp.StatusCode)
	}

	return nil
}
