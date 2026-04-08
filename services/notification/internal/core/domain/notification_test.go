// file: services/notification/internal/core/domain/notification_test.go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

func TestNewNotification_Valid(t *testing.T) {
	tests := []struct {
		name      string
		recipient string
		channel   domain.Channel
		subject   string
		body      string
	}{
		{name: "email", recipient: "alice@example.com", channel: domain.ChannelEmail, subject: "New Review", body: "A review was posted"},
		{name: "sms", recipient: "+1234567890", channel: domain.ChannelSMS, subject: "New Rating", body: "A rating was posted"},
		{name: "push", recipient: "user-123", channel: domain.ChannelPush, subject: "New Book", body: "A book was added"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := domain.NewNotification(tt.recipient, tt.channel, tt.subject, tt.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if n.ID == "" {
				t.Error("expected non-empty ID")
			}
			if n.Recipient != tt.recipient {
				t.Errorf("Recipient = %q, want %q", n.Recipient, tt.recipient)
			}
			if n.Channel != tt.channel {
				t.Errorf("Channel = %q, want %q", n.Channel, tt.channel)
			}
			if n.Subject != tt.subject {
				t.Errorf("Subject = %q, want %q", n.Subject, tt.subject)
			}
			if n.Body != tt.body {
				t.Errorf("Body = %q, want %q", n.Body, tt.body)
			}
			if n.Status != domain.StatusQueued {
				t.Errorf("Status = %q, want %q", n.Status, domain.StatusQueued)
			}
		})
	}
}

func TestNewNotification_EmptyRecipient(t *testing.T) {
	_, err := domain.NewNotification("", domain.ChannelEmail, "Subject", "Body")
	if err == nil {
		t.Fatal("expected error for empty recipient")
	}
}

func TestNewNotification_EmptySubject(t *testing.T) {
	_, err := domain.NewNotification("alice@example.com", domain.ChannelEmail, "", "Body")
	if err == nil {
		t.Fatal("expected error for empty subject")
	}
}

func TestNewNotification_EmptyBody(t *testing.T) {
	_, err := domain.NewNotification("alice@example.com", domain.ChannelEmail, "Subject", "")
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestNewNotification_InvalidChannel(t *testing.T) {
	_, err := domain.NewNotification("alice@example.com", domain.Channel("telegram"), "Subject", "Body")
	if err == nil {
		t.Fatal("expected error for invalid channel")
	}
}

func TestNotification_MarkSent(t *testing.T) {
	n, _ := domain.NewNotification("alice@example.com", domain.ChannelEmail, "Subject", "Body")
	n.MarkSent()

	if n.Status != domain.StatusSent {
		t.Errorf("Status = %q, want %q", n.Status, domain.StatusSent)
	}
	if n.SentAt.IsZero() {
		t.Error("expected non-zero SentAt")
	}
}

func TestNotification_MarkFailed(t *testing.T) {
	n, _ := domain.NewNotification("alice@example.com", domain.ChannelEmail, "Subject", "Body")
	n.MarkFailed()

	if n.Status != domain.StatusFailed {
		t.Errorf("Status = %q, want %q", n.Status, domain.StatusFailed)
	}
}
