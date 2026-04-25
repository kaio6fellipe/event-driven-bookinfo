package kafka_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/kafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

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

type errorClient struct{}

func (e *errorClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	var results kgo.ProduceResults
	for _, r := range rs {
		results = append(results, kgo.ProduceResult{Record: r, Err: context.DeadlineExceeded})
	}
	return results
}

func (e *errorClient) Close() {}

func TestPublishBookAdded(t *testing.T) {
	t.Parallel()

	evt := domain.BookAddedEvent{
		ID:             "det_123",
		Title:          "The Go Programming Language",
		Author:         "Alan Donovan, Brian Kernighan",
		Year:           2015,
		Type:           "paperback",
		Pages:          380,
		Publisher:      "Addison-Wesley",
		Language:       "en",
		ISBN10:         "",
		ISBN13:         "9780134190440",
		IdempotencyKey: "det-idem-123",
	}

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_details_events")

	if err := p.PublishBookAdded(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	if len(fc.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fc.records))
	}
	r := fc.records[0]

	if r.Topic != "bookinfo_details_events" {
		t.Errorf("topic = %q, want %q", r.Topic, "bookinfo_details_events")
	}
	if string(r.Key) != evt.IdempotencyKey {
		t.Errorf("key = %q, want %q", string(r.Key), evt.IdempotencyKey)
	}

	headers := map[string]string{}
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["ce_type"] != "com.bookinfo.details.book-added" {
		t.Errorf("ce_type = %q, want %q", headers["ce_type"], "com.bookinfo.details.book-added")
	}
	if headers["ce_source"] != "details" {
		t.Errorf("ce_source = %q, want %q", headers["ce_source"], "details")
	}
	if headers["ce_specversion"] != "1.0" {
		t.Errorf("ce_specversion = %q, want %q", headers["ce_specversion"], "1.0")
	}
	if headers["ce_subject"] != evt.IdempotencyKey {
		t.Errorf("ce_subject = %q, want %q", headers["ce_subject"], evt.IdempotencyKey)
	}
	if headers["content-type"] != "application/json" {
		t.Errorf("content-type = %q, want %q", headers["content-type"], "application/json")
	}
	if headers["ce_id"] == "" {
		t.Error("ce_id header is empty, expected a UUID")
	}
	if headers["ce_time"] == "" {
		t.Error("ce_time header is empty, expected RFC3339 timestamp")
	}

	var body map[string]interface{}
	if err := json.Unmarshal(r.Value, &body); err != nil {
		t.Fatalf("failed to unmarshal record value: %v", err)
	}
	if body["title"] != evt.Title {
		t.Errorf("body title = %q, want %q", body["title"], evt.Title)
	}
	if body["author"] != evt.Author {
		t.Errorf("body author = %q, want %q", body["author"], evt.Author)
	}
	if body["year"].(float64) != float64(evt.Year) {
		t.Errorf("body year = %v, want %v", body["year"], evt.Year)
	}
	if body["isbn_13"] != evt.ISBN13 {
		t.Errorf("body isbn_13 = %q, want %q", body["isbn_13"], evt.ISBN13)
	}
	if body["idempotency_key"] != evt.IdempotencyKey {
		t.Errorf("body idempotency_key = %q, want %q", body["idempotency_key"], evt.IdempotencyKey)
	}
}

func TestPublishBookAdded_ProduceError(t *testing.T) {
	t.Parallel()

	p := kafkaadapter.NewProducerWithClient(&errorClient{}, "bookinfo_details_events")

	err := p.PublishBookAdded(context.Background(), domain.BookAddedEvent{
		Title: "t", Author: "a", Year: 2024, Type: "paperback",
		IdempotencyKey: "idem-x",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPublishBookAdded_InjectsTraceparent(t *testing.T) {
	t.Parallel()

	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_details_events")

	if err := p.PublishBookAdded(ctx, domain.BookAddedEvent{
		Title: "T", Author: "A", Year: 2024, Type: "paperback",
		IdempotencyKey: "k1",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	headers := map[string]string{}
	for _, h := range fc.records[0].Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["traceparent"] == "" {
		t.Fatal("expected traceparent header, got none")
	}
	want := span.SpanContext().TraceID().String()
	if got := headers["traceparent"]; len(got) < 35 || got[3:35] != want {
		t.Errorf("traceparent = %q, want embedded trace_id %q", got, want)
	}
}
