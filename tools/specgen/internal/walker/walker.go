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
