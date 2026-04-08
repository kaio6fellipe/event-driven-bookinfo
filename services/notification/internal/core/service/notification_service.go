// Package service implements the business logic for the notification service.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/port"
)

// NotificationService implements the port.NotificationService interface.
type NotificationService struct {
	repo       port.NotificationRepository
	dispatcher port.NotificationDispatcher
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(repo port.NotificationRepository, dispatcher port.NotificationDispatcher) *NotificationService {
	return &NotificationService{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

// Dispatch creates, dispatches, and persists a notification.
// If the dispatcher fails, the notification is marked as failed but still persisted.
func (s *NotificationService) Dispatch(ctx context.Context, recipient string, channel domain.Channel, subject, body string) (*domain.Notification, error) {
	notification, err := domain.NewNotification(recipient, channel, subject, body)
	if err != nil {
		return nil, fmt.Errorf("creating notification: %w", err)
	}

	if err := s.dispatcher.Send(ctx, notification); err != nil {
		logger := logging.FromContext(ctx)
		logger.Error("failed to dispatch notification",
			slog.String("notification_id", notification.ID),
			slog.String("error", err.Error()),
		)
		notification.MarkFailed()
	} else {
		notification.MarkSent()
	}

	if err := s.repo.Save(ctx, notification); err != nil {
		return nil, fmt.Errorf("saving notification: %w", err)
	}

	return notification, nil
}

// GetByID returns a notification by its ID.
func (s *NotificationService) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	n, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding notification %s: %w", id, err)
	}
	return n, nil
}

// GetByRecipient returns all notifications for a given recipient.
func (s *NotificationService) GetByRecipient(ctx context.Context, recipient string) ([]domain.Notification, error) {
	notifications, err := s.repo.FindByRecipient(ctx, recipient)
	if err != nil {
		return nil, fmt.Errorf("finding notifications for %s: %w", recipient, err)
	}
	return notifications, nil
}
