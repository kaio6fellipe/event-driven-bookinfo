package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

func TestBook_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		book    domain.Book
		wantErr bool
	}{
		{
			name: "valid book",
			book: domain.Book{
				Title:       "The Go Programming Language",
				Authors:     []string{"Alan Donovan"},
				ISBN:        "9780134190440",
				PublishYear: 2015,
			},
		},
		{
			name: "empty title",
			book: domain.Book{
				Authors:     []string{"Author"},
				ISBN:        "1234567890",
				PublishYear: 2020,
			},
			wantErr: true,
		},
		{
			name: "empty ISBN",
			book: domain.Book{
				Title:       "Some Book",
				Authors:     []string{"Author"},
				PublishYear: 2020,
			},
			wantErr: true,
		},
		{
			name: "no authors",
			book: domain.Book{
				Title:       "Some Book",
				Authors:     []string{},
				ISBN:        "1234567890",
				PublishYear: 2020,
			},
			wantErr: true,
		},
		{
			name: "nil authors",
			book: domain.Book{
				Title:       "Some Book",
				ISBN:        "1234567890",
				PublishYear: 2020,
			},
			wantErr: true,
		},
		{
			name: "zero publish year",
			book: domain.Book{
				Title:       "Some Book",
				Authors:     []string{"Author"},
				ISBN:        "1234567890",
				PublishYear: 0,
			},
			wantErr: true,
		},
		{
			name: "negative publish year",
			book: domain.Book{
				Title:       "Some Book",
				Authors:     []string{"Author"},
				ISBN:        "1234567890",
				PublishYear: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.book.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
