// Package http provides HTTP handlers and DTOs for the reviews service.
package http //nolint:revive // package name matches directory convention

// SubmitReviewRequest is the JSON body for POST /v1/reviews.
type SubmitReviewRequest struct {
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Text      string `json:"text"`
}

// ReviewRatingResponse represents rating data embedded in a review response.
type ReviewRatingResponse struct {
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// ReviewResponse represents a single review in API responses.
type ReviewResponse struct {
	ID        string                `json:"id"`
	ProductID string                `json:"product_id"`
	Reviewer  string                `json:"reviewer"`
	Text      string                `json:"text"`
	Rating    *ReviewRatingResponse `json:"rating,omitempty"`
}

// ProductReviewsResponse wraps multiple reviews for a product.
type ProductReviewsResponse struct {
	ProductID string           `json:"product_id"`
	Reviews   []ReviewResponse `json:"reviews"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
