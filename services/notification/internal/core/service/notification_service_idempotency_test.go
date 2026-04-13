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

func TestDispatch_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher, idempotency.NewMemoryStore())

	// Call twice with the same explicit key; expect ErrAlreadyProcessed on second.
	_, err := svc.Dispatch(ctx, "alice@example.com", domain.ChannelEmail, "New Review", "A review was posted", "key-1")
	if err != nil {
		t.Fatalf("first: err = %v", err)
	}
	_, err = svc.Dispatch(ctx, "alice@example.com", domain.ChannelEmail, "New Review", "A review was posted", "key-1")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("second: err = %v, want ErrAlreadyProcessed", err)
	}
}

func TestDispatch_NaturalKey(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher, idempotency.NewMemoryStore())

	_, err := svc.Dispatch(ctx, "bob@example.com", domain.ChannelEmail, "New Review", "A review was posted", "")
	if err != nil {
		t.Fatalf("first dispatch err = %v", err)
	}

	_, err = svc.Dispatch(ctx, "bob@example.com", domain.ChannelEmail, "New Review", "A review was posted", "")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("duplicate natural key: err = %v, want ErrAlreadyProcessed", err)
	}

	_, err = svc.Dispatch(ctx, "bob@example.com", domain.ChannelEmail, "New Review", "A different review was posted", "")
	if err != nil {
		t.Errorf("different body should succeed, got err = %v", err)
	}
}
