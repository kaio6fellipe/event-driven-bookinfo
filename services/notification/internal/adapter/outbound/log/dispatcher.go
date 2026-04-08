// Package log provides a log-based notification dispatcher for development use.
package log //nolint:revive // package name matches directory convention

import (
	"context"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// Dispatcher is a notification dispatcher that logs instead of actually sending.
type Dispatcher struct{}

// NewDispatcher creates a new log-based notification dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

// Send logs the notification details instead of actually dispatching it.
func (d *Dispatcher) Send(ctx context.Context, notification *domain.Notification) error {
	logger := logging.FromContext(ctx)
	logger.Info("dispatching notification (log mode)",
		slog.String("notification_id", notification.ID),
		slog.String("recipient", notification.Recipient),
		slog.String("channel", string(notification.Channel)),
		slog.String("subject", notification.Subject),
		slog.String("body", notification.Body),
	)
	return nil
}
