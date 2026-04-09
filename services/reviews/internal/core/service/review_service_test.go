// file: services/reviews/internal/core/service/review_service_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

// stubRatingsClient returns fixed rating data for testing.
type stubRatingsClient struct {
	data *domain.RatingData
	err  error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.RatingData, error) {
	return s.data, s.err
}

func TestSubmitReview_Success(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	review, err := svc.SubmitReview(context.Background(), "product-1", "alice", "Great book!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if review.ID == "" {
		t.Error("expected non-empty ID")
	}
	if review.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", review.ProductID, "product-1")
	}
	if review.Reviewer != "alice" {
		t.Errorf("Reviewer = %q, want %q", review.Reviewer, "alice")
	}
}

func TestSubmitReview_ValidationError(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	tests := []struct {
		name      string
		productID string
		reviewer  string
		text      string
	}{
		{name: "empty product ID", productID: "", reviewer: "alice", text: "Great!"},
		{name: "empty reviewer", productID: "product-1", reviewer: "", text: "Great!"},
		{name: "empty text", productID: "product-1", reviewer: "alice", text: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SubmitReview(context.Background(), tt.productID, tt.reviewer, tt.text)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestGetProductReviews_Empty(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{Average: 0, Count: 0, IndividualRatings: map[string]int{}},
	}
	svc := service.NewReviewService(repo, client)

	reviews, err := svc.GetProductReviews(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}
}

func TestGetProductReviews_WithRatings(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{
			Average: 3.5,
			Count:   2,
			IndividualRatings: map[string]int{
				"alice": 5,
				"bob":   2,
			},
		},
	}
	svc := service.NewReviewService(repo, client)

	_, err := svc.SubmitReview(context.Background(), "product-1", "alice", "Excellent!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.SubmitReview(context.Background(), "product-1", "bob", "Good read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviews, err := svc.GetProductReviews(context.Background(), "product-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviews) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(reviews))
	}

	for _, review := range reviews {
		if review.Rating == nil {
			t.Errorf("expected non-nil Rating on review by %s", review.Reviewer)
			continue
		}
		if review.Rating.Average != 3.5 {
			t.Errorf("Rating.Average = %f, want 3.5", review.Rating.Average)
		}
		if review.Rating.Count != 2 {
			t.Errorf("Rating.Count = %d, want 2", review.Rating.Count)
		}
		switch review.Reviewer {
		case "alice":
			if review.Rating.Stars != 5 {
				t.Errorf("alice Rating.Stars = %d, want 5", review.Rating.Stars)
			}
		case "bob":
			if review.Rating.Stars != 2 {
				t.Errorf("bob Rating.Stars = %d, want 2", review.Rating.Stars)
			}
		default:
			t.Errorf("unexpected reviewer: %s", review.Reviewer)
		}
	}
}
