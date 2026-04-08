// file: services/ratings/internal/core/service/rating_service_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
)

func TestSubmitRating_Success(t *testing.T) {
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)

	rating, err := svc.SubmitRating(context.Background(), "product-1", "alice", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rating.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rating.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", rating.ProductID, "product-1")
	}
	if rating.Reviewer != "alice" {
		t.Errorf("Reviewer = %q, want %q", rating.Reviewer, "alice")
	}
	if rating.Stars != 5 {
		t.Errorf("Stars = %d, want %d", rating.Stars, 5)
	}
}

func TestSubmitRating_ValidationError(t *testing.T) {
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)

	tests := []struct {
		name      string
		productID string
		reviewer  string
		stars     int
	}{
		{name: "empty product ID", productID: "", reviewer: "alice", stars: 5},
		{name: "empty reviewer", productID: "product-1", reviewer: "", stars: 5},
		{name: "stars too low", productID: "product-1", reviewer: "alice", stars: 0},
		{name: "stars too high", productID: "product-1", reviewer: "alice", stars: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SubmitRating(context.Background(), tt.productID, tt.reviewer, tt.stars)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestGetProductRatings_Empty(t *testing.T) {
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)

	pr, err := svc.GetProductRatings(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pr.ProductID != "nonexistent" {
		t.Errorf("ProductID = %q, want %q", pr.ProductID, "nonexistent")
	}
	if len(pr.Ratings) != 0 {
		t.Errorf("expected 0 ratings, got %d", len(pr.Ratings))
	}
	if pr.Average() != 0.0 {
		t.Errorf("Average() = %f, want 0.0", pr.Average())
	}
}

func TestGetProductRatings_WithRatings(t *testing.T) {
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)

	_, err := svc.SubmitRating(context.Background(), "product-1", "alice", 4)
	if err != nil {
		t.Fatalf("unexpected error submitting rating 1: %v", err)
	}

	_, err = svc.SubmitRating(context.Background(), "product-1", "bob", 2)
	if err != nil {
		t.Fatalf("unexpected error submitting rating 2: %v", err)
	}

	_, err = svc.SubmitRating(context.Background(), "product-2", "charlie", 5)
	if err != nil {
		t.Fatalf("unexpected error submitting rating 3: %v", err)
	}

	pr, err := svc.GetProductRatings(context.Background(), "product-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pr.Ratings) != 2 {
		t.Fatalf("expected 2 ratings, got %d", len(pr.Ratings))
	}

	avg := pr.Average()
	if avg != 3.0 {
		t.Errorf("Average() = %f, want 3.0", avg)
	}
}
