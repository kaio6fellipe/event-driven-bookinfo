// file: services/ratings/internal/core/domain/rating_test.go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

func TestNewRating_Valid(t *testing.T) {
	tests := []struct {
		name      string
		productID string
		reviewer  string
		stars     int
	}{
		{name: "min stars", productID: "product-1", reviewer: "reviewer-1", stars: 1},
		{name: "max stars", productID: "product-2", reviewer: "reviewer-2", stars: 5},
		{name: "mid stars", productID: "product-3", reviewer: "reviewer-3", stars: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := domain.NewRating(tt.productID, tt.reviewer, tt.stars)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.ID == "" {
				t.Error("expected non-empty ID")
			}
			if r.ProductID != tt.productID {
				t.Errorf("ProductID = %q, want %q", r.ProductID, tt.productID)
			}
			if r.Reviewer != tt.reviewer {
				t.Errorf("Reviewer = %q, want %q", r.Reviewer, tt.reviewer)
			}
			if r.Stars != tt.stars {
				t.Errorf("Stars = %d, want %d", r.Stars, tt.stars)
			}
		})
	}
}

func TestNewRating_InvalidStars(t *testing.T) {
	tests := []struct {
		name  string
		stars int
	}{
		{name: "zero stars", stars: 0},
		{name: "negative stars", stars: -1},
		{name: "six stars", stars: 6},
		{name: "hundred stars", stars: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewRating("product-1", "reviewer-1", tt.stars)
			if err == nil {
				t.Fatal("expected error for invalid stars")
			}
		})
	}
}

func TestNewRating_EmptyProductID(t *testing.T) {
	_, err := domain.NewRating("", "reviewer-1", 5)
	if err == nil {
		t.Fatal("expected error for empty product ID")
	}
}

func TestNewRating_EmptyReviewer(t *testing.T) {
	_, err := domain.NewRating("product-1", "", 5)
	if err == nil {
		t.Fatal("expected error for empty reviewer")
	}
}

func TestProductRatings_Average(t *testing.T) {
	pr := &domain.ProductRatings{
		ProductID: "product-1",
		Ratings: []domain.Rating{
			{ID: "1", ProductID: "product-1", Reviewer: "a", Stars: 4},
			{ID: "2", ProductID: "product-1", Reviewer: "b", Stars: 2},
			{ID: "3", ProductID: "product-1", Reviewer: "c", Stars: 3},
		},
	}

	avg := pr.Average()
	if avg != 3.0 {
		t.Errorf("Average() = %f, want 3.0", avg)
	}
}

func TestProductRatings_Average_Empty(t *testing.T) {
	pr := &domain.ProductRatings{
		ProductID: "product-1",
		Ratings:   []domain.Rating{},
	}

	avg := pr.Average()
	if avg != 0.0 {
		t.Errorf("Average() = %f, want 0.0", avg)
	}
}
