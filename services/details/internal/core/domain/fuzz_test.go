package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

func FuzzNewDetail(f *testing.F) {
	f.Add("The Go Programming Language", "Alan Donovan", 2015, "hardcover", 380, "Addison-Wesley", "EN", "0134190440", "978-0134190440")
	f.Add("", "", 0, "", 0, "", "", "", "")
	f.Add("A", "B", -1, "pdf", -100, "P", "X", "1234567890", "1234567890123")

	f.Fuzz(func(t *testing.T, title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13 string) {
		d, err := domain.NewDetail(title, author, year, bookType, pages, publisher, language, isbn10, isbn13)
		if err != nil {
			return
		}
		if d.ID == "" {
			t.Error("valid detail must have non-empty ID")
		}
		if d.Title != title {
			t.Errorf("Title = %q, want %q", d.Title, title)
		}
	})
}
