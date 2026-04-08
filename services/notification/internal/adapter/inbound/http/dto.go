// file: services/notification/internal/adapter/inbound/http/dto.go
package http

import "time"

// DispatchNotificationRequest is the JSON body for POST /v1/notifications.
type DispatchNotificationRequest struct {
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// NotificationResponse represents a notification in API responses.
type NotificationResponse struct {
	ID        string    `json:"id"`
	Recipient string    `json:"recipient"`
	Channel   string    `json:"channel"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	SentAt    time.Time `json:"sent_at,omitempty"`
}

// NotificationsListResponse wraps multiple notifications.
type NotificationsListResponse struct {
	Notifications []NotificationResponse `json:"notifications"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
