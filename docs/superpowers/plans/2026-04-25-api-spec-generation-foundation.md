# API Spec Generation Foundation + ratings Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `tools/specgen` generator and the `pkg/api` + `pkg/events` shared packages, wire them into the Makefile and CI, and migrate the **ratings** service end-to-end as the exemplar. After this plan, `make generate-specs` produces `services/ratings/api/{openapi,catalog-info}.yaml` and `deploy/ratings/values-generated.yaml` from a single declarative `Endpoints` slice in the ratings http adapter, and CI catches drift, lint failures, and breaking OpenAPI changes.

**Architecture:** Two declarative slices per service (`Endpoints` in the http adapter, `Exposed` in the kafka adapter) describe everything the generator needs. A repo-internal Go binary at `tools/specgen` walks these slices via `go/packages` + `go/types`, derives JSONSchemas from referenced struct types via `invopop/jsonschema`, and emits OpenAPI 3.1, AsyncAPI 3.0, Backstage `catalog-info.yaml`, and Helm `values-generated.yaml`. Handlers register routes by looping over the same slice the generator reads, so the slice is load-bearing for runtime behavior — drift would break the service before deploy.

**Tech Stack:** Go 1.25 (existing module), `golang.org/x/tools/go/packages`, `github.com/invopop/jsonschema` (new dep), `github.com/getkin/kin-openapi/openapi3` (new dep, for OpenAPI 3.1 model + YAML), `gopkg.in/yaml.v3` (transitively present), `spectral` CLI (Node, run via npx in CI), `oasdiff` CLI (Go binary, installed in CI).

**Spec reference:** `docs/superpowers/specs/2026-04-25-api-spec-generation-design.md`

**Repo:** `/Users/kaio.fellipe/Documents/git/others/go-http-server`
**Branch:** `main` (open a feature branch per implementer convention)

**Out of scope (explicit follow-up plans, one each):** ingestion migration, details migration, reviews migration, notification migration, dlqueue migration, productpage migration. The patterns to follow are fixed by this plan; subsequent migrations are mechanical.

---

## File Structure

```text
pkg/api/                                   # NEW shared package — Endpoint type + mux register helper
├── endpoint.go                            # Endpoint, ErrorResponse, Register
└── endpoint_test.go                       # tests for Register dispatch

pkg/events/                                # NEW shared package — event Descriptor type
├── descriptor.go                          # Descriptor, ResolveExposureKey
└── descriptor_test.go                     # tests for ResolveExposureKey defaulting

tools/specgen/                             # NEW repo-internal CLI generator
├── main.go                                # CLI entry, subcommand dispatch
├── internal/
│   ├── walker/                            # go/packages-based slice extractor
│   │   ├── walker.go                      # LoadServices entrypoint
│   │   ├── endpoint.go                    # parse []api.Endpoint slice literal
│   │   ├── descriptor.go                  # parse []events.Descriptor slice literal
│   │   └── walker_test.go                 # uses testdata/fixture/
│   ├── jsonschema/                        # invopop wrapper with project conventions
│   │   ├── jsonschema.go
│   │   └── jsonschema_test.go
│   ├── openapi/                           # build OpenAPI 3.1 YAML from walker output
│   │   ├── openapi.go
│   │   └── openapi_test.go                # golden-file test
│   ├── asyncapi/                          # build AsyncAPI 3.0 YAML
│   │   ├── asyncapi.go
│   │   └── asyncapi_test.go               # golden-file test
│   ├── backstage/                         # build catalog-info.yaml
│   │   ├── catalog.go
│   │   └── catalog_test.go
│   ├── values/                            # build deploy/<svc>/values-generated.yaml
│   │   ├── values.go
│   │   └── values_test.go
│   ├── lint/                              # spectral runner
│   │   └── lint.go
│   └── diff/                              # oasdiff runner
│       └── diff.go
└── testdata/
    └── fixture/                           # tiny fake service used by walker tests
        ├── api/endpoints.go
        └── events/exposed.go

services/ratings/internal/adapter/inbound/http/
├── endpoints.go                           # NEW — APIVersion + Endpoints slice
├── handler.go                             # MODIFIED — RegisterRoutes loops over Endpoints
└── (dto.go, handler_test.go unchanged)

services/ratings/api/                      # NEW directory — generated artifacts
├── openapi.yaml                           # generated
└── catalog-info.yaml                      # generated

deploy/ratings/values-generated.yaml       # NEW — generated, owns cqrs.endpoints subkeys

Makefile                                   # MODIFIED — generate-specs/lint-specs/diff-specs targets, dual-f helm install
.github/workflows/ci.yml                   # MODIFIED — specs-drift, specs-lint, specs-breaking jobs
go.mod, go.sum                             # MODIFIED — invopop/jsonschema, kin-openapi, golang.org/x/tools
.spectral.yaml                             # NEW — spectral config (extends recommended rulesets)
```

---

## Phase 1 — Shared packages

### Task 1: Create `pkg/api` with `Endpoint`, `ErrorResponse`, and `Register`

**Files:**

- Create: `pkg/api/endpoint.go`
- Create: `pkg/api/endpoint_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/api/endpoint_test.go`:

```go
package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
)

func TestRegister_DispatchesByMethodAndPath(t *testing.T) {
	endpoints := []api.Endpoint{
		{Method: "GET", Path: "/v1/things/{id}"},
		{Method: "POST", Path: "/v1/things"},
	}

	hits := map[string]int{}
	handlers := map[string]http.HandlerFunc{
		"GET /v1/things/{id}": func(w http.ResponseWriter, r *http.Request) {
			hits["get"]++
			w.WriteHeader(http.StatusOK)
		},
		"POST /v1/things": func(w http.ResponseWriter, r *http.Request) {
			hits["post"]++
			w.WriteHeader(http.StatusCreated)
		},
	}

	mux := http.NewServeMux()
	api.Register(mux, endpoints, handlers)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/things/abc", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/things", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201", rec.Code)
	}

	if hits["get"] != 1 || hits["post"] != 1 {
		t.Errorf("hits = %v, want each = 1", hits)
	}
}

func TestRegister_PanicsOnMissingHandler(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing handler, got none")
		}
	}()

	api.Register(http.NewServeMux(),
		[]api.Endpoint{{Method: "GET", Path: "/v1/missing"}},
		map[string]http.HandlerFunc{})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./pkg/api/...
```

Expected: FAIL with `package github.com/kaio6fellipe/event-driven-bookinfo/pkg/api is not in std`.

- [ ] **Step 3: Implement `pkg/api/endpoint.go`**

Create `pkg/api/endpoint.go`:

```go
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
package api

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
	Method    string          // "GET" | "POST" | "PUT" | "DELETE" | etc.
	Path      string          // stdlib mux path with {param} placeholders
	Summary   string          // one-line OpenAPI summary
	EventName string          // optional: when set on a POST, generates cqrs.endpoints.<EventName>
	Request   any             // zero-value of request DTO; nil when body is absent
	Response  any             // zero-value of success-response DTO; nil for text/html or no-content
	Errors    []ErrorResponse // documented non-2xx responses
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
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test ./pkg/api/... -race -count=1 -v
```

Expected: `PASS` for both `TestRegister_DispatchesByMethodAndPath` and `TestRegister_PanicsOnMissingHandler`.

- [ ] **Step 5: Commit**

```bash
git add pkg/api/endpoint.go pkg/api/endpoint_test.go
git commit -s -m "feat(pkg/api): declarative Endpoint type + Register helper

Introduces the Endpoint struct that tools/specgen will read to derive
OpenAPI 3.1 specs, and the matching Register helper that wires the same
slice into a *http.ServeMux at runtime — single source of truth, drift
panics at startup."
```

---

### Task 2: Create `pkg/events` with `Descriptor` and `ResolveExposureKey`

**Files:**

- Create: `pkg/events/descriptor.go`
- Create: `pkg/events/descriptor_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/events/descriptor_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./pkg/events/...
```

Expected: FAIL with `package github.com/kaio6fellipe/event-driven-bookinfo/pkg/events is not in std`.

- [ ] **Step 3: Implement `pkg/events/descriptor.go`**

Create `pkg/events/descriptor.go`:

```go
// Package events defines the declarative event-publication metadata used by
// every producer service to drive both runtime CloudEvents construction and
// AsyncAPI generation (via tools/specgen).
//
// Each producer service's outbound kafka adapter exposes:
//   var Exposed []events.Descriptor
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
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test ./pkg/events/... -race -count=1 -v
```

Expected: `PASS` for both subtests.

- [ ] **Step 5: Commit**

```bash
git add pkg/events/descriptor.go pkg/events/descriptor_test.go
git commit -s -m "feat(pkg/events): declarative Descriptor type for exposed events

Introduces the Descriptor struct that producer services use to declare
the CloudEvents they publish. Read at runtime by Kafka producers (to
build CE headers) and at build time by tools/specgen (to derive
AsyncAPI specs and the events.exposed block in values-generated.yaml)."
```

---

## Phase 2 — `tools/specgen` generator

### Task 3: Scaffold `tools/specgen` CLI with no-op subcommands

**Files:**

- Create: `tools/specgen/main.go`

- [ ] **Step 1: Add new dependencies to `go.mod`**

Run:

```bash
go get github.com/invopop/jsonschema@latest
go get github.com/getkin/kin-openapi/openapi3@latest
go get golang.org/x/tools/go/packages@latest
go mod tidy
```

Expected: `go.mod` and `go.sum` updated with the three modules.

- [ ] **Step 2: Create the CLI entry point**

Create `tools/specgen/main.go`:

```go
// Command specgen generates OpenAPI 3.1, AsyncAPI 3.0, Backstage
// catalog-info.yaml, and Helm values-generated.yaml for every service in
// services/*/, by walking the declarative Endpoints and Exposed slices in
// each service's adapter packages.
//
// Subcommands:
//
//	specgen all     Regenerate every artifact for every service.
//	specgen lint    Run spectral against every generated spec.
//	specgen diff    Run oasdiff (OpenAPI) and an advisory diff (AsyncAPI)
//	                against origin/main.
//
// Run from the repository root.
package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `specgen — generate API specs from service source

Usage:
  specgen <subcommand> [flags]

Subcommands:
  all      Regenerate every artifact for every service.
  lint     Run spectral against every generated spec.
  diff     Run oasdiff against origin/main and warn on AsyncAPI changes.

