# Specgen Enrichment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close issues [#60](https://github.com/kaio6fellipe/event-driven-bookinfo/issues/60) (per-op + spec-level metadata to clear ~46 spectral warnings) and [#61](https://github.com/kaio6fellipe/event-driven-bookinfo/issues/61) (walker integration test against real types) in one PR. Per-service code change footprint: zero — hybrid generator defaults populate every new field automatically.

**Architecture:** Two new optional fields on `pkg/api.Endpoint` (`OperationID`, `Description`, `Tags`) and one on `pkg/events.Descriptor` (`Tags`); walker extracts them; generators emit them with smart-default fallbacks (`operationId` from method+path, `description` from `Summary`, `tags` from service name). Spec-level metadata (license, contact, servers) is hardcoded in a new `tools/specgen/internal/runner/metadata.go` file. AsyncAPI bumps 3.0.0 → 3.1.0. Walker gets two integration tests against the real `ratings` service to catch field renames the synthetic fixture would miss.

**Tech Stack:** Go 1.25, `go/types`, `golang.org/x/tools/go/packages`, `gopkg.in/yaml.v3`. spectral 6.x. oasdiff. No new dependencies.

**Spec reference:** `docs/superpowers/specs/2026-04-26-specgen-enrichment-design.md`

**Repo:** `/Users/kaio.fellipe/Documents/git/others/go-http-server`
**Worktree:** `.worktrees/feat-api-spec-generation-foundation/`
**Branch:** `feat/specgen-enrichment` (already created off post-#63 main; HEAD is the spec doc commit `7600fc8`)

---

## File Structure

Two new files, ~10 modified, ~10 generated artifacts regenerated.

```text
pkg/api/endpoint.go                                       # +3 fields on Endpoint
pkg/events/descriptor.go                                  # +1 field on Descriptor
tools/specgen/internal/walker/walker.go                   # +3 fields on EndpointInfo
tools/specgen/internal/walker/endpoint.go                 # +3 cases in setEndpointField (and intLit/stringSliceLit helpers as needed)
tools/specgen/internal/walker/descriptor.go               # +1 case in parseDescriptorFields, +Tags on DescriptorInfo
tools/specgen/internal/runner/metadata.go                 # NEW — SpecMetadata constants
tools/specgen/internal/runner/runner.go                   # thread metadata into builder Inputs
tools/specgen/internal/openapi/openapi.go                 # smart defaults + spec-level metadata; new lowerCamelCase helper
tools/specgen/internal/asyncapi/asyncapi.go               # version bump 3.0.0→3.1.0 + smart defaults + spec-level metadata
tools/specgen/internal/walker/integration_test.go         # NEW — real-service walker tests (#61)

tools/specgen/testdata/fixture/api/endpoints.go           # mirror new Endpoint fields on the local type
tools/specgen/testdata/fixture/events/exposed.go          # mirror new Descriptor field on the local type
tools/specgen/internal/openapi/testdata/golden.yaml       # regenerated
tools/specgen/internal/asyncapi/testdata/golden.yaml      # regenerated
tools/specgen/internal/backstage/testdata/golden.yaml     # potentially regenerated (only if catalog-info changes; should not, but verify)

services/<svc>/api/openapi.yaml                           # regenerated × 6
services/<svc>/api/asyncapi.yaml                          # regenerated × 4
services/<svc>/api/catalog-info.yaml                      # unchanged (verify)
deploy/<svc>/values-generated.yaml                        # unchanged (verify)
```

Net: ~12 source files modified, 2 new, ~12 generated artifacts regenerated.

---

## Task 1: Add OperationID/Tags/Description fields to `pkg/api.Endpoint` and walker

**Files:**

- Modify: `pkg/api/endpoint.go`
- Modify: `tools/specgen/internal/walker/walker.go` (add fields to `EndpointInfo`)
- Modify: `tools/specgen/internal/walker/endpoint.go` (extract new fields in `setEndpointField`)
- Modify: `tools/specgen/testdata/fixture/api/endpoints.go` (mirror the new fields on local Endpoint; populate one fixture entry with explicit overrides)
- Modify: `tools/specgen/internal/walker/walker_test.go` (assert on new fields)

This task wires the type definitions and the walker extraction. It does NOT yet change any builder output — that comes in Tasks 4 and 5. Existing builder tests continue passing because they ignore the new fields.

- [ ] **Step 1: Read the current Endpoint definition and the fixture's local Endpoint**

```bash
sed -n '20,45p' pkg/api/endpoint.go
sed -n '1,40p' tools/specgen/testdata/fixture/api/endpoints.go
```

Confirm `Endpoint` currently has 9 fields (Method, Path, Summary, EventName, SuccessStatus, Request, Response, Errors). Confirm the fixture's local `Endpoint` mirrors them (the walker matches by AST field name, not type identity).

- [ ] **Step 2: Add the three new fields to `pkg/api.Endpoint`**

Edit `pkg/api/endpoint.go`. Replace the `Endpoint` struct definition with:

```go
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

	// Tags is the OpenAPI operationtags array. Leave empty/nil to default
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
```

The new fields are inserted after `Summary` and before `EventName` so logically-related fields cluster.

- [ ] **Step 3: Add the same three fields to walker's `EndpointInfo`**

In `tools/specgen/internal/walker/walker.go`, replace the `EndpointInfo` struct with:

```go
// EndpointInfo is the walker's view of one element in `var Endpoints []api.Endpoint`.
type EndpointInfo struct {
	Method          string
	Path            string
	Summary         string
	OperationID     string
	Description     string
	Tags            []string
	EventName       string
	SuccessStatus   int          // 0 means "use method default" (201 for POST, 200 otherwise)
	RequestType     *types.Named // nil when omitted
	ResponseType    *types.Named // nil when omitted; holds element type when ResponseIsSlice is true
	ResponseIsSlice bool         // true when the response is []NamedType (array of ResponseType)
	Errors          []ErrorInfo
}
```

- [ ] **Step 4: Extend `setEndpointField` to extract OperationID, Description, Tags**

In `tools/specgen/internal/walker/endpoint.go`, find the `setEndpointField` switch and add three cases. The existing function already uses `stringLit(kv.Value)` for string fields — reuse that for `OperationID` and `Description`. For `Tags []string`, add a new helper `stringSliceLit` that walks an `*ast.CompositeLit` of strings.

Add the helper at the bottom of the file (next to the other helpers like `intLit`, `stringLit`, `namedType`):

```go
// stringSliceLit unquotes a Go []string composite literal AST node.
func stringSliceLit(expr ast.Expr) ([]string, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("not a composite literal")
	}
	out := make([]string, 0, len(cl.Elts))
	for i, elt := range cl.Elts {
		s, err := stringLit(elt)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out = append(out, s)
	}
	return out, nil
}
```

In the same file, find the `setEndpointField` switch (the cases for `"Method"`, `"Path"`, etc.) and add three new case branches before the existing `"EventName"` case:

```go
case "OperationID":
	s, err := stringLit(kv.Value)
	if err != nil {
		return ep, fmt.Errorf("OperationID: %w", err)
	}
	ep.OperationID = s
case "Description":
	s, err := stringLit(kv.Value)
	if err != nil {
		return ep, fmt.Errorf("Description: %w", err)
	}
	ep.Description = s
case "Tags":
	tags, err := stringSliceLit(kv.Value)
	if err != nil {
		return ep, fmt.Errorf("Tags: %w", err)
	}
	ep.Tags = tags
```

- [ ] **Step 5: Mirror the new fields on the fixture's local `Endpoint`**

In `tools/specgen/testdata/fixture/api/endpoints.go`, replace the local `Endpoint` struct definition to mirror the real one (same 12 fields), then add OperationID/Description/Tags overrides to the second fixture entry (the POST):

```go
package api

const APIVersion = "0.1.0"

type Endpoint struct {
	Method        string
	Path          string
	Summary       string
	OperationID   string
	Description   string
	Tags          []string
	EventName     string
	SuccessStatus int
	Request       any
	Response      any
	Errors        []ErrorResponse
}

type ErrorResponse struct {
	Status int
	Type   any
}

var Endpoints = []Endpoint{
	{
		Method:   "GET",
		Path:     "/v1/things/{id}",
		Summary:  "Get a thing",
		Response: GetThingResponse{},
		Errors:   []ErrorResponse{{Status: 404, Type: ErrResp{}}},
	},
	{
		Method:        "POST",
		Path:          "/v1/things",
		Summary:       "Create a thing",
		OperationID:   "createThing",
		Description:   "Creates a new thing record and emits a thing-created event.",
		Tags:          []string{"things", "v1"},
		EventName:     "thing-created",
		SuccessStatus: 200,
		Request:       CreateThingRequest{},
		Response:      GetThingResponse{},
		Errors:        []ErrorResponse{{Status: 400, Type: ErrResp{}}},
	},
}
```

- [ ] **Step 6: Update the walker test to assert on the new fields**

In `tools/specgen/internal/walker/walker_test.go`, find `TestLoadEndpoints_Fixture` and add assertions for the new fields. The first endpoint (GET) leaves them empty; the second (POST) has explicit overrides:

```go
// First endpoint (GET): new fields are zero/nil.
get := endpoints[0]
if get.OperationID != "" {
	t.Errorf("endpoints[0].OperationID = %q, want empty", get.OperationID)
}
if get.Description != "" {
	t.Errorf("endpoints[0].Description = %q, want empty", get.Description)
}
if get.Tags != nil {
	t.Errorf("endpoints[0].Tags = %v, want nil", get.Tags)
}

// Second endpoint (POST): explicit overrides.
post := endpoints[1]
if post.OperationID != "createThing" {
	t.Errorf("endpoints[1].OperationID = %q, want createThing", post.OperationID)
}
if post.Description != "Creates a new thing record and emits a thing-created event." {
	t.Errorf("endpoints[1].Description = %q", post.Description)
}
wantTags := []string{"things", "v1"}
if len(post.Tags) != 2 || post.Tags[0] != wantTags[0] || post.Tags[1] != wantTags[1] {
	t.Errorf("endpoints[1].Tags = %v, want %v", post.Tags, wantTags)
}
```

(Add these assertions inside the existing `TestLoadEndpoints_Fixture` function — it already has assertions for Method/Path/Summary/EventName etc.; just append.)

- [ ] **Step 7: Run the walker tests**

```bash
go test ./tools/specgen/internal/walker/... -race -count=1 -v
```

Expected: all walker tests pass. The new assertions verify:
1. Empty values when the field is absent in the slice (default branch).
2. Explicit values when present.

- [ ] **Step 8: Run the full Go test suite**

```bash
go test ./... -race -count=1 -short
```

Expected: 321 tests pass. The other generators (openapi, asyncapi) ignore the new walker fields for now — they'll be wired up in Tasks 4 and 5.

- [ ] **Step 9: Commit**

```bash
git add pkg/api/endpoint.go \
        tools/specgen/internal/walker/walker.go \
        tools/specgen/internal/walker/endpoint.go \
        tools/specgen/testdata/fixture/api/endpoints.go \
        tools/specgen/internal/walker/walker_test.go
git commit -s -m "feat(pkg/api,specgen/walker): OperationID/Tags/Description on Endpoint

Adds three optional fields to api.Endpoint and threads them through the
walker. The fields are populated when services explicitly set them;
when zero/nil, the OpenAPI builder will fall back to smart defaults in
a follow-up commit. Walker fixture's local Endpoint mirrors the new
fields so the existing unit test continues to exercise extraction."
```

---

## Task 2: Add `Tags` field to `pkg/events.Descriptor` and walker

**Files:**

- Modify: `pkg/events/descriptor.go`
- Modify: `tools/specgen/internal/walker/descriptor.go` (add Tags field on `DescriptorInfo`, parse case)
- Modify: `tools/specgen/testdata/fixture/events/exposed.go` (mirror)
- Modify: `tools/specgen/internal/walker/walker_test.go` (assertion)

Mirror of Task 1 for the AsyncAPI side.

- [ ] **Step 1: Read the current Descriptor definition**

```bash
sed -n '13,55p' pkg/events/descriptor.go
sed -n '1,30p' tools/specgen/testdata/fixture/events/exposed.go
```

Confirm `Descriptor` has 9 fields (Name, ExposureKey, Topic, CEType, CESource, Version, ContentType, Payload, Description). Fixture mirrors them.

- [ ] **Step 2: Add `Tags []string` to `pkg/events.Descriptor`**

In `pkg/events/descriptor.go`, append the new field at the end of the struct (after `Description`):

```go
type Descriptor struct {
	Name        string
	ExposureKey string
	Topic       string
	CEType      string
	CESource    string
	Version     string
	ContentType string
	Payload     any
	Description string

	// Tags is the AsyncAPI message/operation tags array. Leave empty/nil
	// to default to [serviceName]. Set explicitly to override.
	Tags []string
}
```

(Keep the existing field-level comments on the other fields as-is; only add `Tags` and its comment.)

- [ ] **Step 3: Mirror `Tags` on walker's `DescriptorInfo`**

In `tools/specgen/internal/walker/descriptor.go`, find the `DescriptorInfo` struct and add the field at the end:

```go
type DescriptorInfo struct {
	Name        string
	ExposureKey string
	Topic       string
	CEType      string
	CESource    string
	Version     string
	ContentType string
	PayloadType *types.Named
	Description string
	Tags        []string
}
```

- [ ] **Step 4: Extend `parseDescriptorFields` to extract `Tags`**

In `tools/specgen/internal/walker/descriptor.go`, find the `parseDescriptorFields` switch (handles `"Name"`, `"ExposureKey"`, etc.) and add a new case before the closing `}` of the switch. Use the `stringSliceLit` helper from Task 1 (it lives in `endpoint.go` but is package-local):

```go
case "Tags":
	tags, err := stringSliceLit(kv.Value)
	if err != nil {
		return d, fmt.Errorf("Tags: %w", err)
	}
	d.Tags = tags
```

- [ ] **Step 5: Mirror the new field on the fixture's local `Descriptor`**

In `tools/specgen/testdata/fixture/events/exposed.go`, replace the local `Descriptor` struct + `Exposed` slice to add `Tags`:

```go
package events

type ThingCreatedPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Descriptor struct {
	Name        string
	ExposureKey string
	Topic       string
	CEType      string
	CESource    string
	Version     string
	ContentType string
	Payload     any
	Description string
	Tags        []string
}

var Exposed = []Descriptor{
	{
		Name:        "thing-created",
		ExposureKey: "events",
		Topic:       "fixture_things_events",
		CEType:      "com.fixture.thing-created",
		CESource:    "fixture",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     ThingCreatedPayload{},
		Description: "Emitted when a thing is created.",
		Tags:        []string{"things"},
	},
}
```

(Also update the `ConsumedDescriptor` declaration at the bottom of the file if present — leave it unchanged since Tags is new only on `Descriptor`. Confirm by reading the file.)

- [ ] **Step 6: Update the walker test for descriptor Tags**

In `tools/specgen/internal/walker/walker_test.go`, find `TestLoadExposed_Fixture` and add an assertion for Tags:

```go
// New: Tags is set on the fixture descriptor.
wantTags := []string{"things"}
if len(d.Tags) != 1 || d.Tags[0] != wantTags[0] {
	t.Errorf("descriptor.Tags = %v, want %v", d.Tags, wantTags)
}
```

(Append inside the existing `TestLoadExposed_Fixture` after the existing `if d.PayloadType ... ` block.)

- [ ] **Step 7: Run walker tests**

```bash
go test ./tools/specgen/internal/walker/... -race -count=1 -v
```

Expected: PASS for both `TestLoadEndpoints_Fixture` and `TestLoadExposed_Fixture` (plus any other walker tests).

- [ ] **Step 8: Commit**

```bash
git add pkg/events/descriptor.go \
        tools/specgen/internal/walker/descriptor.go \
        tools/specgen/testdata/fixture/events/exposed.go \
        tools/specgen/internal/walker/walker_test.go
git commit -s -m "feat(pkg/events,specgen/walker): Tags on events.Descriptor

Mirrors the Tags addition on pkg/api.Endpoint for the AsyncAPI side.
Walker extracts the field; fixture exercises both populated and absent
cases through extraction. AsyncAPI builder will use the value (or fall
back to [serviceName]) in a follow-up commit."
```

---

## Task 3: Create `runner.SpecMetadata` constants

**Files:**

- Create: `tools/specgen/internal/runner/metadata.go`

Hardcode the org-wide metadata in a single repo-internal file. The runner threads it into both builders.

- [ ] **Step 1: Create the file**

Create `tools/specgen/internal/runner/metadata.go`:

```go
package runner

// SpecMetadata is the org-wide metadata shared by every generated spec.
// Repo-internal: services do not override these. Adjust here when the
// org/license/server URL changes.
type SpecMetadata struct {
	OrgName        string
	OrgURL         string
	LicenseName    string
	LicenseURL     string
	OpenAPIServer  ServerEntry
	AsyncAPIServer ServerEntry
}

// ServerEntry models one OpenAPI/AsyncAPI servers entry.
type ServerEntry struct {
	URL         string
	Description string
}

// Metadata is the constant value threaded into every Build call.
var Metadata = SpecMetadata{
	OrgName:     "bookinfo-team",
	OrgURL:      "https://github.com/kaio6fellipe/event-driven-bookinfo",
	LicenseName: "Apache-2.0",
	LicenseURL:  "https://www.apache.org/licenses/LICENSE-2.0",
	OpenAPIServer: ServerEntry{
		URL:         "http://localhost:8080",
		Description: "Local k3d gateway",
	},
	AsyncAPIServer: ServerEntry{
		URL:         "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092",
		Description: "Local Kafka bootstrap",
	},
}
```

- [ ] **Step 2: Compile**

```bash
go build ./tools/specgen/...
```

Expected: clean build. Nothing imports `runner.Metadata` yet — Tasks 4 and 5 wire it in.

- [ ] **Step 3: Commit**

```bash
git add tools/specgen/internal/runner/metadata.go
git commit -s -m "feat(specgen/runner): SpecMetadata for org-wide spec doc info

Adds repo-internal constants for OpenAPI/AsyncAPI info.contact,
info.license, and servers. Pure data; no behavior. The builders will
read this in follow-up commits to populate spec-level metadata that
spectral currently flags as missing."
```

---

## Task 4: OpenAPI builder — smart defaults + spec-level metadata

**Files:**

- Modify: `tools/specgen/internal/openapi/openapi.go`
- Modify: `tools/specgen/internal/runner/runner.go` (pass Metadata into openapi.Build)
- Modify: `tools/specgen/internal/openapi/testdata/golden.yaml` (regenerate via UPDATE=1)

Add `Metadata SpecMetadata` to `openapi.Input`; teach `buildOperation` to fill in operationId/description/tags with smart defaults; teach `Build` to emit `info.contact`/`info.license`/`info.description` and `servers`.

- [ ] **Step 1: Add a `lowerCamelCase` helper with table-driven tests**

Add to `tools/specgen/internal/openapi/openapi.go` (near the bottom, alongside `extractPathParams`):

```go
// lowerCamelCase derives a stable OpenAPI operationId from an HTTP method
// and path template, e.g. ("POST", "/v1/ratings") → "postV1Ratings",
// ("GET", "/v1/things/{id}") → "getV1ThingsId".
//
// Algorithm: lowercase the method, then concat each non-empty path segment
// (with {} stripped) title-cased. The result is unique per (method, path)
// because the path is unique per route.
func lowerCamelCase(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, segment := range strings.Split(path, "/") {
		if segment == "" {
			continue
		}
		// Strip leading "{" and trailing "}" if present.
		segment = strings.TrimPrefix(segment, "{")
		segment = strings.TrimSuffix(segment, "}")
		if segment == "" {
			continue
		}
		// Title-case the segment.
		b.WriteString(strings.ToUpper(segment[:1]))
		if len(segment) > 1 {
			b.WriteString(segment[1:])
		}
	}
	return b.String()
}
```

(`strings` is already imported in `openapi.go`.)

Add a test in a new file `tools/specgen/internal/openapi/openapi_lowercamel_test.go`:

```go
package openapi

import "testing"

func TestLowerCamelCase(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "POST collection", method: "POST", path: "/v1/ratings", want: "postV1Ratings"},
		{name: "GET resource", method: "GET", path: "/v1/ratings/{id}", want: "getV1RatingsId"},
		{name: "DELETE no path", method: "DELETE", path: "/", want: "delete"},
		{name: "complex admin route", method: "POST", path: "/v1/events/{id}/replay", want: "postV1EventsIdReplay"},
		{name: "batch route", method: "POST", path: "/v1/events/batch/replay", want: "postV1EventsBatchReplay"},
		{name: "multiple path params", method: "GET", path: "/v1/users/{userId}/posts/{postId}", want: "getV1UsersUserIdPostsPostId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lowerCamelCase(tt.method, tt.path)
			if got != tt.want {
				t.Errorf("lowerCamelCase(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the new helper test**

```bash
go test ./tools/specgen/internal/openapi/... -race -count=1 -run TestLowerCamelCase -v
```

Expected: 6 PASS. (The test must be in package `openapi` not `openapi_test` because `lowerCamelCase` is unexported.)

- [ ] **Step 3: Add `Metadata` to `openapi.Input`**

Edit `tools/specgen/internal/openapi/openapi.go`. Replace the `Input` struct:

```go
// Input holds everything Build needs.
type Input struct {
	ServiceName string
	Version     string
	Endpoints   []walker.EndpointInfo
	Metadata    SpecMetadata // org-wide info.contact/license/servers; injected by runner
}

// SpecMetadata is the subset of runner.SpecMetadata the openapi builder needs.
// Defined here (not imported from runner) to avoid an import cycle and keep
// openapi consumable as a pure library.
type SpecMetadata struct {
	OrgName       string
	OrgURL        string
	LicenseName   string
	LicenseURL    string
	OpenAPIServer ServerEntry
}

// ServerEntry mirrors runner.ServerEntry.
type ServerEntry struct {
	URL         string
	Description string
}
```

This mirrors `runner.SpecMetadata` to avoid a circular import (`openapi` is consumed by `runner`).

- [ ] **Step 4: Smart defaults inside `buildOperation`**

In the same file, modify `buildOperation` to populate `operationId`, `description`, `tags` with smart defaults. Add the `serviceName` parameter (needed for the Tags fallback) — this requires updating the `Build` call site too:

Replace the `buildOperation` signature and body to add the new metadata:

```go
// buildOperation constructs the operation object for one endpoint and
// accumulates any referenced schemas into the provided map.
func buildOperation(ep walker.EndpointInfo, serviceName string, schemas map[string]*jsonschema.Schema) (map[string]any, error) {
	// Smart defaults: explicit values win, fall back to derived values.
	operationID := ep.OperationID
	if operationID == "" {
		operationID = lowerCamelCase(ep.Method, ep.Path)
	}
	description := ep.Description
	if description == "" {
		description = ep.Summary
	}
	tags := ep.Tags
	if len(tags) == 0 {
		tags = []string{serviceName}
	}

	op := map[string]any{
		"operationId": operationID,
		"summary":     ep.Summary,
		"description": description,
		"tags":        tags,
		"responses":   map[string]any{},
	}
	if ep.EventName != "" {
		op["x-bookinfo-event-name"] = ep.EventName
	}

	// (rest of the function — requestBody, responses, errors — unchanged from current)
	if ep.RequestType != nil {
		// ... existing code ...
	}
	// ... etc
```

Update the `Build` call site (later in the same file) — the loop over endpoints now passes `in.ServiceName`:

```go
for _, ep := range in.Endpoints {
	op, err := buildOperation(ep, in.ServiceName, schemas)
	if err != nil {
		return nil, err
	}
	// ... existing code ...
}
```

- [ ] **Step 5: Spec-level metadata in `Build`**

In `tools/specgen/internal/openapi/openapi.go::Build`, find where `infoNode` is constructed (currently has `title` and `version`). Replace that block with:

```go
infoNode := yamlutil.Mapping()
yamlutil.AddScalar(infoNode, "title", in.ServiceName)
yamlutil.AddScalar(infoNode, "version", in.Version)
yamlutil.AddScalar(infoNode, "description", in.ServiceName+" — generated by tools/specgen")

contactNode := yamlutil.Mapping()
yamlutil.AddScalar(contactNode, "name", in.Metadata.OrgName)
yamlutil.AddScalar(contactNode, "url", in.Metadata.OrgURL)
yamlutil.AddMapping(infoNode, "contact", contactNode)

licenseNode := yamlutil.Mapping()
yamlutil.AddScalar(licenseNode, "name", in.Metadata.LicenseName)
yamlutil.AddScalar(licenseNode, "url", in.Metadata.LicenseURL)
yamlutil.AddMapping(infoNode, "license", licenseNode)

yamlutil.AddMapping(docNode, "info", infoNode)

// servers
serversNode, err := yamlutil.AnyToNode([]any{
	map[string]any{
		"url":         in.Metadata.OpenAPIServer.URL,
		"description": in.Metadata.OpenAPIServer.Description,
	},
})
if err != nil {
	return nil, fmt.Errorf("encoding servers: %w", err)
}
yamlutil.AddMapping(docNode, "servers", serversNode)
```

The `servers` block must come AFTER `info` and BEFORE `paths` per OpenAPI 3.1 conventions. Verify by re-reading the function.

- [ ] **Step 6: Wire the metadata in the runner**

In `tools/specgen/internal/runner/runner.go`, find the `generateOne` function where `openapi.Build` is called. The current call is:

```go
yamlBytes, err := openapi.Build(openapi.Input{
	ServiceName: svc.Name,
	Version:     version,
	Endpoints:   endpoints,
})
```

Replace it with:

```go
yamlBytes, err := openapi.Build(openapi.Input{
	ServiceName: svc.Name,
	Version:     version,
	Endpoints:   endpoints,
	Metadata: openapi.SpecMetadata{
		OrgName:     Metadata.OrgName,
		OrgURL:      Metadata.OrgURL,
		LicenseName: Metadata.LicenseName,
		LicenseURL:  Metadata.LicenseURL,
		OpenAPIServer: openapi.ServerEntry{
			URL:         Metadata.OpenAPIServer.URL,
			Description: Metadata.OpenAPIServer.Description,
		},
	},
})
```

- [ ] **Step 7: Regenerate the OpenAPI golden file**

```bash
UPDATE=1 go test ./tools/specgen/internal/openapi/... -race -count=1 -run TestBuild_FixtureMatchesGolden
```

Expected: golden file regenerated. Inspect:

```bash
cat tools/specgen/internal/openapi/testdata/golden.yaml | head -40
```

Expected content (top of file):

```yaml
# DO NOT EDIT — generated by tools/specgen.
# Source: services/fixture/internal/adapter/inbound/http/endpoints.go
openapi: 3.1.0
info:
  title: fixture
  version: 0.1.0
  description: fixture — generated by tools/specgen
  contact:
    name: bookinfo-team
    url: https://github.com/kaio6fellipe/event-driven-bookinfo
  license:
    name: Apache-2.0
    url: https://www.apache.org/licenses/LICENSE-2.0
servers:
  - url: http://localhost:8080
    description: Local k3d gateway
paths:
  /v1/things:
    post:
      operationId: createThing
      summary: Create a thing
      description: Creates a new thing record and emits a thing-created event.
      tags:
        - things
        - v1
      x-bookinfo-event-name: thing-created
      ...
```

The fixture's POST endpoint had explicit overrides (Task 1 Step 5), so its operationId is `createThing`, description and tags are explicit. The GET endpoint has none — so its values come from the smart defaults: `operationId: getV1ThingsId`, `description: Get a thing`, `tags: [fixture]`.

- [ ] **Step 8: Run all openapi tests for determinism**

```bash
go test ./tools/specgen/internal/openapi/... -race -count=3
```

Expected: 3 consecutive PASS runs (determinism check).

- [ ] **Step 9: Run full Go test suite**

```bash
go test ./... -race -count=1 -short
```

Expected: 321+ tests pass.

- [ ] **Step 10: Commit**

```bash
git add tools/specgen/internal/openapi/ \
        tools/specgen/internal/runner/runner.go
git commit -s -m "feat(specgen/openapi): smart defaults + spec-level metadata

- Adds operationId/description/tags to every operation. Defaults:
    operationId = lowerCamelCase(method, path)
    description = Summary
    tags        = [serviceName]
  Explicit values on Endpoint override the defaults.
- Adds info.contact, info.license, info.description, and servers from
  the runner-level Metadata constants.
- Bumps the openapi golden fixture to reflect the new fields.

Closes the OpenAPI portion of #60: 33 per-op + 6 spec-level warnings
(operation-tags, operation-operationId, operation-description,
oas3-api-servers, info-description, info-contact)."
```

---

## Task 5: AsyncAPI builder — version bump + smart defaults + spec-level metadata

**Files:**

- Modify: `tools/specgen/internal/asyncapi/asyncapi.go`
- Modify: `tools/specgen/internal/runner/runner.go` (pass Metadata into asyncapi.Build)
- Modify: `tools/specgen/internal/asyncapi/testdata/golden.yaml` (regenerate)

Mirror of Task 4 for AsyncAPI. Bump version to 3.1.0; add per-operation/per-message tags + description with smart defaults; add info.* + servers at the spec level.

- [ ] **Step 1: Read the current AsyncAPI builder**

```bash
sed -n '1,90p' tools/specgen/internal/asyncapi/asyncapi.go
```

Confirm `Input` struct has 3 fields (ServiceName, Version, Exposed) and `Build` emits `asyncapi: 3.0.0` near the top.

- [ ] **Step 2: Add `Metadata` and the helper types to `asyncapi.Input`**

Edit `tools/specgen/internal/asyncapi/asyncapi.go`. Replace the `Input` struct and add the helper types at the same level:

```go
// Input holds everything Build needs to produce an AsyncAPI 3.1 document.
type Input struct {
	ServiceName string
	Version     string
	Exposed     []walker.DescriptorInfo
	Metadata    SpecMetadata // org-wide info.* + servers; injected by runner
}

// SpecMetadata is the subset of runner.SpecMetadata the asyncapi builder needs.
type SpecMetadata struct {
	OrgName        string
	OrgURL         string
	LicenseName    string
	LicenseURL     string
	AsyncAPIServer ServerEntry
}

// ServerEntry mirrors runner.ServerEntry.
type ServerEntry struct {
	URL         string
	Description string
}
```

- [ ] **Step 3: Bump the version literal and add spec-level metadata**

Find where the AsyncAPI version scalar is added (currently `yamlutil.AddScalar(docNode, "asyncapi", "3.0.0")`). Replace it with `"3.1.0"`.

After the existing `info` block (which currently has just `title` + `version`), expand to include description/contact/license, then add a `servers` mapping. Pseudo-position: same place as the OpenAPI Step 5 changes:

```go
yamlutil.AddScalar(docNode, "asyncapi", "3.1.0")

infoNode := yamlutil.Mapping()
yamlutil.AddScalar(infoNode, "title", in.ServiceName)
yamlutil.AddScalar(infoNode, "version", in.Version)
yamlutil.AddScalar(infoNode, "description", in.ServiceName+" — generated by tools/specgen")

contactNode := yamlutil.Mapping()
yamlutil.AddScalar(contactNode, "name", in.Metadata.OrgName)
yamlutil.AddScalar(contactNode, "url", in.Metadata.OrgURL)
yamlutil.AddMapping(infoNode, "contact", contactNode)

licenseNode := yamlutil.Mapping()
yamlutil.AddScalar(licenseNode, "name", in.Metadata.LicenseName)
yamlutil.AddScalar(licenseNode, "url", in.Metadata.LicenseURL)
yamlutil.AddMapping(infoNode, "license", licenseNode)

yamlutil.AddMapping(docNode, "info", infoNode)

// servers — AsyncAPI 3.x servers is a mapping (not a list like OpenAPI).
serversNode := yamlutil.Mapping()
kafkaServerNode := yamlutil.Mapping()
yamlutil.AddScalar(kafkaServerNode, "host", in.Metadata.AsyncAPIServer.URL)
yamlutil.AddScalar(kafkaServerNode, "protocol", "kafka")
yamlutil.AddScalar(kafkaServerNode, "description", in.Metadata.AsyncAPIServer.Description)
yamlutil.AddMapping(serversNode, "kafka", kafkaServerNode)
yamlutil.AddMapping(docNode, "servers", serversNode)
```

The `servers` block must come AFTER `info` per AsyncAPI 3.x conventions. Verify by re-reading the function.

- [ ] **Step 4: Smart defaults on per-message and per-operation metadata**

Find where messages and operations are built. For each MESSAGE in the builder, add `description` and `tags` fields:

```go
description := d.Description
// description already on Descriptor; AsyncAPI message has both summary and description
// per the spec; we treat description as a longer-form duplicate of summary when
// only one is present.
tags := d.Tags
if len(tags) == 0 {
	tags = []string{in.ServiceName}
}

// In the message yaml node construction:
yamlutil.AddScalar(messageNode, "name", d.Name)
yamlutil.AddScalar(messageNode, "title", d.Name)
yamlutil.AddScalar(messageNode, "summary", d.Description)
yamlutil.AddScalar(messageNode, "description", description)

// tags is an array of {name: ...} objects in AsyncAPI 3.x
tagsAny := make([]any, 0, len(tags))
for _, tag := range tags {
	tagsAny = append(tagsAny, map[string]any{"name": tag})
}
tagsNode, err := yamlutil.AnyToNode(tagsAny)
if err != nil {
	return nil, fmt.Errorf("encoding message tags: %w", err)
}
yamlutil.AddMapping(messageNode, "tags", tagsNode)
```

For each OPERATION (one per ExposureKey group), add `description` and `tags`:

```go
// Within the operation construction loop (operations.send_<exposureKey>):
opNode := yamlutil.Mapping()
yamlutil.AddScalar(opNode, "action", "send")
yamlutil.AddScalar(opNode, "description", "Publish events from the "+in.ServiceName+" service")

// Re-use any descriptor in the group's tags (or default to [serviceName]).
opTags := []string{in.ServiceName}
if len(group) > 0 && len(group[0].Tags) > 0 {
	opTags = group[0].Tags
}
opTagsAny := make([]any, 0, len(opTags))
for _, tag := range opTags {
	opTagsAny = append(opTagsAny, map[string]any{"name": tag})
}
opTagsNode, err := yamlutil.AnyToNode(opTagsAny)
if err != nil {
	return nil, fmt.Errorf("encoding operation tags: %w", err)
}
yamlutil.AddMapping(opNode, "tags", opTagsNode)

channelRefNode := yamlutil.Mapping()
yamlutil.AddScalar(channelRefNode, "$ref", "#/channels/"+exposureKey)
yamlutil.AddMapping(opNode, "channel", channelRefNode)
```

The exact variable names depend on the existing builder structure. **Read the file first** and adapt; the principles are: (a) emit `description` on each message and operation; (b) emit `tags` arrays; (c) default to `[serviceName]` when explicit tags not set.

- [ ] **Step 5: Wire the metadata in the runner**

In `tools/specgen/internal/runner/runner.go`, find the `generateOne` function where `asyncapi.Build` is called. Replace:

```go
yamlBytes, err := asyncapi.Build(asyncapi.Input{
	ServiceName: svc.Name,
	Version:     version,
	Exposed:     exposed,
})
```

with:

```go
yamlBytes, err := asyncapi.Build(asyncapi.Input{
	ServiceName: svc.Name,
	Version:     version,
	Exposed:     exposed,
	Metadata: asyncapi.SpecMetadata{
		OrgName:     Metadata.OrgName,
		OrgURL:      Metadata.OrgURL,
		LicenseName: Metadata.LicenseName,
		LicenseURL:  Metadata.LicenseURL,
		AsyncAPIServer: asyncapi.ServerEntry{
			URL:         Metadata.AsyncAPIServer.URL,
			Description: Metadata.AsyncAPIServer.Description,
		},
	},
})
```

- [ ] **Step 6: Regenerate AsyncAPI golden file**

```bash
UPDATE=1 go test ./tools/specgen/internal/asyncapi/... -race -count=1 -run TestBuild_FixtureMatchesGolden
```

Inspect:

```bash
cat tools/specgen/internal/asyncapi/testdata/golden.yaml | head -50
```

Expected (key bits):

```yaml
asyncapi: 3.1.0
info:
  title: fixture
  version: 0.1.0
  description: fixture — generated by tools/specgen
  contact:
    name: bookinfo-team
    url: https://github.com/kaio6fellipe/event-driven-bookinfo
  license:
    name: Apache-2.0
    url: https://www.apache.org/licenses/LICENSE-2.0
servers:
  kafka:
    host: bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092
    protocol: kafka
    description: Local Kafka bootstrap
channels:
  events:
    address: fixture_things_events
    messages:
      thing-created:
        $ref: '#/components/messages/thing-created'
operations:
  send_events:
    action: send
    description: Publish events from the fixture service
    tags:
      - name: things
    channel:
      $ref: '#/channels/events'
components:
  messages:
    thing-created:
      name: thing-created
      title: thing-created
      summary: Emitted when a thing is created.
      description: Emitted when a thing is created.
      tags:
        - name: things
      ...
```

The fixture's descriptor has explicit `Tags: []string{"things"}` so `things` appears in both message tags and operation tags.

- [ ] **Step 7: Run determinism check**

```bash
go test ./tools/specgen/internal/asyncapi/... -race -count=3
```

Expected: 3 consecutive PASS.

- [ ] **Step 8: Full test suite**

```bash
go test ./... -race -count=1 -short
```

Expected: 321+ tests pass.

- [ ] **Step 9: Commit**

```bash
git add tools/specgen/internal/asyncapi/ \
        tools/specgen/internal/runner/runner.go
git commit -s -m "feat(specgen/asyncapi): bump 3.1.0 + smart defaults + spec metadata

- Bumps asyncapi version 3.0.0 → 3.1.0 (fully backward-compatible).
- Adds info.contact, info.license, info.description, and servers from
  the runner-level Metadata.
- Adds per-message and per-operation description/tags. Default tags
  is [serviceName]; explicit Descriptor.Tags override.

Closes the AsyncAPI portion of #60: 6 top-level + 1 per-op tags + 1
per-op description warnings (asyncapi-info-* / asyncapi-servers /
asyncapi-3-tags / asyncapi-3-operation-description /
asyncapi-latest-version)."
```

---

## Task 6: Walker integration test against real `ratings` service (#61)

**Files:**

- Create: `tools/specgen/internal/walker/integration_test.go`

The synthetic fixture under `tools/specgen/testdata/fixture/` redeclares types locally — it would not catch a field rename in `pkg/api.Endpoint`. Adding a test that runs `LoadEndpoints`/`LoadExposed` against the real `services/ratings/...` packages closes that gap. (Per the design's #61 resolution, option C: keep synthetic fixture for fast unit tests; add integration test for type-rename guard.)

- [ ] **Step 1: Create the integration test file**

Create `tools/specgen/internal/walker/integration_test.go`:

```go
package walker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

const realModulePath = "github.com/kaio6fellipe/event-driven-bookinfo"

// realRepoRoot returns the absolute path of the parent module's root,
// or fails the test if it can't be located. Walker integration tests
// load real services from this root.
func realRepoRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p, "go.mod")); err != nil {
		t.Fatalf("not in expected repo layout (no go.mod at %s): %v", p, err)
	}
	return p
}

