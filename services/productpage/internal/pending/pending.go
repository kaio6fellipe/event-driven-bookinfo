// Package pending provides a pending review store for the productpage BFF.
// Pending reviews are stored in Redis and merged into read responses
// until the async CQRS write pipeline confirms them.
package pending

import (
	"context"
	"time"
)

// Review represents a review that has been submitted but not yet confirmed
// by the read path.
type Review struct {
	Reviewer  string `json:"reviewer"`
	Text      string `json:"text"`
	Stars     int    `json:"stars"`
	Timestamp int64  `json:"timestamp"`
}

// ConfirmedReview contains the fields used to match a pending review
// against a confirmed review from the read service.
type ConfirmedReview struct {
	Reviewer string
	Text     string
}

// Store defines operations for managing pending reviews.
type Store interface {
	// StorePending appends a pending review for the given product.
	StorePending(ctx context.Context, productID string, review Review) error

	// GetAndReconcile returns pending reviews for a product after removing
	// any that match the confirmed reviews. A pending review matches a
	// confirmed review when both reviewer and text are equal.
	GetAndReconcile(ctx context.Context, productID string, confirmed []ConfirmedReview) ([]Review, error)
}

// NewReview creates a Review with the current timestamp.
func NewReview(reviewer, text string, stars int) Review {
	return Review{
		Reviewer:  reviewer,
		Text:      text,
		Stars:     stars,
		Timestamp: time.Now().Unix(),
	}
}
