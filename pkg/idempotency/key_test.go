package idempotency_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
)

func TestNaturalKey(t *testing.T) {
	// Compute expected value so the test doesn't hard-code a brittle digest.
	const wantLength = 64

	tests := []struct {
		name   string
		fields []string
	}{
		{name: "single field", fields: []string{"hello"}},
		{name: "three fields", fields: []string{"a", "b", "c"}},
		{name: "empty field list", fields: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idempotency.NaturalKey(tt.fields...)
			if len(got) != wantLength {
				t.Errorf("key length = %d, want %d (SHA-256 hex)", len(got), wantLength)
			}
			// Determinism: calling again with same inputs returns same output.
			again := idempotency.NaturalKey(tt.fields...)
			if got != again {
				t.Errorf("non-deterministic: %q != %q", got, again)
			}
		})
	}
}

func TestNaturalKey_OrderSensitive(t *testing.T) {
	k1 := idempotency.NaturalKey("a", "b")
	k2 := idempotency.NaturalKey("b", "a")
	if k1 == k2 {
		t.Error("expected different output for different field order")
	}
}

func TestNaturalKey_SeparatorCollision(t *testing.T) {
	// Without separator, ("ab", "c") would collide with ("a", "bc")
	k1 := idempotency.NaturalKey("ab", "c")
	k2 := idempotency.NaturalKey("a", "bc")
	if k1 == k2 {
		t.Error("expected separator to prevent field boundary collision")
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name         string
		explicitKey  string
		fields       []string
		wantExplicit bool
	}{
		{
			name:         "explicit key takes precedence",
			explicitKey:  "my-key",
			fields:       []string{"a", "b"},
			wantExplicit: true,
		},
		{
			name:         "empty explicit falls back to natural",
			explicitKey:  "",
			fields:       []string{"a", "b"},
			wantExplicit: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idempotency.Resolve(tt.explicitKey, tt.fields...)
			if tt.wantExplicit {
				if got != tt.explicitKey {
					t.Errorf("got %q, want explicit key %q", got, tt.explicitKey)
				}
				return
			}
			want := idempotency.NaturalKey(tt.fields...)
			if got != want {
				t.Errorf("got %q, want natural key %q", got, want)
			}
		})
	}
}
