// Package domain contains the core domain model for the reviews service.
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// ReviewRating holds the rating data attached to a review.
type ReviewRating struct {
	Stars   int
	Average float64
	Count   int
}

// RatingData holds both product-level and per-reviewer rating data.
type RatingData struct {
	Average           float64
	Count             int
	IndividualRatings map[string]int // reviewer -> stars
}

// Review represents a user review for a product.
type Review struct {
	ID        string
	ProductID string
	Reviewer  string
	Text      string
	Rating    *ReviewRating
}

// NewReview creates a new Review with validation.
func NewReview(productID, reviewer, text string) (*Review, error) {
	if productID == "" {
		return nil, fmt.Errorf("product ID is required")
	}
	if reviewer == "" {
		return nil, fmt.Errorf("reviewer is required")
	}
	if text == "" {
		return nil, fmt.Errorf("review text is required")
	}

	return &Review{
		ID:        uuid.New().String(),
		ProductID: productID,
		Reviewer:  reviewer,
		Text:      text,
	}, nil
}
