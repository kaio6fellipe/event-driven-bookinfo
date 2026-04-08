// Package postgres provides a PostgreSQL implementation of the notification repository.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// NotificationRepository is a PostgreSQL implementation of port.NotificationRepository.
type NotificationRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationRepository creates a new PostgreSQL notification repository.
func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

// Save persists a notification in PostgreSQL.
func (r *NotificationRepository) Save(ctx context.Context, notification *domain.Notification) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO notifications (id, recipient, channel, subject, body, status, sent_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		notification.ID, notification.Recipient, string(notification.Channel), notification.Subject, notification.Body, string(notification.Status), notification.SentAt,
	)
	if err != nil {
		return fmt.Errorf("inserting notification %s: %w", notification.ID, err)
	}

	return nil
}

// FindByID returns a notification by its ID. Returns nil, nil if not found.
func (r *NotificationRepository) FindByID(ctx context.Context, id string) (*domain.Notification, error) {
	var n domain.Notification
	var channel string
	var status string
	err := r.pool.QueryRow(ctx,
		"SELECT id, recipient, channel, subject, body, status, sent_at FROM notifications WHERE id = $1",
		id,
	).Scan(&n.ID, &n.Recipient, &channel, &n.Subject, &n.Body, &status, &n.SentAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying notification %s: %w", id, err)
	}

	n.Channel = domain.Channel(channel)
	n.Status = domain.NotificationStatus(status)

	return &n, nil
}

// FindByRecipient returns all notifications for a given recipient.
func (r *NotificationRepository) FindByRecipient(ctx context.Context, recipient string) ([]domain.Notification, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, recipient, channel, subject, body, status, sent_at FROM notifications WHERE recipient = $1",
		recipient,
	)
	if err != nil {
		return nil, fmt.Errorf("querying notifications for recipient %s: %w", recipient, err)
	}
	defer rows.Close()

	var notifications []domain.Notification
	for rows.Next() {
		var n domain.Notification
		var channel string
		var status string
		if err := rows.Scan(&n.ID, &n.Recipient, &channel, &n.Subject, &n.Body, &status, &n.SentAt); err != nil {
			return nil, fmt.Errorf("scanning notification row: %w", err)
		}
		n.Channel = domain.Channel(channel)
		n.Status = domain.NotificationStatus(status)
		notifications = append(notifications, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notification rows: %w", err)
	}

	return notifications, nil
}