// TestLoadEndpoints_RealRatingsService catches field renames in
// pkg/api.Endpoint that the synthetic fixture would silently miss.
// The walker matches by AST field name; if a future PR renames
// EventName → CommandName in pkg/api.Endpoint, the real service's
// endpoints.go won't compile (fixed by the same PR), but the walker's
// extraction would be silently broken if the fixture mirror missed
// the rename. This test loads the real service and asserts on
// known-good extracted fields.
func TestLoadEndpoints_RealRatingsService(t *testing.T) {
	repoRoot := realRepoRoot(t)
	if _, err := os.Stat(filepath.Join(repoRoot, "services", "ratings", "internal", "adapter", "inbound", "http", "endpoints.go")); err != nil {
		t.Skip("services/ratings/internal/adapter/inbound/http/endpoints.go not present")
	}

	endpoints, version, err := walker.LoadEndpoints(repoRoot,
		realModulePath+"/services/ratings/internal/adapter/inbound/http")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	if version == "" {
		t.Error("expected APIVersion to be a non-empty string")
	}
	if len(endpoints) < 2 {
		t.Fatalf("expected ≥2 endpoints, got %d", len(endpoints))
	}

	// Find the POST /v1/ratings endpoint and assert on its known fields.
	var post walker.EndpointInfo
	for _, ep := range endpoints {
		if ep.Method == "POST" && ep.Path == "/v1/ratings" {
			post = ep
			break
		}
	}
	if post.Method != "POST" {
		t.Fatal("POST /v1/ratings not found in the endpoints slice")
	}
	if post.EventName != "rating-submitted" {
		t.Errorf("EventName = %q, want %q", post.EventName, "rating-submitted")
	}
	if post.RequestType == nil {
		t.Error("RequestType expected non-nil for POST /v1/ratings")
	}
	if post.ResponseType == nil {
		t.Error("ResponseType expected non-nil for POST /v1/ratings")
	}
	if post.Summary == "" {
		t.Error("Summary expected non-empty")
	}

	// Find the GET /v1/ratings/{id} endpoint.
	var get walker.EndpointInfo
	for _, ep := range endpoints {
		if ep.Method == "GET" && ep.Path == "/v1/ratings/{id}" {
			get = ep
			break
		}
	}
	if get.Method != "GET" {
		t.Fatal("GET /v1/ratings/{id} not found in the endpoints slice")
	}
	if get.RequestType != nil {
		t.Errorf("RequestType expected nil for GET, got %v", get.RequestType)
	}
	if get.ResponseType == nil {
		t.Error("ResponseType expected non-nil for GET /v1/ratings/{id}")
	}
}

