package events_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
)

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
