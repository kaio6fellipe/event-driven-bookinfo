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

	// StoreDeleting marks a review as being deleted for the given product.
	StoreDeleting(ctx context.Context, productID string, reviewID string) error

	// GetAndReconcile returns pending reviews and deleting review IDs for a product
	// after reconciling against the confirmed reviews.
	// Pending reviews that match confirmed reviews are removed.
	// Deleting IDs that no longer appear in confirmed reviews are removed.
	GetAndReconcile(ctx context.Context, productID string, confirmed []ConfirmedReview, confirmedIDs []string) ([]Review, []string, error)
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
