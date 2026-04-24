package kafka_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/kafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
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

func TestPublishRatingSubmitted(t *testing.T) {
	t.Parallel()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_ratings_events")

	evt := domain.RatingSubmittedEvent{
		ID: "rat_1", ProductID: "prod-42", Reviewer: "alice", Stars: 5,
		IdempotencyKey: "rat-idem-1",
	}
	if err := p.PublishRatingSubmitted(context.Background(), evt); err != nil {
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
	if headers["ce_type"] != "com.bookinfo.ratings.rating-submitted" {
		t.Errorf("ce_type = %q", headers["ce_type"])
	}
	if headers["ce_source"] != "ratings" {
		t.Errorf("ce_source = %q", headers["ce_source"])
	}

	var body map[string]interface{}
	_ = json.Unmarshal(r.Value, &body)
	if body["product_id"] != "prod-42" {
		t.Errorf("product_id = %v", body["product_id"])
	}
	if body["stars"].(float64) != 5 {
		t.Errorf("stars = %v", body["stars"])
	}
}
