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

	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/kafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
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

func TestPublishReviewSubmitted(t *testing.T) {
	t.Parallel()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_reviews_events")

	evt := domain.ReviewSubmittedEvent{
		ID: "rev_1", ProductID: "prod-42", Reviewer: "alice", Text: "Great",
		IdempotencyKey: "rev-idem-1",
	}
	if err := p.PublishReviewSubmitted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fc.records))
	}
	r := fc.records[0]
	headers := map[string]string{}
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["ce_type"] != "com.bookinfo.reviews.review-submitted" {
		t.Errorf("ce_type = %q", headers["ce_type"])
	}
	if headers["ce_source"] != "reviews" {
		t.Errorf("ce_source = %q", headers["ce_source"])
	}
	var body map[string]interface{}
	if err := json.Unmarshal(r.Value, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["product_id"] != "prod-42" {
		t.Errorf("body product_id = %v, want %q", body["product_id"], "prod-42")
	}
	if body["reviewer"] != "alice" {
		t.Errorf("body reviewer = %v", body["reviewer"])
	}
}

func TestPublishReviewDeleted(t *testing.T) {
	t.Parallel()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_reviews_events")

	evt := domain.ReviewDeletedEvent{ReviewID: "rev_99", ProductID: "prod-42", IdempotencyKey: "del-idem-1"}
	if err := p.PublishReviewDeleted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	r := fc.records[0]
	headers := map[string]string{}
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["ce_type"] != "com.bookinfo.reviews.review-deleted" {
		t.Errorf("ce_type = %q", headers["ce_type"])
	}
	var body map[string]interface{}
	_ = json.Unmarshal(r.Value, &body)
	if body["review_id"] != "rev_99" {
		t.Errorf("body review_id = %v", body["review_id"])
	}
}

func TestPublishReviewSubmitted_InjectsTraceparent(t *testing.T) {
	t.Parallel()

	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_reviews_events")

	if err := p.PublishReviewSubmitted(ctx, domain.ReviewSubmittedEvent{
		ID: "r1", ProductID: "p", Reviewer: "u", Text: "ok", IdempotencyKey: "k1",
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
