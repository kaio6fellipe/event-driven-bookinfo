package messaging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/messaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
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

	fp := &fakePub{}
	p := kafkaadapter.NewProducer(fp)

	if err := p.PublishBookAdded(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fp.last.CEType != "com.bookinfo.details.book-added" {
		t.Errorf("ce_type = %q, want %q", fp.last.CEType, "com.bookinfo.details.book-added")
	}
	if fp.last.CESource != "details" {
		t.Errorf("ce_source = %q, want %q", fp.last.CESource, "details")
	}
	if fp.key != evt.IdempotencyKey {
		t.Errorf("record key = %q, want %q", fp.key, evt.IdempotencyKey)
	}
	if fp.idem != evt.IdempotencyKey {
		t.Errorf("idempotency key = %q, want %q", fp.idem, evt.IdempotencyKey)
	}

	body, ok := fp.body.(kafkaadapter.BookAddedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want BookAddedPayload", fp.body)
	}
	if body.Title != evt.Title {
		t.Errorf("title = %q, want %q", body.Title, evt.Title)
	}
	if body.Author != evt.Author {
		t.Errorf("author = %q, want %q", body.Author, evt.Author)
	}
	if body.Year != evt.Year {
		t.Errorf("year = %d, want %d", body.Year, evt.Year)
	}
	if body.ISBN13 != evt.ISBN13 {
		t.Errorf("isbn_13 = %q, want %q", body.ISBN13, evt.ISBN13)
	}
	if body.IdempotencyKey != evt.IdempotencyKey {
		t.Errorf("idempotency_key = %q, want %q", body.IdempotencyKey, evt.IdempotencyKey)
	}
}

func TestPublishBookAdded_ProduceError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("backend unavailable")
	fp := &fakePub{err: wantErr}
	p := kafkaadapter.NewProducer(fp)

	err := p.PublishBookAdded(context.Background(), domain.BookAddedEvent{
		Title: "t", Author: "a", Year: 2024, Type: "paperback",
		IdempotencyKey: "idem-x",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want to wrap %v", err, wantErr)
	}
}
