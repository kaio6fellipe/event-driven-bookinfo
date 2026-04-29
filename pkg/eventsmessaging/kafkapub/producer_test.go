package kafkapub_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/kafkapub"
)

// ---------------------------------------------------------------------------
// Fake clients
// ---------------------------------------------------------------------------

// fakeClient captures every record produced via ProduceSync without error.
type fakeClient struct {
	mu      sync.Mutex
	records []*kgo.Record
}

func (f *fakeClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	f.mu.Lock()
	defer f.mu.Unlock()

	var results kgo.ProduceResults
	for _, r := range rs {
		f.records = append(f.records, r)
		results = append(results, kgo.ProduceResult{Record: r})
	}
	return results
}

func (f *fakeClient) Close() {}

// captured returns the first record produced, or nil when none.
func (f *fakeClient) captured() *kgo.Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.records) == 0 {
		return nil
	}
	return f.records[0]
}

// errClient always returns an error from ProduceSync.
type errClient struct{ err error }

func (e *errClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	var results kgo.ProduceResults
	for _, r := range rs {
		results = append(results, kgo.ProduceResult{Record: r, Err: e.err})
	}
	return results
}

func (e *errClient) Close() {}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testDescriptor returns a fully-populated Descriptor for use in tests.
func testDescriptor() events.Descriptor {
	return events.Descriptor{
		Name:        "book-added",
		CEType:      "com.bookinfo.test.book-added",
		CESource:    "test-service",
		Version:     "1.0",
		ContentType: "application/json",
	}
}

