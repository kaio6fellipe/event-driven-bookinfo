// file: services/notification/internal/core/domain/notification.go
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Channel represents the notification delivery channel.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
	ChannelPush  Channel = "push"
)

// NotificationStatus represents the delivery status.
type NotificationStatus string

const (
	StatusQueued NotificationStatus = "queued"
	StatusSent   NotificationStatus = "sent"
	StatusFailed NotificationStatus = "failed"
)

// Notification represents a notification to be dispatched.
type Notification struct {
	ID        string
	Recipient string
	Channel   Channel
	Subject   string
	Body      string
	Status    NotificationStatus
	SentAt    time.Time
}

// NewNotification creates a new Notification with validation.
func NewNotification(recipient string, channel Channel, subject, body string) (*Notification, error) {
	if recipient == "" {
		return nil, fmt.Errorf("recipient is required")
	}
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if body == "" {
		return nil, fmt.Errorf("body is required")
	}
	if !isValidChannel(channel) {
		return nil, fmt.Errorf("invalid channel: %s", channel)
	}

	return &Notification{
		ID:        uuid.New().String(),
		Recipient: recipient,
		Channel:   channel,
		Subject:   subject,
		Body:      body,
		Status:    StatusQueued,
	}, nil
}

// MarkSent updates the notification status to sent with the current time.
func (n *Notification) MarkSent() {
	n.Status = StatusSent
	n.SentAt = time.Now()
}

// MarkFailed updates the notification status to failed.
func (n *Notification) MarkFailed() {
	n.Status = StatusFailed
}

func isValidChannel(c Channel) bool {
	switch c {
	case ChannelEmail, ChannelSMS, ChannelPush:
		return true
	default:
		return false
	}
}
