// Package openapi builds OpenAPI 3.1 YAML from walker output.
package openapi

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/jsonschema"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/yamlutil"
)

// Input holds everything Build needs.
type Input struct {
	ServiceName string
	Version     string
	Endpoints   []walker.EndpointInfo
}

// pathEntry holds all methods collected for one URL path.
type pathEntry struct {
	path    string
	methods map[string]map[string]any // method -> operation fields
}

// buildOperation constructs the operation object for one endpoint and
// accumulates any referenced schemas into the provided map.
func buildOperation(ep walker.EndpointInfo, schemas map[string]*jsonschema.Schema) (map[string]any, error) {
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
					"schema": map[string]any{
						"$ref": "#/components/schemas/" + ep.RequestType.Obj().Name(),
					},
				},
			},
		}
	}

	responses := op["responses"].(map[string]any)
	successStatus := strconv.Itoa(http.StatusOK)
	if ep.Method == http.MethodPost {
		successStatus = strconv.Itoa(http.StatusCreated)
	}
	if ep.SuccessStatus != 0 {
		successStatus = strconv.Itoa(ep.SuccessStatus)
	}
	if ep.ResponseType != nil {
		s, err := jsonschema.SchemaFromType(ep.ResponseType)
		if err != nil {
			return nil, fmt.Errorf("response schema for %s %s: %w", ep.Method, ep.Path, err)
		}
		schemas[ep.ResponseType.Obj().Name()] = s
		ref := "#/components/schemas/" + ep.ResponseType.Obj().Name()
		var responseSchema map[string]any
		if ep.ResponseIsSlice {
			responseSchema = map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": ref},
			}
		} else {
			responseSchema = map[string]any{"$ref": ref}
		}
		responses[successStatus] = map[string]any{
			"description": "success",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": responseSchema,
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
						"schema": map[string]any{
							"$ref": "#/components/schemas/" + errInfo.Type.Obj().Name(),
						},
					},
				},
			}
		}
	}
	return op, nil
}

// buildPathItemNode encodes one URL path entry (parameters + methods) into a
// YAML mapping node.
func buildPathItemNode(p string, entry *pathEntry) (*yaml.Node, error) {
	pathItemNode := yamlutil.Mapping()

	// Emit path-level parameters for each {param} in the path template.
	params := extractPathParams(p)
	if len(params) > 0 {
		paramNodes := make([]any, 0, len(params))
		for _, param := range params {
			paramNodes = append(paramNodes, map[string]any{
				"name":     param,
				"in":       "path",
				"required": true,
				"schema":   map[string]any{"type": "string"},
			})
		}
		paramNode, err := yamlutil.AnyToNode(paramNodes)
		if err != nil {
			return nil, fmt.Errorf("encoding parameters for %s: %w", p, err)
		}
		yamlutil.AddMapping(pathItemNode, "parameters", paramNode)
	}

	// Sort methods for determinism.
	methods := make([]string, 0, len(entry.methods))
	for m := range entry.methods {
		methods = append(methods, m)
	}
	sort.Strings(methods)

	for _, method := range methods {
		opNode, err := yamlutil.AnyToNode(entry.methods[method])
		if err != nil {
			return nil, fmt.Errorf("encoding operation %s %s: %w", method, p, err)
		}
		yamlutil.AddMapping(pathItemNode, method, opNode)
	}
	return pathItemNode, nil
}

// buildSchemasNode encodes the components.schemas mapping sorted alphabetically.
func buildSchemasNode(schemas map[string]*jsonschema.Schema) (*yaml.Node, error) {
	schemaKeys := make([]string, 0, len(schemas))
	for k := range schemas {
		schemaKeys = append(schemaKeys, k)
	}
	sort.Strings(schemaKeys)

	schemasNode := yamlutil.Mapping()
	for _, k := range schemaKeys {
		sn, err := yamlutil.SchemaToNode(schemas[k])
		if err != nil {
			return nil, fmt.Errorf("encoding schema %s: %w", k, err)
		}
		yamlutil.AddMapping(schemasNode, k, sn)
	}
	return schemasNode, nil
}

// Build returns the YAML bytes of the OpenAPI 3.1 document.
func Build(in Input) ([]byte, error) {
	// Collect schemas keyed by type name so we can sort them later.
	schemas := map[string]*jsonschema.Schema{}

	pathMap := map[string]*pathEntry{}
	pathOrder := []string{}

	for _, ep := range in.Endpoints {
		op, err := buildOperation(ep, schemas)
		if err != nil {
			return nil, err
		}
		if _, exists := pathMap[ep.Path]; !exists {
			pathMap[ep.Path] = &pathEntry{path: ep.Path, methods: map[string]map[string]any{}}
			pathOrder = append(pathOrder, ep.Path)
		}
		pathMap[ep.Path].methods[strings.ToLower(ep.Method)] = op
	}

	// Build the YAML document using *yaml.Node for deterministic key ordering.
	docNode := yamlutil.Mapping()
	yamlutil.AddScalar(docNode, "openapi", "3.1.0")

	infoNode := yamlutil.Mapping()
	yamlutil.AddScalar(infoNode, "title", in.ServiceName)
	yamlutil.AddScalar(infoNode, "version", in.Version)
	yamlutil.AddMapping(docNode, "info", infoNode)

	// paths — sorted alphabetically
	sort.Strings(pathOrder)
	pathsNode := yamlutil.Mapping()
	for _, p := range pathOrder {
		pathItemNode, err := buildPathItemNode(p, pathMap[p])
		if err != nil {
			return nil, err
		}
		yamlutil.AddMapping(pathsNode, p, pathItemNode)
	}
	yamlutil.AddMapping(docNode, "paths", pathsNode)

	schemasNode, err := buildSchemasNode(schemas)
	if err != nil {
		return nil, err
	}
	componentsNode := yamlutil.Mapping()
	yamlutil.AddMapping(componentsNode, "schemas", schemasNode)
	yamlutil.AddMapping(docNode, "components", componentsNode)

	root := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{docNode}}

	var buf bytes.Buffer
	buf.WriteString("# DO NOT EDIT — generated by tools/specgen.\n")
	buf.WriteString("# Source: services/" + in.ServiceName + "/internal/adapter/inbound/http/endpoints.go\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("closing YAML encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// extractPathParams returns the names of all {param} segments in a path
// template, preserving left-to-right order and deduplicating.
func extractPathParams(path string) []string {
	var params []string
	seen := map[string]bool{}
	for _, segment := range strings.Split(path, "/") {
		if len(segment) >= 3 && segment[0] == '{' && segment[len(segment)-1] == '}' {
			name := segment[1 : len(segment)-1]
			if !seen[name] {
				params = append(params, name)
				seen[name] = true
			}
		}
	}
	return params
}
