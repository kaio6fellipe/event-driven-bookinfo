// Package http provides HTTP handlers and DTOs for the notification service.
package http //nolint:revive // package name matches directory convention

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
)

// Handler holds the HTTP handlers for the notification service.
type Handler struct {
	svc port.NotificationService
}

// NewHandler creates a new HTTP handler with the given notification service.
func NewHandler(svc port.NotificationService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the notification routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/notifications", h.dispatch)
	mux.HandleFunc("GET /v1/notifications/{id}", h.getByID)
	mux.HandleFunc("GET /v1/notifications", h.listByRecipient)
}

func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req DispatchNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	notification, err := h.svc.Dispatch(r.Context(), req.Recipient, domain.Channel(req.Channel), req.Subject, req.Body, req.IdempotencyKey)
	if err != nil {
		if errors.Is(err, service.ErrAlreadyProcessed) {
			logger.Info("duplicate dispatch skipped")
			writeJSON(w, http.StatusOK, ErrorResponse{Error: "already processed"})
			return
		}
		logger.Warn("failed to dispatch notification", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("notification dispatched",
		"notification_id", notification.ID,
		"channel", string(notification.Channel),
		"status", string(notification.Status),
	)

	writeJSON(w, http.StatusCreated, NotificationResponse{
		ID:        notification.ID,
		Recipient: notification.Recipient,
		Channel:   string(notification.Channel),
		Subject:   notification.Subject,
		Body:      notification.Body,
		Status:    string(notification.Status),
		SentAt:    notification.SentAt,
	})
}

func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	notification, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		logger.Warn("notification not found", "id", id, "error", err)
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "notification not found"})
		return
	}

	writeJSON(w, http.StatusOK, NotificationResponse{
		ID:        notification.ID,
		Recipient: notification.Recipient,
		Channel:   string(notification.Channel),
		Subject:   notification.Subject,
		Body:      notification.Body,
		Status:    string(notification.Status),
		SentAt:    notification.SentAt,
	})
}

func (h *Handler) listByRecipient(w http.ResponseWriter, r *http.Request) {
	recipient := r.URL.Query().Get("recipient")
	if recipient == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "recipient query parameter is required"})
		return
	}

	logger := logging.FromContext(r.Context())

	notifications, err := h.svc.GetByRecipient(r.Context(), recipient)
	if err != nil {
		logger.Error("failed to get notifications", "error", err, "recipient", recipient)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	responses := make([]NotificationResponse, 0, len(notifications))
	for _, n := range notifications {
		responses = append(responses, NotificationResponse{
			ID:        n.ID,
			Recipient: n.Recipient,
			Channel:   string(n.Channel),
			Subject:   n.Subject,
			Body:      n.Body,
			Status:    string(n.Status),
			SentAt:    n.SentAt,
		})
	}

	writeJSON(w, http.StatusOK, NotificationsListResponse{
		Notifications: responses,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
