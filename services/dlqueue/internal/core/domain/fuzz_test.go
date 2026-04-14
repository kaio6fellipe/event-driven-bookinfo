package domain_test

import (
	"testing"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
)

func FuzzNewDLQEvent(f *testing.F) {
	f.Add("evt-1", "book-added", "webhook", "sub-1", "sensor-1", "trigger-1", "http://example.com", "default", []byte(`{"key":"val"}`), "application/json", int64(1700000000), 3)
	f.Add("", "", "", "", "", "", "", "", []byte{}, "", int64(0), 0)
	f.Add("e", "t", "s", "x", "sn", "ft", "u", "ns", []byte("garbage"), "text/plain", int64(-1), -5)

	f.Fuzz(func(t *testing.T, eventID, eventType, eventSource, eventSubject, sensorName, failedTrigger, eventSourceURL, namespace string, payload []byte, dataContentType string, tsUnix int64, maxRetries int) {
		p := domain.NewDLQEventParams{
			EventID:         eventID,
			EventType:       eventType,
			EventSource:     eventSource,
			EventSubject:    eventSubject,
			SensorName:      sensorName,
			FailedTrigger:   failedTrigger,
			EventSourceURL:  eventSourceURL,
			Namespace:       namespace,
			OriginalPayload: payload,
			DataContentType: dataContentType,
			EventTimestamp:  time.Unix(tsUnix, 0),
			MaxRetries:      maxRetries,
		}

		e, err := domain.NewDLQEvent(p)
		if err != nil {
			return
		}
		if e.ID == "" {
			t.Error("valid DLQ event must have non-empty ID")
		}
		if e.Status != domain.StatusPending {
			t.Errorf("Status = %q, want %q", e.Status, domain.StatusPending)
		}
	})
}
