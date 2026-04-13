// Package http provides HTTP handlers and DTOs for the ratings service.
package http //nolint:revive // package name matches directory convention

// SubmitRatingRequest is the JSON body for POST /v1/ratings.
type SubmitRatingRequest struct {
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Stars          int    `json:"stars"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// RatingResponse represents a single rating in API responses.
type RatingResponse struct {
	ID        string `json:"id"`
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Stars     int    `json:"stars"`
}

// ProductRatingsResponse represents the aggregated ratings for a product.
type ProductRatingsResponse struct {
	ProductID string           `json:"product_id"`
	Average   float64          `json:"average"`
	Count     int              `json:"count"`
	Ratings   []RatingResponse `json:"ratings"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
