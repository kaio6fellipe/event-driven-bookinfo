// file: services/details/internal/core/domain/detail_test.go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

func TestNewDetail_Valid(t *testing.T) {
	d, err := domain.NewDetail(
		"The Art of Go",
		"Jane Doe",
		2024,
		"paperback",
		350,
		"Go Press",
		"English",
		"1234567890",
		"1234567890123",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if d.ID == "" {
		t.Error("expected non-empty ID")
	}
	if d.Author != "Jane Doe" {
		t.Errorf("Author = %q, want %q", d.Author, "Jane Doe")
	}
	if d.Title != "The Art of Go" {
		t.Errorf("Title = %q, want %q", d.Title, "The Art of Go")
	}
	if d.Year != 2024 {
		t.Errorf("Year = %d, want %d", d.Year, 2024)
	}
	if d.Type != "paperback" {
		t.Errorf("Type = %q, want %q", d.Type, "paperback")
	}
	if d.Pages != 350 {
		t.Errorf("Pages = %d, want %d", d.Pages, 350)
	}
	if d.Publisher != "Go Press" {
		t.Errorf("Publisher = %q, want %q", d.Publisher, "Go Press")
	}
	if d.Language != "English" {
		t.Errorf("Language = %q, want %q", d.Language, "English")
	}
	if d.ISBN10 != "1234567890" {
		t.Errorf("ISBN10 = %q, want %q", d.ISBN10, "1234567890")
	}
	if d.ISBN13 != "1234567890123" {
		t.Errorf("ISBN13 = %q, want %q", d.ISBN13, "1234567890123")
	}
}

func TestNewDetail_EmptyTitle(t *testing.T) {
	_, err := domain.NewDetail("", "Jane Doe", 2024, "paperback", 350, "Go Press", "English", "1234567890", "1234567890123")
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestNewDetail_EmptyAuthor(t *testing.T) {
	_, err := domain.NewDetail("The Art of Go", "", 2024, "paperback", 350, "Go Press", "English", "1234567890", "1234567890123")
	if err == nil {
		t.Fatal("expected error for empty author")
	}
}

func TestNewDetail_InvalidYear(t *testing.T) {
	_, err := domain.NewDetail("The Art of Go", "Jane Doe", 0, "paperback", 350, "Go Press", "English", "1234567890", "1234567890123")
	if err == nil {
		t.Fatal("expected error for invalid year")
	}
}

func TestNewDetail_InvalidPages(t *testing.T) {
	_, err := domain.NewDetail("The Art of Go", "Jane Doe", 2024, "paperback", 0, "Go Press", "English", "1234567890", "1234567890123")
	if err == nil {
		t.Fatal("expected error for invalid pages")
	}
}
