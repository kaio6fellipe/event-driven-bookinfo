package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
)

func TestSubmitRating_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo, idempotency.NewMemoryStore(), &fakeRatingPublisher{})

	// Call twice with the same explicit key; expect ErrAlreadyProcessed on second.
	_, err := svc.SubmitRating(ctx, "product-1", "alice", 5, "key-1")
	if err != nil {
		t.Fatalf("first: err = %v", err)
	}
	_, err = svc.SubmitRating(ctx, "product-1", "alice", 5, "key-1")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("second: err = %v, want ErrAlreadyProcessed", err)
	}
}

func TestSubmitRating_NaturalKey(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo, idempotency.NewMemoryStore(), &fakeRatingPublisher{})

	_, err := svc.SubmitRating(ctx, "p1", "bob", 4, "")
	if err != nil {
		t.Fatalf("first submit err = %v", err)
	}

	_, err = svc.SubmitRating(ctx, "p1", "bob", 4, "")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("duplicate natural key: err = %v, want ErrAlreadyProcessed", err)
	}

	_, err = svc.SubmitRating(ctx, "p1", "bob", 5, "")
	if err != nil {
		t.Errorf("different stars should succeed, got err = %v", err)
	}
}
