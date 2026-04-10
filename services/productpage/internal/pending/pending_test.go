package pending_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/pending"
)

func setupRedisStore(t *testing.T) *pending.RedisStore {
	t.Helper()
	mr := miniredis.RunT(t)
	store, err := pending.NewRedisStore("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("failed to create redis store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestStorePending(t *testing.T) {
	store := setupRedisStore(t)
	ctx := context.Background()

	err := store.StorePending(ctx, "product-1", pending.NewReview("alice", "Great book!", 5))
	if err != nil {
		t.Fatalf("StorePending failed: %v", err)
	}

	// Should retrieve 1 pending review with no confirmed reviews
	reviews, err := store.GetAndReconcile(ctx, "product-1", nil)
	if err != nil {
		t.Fatalf("GetAndReconcile failed: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("got %d pending reviews, want 1", len(reviews))
	}
	if reviews[0].Reviewer != "alice" {
		t.Errorf("reviewer = %q, want %q", reviews[0].Reviewer, "alice")
	}
	if reviews[0].Text != "Great book!" {
		t.Errorf("text = %q, want %q", reviews[0].Text, "Great book!")
	}
	if reviews[0].Stars != 5 {
		t.Errorf("stars = %d, want 5", reviews[0].Stars)
	}
}

func TestGetAndReconcile_RemovesConfirmed(t *testing.T) {
	store := setupRedisStore(t)
	ctx := context.Background()

	_ = store.StorePending(ctx, "product-1", pending.NewReview("alice", "Great book!", 5))
	_ = store.StorePending(ctx, "product-1", pending.NewReview("bob", "Decent read", 3))

	confirmed := []pending.ConfirmedReview{
		{Reviewer: "alice", Text: "Great book!"},
	}

	reviews, err := store.GetAndReconcile(ctx, "product-1", confirmed)
	if err != nil {
		t.Fatalf("GetAndReconcile failed: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("got %d pending reviews, want 1", len(reviews))
	}
	if reviews[0].Reviewer != "bob" {
		t.Errorf("remaining reviewer = %q, want %q", reviews[0].Reviewer, "bob")
	}

	// Second call with no confirmed: alice should be gone from Redis
	reviews, err = store.GetAndReconcile(ctx, "product-1", nil)
	if err != nil {
		t.Fatalf("GetAndReconcile failed: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("got %d pending reviews after reconcile, want 1 (bob only)", len(reviews))
	}
}

func TestGetAndReconcile_EmptyProduct(t *testing.T) {
	store := setupRedisStore(t)
	ctx := context.Background()

	reviews, err := store.GetAndReconcile(ctx, "nonexistent", nil)
	if err != nil {
		t.Fatalf("GetAndReconcile failed: %v", err)
	}
	if reviews != nil {
		t.Errorf("expected nil for nonexistent product, got %v", reviews)
	}
}

func TestGetAndReconcile_IsolatesProducts(t *testing.T) {
	store := setupRedisStore(t)
	ctx := context.Background()

	_ = store.StorePending(ctx, "product-1", pending.NewReview("alice", "Review for P1", 5))
	_ = store.StorePending(ctx, "product-2", pending.NewReview("bob", "Review for P2", 4))

	reviews, err := store.GetAndReconcile(ctx, "product-1", nil)
	if err != nil {
		t.Fatalf("GetAndReconcile failed: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("product-1 got %d pending, want 1", len(reviews))
	}
	if reviews[0].Reviewer != "alice" {
		t.Errorf("product-1 reviewer = %q, want alice", reviews[0].Reviewer)
	}
}
