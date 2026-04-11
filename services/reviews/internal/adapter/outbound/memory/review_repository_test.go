package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

func seedReviews(t *testing.T, repo *memory.ReviewRepository, productID string, count int) []domain.Review {
	t.Helper()
	var reviews []domain.Review
	for i := 0; i < count; i++ {
		r, err := domain.NewReview(productID, "reviewer", "text")
		if err != nil {
			t.Fatalf("creating review: %v", err)
		}
		if err := repo.Save(context.Background(), r); err != nil {
			t.Fatalf("saving review: %v", err)
		}
		reviews = append(reviews, *r)
	}
	return reviews
}

func TestFindByProductID_Pagination(t *testing.T) {
	repo := memory.NewReviewRepository()
	seedReviews(t, repo, "product-1", 25)
	seedReviews(t, repo, "product-2", 3)

	tests := []struct {
		name      string
		productID string
		offset    int
		limit     int
		wantCount int
		wantTotal int
	}{
		{name: "first page", productID: "product-1", offset: 0, limit: 10, wantCount: 10, wantTotal: 25},
		{name: "second page", productID: "product-1", offset: 10, limit: 10, wantCount: 10, wantTotal: 25},
		{name: "last page partial", productID: "product-1", offset: 20, limit: 10, wantCount: 5, wantTotal: 25},
		{name: "offset beyond total", productID: "product-1", offset: 30, limit: 10, wantCount: 0, wantTotal: 25},
		{name: "different product", productID: "product-2", offset: 0, limit: 10, wantCount: 3, wantTotal: 3},
		{name: "empty product", productID: "nonexistent", offset: 0, limit: 10, wantCount: 0, wantTotal: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviews, total, err := repo.FindByProductID(context.Background(), tt.productID, tt.offset, tt.limit)
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

func TestDeleteByID_Success(t *testing.T) {
	repo := memory.NewReviewRepository()
	reviews := seedReviews(t, repo, "product-1", 3)

	err := repo.DeleteByID(context.Background(), reviews[1].ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	remaining, total, err := repo.FindByProductID(context.Background(), "product-1", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 remaining, got %d", total)
	}
	for _, r := range remaining {
		if r.ID == reviews[1].ID {
			t.Error("deleted review still present")
		}
	}
}

func TestDeleteByID_NotFound(t *testing.T) {
	repo := memory.NewReviewRepository()

	err := repo.DeleteByID(context.Background(), "nonexistent-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
