package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/dlqueue/api/openapi.yaml.
//
// Only POST /v1/events carries an EventName — it is the destination of
// the dlq-event-received Sensor. The replay/resolve/reset/batch routes
// are direct admin operations not behind the CQRS split, so EventName is
// left empty for them.
var Endpoints = []api.Endpoint{
	{
		Method:    "POST",
		Path:      "/v1/events",
		Summary:   "Ingest a failed event from a Sensor's dlqTrigger",
		EventName: "dlq-event-received",
		Request:   IngestEventRequest{},
		Response:  DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/events",
		Summary:  "List DLQ events with filters and pagination",
		Response: ListEventsResponse{},
		Errors: []api.ErrorResponse{
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/events/{id}",
		Summary:  "Get a single DLQ event by ID",
		Response: DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:        "POST",
		Path:          "/v1/events/{id}/replay",
		Summary:       "Replay a failed event back to its original target",
		SuccessStatus: 200,
		Response:      DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
			{Status: 409, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:        "POST",
		Path:          "/v1/events/{id}/resolve",
		Summary:       "Mark an event as resolved (terminal state)",
		SuccessStatus: 200,
		Request:       ResolveRequest{},
		Response:      DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 404, Type: ErrorResponse{}},
			{Status: 409, Type: ErrorResponse{}},
		},
	},
	{
		Method:        "POST",
		Path:          "/v1/events/{id}/reset",
		Summary:       "Reset a poisoned event back to pending",
		SuccessStatus: 200,
		Response:      DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
			{Status: 409, Type: ErrorResponse{}},
		},
	},
	{
		Method:        "POST",
		Path:          "/v1/events/batch/replay",
		Summary:       "Replay all events matching a filter",
		SuccessStatus: 200,
		Request:       BatchReplayRequest{},
		Response:      BatchActionResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:        "POST",
		Path:          "/v1/events/batch/resolve",
		Summary:       "Resolve a batch of events by IDs",
		SuccessStatus: 200,
		Request:       BatchResolveRequest{},
		Response:      BatchActionResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
}
