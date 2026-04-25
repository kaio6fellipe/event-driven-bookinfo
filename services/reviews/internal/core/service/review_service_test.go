// file: services/reviews/internal/core/service/review_service_test.go
package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

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
	svc := service.NewReviewService(repo, client, idempotency.NewMemoryStore(), &fakeReviewPublisher{})

	review, err := svc.SubmitReview(context.Background(), "product-1", "alice", "Great book!", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if review.ID == "" {
		t.Error("expected non-empty ID")
	}
	if review.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", review.ProductID, "product-1")
	}
}

func TestSubmitReview_ValidationError(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client, idempotency.NewMemoryStore(), &fakeReviewPublisher{})

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
			_, err := svc.SubmitReview(context.Background(), tt.productID, tt.reviewer, tt.text, "")
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
	svc := service.NewReviewService(repo, client, idempotency.NewMemoryStore(), &fakeReviewPublisher{})

	reviews, total, err := svc.GetProductReviews(context.Background(), "nonexistent", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}

func TestGetProductReviews_WithRatings(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{
			Average: 3.5, Count: 2,
			IndividualRatings: map[string]int{"alice": 5, "bob": 2},
		},
	}
	svc := service.NewReviewService(repo, client, idempotency.NewMemoryStore(), &fakeReviewPublisher{})

	_, _ = svc.SubmitReview(context.Background(), "product-1", "alice", "Excellent!", "")
	_, _ = svc.SubmitReview(context.Background(), "product-1", "bob", "Good read", "")

	reviews, total, err := svc.GetProductReviews(context.Background(), "product-1", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(reviews))
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}

	for _, review := range reviews {
		if review.Rating == nil {
			t.Errorf("expected non-nil Rating on review by %s", review.Reviewer)
			continue
		}
		if review.Rating.Average != 3.5 {
			t.Errorf("Rating.Average = %f, want 3.5", review.Rating.Average)
		}
	}
}

func TestGetProductReviews_Pagination(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		data: &domain.RatingData{Average: 4.0, Count: 15, IndividualRatings: map[string]int{}},
	}
	svc := service.NewReviewService(repo, client, idempotency.NewMemoryStore(), &fakeReviewPublisher{})

	for i := 0; i < 15; i++ {
		_, _ = svc.SubmitReview(context.Background(), "product-1", "reviewer", fmt.Sprintf("review text %d", i), "")
	}

	tests := []struct {
		name      string
		page      int
		pageSize  int
		wantCount int
		wantTotal int
	}{
		{name: "first page", page: 1, pageSize: 10, wantCount: 10, wantTotal: 15},
		{name: "second page", page: 2, pageSize: 10, wantCount: 5, wantTotal: 15},
		{name: "page beyond total", page: 3, pageSize: 10, wantCount: 0, wantTotal: 15},
		{name: "custom page size", page: 1, pageSize: 5, wantCount: 5, wantTotal: 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviews, total, err := svc.GetProductReviews(context.Background(), "product-1", tt.page, tt.pageSize)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(reviews) != tt.wantCount {
				t.Errorf("got %d reviews, want %d", len(reviews), tt.wantCount)
			}
			if total != tt.wantTotal {
				t.Errorf("got total %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestDeleteReview_Success(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	pub := &fakeReviewPublisher{}
	svc := service.NewReviewService(repo, client, idempotency.NewMemoryStore(), pub)

	review, _ := svc.SubmitReview(context.Background(), "product-1", "alice", "Great book!", "")
	err := svc.DeleteReview(context.Background(), review.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviews, total, _ := svc.GetProductReviews(context.Background(), "product-1", 1, 10)
	if total != 0 {
		t.Errorf("expected 0 reviews after delete, got %d", total)
	}
	if len(reviews) != 0 {
		t.Errorf("expected empty reviews after delete, got %d", len(reviews))
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.deleted) != 1 {
		t.Fatalf("expected 1 deleted publish, got %d", len(pub.deleted))
	}
	if pub.deleted[0].ReviewID != review.ID {
		t.Errorf("ReviewID = %q, want %q", pub.deleted[0].ReviewID, review.ID)
	}
}

// TestDeleteReview_NotFound verifies that deleting a non-existent review is a no-op (idempotent)
// and still publishes the event — required for Option B sensor retry semantics.
func TestDeleteReview_NotFound(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	pub := &fakeReviewPublisher{}
	svc := service.NewReviewService(repo, client, idempotency.NewMemoryStore(), pub)

	err := svc.DeleteReview(context.Background(), "nonexistent-id")
	if err != nil {
		t.Fatalf("expected nil (idempotent delete), got: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.deleted) != 1 {
		t.Fatalf("expected 1 deleted publish even for missing review, got %d", len(pub.deleted))
	}
}

func TestDeleteReview_PublishesEvent(t *testing.T) {
	repo := memory.NewReviewRepository()
	pub := &fakeReviewPublisher{}
	svc := service.NewReviewService(repo, nil, idempotency.NewMemoryStore(), pub)

	// Deleting a non-existent review succeeds and still publishes.
	if err := svc.DeleteReview(context.Background(), "rev_missing"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.deleted) != 1 {
		t.Fatalf("expected 1 deleted publish, got %d", len(pub.deleted))
	}
	if pub.deleted[0].ReviewID != "rev_missing" {
		t.Errorf("ReviewID = %q", pub.deleted[0].ReviewID)
	}
}