// TestLoadExposed_RealRatingsService catches field renames in
// pkg/events.Descriptor for the same reasons as the endpoints variant.
func TestLoadExposed_RealRatingsService(t *testing.T) {
	repoRoot := realRepoRoot(t)
	if _, err := os.Stat(filepath.Join(repoRoot, "services", "ratings", "internal", "adapter", "outbound", "kafka", "exposed.go")); err != nil {
		t.Skip("services/ratings/internal/adapter/outbound/kafka/exposed.go not present")
	}

	exposed, err := walker.LoadExposed(repoRoot,
		realModulePath+"/services/ratings/internal/adapter/outbound/kafka")
	if err != nil {
		t.Fatalf("LoadExposed: %v", err)
	}
	if len(exposed) < 1 {
		t.Fatal("expected ≥1 exposed descriptor")
	}

	d := exposed[0]
	if d.Name != "rating-submitted" {
		t.Errorf("Name = %q, want %q", d.Name, "rating-submitted")
	}
	if d.CEType != "com.bookinfo.ratings.rating-submitted" {
		t.Errorf("CEType = %q, want %q", d.CEType, "com.bookinfo.ratings.rating-submitted")
	}
	if d.CESource != "ratings" {
		t.Errorf("CESource = %q, want %q", d.CESource, "ratings")
	}
	if d.PayloadType == nil {
		t.Error("PayloadType expected non-nil")
	}
}
```

- [ ] **Step 2: Run the integration tests**

```bash
go test ./tools/specgen/internal/walker/... -race -count=1 -run "RealRatingsService" -v
```

Expected: 2 PASS (or 2 SKIP if `services/ratings` is somehow absent — unlikely in this repo).

- [ ] **Step 3: Commit**

```bash
git add tools/specgen/internal/walker/integration_test.go
git commit -s -m "test(specgen/walker): real-service tests against ratings (#61)

