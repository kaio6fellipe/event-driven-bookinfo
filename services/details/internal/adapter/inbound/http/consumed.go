package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Consumed declares every CloudEvent type this service subscribes to via
// Argo Events Sensors. Surfaced as Backstage 'consumesApis' entries in
// services/details/api/catalog-info.yaml so cross-team integrations are
// discoverable.
var Consumed = []events.ConsumedDescriptor{
	{
		Name:            "raw-books-details",
		SourceService:   "ingestion",
		SourceEventName: "book-added",
		CEType:          "com.bookinfo.ingestion.book-added",
		Description:     "Picks up scraped books from ingestion to materialise as detail records.",
	},
}
