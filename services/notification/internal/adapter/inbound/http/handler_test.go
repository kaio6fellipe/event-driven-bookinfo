package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/log"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
)

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestDispatchNotification_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.DispatchNotificationRequest{
		Recipient: "alice@example.com",
		Channel:   "email",
		Subject:   "New Review",
		Body:      "A review was posted",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body handler.NotificationResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
	if body.Status != "sent" {
		t.Errorf("Status = %q, want %q", body.Status, "sent")
	}
}

func TestDispatchNotification_InvalidBody(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDispatchNotification_InvalidChannel(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.DispatchNotificationRequest{
		Recipient: "alice@example.com",
		Channel:   "telegram",
		Subject:   "Subject",
		Body:      "Body",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetNotificationByID_Found(t *testing.T) {
	mux := setupHandler(t)

	// Create a notification
	reqBody := handler.DispatchNotificationRequest{
		Recipient: "alice@example.com",
		Channel:   "email",
		Subject:   "Subject",
		Body:      "Body",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created handler.NotificationResponse
	_ = json.NewDecoder(createRec.Body).Decode(&created)

	// Get by ID
	getReq := httptest.NewRequest(http.MethodGet, "/v1/notifications/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var body handler.NotificationResponse
	_ = json.NewDecoder(getRec.Body).Decode(&body)

	if body.ID != created.ID {
		t.Errorf("ID = %q, want %q", body.ID, created.ID)
	}
}

func TestGetNotificationByID_NotFound(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetNotificationsByRecipient(t *testing.T) {
	mux := setupHandler(t)

	// Create two notifications for alice
	for i := 0; i < 2; i++ {
		reqBody := handler.DispatchNotificationRequest{
			Recipient: "alice@example.com",
			Channel:   "email",
			Subject:   "Subject",
			Body:      "Body",
		}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}

	// Query by recipient
	req := httptest.NewRequest(http.MethodGet, "/v1/notifications?recipient=alice@example.com", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.NotificationsListResponse
	_ = json.NewDecoder(rec.Body).Decode(&body)

	if len(body.Notifications) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(body.Notifications))
	}
}

func TestGetNotifications_MissingRecipient(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
