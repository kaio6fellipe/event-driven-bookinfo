package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/ingestion/api/openapi.yaml.
//
// ingestion has no CQRS endpoint — the trigger and status routes are
// direct admin calls; events are published to Kafka by the producer
// loop, not by an HTTP handler.
var Endpoints = []api.Endpoint{
	{
		Method:        "POST",
		Path:          "/v1/ingestion/trigger",
		Summary:       "Trigger a one-shot scrape (optionally overriding queries)",
		SuccessStatus: 200,
		Request:       TriggerRequest{},
		Response:      ScrapeResultResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 409, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/ingestion/status",
		Summary:  "Get current scraper state and last result",
		Response: StatusResponse{},
		Errors: []api.ErrorResponse{
			{Status: 500, Type: ErrorResponse{}},
		},
	},
}
