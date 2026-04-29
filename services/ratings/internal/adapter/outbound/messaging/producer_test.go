package messaging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/messaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// fakePub captures the last Publish call for assertion.
type fakePub struct {
	last events.Descriptor
	key  string
	idem string
	body any
	err  error
}

func (f *fakePub) Publish(_ context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error {
	f.last = d
	f.key = recordKey
	f.idem = idempotencyKey
	f.body = payload
	return f.err
}

func (f *fakePub) Close() {}

func TestPublishRatingSubmitted(t *testing.T) {
	t.Parallel()

	fp := &fakePub{}
	p := kafkaadapter.NewProducer(fp)

	evt := domain.RatingSubmittedEvent{
		ID: "rat_1", ProductID: "prod-42", Reviewer: "alice", Stars: 5,
		IdempotencyKey: "rat-idem-1",
	}
	if err := p.PublishRatingSubmitted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fp.last.CEType != "com.bookinfo.ratings.rating-submitted" {
		t.Errorf("ce_type = %q", fp.last.CEType)
	}
	if fp.last.CESource != "ratings" {
		t.Errorf("ce_source = %q", fp.last.CESource)
	}
	if fp.key != evt.ProductID {
		t.Errorf("record key = %q, want %q", fp.key, evt.ProductID)
	}
	if fp.idem != evt.IdempotencyKey {
		t.Errorf("idempotency key = %q, want %q", fp.idem, evt.IdempotencyKey)
	}

	body, ok := fp.body.(kafkaadapter.RatingSubmittedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want RatingSubmittedPayload", fp.body)
	}
	if body.ProductID != evt.ProductID {
		t.Errorf("product_id = %q, want %q", body.ProductID, evt.ProductID)
	}
	if body.Stars != evt.Stars {
		t.Errorf("stars = %d, want %d", body.Stars, evt.Stars)
	}
}

func TestPublishRatingSubmitted_ProduceError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("backend unavailable")
	fp := &fakePub{err: wantErr}
	p := kafkaadapter.NewProducer(fp)

	err := p.PublishRatingSubmitted(context.Background(), domain.RatingSubmittedEvent{
		ID: "rt1", ProductID: "p", Reviewer: "u", Stars: 5, IdempotencyKey: "k1",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want to wrap %v", err, wantErr)
	}
}
