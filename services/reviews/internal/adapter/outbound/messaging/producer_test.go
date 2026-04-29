package messaging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/messaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
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

func TestPublishReviewSubmitted(t *testing.T) {
	t.Parallel()

	fp := &fakePub{}
	p := kafkaadapter.NewProducer(fp)

	evt := domain.ReviewSubmittedEvent{
		ID: "rev_1", ProductID: "prod-42", Reviewer: "alice", Text: "Great",
		IdempotencyKey: "rev-idem-1",
	}
	if err := p.PublishReviewSubmitted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fp.last.CEType != "com.bookinfo.reviews.review-submitted" {
		t.Errorf("ce_type = %q", fp.last.CEType)
	}
	if fp.last.CESource != "reviews" {
		t.Errorf("ce_source = %q", fp.last.CESource)
	}
	if fp.key != evt.ProductID {
		t.Errorf("record key = %q, want %q", fp.key, evt.ProductID)
	}
	if fp.idem != evt.IdempotencyKey {
		t.Errorf("idempotency key = %q, want %q", fp.idem, evt.IdempotencyKey)
	}

	body, ok := fp.body.(kafkaadapter.ReviewSubmittedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want ReviewSubmittedPayload", fp.body)
	}
	if body.ProductID != evt.ProductID {
		t.Errorf("product_id = %q, want %q", body.ProductID, evt.ProductID)
	}
	if body.Reviewer != evt.Reviewer {
		t.Errorf("reviewer = %q, want %q", body.Reviewer, evt.Reviewer)
	}
}

func TestPublishReviewDeleted(t *testing.T) {
	t.Parallel()

	fp := &fakePub{}
	p := kafkaadapter.NewProducer(fp)

	evt := domain.ReviewDeletedEvent{ReviewID: "rev_99", ProductID: "prod-42", IdempotencyKey: "del-idem-1"}
	if err := p.PublishReviewDeleted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fp.last.CEType != "com.bookinfo.reviews.review-deleted" {
		t.Errorf("ce_type = %q", fp.last.CEType)
	}

	body, ok := fp.body.(kafkaadapter.ReviewDeletedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want ReviewDeletedPayload", fp.body)
	}
	if body.ReviewID != evt.ReviewID {
		t.Errorf("review_id = %q, want %q", body.ReviewID, evt.ReviewID)
	}
}

func TestPublishReviewSubmitted_ProduceError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("backend unavailable")
	fp := &fakePub{err: wantErr}
	p := kafkaadapter.NewProducer(fp)

	err := p.PublishReviewSubmitted(context.Background(), domain.ReviewSubmittedEvent{
		ID: "r1", ProductID: "p", Reviewer: "u", Text: "ok", IdempotencyKey: "k1",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want to wrap %v", err, wantErr)
	}
}
