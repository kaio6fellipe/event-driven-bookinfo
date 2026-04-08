// file: services/reviews/internal/core/domain/review_test.go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

func TestNewReview_Valid(t *testing.T) {
	r, err := domain.NewReview("product-1", "alice", "Great book!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.ID == "" {
		t.Error("expected non-empty ID")
	}
	if r.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", r.ProductID, "product-1")
	}
	if r.Reviewer != "alice" {
		t.Errorf("Reviewer = %q, want %q", r.Reviewer, "alice")
	}
	if r.Text != "Great book!" {
		t.Errorf("Text = %q, want %q", r.Text, "Great book!")
	}
	if r.Rating != nil {
		t.Error("expected nil Rating on new review")
	}
}

func TestNewReview_EmptyProductID(t *testing.T) {
	_, err := domain.NewReview("", "alice", "Great book!")
	if err == nil {
		t.Fatal("expected error for empty product ID")
	}
}

func TestNewReview_EmptyReviewer(t *testing.T) {
	_, err := domain.NewReview("product-1", "", "Great book!")
	if err == nil {
		t.Fatal("expected error for empty reviewer")
	}
}

func TestNewReview_EmptyText(t *testing.T) {
	_, err := domain.NewReview("product-1", "alice", "")
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}
