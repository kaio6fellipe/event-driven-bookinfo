package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the semantic version of the details service API.
const APIVersion = "1.0.0"

// Endpoints is the declarative route catalog for the details service.
// It drives both runtime route registration (via api.Register) and
// OpenAPI spec generation (via tools/specgen).
var Endpoints = []api.Endpoint{
	{
		Method:   "GET",
		Path:     "/v1/details",
		Summary:  "List all book details",
		Response: []DetailResponse{},
		Errors: []api.ErrorResponse{
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/details/{id}",
		Summary:  "Get a single book detail by ID",
		Response: DetailResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
		},
	},
	{
		Method:    "POST",
		Path:      "/v1/details",
		Summary:   "Add a new book detail",
		EventName: "book-added",
		Request:   AddDetailRequest{},
		Response:  DetailResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
		},
	},
}
