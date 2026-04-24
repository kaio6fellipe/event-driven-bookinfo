package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)

func TestAddDetail_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})

	// Call twice with the same explicit key; expect ErrAlreadyProcessed on second.
	_, err := svc.AddDetail(ctx,
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "key-1",
	)
	if err != nil {
		t.Fatalf("first: err = %v", err)
	}
	_, err = svc.AddDetail(ctx,
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "key-1",
	)
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("second: err = %v, want ErrAlreadyProcessed", err)
	}
}

func TestAddDetail_NaturalKey(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})

	_, err := svc.AddDetail(ctx,
		"Book One", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if err != nil {
		t.Fatalf("first submit err = %v", err)
	}

	_, err = svc.AddDetail(ctx,
		"Book One", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("duplicate natural key: err = %v, want ErrAlreadyProcessed", err)
	}

	_, err = svc.AddDetail(ctx,
		"Book Two", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if err != nil {
		t.Errorf("different title should succeed, got err = %v", err)
	}
}
