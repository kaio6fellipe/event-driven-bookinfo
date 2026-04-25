// Package openapi builds OpenAPI 3.1 YAML from walker output.
package openapi

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

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
	// Collect schemas keyed by type name so we can sort them later.
	schemas := map[string]*jsonschema.Schema{}

	// pathItems[path][method] = operation node.
	type operationEntry struct {
		path   string
		method string
		op     map[string]any
	}
	// Use a slice to collect paths in declaration order; sort before encoding.
	type pathEntry struct {
		path    string
		methods map[string]map[string]any // method -> operation fields
	}
	pathMap := map[string]*pathEntry{}
	pathOrder := []string{}

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
						"schema": map[string]any{
							"$ref": "#/components/schemas/" + ep.ResponseType.Obj().Name(),
						},
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

		if _, exists := pathMap[ep.Path]; !exists {
			pathMap[ep.Path] = &pathEntry{path: ep.Path, methods: map[string]map[string]any{}}
			pathOrder = append(pathOrder, ep.Path)
		}
		pathMap[ep.Path].methods[strings.ToLower(ep.Method)] = op
	}

	// Build the YAML document using *yaml.Node for deterministic key ordering.
	docNode := mappingNode()

	// openapi field
	addScalar(docNode, "openapi", "3.1.0")

	// info
	infoNode := mappingNode()
	addScalar(infoNode, "title", in.ServiceName)
	addScalar(infoNode, "version", in.Version)
	addMapping(docNode, "info", infoNode)

	// paths — sorted alphabetically
	sort.Strings(pathOrder)
	pathsNode := mappingNode()
	for _, p := range pathOrder {
		entry := pathMap[p]
		pathItemNode := mappingNode()

		// Sort methods for determinism
		methods := make([]string, 0, len(entry.methods))
		for m := range entry.methods {
			methods = append(methods, m)
		}
		sort.Strings(methods)

		for _, method := range methods {
			opNode, err := anyToNode(entry.methods[method])
			if err != nil {
				return nil, fmt.Errorf("encoding operation %s %s: %w", method, p, err)
			}
			addMapping(pathItemNode, method, opNode)
		}
		addMapping(pathsNode, p, pathItemNode)
	}
	addMapping(docNode, "paths", pathsNode)

	// components.schemas — sorted alphabetically
	schemaKeys := make([]string, 0, len(schemas))
	for k := range schemas {
		schemaKeys = append(schemaKeys, k)
	}
	sort.Strings(schemaKeys)

	schemasNode := mappingNode()
	for _, k := range schemaKeys {
		sn, err := schemaToNode(schemas[k])
		if err != nil {
			return nil, fmt.Errorf("encoding schema %s: %w", k, err)
		}
		addMapping(schemasNode, k, sn)
	}
	componentsNode := mappingNode()
	addMapping(componentsNode, "schemas", schemasNode)
	addMapping(docNode, "components", componentsNode)

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

// mappingNode returns an empty YAML mapping node.
func mappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

// addScalar appends a key/scalar-value pair to a mapping node.
func addScalar(parent *yaml.Node, key, value string) {
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// addMapping appends a key/mapping-value pair to a mapping node.
func addMapping(parent *yaml.Node, key string, value *yaml.Node) {
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

// anyToNode converts a map[string]any (recursively) to a *yaml.Node with
// sorted keys. Supports: map[string]any, string, bool, int, and nil.
func anyToNode(v any) (*yaml.Node, error) {
	if v == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		// Only map[string]any is expected.
		node := mappingNode()
		keys := make([]string, 0, rv.Len())
		for _, k := range rv.MapKeys() {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		for _, k := range keys {
			valNode, err := anyToNode(rv.MapIndex(reflect.ValueOf(k)).Interface())
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
				valNode,
			)
		}
		return node, nil
	case reflect.String:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: rv.String()}, nil
	case reflect.Bool:
		val := "false"
		if rv.Bool() {
			val = "true"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: val}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(rv.Int(), 10)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatUint(rv.Uint(), 10)}, nil
	case reflect.Slice:
		seqNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for i := 0; i < rv.Len(); i++ {
			elemNode, err := anyToNode(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			seqNode.Content = append(seqNode.Content, elemNode)
		}
		return seqNode, nil
	default:
		return nil, fmt.Errorf("anyToNode: unsupported type %T", v)
	}
}

// schemaToNode converts a *jsonschema.Schema to a *yaml.Node with sorted
// property keys, so the output is deterministic regardless of map iteration.
func schemaToNode(s *jsonschema.Schema) (*yaml.Node, error) {
	if s == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	node := mappingNode()

	if s.Type != "" {
		addScalar(node, "type", s.Type)
	}
	if s.Format != "" {
		addScalar(node, "format", s.Format)
	}
	if s.Description != "" {
		addScalar(node, "description", s.Description)
	}
	if len(s.Properties) > 0 {
		propsNode := mappingNode()
		keys := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			propNode, err := schemaToNode(s.Properties[k])
			if err != nil {
				return nil, fmt.Errorf("property %q: %w", k, err)
			}
			addMapping(propsNode, k, propNode)
		}
		addMapping(node, "properties", propsNode)
	}
	if len(s.Required) > 0 {
		seqNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, r := range s.Required {
			seqNode.Content = append(seqNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: r},
			)
		}
		addMapping(node, "required", seqNode)
	}
	if s.Items != nil {
		itemNode, err := schemaToNode(s.Items)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		addMapping(node, "items", itemNode)
	}
	return node, nil
}
