// file: services/productpage/internal/client/reviews.go
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ReviewRatingResponse represents rating data from the reviews service.
type ReviewRatingResponse struct {
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

// ProductReviewsResponse represents the reviews service aggregated response.
type ProductReviewsResponse struct {
	ProductID string           `json:"product_id"`
	Reviews   []ReviewResponse `json:"reviews"`
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
			Timeout: 5 * time.Second,
		},
	}
}

// GetProductReviews fetches all reviews for a product.
func (c *ReviewsClient) GetProductReviews(ctx context.Context, productID string) (*ProductReviewsResponse, error) {
	url := fmt.Sprintf("%s/v1/reviews/%s", c.baseURL, productID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching reviews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reviews service returned status %d", resp.StatusCode)
	}

	var body ProductReviewsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding reviews response: %w", err)
	}

	return &body, nil
}
