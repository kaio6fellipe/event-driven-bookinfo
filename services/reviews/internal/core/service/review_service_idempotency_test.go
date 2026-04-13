package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

func TestSubmitReview_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewReviewRepository()
	svc := service.NewReviewService(repo, nil, idempotency.NewMemoryStore())

	_, err := svc.SubmitReview(ctx, "p1", "alice", "great book", "key-1")
	if err != nil {
		t.Fatalf("first submit err = %v", err)
	}

	_, err = svc.SubmitReview(ctx, "p1", "alice", "great book", "key-1")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("second submit: err = %v, want ErrAlreadyProcessed", err)
	}
}

func TestSubmitReview_NaturalKey(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewReviewRepository()
	svc := service.NewReviewService(repo, nil, idempotency.NewMemoryStore())

	_, err := svc.SubmitReview(ctx, "p1", "bob", "good", "")
	if err != nil {
		t.Fatalf("first submit err = %v", err)
	}

	_, err = svc.SubmitReview(ctx, "p1", "bob", "good", "")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("duplicate natural key: err = %v, want ErrAlreadyProcessed", err)
	}

	_, err = svc.SubmitReview(ctx, "p1", "bob", "different text", "")
	if err != nil {
		t.Errorf("different text should succeed, got err = %v", err)
	}
}
