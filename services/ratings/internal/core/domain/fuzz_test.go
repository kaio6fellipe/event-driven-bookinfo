package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

func FuzzNewRating(f *testing.F) {
	f.Add("product-1", "reviewer-1", 5)
	f.Add("", "", 0)
	f.Add("p", "r", -1)
	f.Add("product-2", "reviewer-2", 100)

	f.Fuzz(func(t *testing.T, productID, reviewer string, stars int) {
		r, err := domain.NewRating(productID, reviewer, stars)
		if err != nil {
			return
		}
		if r.ID == "" {
			t.Error("valid rating must have non-empty ID")
		}
		if r.Stars < 1 || r.Stars > 5 {
			t.Errorf("Stars = %d, want 1-5", r.Stars)
		}
	})
}