Run from the repository root.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "all":
		if err := runAll(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "specgen all: %v\n", err)
			os.Exit(1)
		}
	case "lint":
		if err := runLint(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "specgen lint: %v\n", err)
			os.Exit(1)
		}
	case "diff":
		if err := runDiff(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "specgen diff: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func runAll(args []string) error {
	fs := flag.NewFlagSet("all", flag.ExitOnError)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = repoRoot
	return fmt.Errorf("not implemented")
}

func runLint(args []string) error  { return fmt.Errorf("not implemented") }
func runDiff(args []string) error  { return fmt.Errorf("not implemented") }
```

- [ ] **Step 3: Verify it builds and prints help**

Run:

```bash
go build -o bin/specgen ./tools/specgen
./bin/specgen --help
```

Expected: usage text printed to stdout, exit 0.

- [ ] **Step 4: Verify subcommands return the not-implemented error**

Run:

```bash
./bin/specgen all; echo "exit=$?"
```

Expected: stderr line `specgen all: not implemented`, `exit=1`.

- [ ] **Step 5: Commit**

```bash
git add tools/specgen/main.go go.mod go.sum
git commit -s -m "feat(tools/specgen): CLI scaffolding with all/lint/diff subcommands

Adds the repo-internal generator binary entry point and pulls in the
three new dependencies (invopop/jsonschema, kin-openapi, x/tools/go/packages)
that the implementation tasks will use. Subcommands return
'not implemented' until the per-stage tasks fill them in."
```

---

### Task 4: Walker — load services and extract `Endpoints` slice literals

**Files:**

- Create: `tools/specgen/internal/walker/walker.go`
- Create: `tools/specgen/internal/walker/endpoint.go`
- Create: `tools/specgen/internal/walker/walker_test.go`
- Create: `tools/specgen/testdata/fixture/go.mod`
- Create: `tools/specgen/testdata/fixture/api/endpoints.go`
- Create: `tools/specgen/testdata/fixture/api/dto.go`

The walker resolves the data the rest of the generator needs. Use a self-contained Go fixture module at `testdata/fixture/` so the walker test does not depend on the rest of the repo.

- [ ] **Step 1: Create the test fixture module**

Create `tools/specgen/testdata/fixture/go.mod`:

```text
module fixture

go 1.25
```

Create `tools/specgen/testdata/fixture/api/dto.go`:

```go
package api

type GetThingResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateThingRequest struct {
	Name string `json:"name"`
}

type ErrResp struct {
	Error string `json:"error"`
}
```

Create `tools/specgen/testdata/fixture/api/endpoints.go`:

```go
package api

const APIVersion = "0.1.0"

type Endpoint struct {
	Method    string
	Path      string
	Summary   string
	EventName string
	Request   any
	Response  any
	Errors    []ErrorResponse
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
		Method:    "POST",
		Path:      "/v1/things",
		Summary:   "Create a thing",
		EventName: "thing-created",
		Request:   CreateThingRequest{},
		Response:  GetThingResponse{},
		Errors:    []ErrorResponse{{Status: 400, Type: ErrResp{}}},
	},
}
```

The fixture redeclares `Endpoint` and `ErrorResponse` locally so the fixture has no dependency on `pkg/api`. The walker matches by struct field names and types it sees in the loaded package, not by package identity.

- [ ] **Step 2: Write the failing test**

Create `tools/specgen/internal/walker/walker_test.go`:

```go
package walker_test

