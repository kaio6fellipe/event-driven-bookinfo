// Package events defines the declarative event-publication metadata used by
// every producer service to drive both runtime CloudEvents construction and
// AsyncAPI generation (via tools/specgen).
//
// Each producer service's outbound kafka adapter exposes:
//
//	var Exposed []events.Descriptor
//
// The producer reads each Descriptor to build CE headers; tools/specgen
// reads the same slice to emit AsyncAPI channels and the events.exposed
// block in deploy/<svc>/values-generated.yaml.
package events

// Descriptor describes one event type a service publishes.
type Descriptor struct {
	// Name is the descriptor identifier (typically the dash-cased event name,
	// e.g. "book-added"). Used as the AsyncAPI message name and as the
	// default ExposureKey.
	Name string

	// ExposureKey is the grouping key for events.exposed.<key> in the chart.
	// Multiple descriptors with the same ExposureKey are emitted under one
	// Helm block whose eventTypes array collects all their CETypes. Defaults
	// to Name if empty.
	ExposureKey string

	// CEType is the CloudEvents `type` attribute, e.g.
	// "com.bookinfo.details.book-added".
	CEType string

	// CESource is the CloudEvents `source` attribute, e.g. "details".
	CESource string

	// Version is the CloudEvents `specversion` attribute payload version.
	Version string

	// ContentType is the message contentType, almost always
	// "application/json".
	ContentType string

	// Payload is a zero-value of the producer-side payload struct;
	// tools/specgen resolves it to a JSONSchema.
	Payload any

	// Description is a human-readable summary surfaced in AsyncAPI.
	Description string
}

// ResolveExposureKey returns ExposureKey when set, falling back to Name.
func (d Descriptor) ResolveExposureKey() string {
	if d.ExposureKey != "" {
		return d.ExposureKey
	}
	return d.Name
}
