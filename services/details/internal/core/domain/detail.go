// file: services/details/internal/core/domain/detail.go
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// Detail represents book metadata.
type Detail struct {
	ID        string
	Title     string
	Author    string
	Year      int
	Type      string
	Pages     int
	Publisher string
	Language  string
	ISBN10    string
	ISBN13    string
}

// NewDetail creates a new Detail with validation.
func NewDetail(title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13 string) (*Detail, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if author == "" {
		return nil, fmt.Errorf("author is required")
	}
	if year <= 0 {
		return nil, fmt.Errorf("year must be positive, got %d", year)
	}
	if pages <= 0 {
		return nil, fmt.Errorf("pages must be positive, got %d", pages)
	}

	return &Detail{
		ID:        uuid.New().String(),
		Title:     title,
		Author:    author,
		Year:      year,
		Type:      bookType,
		Pages:     pages,
		Publisher: publisher,
		Language:  language,
		ISBN10:    isbn10,
		ISBN13:    isbn13,
	}, nil
}
