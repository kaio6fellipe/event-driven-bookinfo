// Package api defines the declarative HTTP endpoint metadata used by every
// service to drive both runtime route registration and OpenAPI generation
// (via tools/specgen).
//
// Each service's inbound HTTP adapter exposes:
//   - const APIVersion string         // becomes openapi.yaml info.version
//   - var Endpoints []api.Endpoint    // the route + DTO catalog
//
// At runtime, the handler's RegisterRoutes loops over the same slice the
// generator reads, so any drift between the slice and the handlers fails at
// program start (Register panics on a missing handler).
package api //nolint:revive // small focused package; name matches directory convention

import (
	"fmt"
	"net/http"
)

// Endpoint describes one HTTP route plus the DTOs it consumes and produces.
//
// Request and Response hold zero-value instances of the DTO struct types so
// tools/specgen can resolve them via go/types and build JSONSchemas. Leave
// nil when the operation has no body in that direction.
type Endpoint struct {
	Method  string // "GET" | "POST" | "PUT" | "DELETE" | etc.
	Path    string // stdlib mux path with {param} placeholders
	Summary string // one-line OpenAPI summary

	// OperationID is the OpenAPI operationId. Leave empty to auto-generate
	// from method+path (e.g. "POST /v1/ratings" → "postV1Ratings"). When set,
	// the explicit value wins.
	OperationID string

	// Description is the multi-line OpenAPI operation description. Leave
	// empty to default to Summary. Set explicitly for richer text or when
	// Summary is too terse.
	Description string

	// Tags is the OpenAPI operation tags array. Leave empty/nil to default
	// to [serviceName]. Set explicitly to override (e.g. ["operations"] for
	// admin endpoints, or [serviceName, "v1"] for versioned APIs).
	Tags []string

	EventName string // optional: when set on a POST, generates cqrs.endpoints.<EventName>
	// SuccessStatus overrides the default success-response status code in
	// the generated OpenAPI spec. Zero (the default) means "use the
	// method default": 201 for POST, 200 for everything else. Set to a
	// specific code (200, 204, etc.) when the handler returns something
	// other than the method default.
	SuccessStatus int
	Request       any             // zero-value of request DTO; nil when body is absent
	Response      any             // zero-value of success-response DTO; nil for text/html or no-content
	Errors        []ErrorResponse // documented non-2xx responses
}

// ErrorResponse documents a non-2xx response with its DTO type.
type ErrorResponse struct {
	Status int // e.g. 400, 404, 500
	Type   any // zero-value of the error DTO
}

// Register wires every Endpoint into the mux, dispatching to the handler
// keyed by "METHOD PATH" in the handlers map. Panics if any endpoint has no
// matching handler — surfaces drift between the slice and the http package
// at program start, before any traffic is served.
func Register(mux *http.ServeMux, endpoints []Endpoint, handlers map[string]http.HandlerFunc) {
	for _, ep := range endpoints {
		key := ep.Method + " " + ep.Path
		h, ok := handlers[key]
		if !ok {
			panic(fmt.Sprintf("api.Register: no handler registered for %q", key))
		}
		mux.HandleFunc(key, h)
	}
}