// headerMap converts a kgo.Record's headers to a map for easy assertion.
func headerMap(r *kgo.Record) map[string]string {
	m := make(map[string]string, len(r.Headers))
	for _, h := range r.Headers {
		m[h.Key] = string(h.Value)
	}
	return m
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestProducer_Publish_BuildsCloudEventsHeaders verifies that all required
// CloudEvents binary-encoding headers are present and have the correct values.
func TestProducer_Publish_BuildsCloudEventsHeaders(t *testing.T) {
	t.Parallel()

	fc := &fakeClient{}
	p := kafkapub.NewProducerWithClient(fc, "test-topic")
	d := testDescriptor()

	type payload struct {
		Field string `json:"field"`
	}
	// RFC3339 has second-level precision; truncate the window bounds so the
	// comparison works even when sub-second time passes between before and Publish.
	before := time.Now().UTC().Truncate(time.Second)
	err := p.Publish(context.Background(), d, payload{Field: "val"}, "record-key", "idem-key")
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := fc.captured()
	if r == nil {
		t.Fatal("no record was produced")
	}

	h := headerMap(r)

	// ce_specversion
	if h["ce_specversion"] != d.Version {
		t.Errorf("ce_specversion = %q, want %q", h["ce_specversion"], d.Version)
	}

	// ce_type
	if h["ce_type"] != d.CEType {
		t.Errorf("ce_type = %q, want %q", h["ce_type"], d.CEType)
	}

	// ce_source
	if h["ce_source"] != d.CESource {
		t.Errorf("ce_source = %q, want %q", h["ce_source"], d.CESource)
	}

	// ce_subject — uses recordKey when non-empty
	if h["ce_subject"] != "record-key" {
		t.Errorf("ce_subject = %q, want %q", h["ce_subject"], "record-key")
	}

	// content-type
	if h["content-type"] != d.ContentType {
		t.Errorf("content-type = %q, want %q", h["content-type"], d.ContentType)
	}

	// ce_id — must be a valid UUID
	if _, err := uuid.Parse(h["ce_id"]); err != nil {
		t.Errorf("ce_id = %q is not a valid UUID: %v", h["ce_id"], err)
	}

	// ce_time — must parse as RFC3339 and fall within the test window.
	// RFC3339 has second-level precision so we compare against the truncated
	// lower bound captured before the call.
	ts, err := time.Parse(time.RFC3339, h["ce_time"])
	if err != nil {
		t.Errorf("ce_time = %q is not valid RFC3339: %v", h["ce_time"], err)
	} else {
		if ts.Before(before) || ts.After(after) {
			t.Errorf("ce_time %v is outside the expected window [%v, %v]", ts, before, after)
		}
	}
}

// TestProducer_Publish_KeyFallback verifies the record key selection logic:
// when recordKey is non-empty it is used as-is; when empty, idempotencyKey
// is used as the fallback.
func TestProducer_Publish_KeyFallback(t *testing.T) {
	t.Parallel()

	type payload struct{ V int }
	d := testDescriptor()

	tests := []struct {
		name           string
		recordKey      string
		idempotencyKey string
		wantKey        string
	}{
		{
			name:           "explicit record key",
			recordKey:      "explicit-key",
			idempotencyKey: "idem-1",
			wantKey:        "explicit-key",
		},
		{
			name:           "fallback to idempotency key when record key is empty",
			recordKey:      "",
			idempotencyKey: "idem-1",
			wantKey:        "idem-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fc := &fakeClient{}
			p := kafkapub.NewProducerWithClient(fc, "test-topic")

			err := p.Publish(context.Background(), d, payload{V: 1}, tt.recordKey, tt.idempotencyKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			r := fc.captured()
			if r == nil {
				t.Fatal("no record was produced")
			}

			if got := string(r.Key); got != tt.wantKey {
				t.Errorf("record key = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

// TestProducer_Publish_MarshalsPayloadAsJSON verifies that the record Value is
// the JSON-marshaled form of the payload struct.
func TestProducer_Publish_MarshalsPayloadAsJSON(t *testing.T) {
	t.Parallel()

	type bookPayload struct {
		Title          string `json:"title"`
		Author         string `json:"author"`
		IdempotencyKey string `json:"idempotency_key"`
	}

	tests := []struct {
		name    string
		payload bookPayload
	}{
		{
			name:    "full payload",
			payload: bookPayload{Title: "The Go Programming Language", Author: "Donovan, Kernighan", IdempotencyKey: "idem-abc"},
		},
		{
			name:    "minimal payload",
			payload: bookPayload{Title: "Learning Go", IdempotencyKey: "idem-xyz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fc := &fakeClient{}
			p := kafkapub.NewProducerWithClient(fc, "test-topic")

			err := p.Publish(context.Background(), testDescriptor(), tt.payload, "k", "k")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			r := fc.captured()
			if r == nil {
				t.Fatal("no record was produced")
			}

			var got bookPayload
			if err := json.Unmarshal(r.Value, &got); err != nil {
				t.Fatalf("failed to unmarshal record value: %v", err)
			}
			if got != tt.payload {
				t.Errorf("unmarshaled payload = %+v, want %+v", got, tt.payload)
			}
		})
	}
}

// TestProducer_Publish_ProduceError verifies that when the client returns an
// error, Publish wraps it with "producing to Kafka: %w".
func TestProducer_Publish_ProduceError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("broker unavailable")
	p := kafkapub.NewProducerWithClient(&errClient{err: sentinel}, "test-topic")

	err := p.Publish(context.Background(), testDescriptor(), struct{}{}, "k", "k")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("error chain does not contain sentinel: %v", err)
	}

	const wantPrefix = "producing to Kafka:"
	if len(err.Error()) < len(wantPrefix) || err.Error()[:len(wantPrefix)] != wantPrefix {
		t.Errorf("error message = %q, expected it to start with %q", err.Error(), wantPrefix)
	}
}

// TestProducer_Publish_TraceparentInjected verifies that when the context
// carries an active OTel span, the produced record contains a non-empty
// `traceparent` header whose trace-id segment matches the span.
func TestProducer_Publish_TraceparentInjected(t *testing.T) {
	t.Parallel()

	// Set up a real SDK tracer provider so spans are sampled.
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	fc := &fakeClient{}
	p := kafkapub.NewProducerWithClient(fc, "test-topic")

	err := p.Publish(ctx, testDescriptor(), struct{}{}, "k", "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := fc.captured()
	if r == nil {
		t.Fatal("no record was produced")
	}

	h := headerMap(r)

	traceparent := h["traceparent"]
	if traceparent == "" {
		t.Fatal("traceparent header is absent; expected W3C trace context to be injected")
	}

	// W3C traceparent format: 00-<trace-id>-<span-id>-<flags>
	// trace-id occupies characters [3:35].
	wantTraceID := span.SpanContext().TraceID().String()
	if len(traceparent) < 35 {
		t.Fatalf("traceparent %q is too short to contain a trace-id", traceparent)
	}
	if got := traceparent[3:35]; got != wantTraceID {
		t.Errorf("traceparent trace-id = %q, want %q", got, wantTraceID)
	}
}

// TestProducer_Publish_TopicSet verifies that the record's Topic field equals
// the topic passed to NewProducerWithClient.
func TestProducer_Publish_TopicSet(t *testing.T) {
	t.Parallel()

	const wantTopic = "my-service-events"

	fc := &fakeClient{}
	p := kafkapub.NewProducerWithClient(fc, wantTopic)

	err := p.Publish(context.Background(), testDescriptor(), struct{}{}, "k", "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := fc.captured()
	if r == nil {
		t.Fatal("no record was produced")
	}

	if r.Topic != wantTopic {
		t.Errorf("record.Topic = %q, want %q", r.Topic, wantTopic)
	}
}
