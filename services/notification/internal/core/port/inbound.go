// file: services/notification/internal/core/port/inbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// NotificationService defines the inbound operations for the notification domain.
type NotificationService interface {
	// Dispatch creates and dispatches a notification.
	Dispatch(ctx context.Context, recipient string, channel domain.Channel, subject, body string) (*domain.Notification, error)

	// GetByID returns a notification by its ID.
	GetByID(ctx context.Context, id string) (*domain.Notification, error)

	// GetByRecipient returns all notifications for a given recipient.
	GetByRecipient(ctx context.Context, recipient string) ([]domain.Notification, error)
}
