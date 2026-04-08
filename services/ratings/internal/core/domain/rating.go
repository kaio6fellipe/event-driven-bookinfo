// file: services/ratings/internal/core/domain/rating.go
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// Rating represents a single star rating given by a reviewer for a product.
type Rating struct {
	ID        string
	ProductID string
	Reviewer  string
	Stars     int
}

// ProductRatings aggregates all ratings for a single product.
type ProductRatings struct {
	ProductID string
	Ratings   []Rating
}

// NewRating creates a new Rating with validation.
// Stars must be between 1 and 5 inclusive.
// ProductID and Reviewer must be non-empty.
func NewRating(productID, reviewer string, stars int) (*Rating, error) {
	if productID == "" {
		return nil, fmt.Errorf("product ID is required")
	}
	if reviewer == "" {
		return nil, fmt.Errorf("reviewer is required")
	}
	if stars < 1 || stars > 5 {
		return nil, fmt.Errorf("stars must be between 1 and 5, got %d", stars)
	}

	return &Rating{
		ID:        uuid.New().String(),
		ProductID: productID,
		Reviewer:  reviewer,
		Stars:     stars,
	}, nil
}

// Average returns the average star rating. Returns 0 if there are no ratings.
func (pr *ProductRatings) Average() float64 {
	if len(pr.Ratings) == 0 {
		return 0
	}

	total := 0
	for _, r := range pr.Ratings {
		total += r.Stars
	}

	return float64(total) / float64(len(pr.Ratings))
}
