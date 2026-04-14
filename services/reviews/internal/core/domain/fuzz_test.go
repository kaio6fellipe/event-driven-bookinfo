package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

func FuzzNewReview(f *testing.F) {
	f.Add("product-1", "reviewer-1", "Great book!")
	f.Add("", "", "")
	f.Add("p", "r", "A really long review text that goes on and on and on")

	f.Fuzz(func(t *testing.T, productID, reviewer, text string) {
		r, err := domain.NewReview(productID, reviewer, text)
		if err != nil {
			return
		}
		if r.ID == "" {
			t.Error("valid review must have non-empty ID")
		}
		if r.ProductID != productID {
			t.Errorf("ProductID = %q, want %q", r.ProductID, productID)
		}
	})
}
