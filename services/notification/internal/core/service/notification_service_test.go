// file: services/notification/internal/core/service/notification_service_test.go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/log"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
)

func TestDispatch_Success(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher, idempotency.NewMemoryStore())

	n, err := svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelEmail, "New Review", "A review was posted", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n.ID == "" {
		t.Error("expected non-empty ID")
	}
	if n.Status != domain.StatusSent {
		t.Errorf("Status = %q, want %q", n.Status, domain.StatusSent)
	}
}

func TestDispatch_ValidationError(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher, idempotency.NewMemoryStore())

	_, err := svc.Dispatch(context.Background(), "", domain.ChannelEmail, "Subject", "Body", "")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDispatch_DispatcherFailure(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := &failingDispatcher{}
	svc := service.NewNotificationService(repo, dispatcher, idempotency.NewMemoryStore())

	n, err := svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelEmail, "Subject", "Body", "")
	// Dispatch should still succeed but the notification should be marked as failed
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n.Status != domain.StatusFailed {
		t.Errorf("Status = %q, want %q", n.Status, domain.StatusFailed)
	}
}

type failingDispatcher struct{}

func (d *failingDispatcher) Send(_ context.Context, _ *domain.Notification) error {
	return errors.New("send failed")
}

func TestGetByID_Found(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher, idempotency.NewMemoryStore())

	created, _ := svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelEmail, "Subject", "Body", "")

	found, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("ID = %q, want %q", found.ID, created.ID)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher, idempotency.NewMemoryStore())

	_, err := svc.GetByID(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent notification")
	}
}

func TestGetByRecipient(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher, idempotency.NewMemoryStore())

	_, _ = svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelEmail, "Subject 1", "Body 1", "")
	_, _ = svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelSMS, "Subject 2", "Body 2", "")
	_, _ = svc.Dispatch(context.Background(), "bob@example.com", domain.ChannelEmail, "Subject 3", "Body 3", "")

	notifications, err := svc.GetByRecipient(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifications) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(notifications))
	}
}
