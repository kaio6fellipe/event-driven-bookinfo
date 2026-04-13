package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	dlqhttp "github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/service"
)

// fakeReplay satisfies port.EventReplayClient.
type fakeReplay struct{}

func (fakeReplay) Replay(_ context.Context, _ string, _ []byte, _ map[string][]string) error {
	return nil
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	repo := memory.NewDLQRepository()
	svc := service.NewDLQService(repo, fakeReplay{}, 3, nil)
	h := dlqhttp.NewHandler(svc)
	mux := stdhttp.NewServeMux()
	h.RegisterRoutes(mux)
	return httptest.NewServer(mux)
}

func TestIngestAndList(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"event_id":         "evt-1",
		"event_type":       "webhook",
		"event_source":     "review-submitted",
		"event_subject":    "review-submitted",
		"sensor_name":      "review-submitted-sensor",
		"failed_trigger":   "create-review",
		"eventsource_url":  "http://esvc/v1",
		"namespace":        "bookinfo",
		"original_payload": map[string]any{"product_id": "p1"},
		"original_headers": map[string][]string{"traceparent": {"00-abc"}},
		"datacontenttype":  "application/json",
		"event_timestamp":  "2026-04-13T10:00:00Z",
	})
	resp, err := stdhttp.Post(ts.URL+"/v1/events", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST err = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}

	resp2, err := stdhttp.Get(ts.URL + "/v1/events?status=pending")
	if err != nil {
		t.Fatalf("GET err = %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != stdhttp.StatusOK {
		t.Errorf("status = %d, want 200", resp2.StatusCode)
	}
	var list dlqhttp.ListEventsResponse
	if err := json.NewDecoder(resp2.Body).Decode(&list); err != nil {
		t.Fatalf("decode err = %v", err)
	}
	if list.TotalItems != 1 {
		t.Errorf("total = %d, want 1", list.TotalItems)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()
	resp, err := stdhttp.Get(ts.URL + "/v1/events/missing-id")
	if err != nil {
		t.Fatalf("GET err = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestResolve_RequiresResolvedBy(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()
	resp, err := stdhttp.Post(ts.URL+"/v1/events/any/resolve", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST err = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