Closes #61. The synthetic fixture under testdata/ redeclares Endpoint
and Descriptor types locally so the walker's AST field matching works
coincidentally; a field rename in pkg/api.Endpoint or
pkg/events.Descriptor would silently break extraction without the
fixture catching it.

Adds two integration tests that load the real ratings service via
walker.LoadEndpoints and walker.LoadExposed, asserting on known-good
field values (EventName, CEType, etc.). Future field renames break
the integration test until the walker is updated to track the new
field name."
```

---

## Task 7: Regenerate all service specs and verify

**Files:** modifies the 6 services' generated artifacts.

- [ ] **Step 1: Build the binary and regenerate**

```bash
make generate-specs 2>&1 | tail -10
```

Expected: 6 services report OK (`specgen: ratings OK`, `specgen: details OK`, etc.).

- [ ] **Step 2: Inspect one regenerated spec**

```bash
cat services/ratings/api/openapi.yaml | head -30
```

Expected (top of file):

```yaml
# DO NOT EDIT — generated by tools/specgen.
# Source: services/ratings/internal/adapter/inbound/http/endpoints.go
openapi: 3.1.0
info:
  title: ratings
  version: 1.0.0
  description: ratings — generated by tools/specgen
  contact:
    name: bookinfo-team
    url: https://github.com/kaio6fellipe/event-driven-bookinfo
  license:
    name: Apache-2.0
    url: https://www.apache.org/licenses/LICENSE-2.0
