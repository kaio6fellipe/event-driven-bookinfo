package events_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
)

func TestResolveExposureKey_DefaultsToName(t *testing.T) {
	d := events.Descriptor{Name: "rating-submitted"}
	if got := d.ResolveExposureKey(); got != "rating-submitted" {
		t.Errorf("ResolveExposureKey() = %q, want %q", got, "rating-submitted")
	}
}

func TestResolveExposureKey_UsesExplicitWhenSet(t *testing.T) {
	d := events.Descriptor{Name: "book-added", ExposureKey: "events"}
	if got := d.ResolveExposureKey(); got != "events" {
		t.Errorf("ResolveExposureKey() = %q, want %q", got, "events")
	}
}
