// file: services/details/internal/core/service/detail_service_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)

func TestAddDetail_Success(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore())

	detail, err := svc.AddDetail(context.Background(),
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.ID == "" {
		t.Error("expected non-empty ID")
	}
	if detail.Title != "The Art of Go" {
		t.Errorf("Title = %q, want %q", detail.Title, "The Art of Go")
	}
}

func TestAddDetail_ValidationError(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore())

	_, err := svc.AddDetail(context.Background(),
		"", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if err == nil {
		t.Fatal("expected validation error for empty title")
	}
}

func TestGetDetail_Found(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore())

	created, err := svc.AddDetail(context.Background(),
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	found, err := svc.GetDetail(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error getting: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("ID = %q, want %q", found.ID, created.ID)
	}
	if found.Title != "The Art of Go" {
		t.Errorf("Title = %q, want %q", found.Title, "The Art of Go")
	}
}

func TestGetDetail_NotFound(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore())

	_, err := svc.GetDetail(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent detail")
	}
}