servers:
  - url: http://localhost:8080
    description: Local k3d gateway
paths:
  /v1/ratings:
    post:
      operationId: postV1Ratings
      summary: Submit a new rating
      description: Submit a new rating
      tags:
        - ratings
      x-bookinfo-event-name: rating-submitted
      ...
```

```bash
cat services/ratings/api/asyncapi.yaml | head -25
```

Expected:

```yaml
asyncapi: 3.1.0
info:
  title: ratings
  ...
servers:
  kafka:
    host: bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092
    protocol: kafka
    ...
operations:
  send_events:
    action: send
    description: Publish events from the ratings service
    tags:
      - name: ratings
    ...
```

- [ ] **Step 3: Spectral lint — drop from 46 warnings**

```bash
make lint-specs 2>&1 | tail -3
```

Expected: 0 errors. Warnings dropped from 46 to ≤2 (any remaining are structural, not metadata-related).

If some warnings persist, inspect them:

```bash
make lint-specs 2>&1 | grep "warning"
```

Acceptable persistent warnings:
- `oas3-unused-component` if a DTO is referenced via $ref but never used in operations (rare; investigate if seen).
- Anything not in the 46-warning categories from #60.

If a warning category from #60 persists, return to Tasks 4 or 5 to fix.

- [ ] **Step 4: Drift gate**

```bash
make generate-specs && git diff --stat
```

Expected: no NEW changes after re-running (the artifacts committed in Step 5 below should remain stable).

- [ ] **Step 5: Verify catalog-info and values-generated are unchanged**

```bash
git diff --stat services/*/api/catalog-info.yaml deploy/*/values-generated.yaml
```

Expected: empty (these aren't touched by the metadata enrichment; only the openapi/asyncapi changes).

- [ ] **Step 6: Run the full Go test suite**

```bash
go test ./... -race -count=1 -short 2>&1 | tail -3
```

Expected: 321+ tests pass.

- [ ] **Step 7: Run golangci-lint**

```bash
golangci-lint run ./... 2>&1 | tail -3
```

Expected: 0 issues.

- [ ] **Step 8: Commit the regenerated artifacts**

```bash
git add services/*/api/*.yaml
git commit -s -m "chore(specs): regenerate after metadata enrichment

All 6 services regenerated with:
- OpenAPI per-op metadata (operationId, description, tags)
- AsyncAPI per-message + per-op metadata
- info.contact / info.license / info.description / servers
- AsyncAPI version 3.0.0 → 3.1.0

Spectral warnings drop from 46 to ≤2."
```

---

## Task 8: End-to-end k3d verification

**Files:** none modified — verification only. Per the standing rule from PR #63, every PR runs the clean k3d cycle.

- [ ] **Step 1: Bring up a clean cluster**

```bash
make stop-k8s
make run-k8s
```

Expected: completes successfully, all 11 deployments `Available`.

- [ ] **Step 2: Run smoke tests against the gateway**

The smoke script from PR #63's verification still applies. Since this PR doesn't change runtime behavior — only generated docs — the same smoke checks should pass:

```bash
# Happy path: rating submission flows through
curl -fsS http://localhost:8080/v1/ratings/product-1 | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'before count={d[\"count\"]}')"

curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"product_id":"product-1","reviewer":"specgen-enrichment","stars":5}' \
  http://localhost:8080/v1/ratings | head -1

sleep 3
curl -fsS http://localhost:8080/v1/ratings/product-1 | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'after count={d[\"count\"]}')"
```

Expected: `before count` < `after count`. Confirms the runtime contract is unchanged.

- [ ] **Step 3: Validate Tempo trace (optional but recommended)**

Per the standing rule and PR #59 precedent — confirm the full event chain still works:

```bash
kubectl --context=k3d-bookinfo-local port-forward -n observability svc/tempo 3200:3200 2>/dev/null &
PF_PID=$!
sleep 3

# Trigger a fresh book-added chain
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"title":"Specgen Enrichment Probe","author":"Plan","year":2026,"type":"paperback","pages":1,"publisher":"P","language":"en","isbn_10":"","isbn_13":"9999999999333","idempotency_key":"specgen-enrichment-trace-1"}' \
  http://localhost:8080/v1/details | head -1

sleep 6

# Find the trace by topic
TRACE_ID=$(curl -fsS "http://localhost:3200/api/search?tags=messaging.destination.name%3Dbookinfo_details_events&limit=1" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['traces'][0]['traceID'])")
echo "Trace ID: $TRACE_ID"

# Confirm spans from the expected services
curl -fsS "http://localhost:3200/api/traces/$TRACE_ID" | python3 -c "
import sys,json
d = json.load(sys.stdin)
services = set()
for batch in d['batches']:
    for attr in batch.get('resource', {}).get('attributes', []):
        if attr['key'] == 'service.name':
            services.add(attr['value']['stringValue'])
expected = {'details', 'details-sensor', 'notification', 'notification-consumer-sensor', 'book-added-eventsource'}
missing = expected - services
if missing:
    print(f'MISSING: {missing}')
    sys.exit(1)
print(f'OK — all expected services present: {sorted(services)}')
"

kill $PF_PID 2>/dev/null
```

Expected: `OK — all expected services present: [...]`. Confirms the runtime path through gateway → EventSource → Sensor → write service is unchanged by the doc-only metadata enrichment.

- [ ] **Step 4: Final lint + test sweep**

```bash
make lint
go test ./... -race -count=1 -short 2>&1 | tail -3
```

Expected: 0 issues, 321+ tests pass.

- [ ] **Step 5: No commit**

Verification only. If everything above passes, the implementation is complete.

---

## Self-Review

**1. Spec coverage**

| Spec section | Implementing task |
|---|---|
| `pkg/api.Endpoint` gains 3 fields | Task 1 |
| `pkg/events.Descriptor` gains 1 field | Task 2 |
| Walker extracts new fields | Tasks 1 + 2 |
| `runner.SpecMetadata` constants | Task 3 |
| OpenAPI builder smart defaults + spec metadata | Task 4 |
| AsyncAPI builder smart defaults + spec metadata + 3.1.0 bump | Task 5 |
| Walker integration test against real ratings (#61) | Task 6 |
| Regenerate all 6 services + verify lint drops to ≤2 warnings | Task 7 |
| End-to-end k3d clean cycle (per PR #63 standing rule) | Task 8 |
| Per-service code change footprint = zero | Confirmed in Tasks 7 + 8 (no `services/<svc>/...` source changes) |

**2. Placeholder scan**

No `TBD`, `TODO`, "implement later", or "fill in details" present. Every code block contains exact code; every command has expected output described.

One semi-placeholder: in Task 5 Step 4, the message and operation building loops say "The exact variable names depend on the existing builder structure. Read the file first and adapt; the principles are: (a)…(b)…(c)…". This is because I haven't read the inside of `tools/specgen/internal/asyncapi/asyncapi.go::Build` line-by-line. The implementer should read it and adapt. Acceptable — the principles are explicit and the test is the authority (golden file regeneration will catch any deviation).

**3. Type / signature consistency**

- `OperationID`, `Description`, `Tags` consistent across `pkg/api.Endpoint`, `walker.EndpointInfo`, fixture's local `Endpoint`, and the smart-default fallbacks.
- `Tags` consistent across `pkg/events.Descriptor`, `walker.DescriptorInfo`, fixture, asyncapi builder.
- `SpecMetadata` declared in three places (`runner`, `openapi`, `asyncapi`) — deliberate, to avoid an import cycle. Field names match across all three; the runner constructs the smaller per-builder versions explicitly.
- `lowerCamelCase` is package-local to `openapi`; tested directly via the unexported-test pattern.

**4. Scope check**

Single PR territory. ~12 source files modified, 2 new, ~12 generated artifacts regenerated. Verifiable end-to-end on local k3d. No runtime contract changes.

No issues found in self-review.

---

## Notes on AsyncAPI 3.1 compatibility

AsyncAPI 3.1 is fully backward-compatible with 3.0 channels/messages/operations. The only structural difference relevant to our specs is that 3.1 adds optional `info.contact.email` (we don't use), `info.termsOfService` (we don't use), and tightens some existing schemas. The Backstage AsyncAPI plugin (used by the IDP scaffolder) supports both 3.0 and 3.1.

If spectral surfaces new warnings on 3.1.0 that didn't apply to 3.0.0, the implementer can address them in this PR (small fixes) or defer with a tracked issue (bigger). The known categories closed by this plan are listed in #60; new categories were not anticipated at design time.
