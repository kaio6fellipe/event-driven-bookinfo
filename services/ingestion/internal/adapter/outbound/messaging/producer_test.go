package messaging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/messaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
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

	tests := []struct {
		name       string
		book       domain.Book
		wantKey    string
		wantISBN10 string
		wantISBN13 string
		wantCEType string
		wantAuthor string
	}{
		{
			name: "valid book with ISBN-13",
			book: domain.Book{
				Title:       "The Go Programming Language",
				Authors:     []string{"Alan Donovan", "Brian Kernighan"},
				ISBN:        "9780134190440",
				PublishYear: 2015,
				Pages:       380,
				Publisher:   "Addison-Wesley",
				Language:    "en",
			},
			wantKey:    "9780134190440",
			wantISBN10: "",
			wantISBN13: "9780134190440",
			wantCEType: "com.bookinfo.ingestion.book-added",
			wantAuthor: "Alan Donovan, Brian Kernighan",
		},
		{
			name: "valid book with ISBN-10",
			book: domain.Book{
				Title:       "Learning Go",
				Authors:     []string{"Jon Bodner"},
				ISBN:        "1492077216",
				PublishYear: 2021,
				Pages:       375,
				Publisher:   "O'Reilly",
				Language:    "en",
			},
			wantKey:    "1492077216",
			wantISBN10: "1492077216",
			wantISBN13: "",
			wantCEType: "com.bookinfo.ingestion.book-added",
			wantAuthor: "Jon Bodner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fp := &fakePub{}
			prod := kafkaadapter.NewProducer(fp)

			if err := prod.PublishBookAdded(context.Background(), tt.book); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if fp.last.CEType != tt.wantCEType {
				t.Errorf("ce_type = %q, want %q", fp.last.CEType, tt.wantCEType)
			}
			if fp.key != tt.wantKey {
				t.Errorf("record key = %q, want %q", fp.key, tt.wantKey)
			}
			if fp.idem == "" {
				t.Error("idempotency key is empty")
			}

			be, ok := fp.body.(kafkaadapter.BookEvent)
			if !ok {
				t.Fatalf("payload type = %T, want BookEvent", fp.body)
			}
			if be.Title != tt.book.Title {
				t.Errorf("title = %q, want %q", be.Title, tt.book.Title)
			}
			if be.Author != tt.wantAuthor {
				t.Errorf("author = %q, want %q", be.Author, tt.wantAuthor)
			}
			if be.Year != tt.book.PublishYear {
				t.Errorf("year = %d, want %d", be.Year, tt.book.PublishYear)
			}
			if be.Type != "paperback" {
				t.Errorf("type = %q, want %q", be.Type, "paperback")
			}
			if be.ISBN10 != tt.wantISBN10 {
				t.Errorf("isbn_10 = %q, want %q", be.ISBN10, tt.wantISBN10)
			}
			if be.ISBN13 != tt.wantISBN13 {
				t.Errorf("isbn_13 = %q, want %q", be.ISBN13, tt.wantISBN13)
			}
			if be.IdempotencyKey == "" {
				t.Error("idempotency_key is empty")
			}
		})
	}
}

func TestPublishBookAdded_ProduceError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("backend unavailable")
	fp := &fakePub{err: wantErr}
	prod := kafkaadapter.NewProducer(fp)

	book := domain.Book{
		Title:       "Test Book",
		Authors:     []string{"Author"},
		ISBN:        "1234567890",
		PublishYear: 2024,
	}

	err := prod.PublishBookAdded(context.Background(), book)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want to wrap %v", err, wantErr)
	}
}