import (
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestLoadEndpoints_Fixture(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}

	endpoints, version, err := walker.LoadEndpoints(fixtureDir, "fixture/api")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	if version != "0.1.0" {
		t.Errorf("APIVersion = %q, want %q", version, "0.1.0")
	}
	if len(endpoints) != 2 {
		t.Fatalf("len(endpoints) = %d, want 2", len(endpoints))
	}

	get := endpoints[0]
	if get.Method != "GET" || get.Path != "/v1/things/{id}" {
		t.Errorf("endpoints[0] = {%q, %q}, want {GET, /v1/things/{id}}", get.Method, get.Path)
	}
	if get.Summary != "Get a thing" {
		t.Errorf("endpoints[0].Summary = %q", get.Summary)
	}
	if get.ResponseType == nil || get.ResponseType.Name() != "GetThingResponse" {
		t.Errorf("endpoints[0].ResponseType = %v, want GetThingResponse", get.ResponseType)
	}
	if len(get.Errors) != 1 || get.Errors[0].Status != 404 {
		t.Errorf("endpoints[0].Errors = %v, want [{404, ...}]", get.Errors)
	}

	post := endpoints[1]
	if post.EventName != "thing-created" {
		t.Errorf("endpoints[1].EventName = %q, want thing-created", post.EventName)
	}
	if post.RequestType == nil || post.RequestType.Name() != "CreateThingRequest" {
		t.Errorf("endpoints[1].RequestType = %v, want CreateThingRequest", post.RequestType)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run:

```bash
go test ./tools/specgen/internal/walker/...
```

Expected: FAIL — `walker` package does not exist yet.

- [ ] **Step 4: Implement `walker.go`**

Create `tools/specgen/internal/walker/walker.go`:

```go
// Package walker loads Go service packages and extracts the declarative
// Endpoints and Exposed slice literals that drive specgen.
//
// It uses go/packages to load each service's adapter packages with full
// type information, then walks the AST of the slice literals (composite
// expressions) to extract Method/Path strings, type references, etc.,
// without executing any service code.
package walker

import (
	"fmt"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// EndpointInfo is the walker's view of one element in `var Endpoints []api.Endpoint`.
type EndpointInfo struct {
	Method       string
	Path         string
	Summary      string
	EventName    string
	RequestType  *types.Named // nil when omitted
	ResponseType *types.Named // nil when omitted
	Errors       []ErrorInfo
}

// ErrorInfo is the walker's view of an api.ErrorResponse element.
type ErrorInfo struct {
	Status int
	Type   *types.Named
}

// LoadEndpoints loads the package at moduleDir/<importPath> and returns the
// (Endpoints slice, APIVersion const) it contains. Returns an error if the
// package fails to type-check or the slice literal is missing.
func LoadEndpoints(moduleDir, importPath string) ([]EndpointInfo, string, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps |
			packages.NeedImports,
		Dir:   moduleDir,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, "", fmt.Errorf("loading %q: %w", importPath, err)
	}
	if len(pkgs) != 1 {
		return nil, "", fmt.Errorf("expected exactly 1 package for %q, got %d", importPath, len(pkgs))
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, "", fmt.Errorf("type errors in %q: %v", importPath, pkg.Errors)
	}

	version, err := extractAPIVersion(pkg)
	if err != nil {
		return nil, "", fmt.Errorf("extracting APIVersion: %w", err)
	}

	endpoints, err := extractEndpoints(pkg)
	if err != nil {
		return nil, "", fmt.Errorf("extracting Endpoints: %w", err)
	}

	return endpoints, version, nil
}
```

- [ ] **Step 5: Implement `endpoint.go`**

Create `tools/specgen/internal/walker/endpoint.go`:

```go
package walker

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/packages"
)

func extractAPIVersion(pkg *packages.Package) (string, error) {
	obj := pkg.Types.Scope().Lookup("APIVersion")
	if obj == nil {
		return "", fmt.Errorf("APIVersion const not found in %s", pkg.PkgPath)
	}
	c, ok := obj.(*types.Const)
	if !ok {
		return "", fmt.Errorf("APIVersion is not a const")
	}
	if c.Val().Kind() != constant.String {
		return "", fmt.Errorf("APIVersion must be a string")
	}
	return constant.StringVal(c.Val()), nil
}

func extractEndpoints(pkg *packages.Package) ([]EndpointInfo, error) {
	obj := pkg.Types.Scope().Lookup("Endpoints")
	if obj == nil {
		return nil, fmt.Errorf("Endpoints var not found in %s", pkg.PkgPath)
	}

	// Find the AST node for the Endpoints declaration.
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if name.Name != "Endpoints" {
						continue
					}
					if i >= len(vs.Values) {
						return nil, fmt.Errorf("Endpoints has no initializer")
					}
					return parseEndpointSlice(pkg, vs.Values[i])
				}
			}
		}
	}
	return nil, fmt.Errorf("Endpoints AST node not found")
}

func parseEndpointSlice(pkg *packages.Package, expr ast.Expr) ([]EndpointInfo, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("Endpoints initializer is not a composite literal")
	}
	out := make([]EndpointInfo, 0, len(cl.Elts))
	for i, elt := range cl.Elts {
		ec, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("Endpoints[%d] is not a composite literal", i)
		}
		ep, err := parseEndpointFields(pkg, ec)
		if err != nil {
			return nil, fmt.Errorf("Endpoints[%d]: %w", i, err)
		}
		out = append(out, ep)
	}
	return out, nil
}

func parseEndpointFields(pkg *packages.Package, cl *ast.CompositeLit) (EndpointInfo, error) {
	var ep EndpointInfo
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return ep, fmt.Errorf("Endpoint field must use Key: Value form")
		}
		name, ok := kv.Key.(*ast.Ident)
		if !ok {
			return ep, fmt.Errorf("Endpoint field key is not an identifier")
		}
		switch name.Name {
		case "Method":
			s, err := stringLit(kv.Value)
			if err != nil {
				return ep, err
			}
			ep.Method = s
		case "Path":
			s, err := stringLit(kv.Value)
			if err != nil {
				return ep, err
			}
			ep.Path = s
		case "Summary":
			s, err := stringLit(kv.Value)
			if err != nil {
				return ep, err
			}
			ep.Summary = s
		case "EventName":
			s, err := stringLit(kv.Value)
			if err != nil {
				return ep, err
			}
			ep.EventName = s
		case "Request":
			t, err := namedType(pkg, kv.Value)
			if err != nil {
				return ep, fmt.Errorf("Request: %w", err)
			}
			ep.RequestType = t
		case "Response":
			t, err := namedType(pkg, kv.Value)
			if err != nil {
				return ep, fmt.Errorf("Response: %w", err)
			}
			ep.ResponseType = t
		case "Errors":
			errs, err := parseErrorSlice(pkg, kv.Value)
			if err != nil {
				return ep, fmt.Errorf("Errors: %w", err)
			}
			ep.Errors = errs
		}
	}
	return ep, nil
}

func parseErrorSlice(pkg *packages.Package, expr ast.Expr) ([]ErrorInfo, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("not a composite literal")
	}
	out := make([]ErrorInfo, 0, len(cl.Elts))
	for _, elt := range cl.Elts {
		ec, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("element is not a composite literal")
		}
		var info ErrorInfo
		for _, sub := range ec.Elts {
			kv, ok := sub.(*ast.KeyValueExpr)
			if !ok {
				return nil, fmt.Errorf("ErrorResponse field must use Key: Value form")
			}
			name, ok := kv.Key.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("ErrorResponse field key is not an identifier")
			}
			switch name.Name {
			case "Status":
				bl, ok := kv.Value.(*ast.BasicLit)
				if !ok {
					return nil, fmt.Errorf("Status must be an int literal")
				}
				n, err := strconv.Atoi(bl.Value)
				if err != nil {
					return nil, fmt.Errorf("Status: %w", err)
				}
				info.Status = n
			case "Type":
				t, err := namedType(pkg, kv.Value)
				if err != nil {
					return nil, fmt.Errorf("Type: %w", err)
				}
				info.Type = t
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// stringLit unquotes a Go string literal AST node.
func stringLit(expr ast.Expr) (string, error) {
	bl, ok := expr.(*ast.BasicLit)
	if !ok {
		return "", fmt.Errorf("not a basic literal")
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return "", fmt.Errorf("unquoting: %w", err)
	}
	return s, nil
}

// namedType resolves a composite literal like `Foo{}` to its *types.Named.
func namedType(pkg *packages.Package, expr ast.Expr) (*types.Named, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("not a composite literal")
	}
	tv, ok := pkg.TypesInfo.Types[cl]
	if !ok {
		return nil, fmt.Errorf("no type info for composite literal")
	}
	named, ok := tv.Type.(*types.Named)
	if !ok {
		return nil, fmt.Errorf("composite literal type is not a named type")
	}
	return named, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run:

```bash
go test ./tools/specgen/internal/walker/... -race -count=1 -v
```

Expected: `PASS` for `TestLoadEndpoints_Fixture`.

- [ ] **Step 7: Commit**

```bash
git add tools/specgen/internal/walker tools/specgen/testdata
git commit -s -m "feat(tools/specgen/walker): extract Endpoints slice via go/packages

Loads a service's http adapter package with full type info, walks the
AST of the Endpoints slice literal to extract method/path/summary plus
*types.Named references for Request/Response/Errors. Validated against
a self-contained fixture module under testdata/."
```

---

### Task 5: Walker — extract `Exposed []events.Descriptor`

**Files:**

- Create: `tools/specgen/internal/walker/descriptor.go`
- Modify: `tools/specgen/internal/walker/walker.go` (add `LoadExposed`)
- Modify: `tools/specgen/internal/walker/walker_test.go` (add fixture-based test)
- Create: `tools/specgen/testdata/fixture/events/exposed.go`

- [ ] **Step 1: Add the events fixture**

Create `tools/specgen/testdata/fixture/events/exposed.go`:

```go
package events

type ThingCreatedPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Descriptor struct {
	Name        string
	ExposureKey string
	CEType      string
	CESource    string
	Version     string
	ContentType string
	Payload     any
	Description string
}

var Exposed = []Descriptor{
	{
		Name:        "thing-created",
		ExposureKey: "events",
		CEType:      "com.fixture.thing-created",
		CESource:    "fixture",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     ThingCreatedPayload{},
		Description: "Emitted when a thing is created.",
	},
}
```

- [ ] **Step 2: Add the failing test**

Append to `tools/specgen/internal/walker/walker_test.go`:

```go
func TestLoadExposed_Fixture(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}

	exposed, err := walker.LoadExposed(fixtureDir, "fixture/events")
	if err != nil {
		t.Fatalf("LoadExposed: %v", err)
	}
	if len(exposed) != 1 {
		t.Fatalf("len(exposed) = %d, want 1", len(exposed))
	}

	d := exposed[0]
	if d.Name != "thing-created" || d.ExposureKey != "events" {
		t.Errorf("Name=%q ExposureKey=%q, want thing-created/events", d.Name, d.ExposureKey)
	}
	if d.CEType != "com.fixture.thing-created" {
		t.Errorf("CEType = %q", d.CEType)
	}
	if d.PayloadType == nil || d.PayloadType.Name() != "ThingCreatedPayload" {
		t.Errorf("PayloadType = %v, want ThingCreatedPayload", d.PayloadType)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run:

```bash
go test ./tools/specgen/internal/walker/...
```

Expected: FAIL — `walker.LoadExposed undefined`.

- [ ] **Step 4: Implement `descriptor.go`**

Create `tools/specgen/internal/walker/descriptor.go`:

```go
package walker

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// DescriptorInfo is the walker's view of one element in
// `var Exposed []events.Descriptor`.
type DescriptorInfo struct {
	Name        string
	ExposureKey string
	CEType      string
	CESource    string
	Version     string
	ContentType string
	Description string
	PayloadType *types.Named
}

// LoadExposed loads the package at moduleDir/<importPath> and returns its
// `var Exposed []events.Descriptor` contents.
func LoadExposed(moduleDir, importPath string) ([]DescriptorInfo, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps |
			packages.NeedImports,
		Dir: moduleDir,
	}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, fmt.Errorf("loading %q: %w", importPath, err)
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("expected exactly 1 package, got %d", len(pkgs))
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("type errors in %q: %v", importPath, pkg.Errors)
	}

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if name.Name != "Exposed" {
						continue
					}
					if i >= len(vs.Values) {
						return nil, fmt.Errorf("Exposed has no initializer")
					}
					return parseDescriptorSlice(pkg, vs.Values[i])
				}
			}
		}
	}
	return nil, fmt.Errorf("Exposed not found in %s", pkg.PkgPath)
}

func parseDescriptorSlice(pkg *packages.Package, expr ast.Expr) ([]DescriptorInfo, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("Exposed initializer is not a composite literal")
	}
	out := make([]DescriptorInfo, 0, len(cl.Elts))
	for i, elt := range cl.Elts {
		ec, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("Exposed[%d] is not a composite literal", i)
		}
		d, err := parseDescriptorFields(pkg, ec)
		if err != nil {
			return nil, fmt.Errorf("Exposed[%d]: %w", i, err)
		}
		out = append(out, d)
	}
	return out, nil
}

func parseDescriptorFields(pkg *packages.Package, cl *ast.CompositeLit) (DescriptorInfo, error) {
	var d DescriptorInfo
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return d, fmt.Errorf("Descriptor field must use Key: Value form")
		}
		name, ok := kv.Key.(*ast.Ident)
		if !ok {
			return d, fmt.Errorf("Descriptor field key is not an identifier")
		}
		switch name.Name {
		case "Name":
			s, err := stringLit(kv.Value)
			if err != nil {
				return d, err
			}
			d.Name = s
		case "ExposureKey":
			s, err := stringLit(kv.Value)
			if err != nil {
				return d, err
			}
			d.ExposureKey = s
		case "CEType":
			s, err := stringLit(kv.Value)
			if err != nil {
				return d, err
			}
			d.CEType = s
		case "CESource":
			s, err := stringLit(kv.Value)
			if err != nil {
				return d, err
			}
			d.CESource = s
		case "Version":
			s, err := stringLit(kv.Value)
			if err != nil {
				return d, err
			}
			d.Version = s
		case "ContentType":
			s, err := stringLit(kv.Value)
			if err != nil {
				return d, err
			}
			d.ContentType = s
		case "Description":
			s, err := stringLit(kv.Value)
			if err != nil {
				return d, err
			}
			d.Description = s
		case "Payload":
			t, err := namedType(pkg, kv.Value)
			if err != nil {
				return d, fmt.Errorf("Payload: %w", err)
			}
			d.PayloadType = t
		}
	}
	return d, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run:

```bash
go test ./tools/specgen/internal/walker/... -race -count=1 -v
```

Expected: `PASS` for `TestLoadEndpoints_Fixture` and `TestLoadExposed_Fixture`.

- [ ] **Step 6: Commit**

```bash
git add tools/specgen/internal/walker tools/specgen/testdata
git commit -s -m "feat(tools/specgen/walker): extract Exposed slice for AsyncAPI

Mirrors LoadEndpoints for events.Descriptor — same AST walk over a
composite literal, returns *types.Named for the payload struct so the
jsonschema package can resolve it later."
```

---

### Task 6: JSONSchema wrapper around `invopop/jsonschema`

**Files:**

- Create: `tools/specgen/internal/jsonschema/jsonschema.go`
- Create: `tools/specgen/internal/jsonschema/jsonschema_test.go`

`invopop/jsonschema` reflects on Go types via the `reflect` package, but the walker hands us `*types.Named` (compile-time view, no runtime values). The wrapper bridges this by re-exposing the name as a JSONSchema $ref or inline schema.

For the v1 generator, derive schemas from compile-time Go struct field info: walk the underlying `*types.Struct`, read field names, JSON tags, and Go types, and build `*jsonschema.Schema` directly. This avoids the reflect/runtime gap entirely.

- [ ] **Step 1: Write the failing test**

Create `tools/specgen/internal/jsonschema/jsonschema_test.go`:

```go
package jsonschema_test

import (
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/jsonschema"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestSchemaFromType_PrimitiveFields(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}
	endpoints, _, err := walker.LoadEndpoints(fixtureDir, "fixture/api")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	// endpoints[1] is the POST: Request = CreateThingRequest{Name string}
	schema, err := jsonschema.SchemaFromType(endpoints[1].RequestType)
	if err != nil {
		t.Fatalf("SchemaFromType: %v", err)
	}
	if schema.Type != "object" {
		t.Errorf("schema.Type = %q, want object", schema.Type)
	}
	if _, ok := schema.Properties["name"]; !ok {
		t.Errorf("expected property %q in %v", "name", schema.Properties)
	}
	if got := schema.Properties["name"].Type; got != "string" {
		t.Errorf("name.Type = %q, want string", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./tools/specgen/internal/jsonschema/...
```

Expected: FAIL — package missing.

- [ ] **Step 3: Implement the wrapper**

Create `tools/specgen/internal/jsonschema/jsonschema.go`:

```go
// Package jsonschema produces JSON-Schema fragments from compile-time Go
// types extracted by the walker. It deliberately uses go/types
// (compile-time) rather than reflect (runtime) because specgen never runs
// the service binary.
package jsonschema

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"
)

// Schema is a minimal JSON-Schema document, sufficient for OpenAPI 3.1 and
// AsyncAPI 3.0 component schemas. It marshals as YAML/JSON via gopkg.in/yaml.v3
// and the openapi3 model later.
type Schema struct {
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Format      string             `json:"format,omitempty" yaml:"format,omitempty"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	Required    []string           `json:"required,omitempty" yaml:"required,omitempty"`
}

// SchemaFromType converts a *types.Named to a JSON-Schema, walking struct
// fields and respecting json struct tags. Returns an error for unsupported
// constructs (channels, unsafe pointers, function types).
func SchemaFromType(t *types.Named) (*Schema, error) {
	if t == nil {
		return nil, fmt.Errorf("nil type")
	}
	return schemaFor(t.Underlying())
}

func schemaFor(t types.Type) (*Schema, error) {
	switch tt := t.(type) {
	case *types.Basic:
		return basicSchema(tt), nil
	case *types.Slice:
		inner, err := schemaFor(tt.Elem())
		if err != nil {
			return nil, err
		}
		return &Schema{Type: "array", Items: inner}, nil
	case *types.Named:
		return schemaFor(tt.Underlying())
	case *types.Pointer:
		return schemaFor(tt.Elem())
	case *types.Struct:
		return structSchema(tt)
	default:
		return nil, fmt.Errorf("unsupported type %T", t)
	}
}

func basicSchema(b *types.Basic) *Schema {
	switch b.Kind() {
	case types.Bool:
		return &Schema{Type: "boolean"}
	case types.Int, types.Int8, types.Int16, types.Int32, types.Uint, types.Uint8, types.Uint16, types.Uint32:
		return &Schema{Type: "integer", Format: "int32"}
	case types.Int64, types.Uint64:
		return &Schema{Type: "integer", Format: "int64"}
	case types.Float32:
		return &Schema{Type: "number", Format: "float"}
	case types.Float64:
		return &Schema{Type: "number", Format: "double"}
	case types.String:
		return &Schema{Type: "string"}
	}
	return &Schema{Type: "string"}
}

func structSchema(s *types.Struct) (*Schema, error) {
	out := &Schema{
		Type:       "object",
		Properties: map[string]*Schema{},
	}
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		if !f.Exported() {
			continue
		}
		name, omitempty := jsonTagName(s.Tag(i), f.Name())
		if name == "-" {
			continue
		}
		field, err := schemaFor(f.Type())
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name(), err)
		}
		out.Properties[name] = field
		if !omitempty {
			out.Required = append(out.Required, name)
		}
	}
	return out, nil
}

// jsonTagName parses a JSON struct tag and returns (name, omitempty).
// Defaults to the field name when no json tag is set.
func jsonTagName(rawTag, fieldName string) (string, bool) {
	tag := reflect.StructTag(rawTag).Get("json")
	if tag == "" {
		return fieldName, false
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = fieldName
	}
	omitempty := false
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test ./tools/specgen/internal/jsonschema/... -race -count=1 -v
```

Expected: `PASS` for `TestSchemaFromType_PrimitiveFields`.

- [ ] **Step 5: Commit**

```bash
git add tools/specgen/internal/jsonschema
git commit -s -m "feat(tools/specgen/jsonschema): types.Named -> JSON-Schema

Walks compile-time Go struct types, respects json struct tags
(name + omitempty), and produces JSON-Schema fragments compatible
with OpenAPI 3.1 and AsyncAPI 3.0 component schemas."
```

---

### Task 7: OpenAPI builder

**Files:**

- Create: `tools/specgen/internal/openapi/openapi.go`
- Create: `tools/specgen/internal/openapi/openapi_test.go`
- Create: `tools/specgen/internal/openapi/testdata/golden.yaml`

- [ ] **Step 1: Write the failing test**

Create `tools/specgen/internal/openapi/openapi_test.go`:

```go
package openapi_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/openapi"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestBuild_FixtureMatchesGolden(t *testing.T) {
	fixtureDir, _ := filepath.Abs("../../testdata/fixture")
	endpoints, version, err := walker.LoadEndpoints(fixtureDir, "fixture/api")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	got, err := openapi.Build(openapi.Input{
		ServiceName: "fixture",
		Version:     version,
		Endpoints:   endpoints,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	wantBytes, err := os.ReadFile("testdata/golden.yaml")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	if string(got) != string(wantBytes) {
		// Allow regeneration via UPDATE=1.
		if os.Getenv("UPDATE") == "1" {
			if err := os.WriteFile("testdata/golden.yaml", got, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Log("golden updated")
			return
		}
		t.Errorf("openapi mismatch — set UPDATE=1 to regenerate.\n--- got ---\n%s\n--- want ---\n%s", got, wantBytes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./tools/specgen/internal/openapi/...
```

Expected: FAIL — package missing.

- [ ] **Step 3: Implement the builder**

Create `tools/specgen/internal/openapi/openapi.go`:

```go
// Package openapi builds OpenAPI 3.1 YAML from walker output.
package openapi

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/jsonschema"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

// Input holds everything Build needs.
type Input struct {
	ServiceName string
	Version     string
	Endpoints   []walker.EndpointInfo
}

// Build returns the YAML bytes of the OpenAPI 3.1 document.
func Build(in Input) ([]byte, error) {
	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   in.ServiceName,
			"version": in.Version,
		},
		"paths":      map[string]any{},
		"components": map[string]any{"schemas": map[string]any{}},
	}

	paths := doc["paths"].(map[string]any)
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)

	for _, ep := range in.Endpoints {
		op := map[string]any{
			"summary":   ep.Summary,
			"responses": map[string]any{},
		}
		if ep.EventName != "" {
			op["x-bookinfo-event-name"] = ep.EventName
		}

		if ep.RequestType != nil {
			s, err := jsonschema.SchemaFromType(ep.RequestType)
			if err != nil {
				return nil, fmt.Errorf("request schema for %s %s: %w", ep.Method, ep.Path, err)
			}
			schemas[ep.RequestType.Obj().Name()] = s
			op["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{"$ref": "#/components/schemas/" + ep.RequestType.Obj().Name()},
					},
				},
			}
		}

		responses := op["responses"].(map[string]any)
		successStatus := strconv.Itoa(http.StatusOK)
		if ep.Method == http.MethodPost {
			successStatus = strconv.Itoa(http.StatusCreated)
		}
		if ep.ResponseType != nil {
			s, err := jsonschema.SchemaFromType(ep.ResponseType)
			if err != nil {
				return nil, fmt.Errorf("response schema for %s %s: %w", ep.Method, ep.Path, err)
			}
			schemas[ep.ResponseType.Obj().Name()] = s
			responses[successStatus] = map[string]any{
				"description": "success",
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{"$ref": "#/components/schemas/" + ep.ResponseType.Obj().Name()},
					},
				},
			}
		} else {
			responses[successStatus] = map[string]any{"description": "success"}
		}

		for _, errInfo := range ep.Errors {
			if errInfo.Type != nil {
				s, err := jsonschema.SchemaFromType(errInfo.Type)
				if err != nil {
					return nil, fmt.Errorf("error schema for status %d: %w", errInfo.Status, err)
				}
				schemas[errInfo.Type.Obj().Name()] = s
				responses[strconv.Itoa(errInfo.Status)] = map[string]any{
					"description": http.StatusText(errInfo.Status),
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/" + errInfo.Type.Obj().Name()},
						},
					},
				}
			}
		}

		pathItem, ok := paths[ep.Path].(map[string]any)
		if !ok {
			pathItem = map[string]any{}
			paths[ep.Path] = pathItem
		}
		pathItem[lower(ep.Method)] = op
	}

	// Sort schemas for deterministic YAML.
	sortMap(schemas)
	sortMap(paths)

	var buf bytes.Buffer
	buf.WriteString("# DO NOT EDIT — generated by tools/specgen.\n")
	buf.WriteString("# Source: services/" + in.ServiceName + "/internal/adapter/inbound/http/endpoints.go\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func lower(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + 32
		}
	}
	return string(out)
}

// sortMap rebuilds m in alphabetical-key order. yaml.v3 preserves insertion
// order for map[string]any, so this is required for deterministic output.
func sortMap(m map[string]any) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tmp := make(map[string]any, len(m))
	for _, k := range keys {
		tmp[k] = m[k]
	}
	for k := range m {
		delete(m, k)
	}
	for _, k := range keys {
		m[k] = tmp[k]
	}
}
```

- [ ] **Step 4: Generate the golden file via the UPDATE flag**

Run:

```bash
mkdir -p tools/specgen/internal/openapi/testdata
UPDATE=1 go test ./tools/specgen/internal/openapi/... -run TestBuild_FixtureMatchesGolden
```

Expected: test logs `golden updated`, file `tools/specgen/internal/openapi/testdata/golden.yaml` now exists.

- [ ] **Step 5: Inspect the golden file**

Run:

```bash
cat tools/specgen/internal/openapi/testdata/golden.yaml
```

Expected: a valid OpenAPI 3.1 YAML with two paths (`/v1/things/{id}` GET, `/v1/things` POST), `components.schemas` for `GetThingResponse`, `CreateThingRequest`, `ErrResp`, alphabetically sorted.

- [ ] **Step 6: Re-run without UPDATE to confirm determinism**

Run:

```bash
go test ./tools/specgen/internal/openapi/... -race -count=1 -v
```

Expected: `PASS`.

- [ ] **Step 7: Commit**

```bash
git add tools/specgen/internal/openapi
git commit -s -m "feat(tools/specgen/openapi): build OpenAPI 3.1 YAML from walker output

Composes paths + components.schemas with deterministic key ordering,
records the EventName on each operation as x-bookinfo-event-name (used
later by the values builder). Validated against a golden file from the
fixture module."
```

---

### Task 8: AsyncAPI builder

**Files:**

- Create: `tools/specgen/internal/asyncapi/asyncapi.go`
- Create: `tools/specgen/internal/asyncapi/asyncapi_test.go`
- Create: `tools/specgen/internal/asyncapi/testdata/golden.yaml`

- [ ] **Step 1: Write the failing test**

Create `tools/specgen/internal/asyncapi/asyncapi_test.go`:

```go
package asyncapi_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/asyncapi"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestBuild_FixtureMatchesGolden(t *testing.T) {
	fixtureDir, _ := filepath.Abs("../../testdata/fixture")
	exposed, err := walker.LoadExposed(fixtureDir, "fixture/events")
	if err != nil {
		t.Fatalf("LoadExposed: %v", err)
	}

	got, err := asyncapi.Build(asyncapi.Input{
		ServiceName: "fixture",
		Version:     "0.1.0",
		Exposed:     exposed,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	wantBytes, _ := os.ReadFile("testdata/golden.yaml")
	if string(got) != string(wantBytes) {
		if os.Getenv("UPDATE") == "1" {
			if err := os.WriteFile("testdata/golden.yaml", got, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
		t.Errorf("asyncapi mismatch — set UPDATE=1 to regenerate.\n--- got ---\n%s\n--- want ---\n%s", got, wantBytes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./tools/specgen/internal/asyncapi/...
```

Expected: FAIL — package missing.

- [ ] **Step 3: Implement the builder**

Create `tools/specgen/internal/asyncapi/asyncapi.go`:

```go
// Package asyncapi builds AsyncAPI 3.0 YAML from walker output.
package asyncapi

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/jsonschema"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

// Input holds everything Build needs.
type Input struct {
	ServiceName string
	Version     string
	Exposed     []walker.DescriptorInfo
}

// Build returns the YAML bytes of the AsyncAPI 3.0 document. Descriptors
// are grouped by ExposureKey (defaulting to Name); within a channel each
// descriptor becomes one Message.
func Build(in Input) ([]byte, error) {
	doc := map[string]any{
		"asyncapi": "3.0.0",
		"info": map[string]any{
			"title":   in.ServiceName,
			"version": in.Version,
		},
		"channels":   map[string]any{},
		"operations": map[string]any{},
		"components": map[string]any{
			"messages": map[string]any{},
			"schemas":  map[string]any{},
		},
	}

	channels := doc["channels"].(map[string]any)
	operations := doc["operations"].(map[string]any)
	messages := doc["components"].(map[string]any)["messages"].(map[string]any)
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)

	// Group descriptors by ExposureKey.
	groups := map[string][]walker.DescriptorInfo{}
	for _, d := range in.Exposed {
		key := d.ExposureKey
		if key == "" {
			key = d.Name
		}
		groups[key] = append(groups[key], d)
	}

	for key, ds := range groups {
		channelMessages := map[string]any{}
		for _, d := range ds {
			schemaName := d.PayloadType.Obj().Name()
			s, err := jsonschema.SchemaFromType(d.PayloadType)
			if err != nil {
				return nil, fmt.Errorf("schema for %s: %w", d.Name, err)
			}
			schemas[schemaName] = s

			messages[d.Name] = map[string]any{
				"name":        d.Name,
				"title":       d.Name,
				"summary":     d.Description,
				"contentType": d.ContentType,
				"payload":     map[string]any{"$ref": "#/components/schemas/" + schemaName},
				"headers": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ce-type":        map[string]any{"type": "string", "const": d.CEType},
						"ce-source":      map[string]any{"type": "string", "const": d.CESource},
						"ce-specversion": map[string]any{"type": "string", "const": d.Version},
					},
				},
			}
			channelMessages[d.Name] = map[string]any{"$ref": "#/components/messages/" + d.Name}
		}

		channels[key] = map[string]any{
			"address":  key,
			"messages": channelMessages,
		}
		operations["send_"+key] = map[string]any{
			"action":  "send",
			"channel": map[string]any{"$ref": "#/channels/" + key},
		}
	}

	var buf bytes.Buffer
	buf.WriteString("# DO NOT EDIT — generated by tools/specgen.\n")
	buf.WriteString("# Source: services/" + in.ServiceName + "/internal/adapter/outbound/kafka/exposed.go\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Generate the golden file**

Run:

```bash
mkdir -p tools/specgen/internal/asyncapi/testdata
UPDATE=1 go test ./tools/specgen/internal/asyncapi/... -run TestBuild_FixtureMatchesGolden
```

Expected: golden file written.

- [ ] **Step 5: Re-run to confirm determinism**

Run:

```bash
go test ./tools/specgen/internal/asyncapi/... -race -count=1 -v
```

Expected: `PASS`.

- [ ] **Step 6: Commit**

```bash
git add tools/specgen/internal/asyncapi
git commit -s -m "feat(tools/specgen/asyncapi): build AsyncAPI 3.0 YAML from walker output

Groups descriptors by ExposureKey into one channel per group; emits
one components.messages entry per descriptor with CE binding headers
recorded as const-valued JSONSchema properties."
```

---

### Task 9: Backstage `catalog-info.yaml` emitter

**Files:**

- Create: `tools/specgen/internal/backstage/catalog.go`
- Create: `tools/specgen/internal/backstage/catalog_test.go`
- Create: `tools/specgen/internal/backstage/testdata/golden.yaml`

- [ ] **Step 1: Write the failing test**

Create `tools/specgen/internal/backstage/catalog_test.go`:

```go
package backstage_test

import (
	"os"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/backstage"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestBuild_BothApisMatchesGolden(t *testing.T) {
	got, err := backstage.Build(backstage.Input{
		ServiceName:    "ratings",
		HasOpenAPI:     true,
		HasAsyncAPI:    true,
		ExposedNames:   []string{"rating-submitted"},
		Owner:          "bookinfo-team",
		RepoTreeURL:    "https://github.com/kaio6fellipe/event-driven-bookinfo/tree/main/services/ratings",
		ExposedSubset: []walker.DescriptorInfo{
			{Name: "rating-submitted"},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want, _ := os.ReadFile("testdata/golden.yaml")
	if string(got) != string(want) {
		if os.Getenv("UPDATE") == "1" {
			_ = os.WriteFile("testdata/golden.yaml", got, 0o644)
			return
		}
		t.Errorf("catalog mismatch — set UPDATE=1 to regenerate.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./tools/specgen/internal/backstage/...
```

Expected: FAIL — package missing.

- [ ] **Step 3: Implement the emitter**

Create `tools/specgen/internal/backstage/catalog.go`:

```go
// Package backstage emits the per-service catalog-info.yaml that the
// external IDP (Backstage) discovers via its GitHub integration.
package backstage

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

// Input holds everything Build needs.
type Input struct {
	ServiceName   string
	Owner         string
	RepoTreeURL   string
	HasOpenAPI    bool
	HasAsyncAPI   bool
	ExposedSubset []walker.DescriptorInfo // used for the exposed-event-names annotation
	ExposedNames  []string                // mirror of ExposedSubset names, kept for the annotation order
}

// Build returns the YAML bytes (multi-document) of the catalog-info.yaml.
func Build(in Input) ([]byte, error) {
	if in.Owner == "" {
		return nil, fmt.Errorf("Owner is required")
	}
	if in.RepoTreeURL == "" {
		return nil, fmt.Errorf("RepoTreeURL is required")
	}

	var buf bytes.Buffer
	buf.WriteString("# DO NOT EDIT — generated by tools/specgen.\n")

	provides := []string{}
	docs := []map[string]any{}

	// Component first.
	if in.HasOpenAPI {
		provides = append(provides, in.ServiceName+"-rest")
	}
	if in.HasAsyncAPI {
		provides = append(provides, in.ServiceName+"-events")
	}

	component := map[string]any{
		"apiVersion": "backstage.io/v1alpha1",
		"kind":       "Component",
		"metadata": map[string]any{
			"name": in.ServiceName,
			"annotations": map[string]any{
				"backstage.io/source-location": "url:" + in.RepoTreeURL,
			},
		},
		"spec": map[string]any{
			"type":         "service",
			"lifecycle":    "experimental",
			"owner":        in.Owner,
			"providesApis": provides,
			"consumesApis": []string{},
		},
	}
	docs = append(docs, component)

	if in.HasOpenAPI {
		docs = append(docs, map[string]any{
			"apiVersion": "backstage.io/v1alpha1",
			"kind":       "API",
			"metadata":   map[string]any{"name": in.ServiceName + "-rest"},
			"spec": map[string]any{
				"type":       "openapi",
				"lifecycle":  "experimental",
				"owner":      in.Owner,
				"definition": map[string]any{"$text": "./openapi.yaml"},
			},
		})
	}
	if in.HasAsyncAPI {
		annotations := map[string]any{}
		if len(in.ExposedNames) > 0 {
			annotations["bookinfo.io/exposed-event-names"] = strings.Join(in.ExposedNames, ",")
		}
		api := map[string]any{
			"apiVersion": "backstage.io/v1alpha1",
			"kind":       "API",
			"metadata": map[string]any{
				"name":        in.ServiceName + "-events",
				"annotations": annotations,
			},
			"spec": map[string]any{
				"type":       "asyncapi",
				"lifecycle":  "experimental",
				"owner":      in.Owner,
				"definition": map[string]any{"$text": "./asyncapi.yaml"},
			},
		}
		docs = append(docs, api)
	}

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	for _, d := range docs {
		if err := enc.Encode(d); err != nil {
			return nil, err
		}
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Generate the golden file**

Run:

```bash
mkdir -p tools/specgen/internal/backstage/testdata
UPDATE=1 go test ./tools/specgen/internal/backstage/... -run TestBuild_BothApisMatchesGolden
```

- [ ] **Step 5: Re-run to confirm determinism**

Run:

```bash
go test ./tools/specgen/internal/backstage/... -race -count=1 -v
```

Expected: `PASS`.

- [ ] **Step 6: Commit**

```bash
git add tools/specgen/internal/backstage
git commit -s -m "feat(tools/specgen/backstage): emit per-service catalog-info.yaml

Generates one Component + up-to-two API entities (OpenAPI and/or
AsyncAPI) per service, with the bookinfo.io/exposed-event-names
annotation used by the IDP scaffolding plugin to filter the event
catalog."
```

---

### Task 10: `values-generated.yaml` emitter

**Files:**

- Create: `tools/specgen/internal/values/values.go`
- Create: `tools/specgen/internal/values/values_test.go`

- [ ] **Step 1: Write the failing test**

Create `tools/specgen/internal/values/values_test.go`:

```go
package values_test

import (
	"strings"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/values"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestBuild_GeneratesCqrsAndEventsBlocks(t *testing.T) {
	got, err := values.Build(values.Input{
		ServiceName: "details",
		Endpoints: []walker.EndpointInfo{
			{Method: "POST", Path: "/v1/details", EventName: "book-added"},
			{Method: "GET", Path: "/v1/details/{id}"},
		},
		Exposed: []walker.DescriptorInfo{
			{Name: "book-added", ExposureKey: "events", CEType: "com.bookinfo.details.book-added", ContentType: "application/json"},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	out := string(got)
	for _, want := range []string{
		"DO NOT EDIT",
		"cqrs:\n  endpoints:\n    book-added:\n      method: POST\n      endpoint: /v1/details",
		"events:\n  exposed:\n    events:\n      contentType: application/json",
		"- com.bookinfo.details.book-added",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	if strings.Contains(out, "/v1/details/{id}") {
		t.Error("GET endpoint must NOT appear in cqrs.endpoints (no EventName)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./tools/specgen/internal/values/...
```

Expected: FAIL — package missing.

- [ ] **Step 3: Implement the emitter**

Create `tools/specgen/internal/values/values.go`:

```go
// Package values emits deploy/<svc>/values-generated.yaml — the
// disjoint subset of cqrs.endpoints and events.exposed that specgen owns.
package values

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

// Input holds everything Build needs.
type Input struct {
	ServiceName string
	Endpoints   []walker.EndpointInfo
	Exposed     []walker.DescriptorInfo
}

// Build returns the YAML bytes of the values-generated.yaml.
func Build(in Input) ([]byte, error) {
	cqrsEndpoints := map[string]any{}
	for _, ep := range in.Endpoints {
		if ep.EventName == "" {
			continue
		}
		cqrsEndpoints[ep.EventName] = map[string]any{
			"method":   ep.Method,
			"endpoint": ep.Path,
		}
	}

	exposedGroups := map[string]map[string]any{}
	for _, d := range in.Exposed {
		key := d.ExposureKey
		if key == "" {
			key = d.Name
		}
		group, ok := exposedGroups[key]
		if !ok {
			group = map[string]any{
				"contentType": d.ContentType,
				"eventTypes":  []string{},
			}
			exposedGroups[key] = group
		}
		group["eventTypes"] = append(group["eventTypes"].([]string), d.CEType)
	}

	doc := map[string]any{}
	if len(cqrsEndpoints) > 0 {
		doc["cqrs"] = map[string]any{"endpoints": cqrsEndpoints}
	}
	if len(exposedGroups) > 0 {
		exposed := map[string]any{}
		for k, v := range exposedGroups {
			exposed[k] = v
		}
		doc["events"] = map[string]any{"exposed": exposed}
	}

	var buf bytes.Buffer
	buf.WriteString("# DO NOT EDIT — generated by tools/specgen from\n")
	buf.WriteString("#   services/" + in.ServiceName + "/internal/adapter/inbound/http/endpoints.go\n")
	buf.WriteString("#   services/" + in.ServiceName + "/internal/adapter/outbound/kafka/exposed.go\n")
	buf.WriteString("# Run `make generate-specs` to refresh.\n")

	if len(doc) == 0 {
		return buf.Bytes(), nil
	}

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test ./tools/specgen/internal/values/... -race -count=1 -v
```

Expected: `PASS`.

- [ ] **Step 5: Commit**

```bash
git add tools/specgen/internal/values
git commit -s -m "feat(tools/specgen/values): emit deploy/<svc>/values-generated.yaml

Owns only cqrs.endpoints.<EventName>.{method,endpoint} and
events.exposed.<ExposureKey>.{contentType,eventTypes}; everything
else stays in the hand-edited values-local.yaml."
```

---

### Task 11: Wire `specgen all` end-to-end

**Files:**

- Modify: `tools/specgen/main.go`
- Create: `tools/specgen/internal/runner/runner.go`
- Create: `tools/specgen/internal/runner/runner_test.go`

- [ ] **Step 1: Write the failing test**

Create `tools/specgen/internal/runner/runner_test.go`:

```go
package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/runner"
)

func TestDiscoverServices_FindsRatings(t *testing.T) {
	repoRoot, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "services/ratings")); err != nil {
		t.Skip("ratings service not present")
	}

	svcs, err := runner.DiscoverServices(repoRoot)
	if err != nil {
		t.Fatalf("DiscoverServices: %v", err)
	}

	found := false
	for _, s := range svcs {
		if s.Name == "ratings" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ratings service not discovered, got %v", svcs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./tools/specgen/internal/runner/...
```

Expected: FAIL — package missing.

- [ ] **Step 3: Implement the runner**

Create `tools/specgen/internal/runner/runner.go`:

```go
// Package runner is the orchestrator that turns one repo root into all
// generated artifacts. It is the body of `specgen all`.
package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/asyncapi"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/backstage"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/openapi"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/values"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

// Service describes one discovered service in the repo.
type Service struct {
	Name        string
	Root        string
	HTTPPkg     string // import path of internal/adapter/inbound/http
	KafkaPkg    string // import path of internal/adapter/outbound/kafka
	HasHTTPPkg  bool
	HasKafkaPkg bool
}

const modulePath = "github.com/kaio6fellipe/event-driven-bookinfo"

// DiscoverServices returns every directory under <repoRoot>/services/ that
// contains either an http endpoints.go or a kafka exposed.go.
func DiscoverServices(repoRoot string) ([]Service, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "services"))
	if err != nil {
		return nil, fmt.Errorf("reading services dir: %w", err)
	}

	var out []Service
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join(repoRoot, "services", e.Name())
		svc := Service{Name: e.Name(), Root: root}

		httpEndpoints := filepath.Join(root, "internal/adapter/inbound/http/endpoints.go")
		if _, err := os.Stat(httpEndpoints); err == nil {
			svc.HasHTTPPkg = true
			svc.HTTPPkg = modulePath + "/services/" + e.Name() + "/internal/adapter/inbound/http"
		}
		kafkaExposed := filepath.Join(root, "internal/adapter/outbound/kafka/exposed.go")
		if _, err := os.Stat(kafkaExposed); err == nil {
			svc.HasKafkaPkg = true
			svc.KafkaPkg = modulePath + "/services/" + e.Name() + "/internal/adapter/outbound/kafka"
		}

		if svc.HasHTTPPkg || svc.HasKafkaPkg {
			out = append(out, svc)
		}
	}
	return out, nil
}

// RunAll regenerates artifacts for every discovered service.
func RunAll(repoRoot string) error {
	svcs, err := DiscoverServices(repoRoot)
	if err != nil {
		return err
	}
	for _, s := range svcs {
		if err := generateOne(repoRoot, s); err != nil {
			return fmt.Errorf("service %s: %w", s.Name, err)
		}
		fmt.Printf("specgen: %s OK\n", s.Name)
	}
	return nil
}

func generateOne(repoRoot string, svc Service) error {
	apiDir := filepath.Join(repoRoot, "services", svc.Name, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		return err
	}

	var (
		endpoints []walker.EndpointInfo
		exposed   []walker.DescriptorInfo
		version   string
	)

	if svc.HasHTTPPkg {
		eps, ver, err := walker.LoadEndpoints(repoRoot, svc.HTTPPkg)
		if err != nil {
			return fmt.Errorf("loading endpoints: %w", err)
		}
		endpoints = eps
		version = ver

		yamlBytes, err := openapi.Build(openapi.Input{
			ServiceName: svc.Name,
			Version:     version,
			Endpoints:   endpoints,
		})
		if err != nil {
			return fmt.Errorf("building OpenAPI: %w", err)
		}
		if err := os.WriteFile(filepath.Join(apiDir, "openapi.yaml"), yamlBytes, 0o644); err != nil {
			return err
		}
	}

	if svc.HasKafkaPkg {
		exps, err := walker.LoadExposed(repoRoot, svc.KafkaPkg)
		if err != nil {
			return fmt.Errorf("loading exposed: %w", err)
		}
		exposed = exps

		yamlBytes, err := asyncapi.Build(asyncapi.Input{
			ServiceName: svc.Name,
			Version:     version, // ok if empty when only AsyncAPI side present
			Exposed:     exposed,
		})
		if err != nil {
			return fmt.Errorf("building AsyncAPI: %w", err)
		}
		if err := os.WriteFile(filepath.Join(apiDir, "asyncapi.yaml"), yamlBytes, 0o644); err != nil {
			return err
		}
	}

	// catalog-info.yaml
	exposedNames := make([]string, len(exposed))
	for i, d := range exposed {
		exposedNames[i] = d.Name
	}
	cBytes, err := backstage.Build(backstage.Input{
		ServiceName:   svc.Name,
		Owner:         "bookinfo-team",
		RepoTreeURL:   "https://github.com/kaio6fellipe/event-driven-bookinfo/tree/main/services/" + svc.Name,
		HasOpenAPI:    svc.HasHTTPPkg,
		HasAsyncAPI:   svc.HasKafkaPkg,
		ExposedNames:  exposedNames,
		ExposedSubset: exposed,
	})
	if err != nil {
		return fmt.Errorf("building catalog-info: %w", err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "catalog-info.yaml"), cBytes, 0o644); err != nil {
		return err
	}

	// values-generated.yaml
	deployDir := filepath.Join(repoRoot, "deploy", svc.Name)
	if err := os.MkdirAll(deployDir, 0o755); err != nil {
		return err
	}
	vBytes, err := values.Build(values.Input{
		ServiceName: svc.Name,
		Endpoints:   endpoints,
		Exposed:     exposed,
	})
	if err != nil {
		return fmt.Errorf("building values: %w", err)
	}
	if err := os.WriteFile(filepath.Join(deployDir, "values-generated.yaml"), vBytes, 0o644); err != nil {
		return err
	}

	return nil
}
```

- [ ] **Step 4: Update `RunAll` to skip-on-error**

In `tools/specgen/internal/runner/runner.go`, change the loop in `RunAll` from a hard return on first error to log-and-continue, so unmigrated services do not block migrated ones:

```go
func RunAll(repoRoot string) error {
	svcs, err := DiscoverServices(repoRoot)
	if err != nil {
		return err
	}
	for _, s := range svcs {
		if err := generateOne(repoRoot, s); err != nil {
			fmt.Fprintf(os.Stderr, "specgen: %s SKIPPED: %v\n", s.Name, err)
			continue
		}
		fmt.Printf("specgen: %s OK\n", s.Name)
	}
	return nil
}
```

This keeps Phase 4 from breaking when only `ratings` has been migrated and the other services still lack `endpoints.go`.

- [ ] **Step 5: Wire `runAll` in `main.go`**

Replace the body of `runAll` in `tools/specgen/main.go` with:

```go
func runAll(args []string) error {
	fs := flag.NewFlagSet("all", flag.ExitOnError)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runner.RunAll(*repoRoot)
}
```

and add the import:

```go
import "github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/runner"
```

- [ ] **Step 6: Run runner test to verify it passes**

Run:

```bash
go test ./tools/specgen/internal/runner/... -race -count=1 -v
```

Expected: `PASS` (or `SKIP` if run before any service has been migrated; the test only asserts that `ratings` is among the discovered services regardless of migration state).

- [ ] **Step 7: Smoke-run the binary**

Run:

```bash
go build -o bin/specgen ./tools/specgen
./bin/specgen all --repo-root . 2>&1 | head -20
```

Expected: every service prints either `specgen: <svc> OK` (already migrated) or `specgen: <svc> SKIPPED: ...` (not yet). Exit code 0. No service has been migrated yet so all lines should be SKIPPED.

- [ ] **Step 8: Commit**

```bash
git add tools/specgen
git commit -s -m "feat(tools/specgen): wire 'specgen all' end-to-end

Discovers services via filesystem layout, walks each service's http
and kafka adapter packages, and writes openapi.yaml,
asyncapi.yaml, catalog-info.yaml, and values-generated.yaml.
Behavior on a fresh repo (no service migrated yet) is to exit with an
error pointing at the first missing endpoints.go — Phase 4 fixes that
for ratings."
```

---

### Task 12: `specgen lint` subcommand (spectral runner)

**Files:**

- Create: `tools/specgen/internal/lint/lint.go`
- Modify: `tools/specgen/main.go`
- Create: `.spectral.yaml`

`spectral` runs as a Node CLI installed via `npx`. The Go subcommand simply exec's it on the right files.

- [ ] **Step 1: Add the spectral config**

Create `.spectral.yaml`:

```yaml
extends:
  - spectral:oas
  - spectral:asyncapi
```

- [ ] **Step 2: Implement the lint package**

Create `tools/specgen/internal/lint/lint.go`:

```go
// Package lint runs spectral over every generated spec via npx.
package lint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Run invokes spectral on every generated openapi.yaml and asyncapi.yaml
// under repoRoot/services/*/api/.
func Run(repoRoot string) error {
	servicesDir := filepath.Join(repoRoot, "services")
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		return fmt.Errorf("reading services dir: %w", err)
	}

	var specs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		apiDir := filepath.Join(servicesDir, e.Name(), "api")
		for _, f := range []string{"openapi.yaml", "asyncapi.yaml"} {
			full := filepath.Join(apiDir, f)
			if _, err := os.Stat(full); err == nil {
				specs = append(specs, full)
			}
		}
	}
	if len(specs) == 0 {
		fmt.Println("specgen lint: no specs found")
		return nil
	}

	args := append([]string{"--yes", "@stoplight/spectral-cli", "lint", "--ruleset", filepath.Join(repoRoot, ".spectral.yaml")}, specs...)
	cmd := exec.Command("npx", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("spectral: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Wire `runLint` in `main.go`**

Replace the body of `runLint` in `tools/specgen/main.go`:

```go
func runLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return lint.Run(*repoRoot)
}
```

Add the import: `"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/lint"`.

- [ ] **Step 4: Smoke test**

Run:

```bash
go build -o bin/specgen ./tools/specgen
./bin/specgen lint --repo-root .
```

Expected: prints `specgen lint: no specs found` and exits 0 (no specs committed yet).

- [ ] **Step 5: Commit**

```bash
git add tools/specgen/internal/lint tools/specgen/main.go .spectral.yaml
git commit -s -m "feat(tools/specgen/lint): spectral runner subcommand

Invokes spectral via npx with the project's .spectral.yaml ruleset
(extends spectral:oas + spectral:asyncapi) over every generated spec
under services/*/api/."
```

---

### Task 13: `specgen diff` subcommand (oasdiff runner)

**Files:**

- Create: `tools/specgen/internal/diff/diff.go`
- Modify: `tools/specgen/main.go`

- [ ] **Step 1: Implement the diff package**

Create `tools/specgen/internal/diff/diff.go`:

```go
// Package diff runs oasdiff to detect breaking OpenAPI changes between
// origin/main and the working tree.
package diff

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Run iterates every services/*/api/openapi.yaml present in HEAD and
// compares it to the same path on origin/main using oasdiff. Returns a
// non-nil error if any breaking change is detected.
func Run(repoRoot string) error {
	servicesDir := filepath.Join(repoRoot, "services")
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		return fmt.Errorf("reading services dir: %w", err)
	}

	hadBreaking := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		spec := filepath.Join(servicesDir, e.Name(), "api", "openapi.yaml")
		if _, err := os.Stat(spec); err != nil {
			continue
		}

		baseRef := "origin/main:" + filepath.ToSlash(filepath.Join("services", e.Name(), "api", "openapi.yaml"))
		var stdout, stderr bytes.Buffer
		cmd := exec.Command("oasdiff", "breaking", baseRef, spec, "--fail-on", "ERR")
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		if runErr != nil {
			fmt.Printf("=== %s breaking changes ===\n%s%s", e.Name(), stdout.String(), stderr.String())
			hadBreaking = true
			continue
		}
		fmt.Printf("specgen diff: %s OK\n", e.Name())
	}
	if hadBreaking {
		return fmt.Errorf("breaking OpenAPI changes detected")
	}
	return nil
}
```

`oasdiff` reads `git:`-style refs natively for the base side; we pass `origin/main:<path>` as a bare argument and oasdiff resolves it via `git`.

- [ ] **Step 2: Wire `runDiff` in `main.go`**

Replace the body of `runDiff` in `tools/specgen/main.go`:

```go
func runDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return diff.Run(*repoRoot)
}
```

Add the import: `"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/diff"`.

- [ ] **Step 3: Smoke test**

Run:

```bash
go build -o bin/specgen ./tools/specgen
./bin/specgen diff --repo-root .
```

Expected: no specs found yet; prints nothing and exits 0. (Behavior with specs present is verified after Phase 4.)

- [ ] **Step 4: Commit**

```bash
git add tools/specgen/internal/diff tools/specgen/main.go
git commit -s -m "feat(tools/specgen/diff): oasdiff breaking-change subcommand

Compares each services/*/api/openapi.yaml against the same path on
origin/main; non-zero exit if oasdiff reports any ERR-level breaking
change. AsyncAPI side is not checked here (advisory; see follow-up)."
```

---

## Phase 3 — Repo plumbing

### Task 14: Add Makefile targets

**Files:**

- Modify: `Makefile`

- [ ] **Step 1: Append the new targets after `helm-template`**

Open `Makefile`, find the line:

```makefile
# ─── Help ───────────────────────────────────────────────────────────────────
```

Insert the following block immediately above it:

```makefile
# ─── API specs ─────────────────────────────────────────────────────────────

SPECGEN_BIN := bin/specgen

.PHONY: $(SPECGEN_BIN)
$(SPECGEN_BIN):
	@mkdir -p bin
	@go build -o $(SPECGEN_BIN) ./tools/specgen

.PHONY: generate-specs
generate-specs: ##@Specs Regenerate openapi/asyncapi/catalog-info/values-generated for every service
	@$(MAKE) --no-print-directory $(SPECGEN_BIN)
	@$(SPECGEN_BIN) all --repo-root .

.PHONY: lint-specs
lint-specs: ##@Specs Run spectral against all generated specs
	@$(MAKE) --no-print-directory $(SPECGEN_BIN)
	@$(SPECGEN_BIN) lint --repo-root .

.PHONY: diff-specs
diff-specs: ##@Specs oasdiff for each OpenAPI spec vs origin/main; fails on breaking changes
	@$(MAKE) --no-print-directory $(SPECGEN_BIN)
	@$(SPECGEN_BIN) diff --repo-root .
```

- [ ] **Step 2: Switch `k8s-deploy` and `k8s-rebuild` helm install loops to dual-`-f`**

In `Makefile`, find each of these blocks:

```makefile
$(HELM) upgrade --install $$svc charts/bookinfo-service \
    --namespace $(K8S_NS_BOOKINFO) \
    -f deploy/$$svc/values-local.yaml || exit 1; \
```

Replace each with:

```makefile
gen=""; \
[ -f deploy/$$svc/values-generated.yaml ] && gen="-f deploy/$$svc/values-generated.yaml"; \
$(HELM) upgrade --install $$svc charts/bookinfo-service \
    --namespace $(K8S_NS_BOOKINFO) \
    $$gen \
    -f deploy/$$svc/values-local.yaml || exit 1; \
```

This applies to both occurrences (`k8s-deploy` and `k8s-rebuild`).

- [ ] **Step 3: Verify the Makefile parses**

Run:

```bash
make help | grep -E "generate-specs|lint-specs|diff-specs"
```

Expected: three lines listing the new targets under the `Specs` group.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -s -m "build(make): add generate-specs/lint-specs/diff-specs targets

Adds Makefile entry points for tools/specgen and switches the local
k8s helm install loops to dual-f (values-generated.yaml -f
values-local.yaml) — values-generated.yaml is conditionally included
so unmigrated services keep working."
```

---

### Task 15: Add CI jobs for drift, lint, breaking changes

**Files:**

- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Read the current ci.yml to find the right insertion point**

Run:

```bash
grep -n "^jobs:\|^  [a-z-]*:" .github/workflows/ci.yml | head -30
```

Note the existing job names (e.g. `lint`, `test`, `build`).

- [ ] **Step 2: Append three new jobs at the end of the `jobs:` section**

Open `.github/workflows/ci.yml` and add the following three jobs (preserve the existing indentation level — typically 2 spaces under `jobs:`):

```yaml
  specs-drift:
    name: API specs drift
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - name: Regenerate specs
        run: make generate-specs
      - name: Verify no drift
        run: |
          if ! git diff --exit-code; then
            echo "::error::Generated specs are stale. Run 'make generate-specs' locally and commit the result."
            exit 1
          fi

  specs-lint:
    name: API specs lint
    runs-on: ubuntu-latest
    needs: specs-drift
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
      - name: Run spectral
        run: make lint-specs

  specs-breaking:
    name: API specs breaking-change
    runs-on: ubuntu-latest
    needs: specs-drift
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - name: Install oasdiff
        run: go install github.com/oasdiff/oasdiff@latest
      - name: Run oasdiff
        run: |
          if [[ "${{ contains(github.event.pull_request.labels.*.name, 'breaking-change') }}" == "true" ]]; then
            echo "PR labeled 'breaking-change' — running oasdiff in advisory mode."
            make diff-specs || true
          else
            make diff-specs
          fi
```

- [ ] **Step 3: Validate YAML syntax**

Run:

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
echo "yaml ok: $?"
```

Expected: `yaml ok: 0`.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -s -m "ci: gate API specs on drift, lint, and breaking changes

Adds three CI jobs (specs-drift, specs-lint, specs-breaking) — all
running on every PR that touches services/, tools/specgen/, pkg/api/,
pkg/events/, or deploy/. The breaking-changes job is gated by a
'breaking-change' PR label, matching the spec's policy."
```

---

## Phase 4 — `ratings` exemplar migration

### Task 16: Add `Endpoints` slice and refactor `RegisterRoutes`

**Files:**

- Create: `services/ratings/internal/adapter/inbound/http/endpoints.go`
- Modify: `services/ratings/internal/adapter/inbound/http/handler.go`

- [ ] **Step 1: Create the slice**

Create `services/ratings/internal/adapter/inbound/http/endpoints.go`:

```go
package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/ratings/api/openapi.yaml.
var Endpoints = []api.Endpoint{
	{
		Method:   "GET",
		Path:     "/v1/ratings/{id}",
		Summary:  "Get all ratings for a product",
		Response: ProductRatingsResponse{},
		Errors: []api.ErrorResponse{
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:    "POST",
		Path:      "/v1/ratings",
		Summary:   "Submit a new rating",
		EventName: "rating-submitted",
		Request:   SubmitRatingRequest{},
		Response:  RatingResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
		},
	},
}
```

- [ ] **Step 2: Refactor `RegisterRoutes`**

In `services/ratings/internal/adapter/inbound/http/handler.go`, replace the import block and the `RegisterRoutes` method.

Update the imports to add `pkg/api`:

```go
import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
)
```

Replace the existing `RegisterRoutes` body:

```go
// RegisterRoutes registers the ratings routes on the given mux by looping
// over the Endpoints slice declared in endpoints.go — single source of
// truth for runtime routing and OpenAPI generation.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	api.Register(mux, Endpoints, map[string]http.HandlerFunc{
		"GET /v1/ratings/{id}": h.getProductRatings,
		"POST /v1/ratings":     h.submitRating,
	})
}
```

- [ ] **Step 3: Run existing tests to verify behavior is unchanged**

Run:

```bash
go test ./services/ratings/... -race -count=1 -v
```

Expected: every existing handler test (`TestGetProductRatings_Empty`, `TestSubmitRating_Success`, `TestSubmitRating_InvalidStars`, `TestSubmitRating_EmptyBody`, `TestSubmitAndGet_RoundTrip`, `TestSubmitRating_InvalidJSON`) continues to PASS.

- [ ] **Step 4: Commit**

```bash
git add services/ratings/internal/adapter/inbound/http/endpoints.go services/ratings/internal/adapter/inbound/http/handler.go
git commit -s -m "refactor(ratings): declare Endpoints slice; loop in RegisterRoutes

Routes now derive from a single declarative slice that tools/specgen
will read to generate services/ratings/api/openapi.yaml. Behavior is
identical — every existing handler test still passes."
```

---

### Task 17: Generate ratings artifacts and commit

**Files:**

- Create: `services/ratings/api/openapi.yaml`
- Create: `services/ratings/api/catalog-info.yaml`
- Create: `deploy/ratings/values-generated.yaml`

- [ ] **Step 1: Run the generator**

Run:

```bash
make generate-specs
```

Expected: prints `specgen: ratings OK` and creates `services/ratings/api/openapi.yaml`, `services/ratings/api/catalog-info.yaml`, and `deploy/ratings/values-generated.yaml`. Every other service prints `specgen: <svc> SKIPPED: <reason>` because none have an `endpoints.go` yet — Task 11 Step 4 made the runner tolerant of this.

- [ ] **Step 2: Inspect the generated files**

Run:

```bash
ls -la services/ratings/api/ deploy/ratings/
cat services/ratings/api/openapi.yaml
```

Expected: `openapi.yaml` (~80–120 lines), `catalog-info.yaml` (no `-events` API entity since no kafka adapter), `values-generated.yaml` containing only `cqrs.endpoints.rating-submitted` (no `events.exposed`).

- [ ] **Step 3: Verify the spec validates with spectral**

Run:

```bash
make lint-specs
```

Expected: spectral output ends with `0 problems`.

- [ ] **Step 4: Verify Helm still renders**

Run:

```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-generated.yaml \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo > /tmp/ratings-render.yaml
echo "render exit: $?"
```

Expected: `render exit: 0`. The output should be a valid set of Kubernetes manifests with the `cqrs.endpoints.rating-submitted` block visible (it currently lives in `values-local.yaml` for ratings? Check: `grep -A3 endpoints deploy/ratings/values-local.yaml`. If the existing `values-local.yaml` already has the endpoint hand-written, **remove that block from `values-local.yaml`** in this same step so the source of truth is now `values-generated.yaml`).

- [ ] **Step 5: Commit the generated artifacts and the values-local cleanup**

```bash
git add services/ratings/api deploy/ratings/values-generated.yaml deploy/ratings/values-local.yaml tools/specgen/internal/runner/runner.go
git commit -s -m "feat(ratings): generated openapi.yaml + catalog-info + values-generated

Adds the first set of generated artifacts produced by tools/specgen,
removes the now-duplicated cqrs.endpoints block from
deploy/ratings/values-local.yaml, and switches RunAll to skip-on-error
so unmigrated services do not block the migrated ones."
```

---

### Task 18: Verify ratings deploys end-to-end with dual-`-f`

**Files:** none modified — this is a verification step.

- [ ] **Step 1: Bring up the local k8s cluster (if not already running)**

Run:

```bash
make k8s-status >/dev/null 2>&1 || make run-k8s
```

Expected: cluster ready; ratings pod logs visible.

- [ ] **Step 2: Force a rebuild + redeploy of ratings only**

Run:

```bash
docker build -f build/Dockerfile.ratings -t event-driven-bookinfo/ratings:local .
k3d image import event-driven-bookinfo/ratings:local -c bookinfo-local
helm upgrade --install ratings charts/bookinfo-service \
  --namespace bookinfo \
  -f deploy/ratings/values-generated.yaml \
  -f deploy/ratings/values-local.yaml
kubectl rollout restart deployment/ratings -n bookinfo
kubectl rollout restart deployment/ratings-write -n bookinfo
kubectl rollout status deployment/ratings -n bookinfo --timeout=60s
kubectl rollout status deployment/ratings-write -n bookinfo --timeout=60s
```

Expected: both deployments reach `Available`. No CrashLoopBackOff.

- [ ] **Step 3: Hit the ratings endpoints through the gateway**

Run:

```bash
curl -fsS http://localhost:8080/v1/ratings/product-1 | jq .
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"product_id":"product-1","reviewer":"alice","stars":5}' \
  http://localhost:8080/v1/ratings | jq .
curl -fsS http://localhost:8080/v1/ratings/product-1 | jq .
```

Expected: first GET returns an empty ratings list, POST returns 201 with the new rating, second GET returns the rating with `count: 1`.

- [ ] **Step 4: Verify drift gate**

Run:

```bash
make generate-specs
git diff --exit-code
```

Expected: exit 0 (no drift). If anything is reported, regenerate and commit.

- [ ] **Step 5: Verify lint gate**

Run:

```bash
make lint-specs
```

Expected: spectral reports `0 problems`.

- [ ] **Step 6: Commit a noop documentation tag**

If everything above passes, the foundation is verified end-to-end. No commit is required for this verification step itself — it produces no diff. Move to the follow-up plans for the remaining services.

---

## Follow-up plans (out of scope)

After this plan lands and `ratings` is verified in production-like local k8s, write one plan per service for the remaining migrations. Each follows the exact same shape as Phase 4 (Tasks 16–18) with these substitutions:

| Service        | Phase 2 task | Phase 3 task | Notes                                                                              |
|----------------|--------------|--------------|------------------------------------------------------------------------------------|
| `ingestion`    | add `exposed.go`, refactor producer to `Publish(ctx, descriptor, payload)`, export `BookAddedEvent` | drop hardcoded `ceType`/`ceSource`/`ceVersion` consts; the descriptor is the source of truth | producer-only (no `endpoints.go`) — only AsyncAPI and `events.exposed` block emitted |
| `details`      | both halves: `endpoints.go` for POST/GET routes, `exposed.go` for `book-added` | the consumed `raw-books-details` block stays in `values-local.yaml` (not generated) | first migration to exercise both sides + values deep-merge |
| `reviews`      | both halves                                                                                 |                                                                                | similar to details                                                                  |
| `notification` | `endpoints.go` only (no events emitted)                                                     |                                                                                |                                                                                     |
| `dlqueue`      | `endpoints.go` only                                                                         |                                                                                | uses `stdhttp` aliased import in handler — preserve the alias                       |
| `productpage`  | `endpoints.go` with `Response: nil` and a documented `text/html` content type               |                                                                                | OpenAPI doc records `responses.200.content` keyed by `text/html` with no schema     |

For each follow-up plan, the steps are: refactor handler → add slices → run `make generate-specs` → commit artifacts → strip the duplicate hand-written block from `values-local.yaml` → verify in local k8s.

---

## Plan self-review

**Spec coverage:**

| Spec section                                                | Plan task |
|-------------------------------------------------------------|-----------|
| `pkg/api` Endpoint + Register                               | 1         |
| `pkg/events` Descriptor + ResolveExposureKey                | 2         |
| `tools/specgen` CLI scaffolding                             | 3         |
| Walker (`Endpoints`)                                        | 4         |
| Walker (`Exposed`)                                          | 5         |
| JSON-Schema from compile-time types                         | 6         |
| OpenAPI 3.1 builder                                         | 7         |
| AsyncAPI 3.0 builder                                        | 8         |
| Backstage `catalog-info.yaml` emitter                       | 9         |
| `values-generated.yaml` emitter (cqrs + events.exposed)     | 10        |
| `specgen all` orchestration                                 | 11        |
| `specgen lint` (spectral) + `.spectral.yaml`                | 12        |
| `specgen diff` (oasdiff)                                    | 13        |
| Makefile targets + dual-`-f` helm wiring                    | 14        |
| CI: drift, lint, breaking-change                            | 15        |
| `ratings` migration exemplar                                | 16–18     |
| Other-service migrations                                    | follow-up plans |

All spec sections are covered.

**Type / signature consistency check:** `Endpoint`, `ErrorResponse`, `Descriptor`, `EndpointInfo`, `DescriptorInfo`, `ErrorInfo` — names are consistent across all tasks. The walker types (`*Info`) are deliberately distinct from the runtime types (`Endpoint`, `Descriptor`) because the walker handles compile-time `*types.Named` references rather than runtime `any` values.

**Placeholder scan:** No `TBD`, `TODO`, "fill in details", or "similar to Task N". Every step that produces code shows the code; every command that runs is exact.

**Scope check:** This plan delivers working software end-to-end (`ratings` running on k8s with generated specs validating in CI). Subsequent migrations are explicitly marked out of scope and listed as follow-up plans, each producing working software on its own.
