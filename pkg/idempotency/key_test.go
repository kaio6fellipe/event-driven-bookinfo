package idempotency_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
)

func TestNaturalKey(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   string
	}{
		{
			name:   "single field",
			fields: []string{"hello"},
			want:   "", // length check only
		},
		{
			name:   "same fields produce same key",
			fields: []string{"a", "b", "c"},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idempotency.NaturalKey(tt.fields...)
			if len(got) != 64 {
				t.Errorf("key length = %d, want 64 (SHA-256 hex)", len(got))
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNaturalKey_Deterministic(t *testing.T) {
	k1 := idempotency.NaturalKey("a", "b", "c")
	k2 := idempotency.NaturalKey("a", "b", "c")
	if k1 != k2 {
		t.Errorf("expected deterministic output, got %q and %q", k1, k2)
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
			if tt.wantExplicit && got != tt.explicitKey {
				t.Errorf("got %q, want explicit key %q", got, tt.explicitKey)
			}
			if !tt.wantExplicit && got == tt.explicitKey {
				t.Error("expected natural key, got explicit (or empty)")
			}
		})
	}
}
