package events_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
)

func TestFind(t *testing.T) {
	descriptors := []events.Descriptor{
		{Name: "rating-submitted"},
		{Name: "rating-deleted"},
	}

	tests := []struct {
		name   string
		search string
		want   string
		panics bool
	}{
		{name: "found first", search: "rating-submitted", want: "rating-submitted"},
		{name: "found last", search: "rating-deleted", want: "rating-deleted"},
		{name: "missing panics", search: "nope", panics: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tt.panics && r == nil {
					t.Fatal("expected panic, got none")
				}
				if !tt.panics && r != nil {
					t.Fatalf("unexpected panic: %v", r)
				}
			}()
			got := events.Find(descriptors, tt.search)
			if got.Name != tt.want {
				t.Errorf("got %q, want %q", got.Name, tt.want)
			}
		})
	}
}

func TestResolveExposureKey(t *testing.T) {
	tests := []struct {
		name       string
		descriptor events.Descriptor
		want       string
	}{
		{
			name:       "defaults to Name when ExposureKey is empty",
			descriptor: events.Descriptor{Name: "rating-submitted"},
			want:       "rating-submitted",
		},
		{
			name:       "uses explicit ExposureKey when set",
			descriptor: events.Descriptor{Name: "book-added", ExposureKey: "events"},
			want:       "events",
		},
		{
			name:       "returns empty when both are empty",
			descriptor: events.Descriptor{},
			want:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.descriptor.ResolveExposureKey(); got != tt.want {
				t.Errorf("ResolveExposureKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
