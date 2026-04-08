// Package port defines the inbound and outbound interfaces for the notification service.
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// NotificationRepository defines the outbound persistence operations for notifications.
type NotificationRepository interface {
	// Save persists a notification.
	Save(ctx context.Context, notification *domain.Notification) error

	// FindByID returns a notification by its ID.
	FindByID(ctx context.Context, id string) (*domain.Notification, error)

	// FindByRecipient returns all notifications for a given recipient.
	FindByRecipient(ctx context.Context, recipient string) ([]domain.Notification, error)
}

// NotificationDispatcher defines the outbound operations for actually sending notifications.
type NotificationDispatcher interface {
	// Send dispatches the notification via the appropriate channel.
	Send(ctx context.Context, notification *domain.Notification) error
}
