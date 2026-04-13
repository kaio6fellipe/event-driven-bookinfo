package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
)

func newTestEvent(t *testing.T) *domain.DLQEvent {
	t.Helper()
	e, err := domain.NewDLQEvent(domain.NewDLQEventParams{
		EventID:         "evt-1",
		EventType:       "webhook",
		EventSource:     "review-submitted",
		EventSubject:    "review-submitted",
		SensorName:      "review-submitted-sensor",
		FailedTrigger:   "create-review",
		EventSourceURL:  "http://review-submitted-eventsource-svc:12001/review-submitted",
		Namespace:       "bookinfo",
		OriginalPayload: []byte(`{"product_id":"p1"}`),
		OriginalHeaders: map[string][]string{"traceparent": {"00-abc"}},
		DataContentType: "application/json",
		EventTimestamp:  time.Now(),
		MaxRetries:      3,
	})
	if err != nil {
		t.Fatalf("NewDLQEvent err = %v", err)
	}
	return e
}

func TestNewDLQEvent_InitialState(t *testing.T) {
	e := newTestEvent(t)
	if e.Status != domain.StatusPending {
		t.Errorf("initial status = %v, want pending", e.Status)
	}
	if e.RetryCount != 0 {
		t.Errorf("initial retry_count = %d, want 0", e.RetryCount)
	}
	if e.ID == "" {
		t.Error("expected generated ID")
	}
	if e.PayloadHash == "" {
		t.Error("expected computed payload hash")
	}
}

func TestReplay_FromPending(t *testing.T) {
	e := newTestEvent(t)
	if err := e.Replay(); err != nil {
		t.Fatalf("Replay err = %v", err)
	}
	if e.Status != domain.StatusReplayed {
		t.Errorf("status = %v, want replayed", e.Status)
	}
	if e.LastReplayedAt == nil {
		t.Error("expected LastReplayedAt set")
	}
}

func TestReplay_FromResolved_Rejected(t *testing.T) {
	e := newTestEvent(t)
	_ = e.Replay()
	_ = e.Resolve("operator")
	if err := e.Replay(); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestResolve_FromReplayed(t *testing.T) {
	e := newTestEvent(t)
	_ = e.Replay()
	if err := e.Resolve("ratings-service"); err != nil {
		t.Fatalf("Resolve err = %v", err)
	}
	if e.Status != domain.StatusResolved {
		t.Errorf("status = %v, want resolved", e.Status)
	}
	if e.ResolvedBy != "ratings-service" {
		t.Errorf("resolved_by = %q, want ratings-service", e.ResolvedBy)
	}
	if e.ResolvedAt == nil {
		t.Error("expected ResolvedAt set")
	}
}

func TestResolve_AlreadyResolved(t *testing.T) {
	e := newTestEvent(t)
	_ = e.Replay()
	_ = e.Resolve("op1")
	if err := e.Resolve("op2"); !errors.Is(err, domain.ErrAlreadyResolved) {
		t.Errorf("err = %v, want ErrAlreadyResolved", err)
	}
}

func TestReIngest_IncrementsRetryCount(t *testing.T) {
	e := newTestEvent(t)
	_ = e.Replay()
	if err := e.ReIngest(); err != nil {
		t.Fatalf("ReIngest err = %v", err)
	}
	if e.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending", e.Status)
	}
	if e.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", e.RetryCount)
	}
}

func TestReIngest_HitsMaxRetries_Poisons(t *testing.T) {
	e := newTestEvent(t)
	e.MaxRetries = 2

	// Simulate 2 failed replays.
	_ = e.Replay()
	_ = e.ReIngest() // retry=1, pending
	_ = e.Replay()
	_ = e.ReIngest() // retry=2, poisoned

	if e.Status != domain.StatusPoisoned {
		t.Errorf("status = %v, want poisoned (retry=%d, max=%d)", e.Status, e.RetryCount, e.MaxRetries)
	}
}

func TestResetPoison_FromPoisoned(t *testing.T) {
	e := newTestEvent(t)
	e.MaxRetries = 1
	_ = e.Replay()
	_ = e.ReIngest() // poisoned

	if err := e.ResetPoison(); err != nil {
		t.Fatalf("ResetPoison err = %v", err)
	}
	if e.Status != domain.StatusPending {
		t.Errorf("status = %v, want pending", e.Status)
	}
	if e.RetryCount != 0 {
		t.Errorf("retry_count = %d, want reset to 0", e.RetryCount)
	}
}

func TestResetPoison_FromPending_Rejected(t *testing.T) {
	e := newTestEvent(t)
	if err := e.ResetPoison(); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestPayloadHash_Deterministic(t *testing.T) {
	e1, _ := domain.NewDLQEvent(domain.NewDLQEventParams{
		OriginalPayload: []byte(`{"a":1}`),
		MaxRetries:      3,
	})
	e2, _ := domain.NewDLQEvent(domain.NewDLQEventParams{
		OriginalPayload: []byte(`{"a":1}`),
		MaxRetries:      3,
	})
	if e1.PayloadHash != e2.PayloadHash {
		t.Errorf("expected same hash for identical payload, got %q and %q", e1.PayloadHash, e2.PayloadHash)
	}
}
