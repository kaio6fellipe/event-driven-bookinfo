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
	Metadata    SpecMetadata // org-wide info.contact/license/servers; injected by runner
}

// SpecMetadata is the subset of runner.SpecMetadata the openapi builder needs.
// Defined here (not imported from runner) to avoid an import cycle.
type SpecMetadata struct {
	OrgName       string
	OrgURL        string
	OrgEmail      string
	LicenseName   string
	LicenseURL    string
	OpenAPIServer ServerEntry
}

// ServerEntry mirrors runner.ServerEntry.
type ServerEntry struct {
	URL         string
	Description string
}

// pathEntry holds all methods collected for one URL path.
type pathEntry struct {
	path    string
	methods map[string]map[string]any // method -> operation fields
}

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

// buildTagsNode returns a YAML sequence node of {name: tag} objects for all
// unique tag names in endpoints (sorted alphabetically). Returns nil when the
// endpoints slice is empty so the caller can skip emitting the field.
func buildTagsNode(endpoints []walker.EndpointInfo, serviceName string) (*yaml.Node, error) {
	tagSet := map[string]struct{}{}
	for _, ep := range endpoints {
		tags := ep.Tags
		if len(tags) == 0 {
			tags = []string{serviceName}
		}
		for _, t := range tags {
			tagSet[t] = struct{}{}
		}
	}
	if len(tagSet) == 0 {
		return nil, nil
	}
	tagNames := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tagNames = append(tagNames, t)
	}
	sort.Strings(tagNames)
	tagEntries := make([]any, 0, len(tagNames))
	for _, t := range tagNames {
		tagEntries = append(tagEntries, map[string]any{"name": t})
	}
	return yamlutil.AnyToNode(tagEntries)
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
		op, err := buildOperation(ep, in.ServiceName, schemas)
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
	yamlutil.AddScalar(infoNode, "description", in.ServiceName+" — generated by tools/specgen")

	contactNode := yamlutil.Mapping()
	yamlutil.AddScalar(contactNode, "name", in.Metadata.OrgName)
	yamlutil.AddScalar(contactNode, "url", in.Metadata.OrgURL)
	yamlutil.AddScalar(contactNode, "email", in.Metadata.OrgEmail)
	yamlutil.AddMapping(infoNode, "contact", contactNode)

	licenseNode := yamlutil.Mapping()
	yamlutil.AddScalar(licenseNode, "name", in.Metadata.LicenseName)
	yamlutil.AddScalar(licenseNode, "url", in.Metadata.LicenseURL)
	yamlutil.AddMapping(infoNode, "license", licenseNode)

	yamlutil.AddMapping(docNode, "info", infoNode)

	// servers — must come AFTER info, BEFORE paths per OpenAPI 3.1 conventions.
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

	// tags — top-level array required by spectral operation-tag-defined rule.
	tagsNode, err := buildTagsNode(in.Endpoints, in.ServiceName)
	if err != nil {
		return nil, fmt.Errorf("encoding top-level tags: %w", err)
	}
	if tagsNode != nil {
		yamlutil.AddMapping(docNode, "tags", tagsNode)
	}

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
		segment = strings.TrimPrefix(segment, "{")
		segment = strings.TrimSuffix(segment, "}")
		if segment == "" {
			continue
		}
		b.WriteString(strings.ToUpper(segment[:1]))
		if len(segment) > 1 {
			b.WriteString(segment[1:])
		}
	}
	return b.String()
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
