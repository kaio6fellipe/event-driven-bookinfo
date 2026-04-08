// Package memory provides an in-memory implementation of the notification repository.
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// NotificationRepository is an in-memory implementation of port.NotificationRepository.
type NotificationRepository struct {
	mu            sync.RWMutex
	notifications map[string]domain.Notification
}

// NewNotificationRepository creates a new in-memory notification repository.
func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{
		notifications: make(map[string]domain.Notification),
	}
}

// Save persists a notification in memory.
func (r *NotificationRepository) Save(_ context.Context, notification *domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.notifications[notification.ID] = *notification
	return nil
}

// FindByID returns a notification by its ID.
func (r *NotificationRepository) FindByID(_ context.Context, id string) (*domain.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n, ok := r.notifications[id]
	if !ok {
		return nil, fmt.Errorf("notification not found: %s", id)
	}

	return &n, nil
}

// FindByRecipient returns all notifications for a given recipient.
func (r *NotificationRepository) FindByRecipient(_ context.Context, recipient string) ([]domain.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []domain.Notification
	for _, n := range r.notifications {
		if n.Recipient == recipient {
			result = append(result, n)
		}
	}

	return result, nil
}
