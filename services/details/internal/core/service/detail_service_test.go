// file: services/details/internal/core/service/detail_service_test.go
package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)

type fakePublisher struct {
	mu       sync.Mutex
	calls    []domain.BookAddedEvent
	forceErr error
}

func (f *fakePublisher) PublishBookAdded(_ context.Context, evt domain.BookAddedEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, evt)
	return f.forceErr
}

func TestAddDetail_Success(t *testing.T) {
	repo := memory.NewDetailRepository()
	pub := &fakePublisher{}
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), pub)

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

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].Title != "The Art of Go" {
		t.Errorf("event Title = %q, want %q", pub.calls[0].Title, "The Art of Go")
	}
}

func TestAddDetail_ValidationError(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})

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
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})

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
}

func TestGetDetail_NotFound(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})

	_, err := svc.GetDetail(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent detail")
	}
}

func TestAddDetail_PublishesOnIdempotencyDedup(t *testing.T) {
	repo := memory.NewDetailRepository()
	pub := &fakePublisher{}
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), pub)

	args := []interface{}{
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "fixed-key",
	}

	_, err := svc.AddDetail(context.Background(),
		args[0].(string), args[1].(string), args[2].(int), args[3].(string),
		args[4].(int), args[5].(string), args[6].(string), args[7].(string), args[8].(string), args[9].(string),
	)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Second call with same idempotency key
	_, err = svc.AddDetail(context.Background(),
		args[0].(string), args[1].(string), args[2].(int), args[3].(string),
		args[4].(int), args[5].(string), args[6].(string), args[7].(string), args[8].(string), args[9].(string),
	)
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Fatalf("second call: expected ErrAlreadyProcessed, got %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.calls) != 2 {
		t.Fatalf("expected 2 publish calls (one per attempt), got %d", len(pub.calls))
	}
}

func TestAddDetail_PublishErrorPropagates(t *testing.T) {
	repo := memory.NewDetailRepository()
	pub := &fakePublisher{forceErr: errors.New("kafka down")}
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), pub)

	_, err := svc.AddDetail(context.Background(),
		"Test", "Author", 2024, "paperback", 10, "Pub", "en", "", "9780000000002", "",
	)
	if err == nil {
		t.Fatal("expected publish error to propagate, got nil")
	}
}
