// Package yamlutil contains helpers for building deterministic *yaml.Node
// trees that downstream packages (openapi, asyncapi) marshal into final
// YAML documents.
//
// Determinism is critical because the generated specs are committed and
// CI gates on `make generate-specs && git diff --exit-code`. Map iteration
// in Go is randomized; this package centralizes the sort-then-emit pattern
// so every spec generator gets the same guarantee.
package yamlutil

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/jsonschema"
)

// Mapping returns an empty YAML mapping node.
func Mapping() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

// AddScalar appends a key/scalar-value pair to a mapping node.
func AddScalar(parent *yaml.Node, key, value string) {
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// AddMapping appends a key/mapping-value pair to a mapping node.
func AddMapping(parent *yaml.Node, key string, value *yaml.Node) {
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

// AnyToNode converts a map[string]any (recursively) to a *yaml.Node with
// sorted keys. Supports: map[string]any, string, bool, int, and nil.
func AnyToNode(v any) (*yaml.Node, error) {
	if v == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		// Only map[string]any is expected.
		node := Mapping()
		keys := make([]string, 0, rv.Len())
		for _, k := range rv.MapKeys() {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		for _, k := range keys {
			valNode, err := AnyToNode(rv.MapIndex(reflect.ValueOf(k)).Interface())
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
			elemNode, err := AnyToNode(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			seqNode.Content = append(seqNode.Content, elemNode)
		}
		return seqNode, nil
	default:
		return nil, fmt.Errorf("AnyToNode: unsupported type %T", v)
	}
}

// SchemaToNode converts a *jsonschema.Schema to a *yaml.Node with sorted
// property keys, so the output is deterministic regardless of map iteration.
func SchemaToNode(s *jsonschema.Schema) (*yaml.Node, error) {
	if s == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	node := Mapping()

	if s.Type != "" {
		AddScalar(node, "type", s.Type)
	}
	if s.Format != "" {
		AddScalar(node, "format", s.Format)
	}
	if s.Description != "" {
		AddScalar(node, "description", s.Description)
	}
	if len(s.Properties) > 0 {
		propsNode := Mapping()
		keys := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			propNode, err := SchemaToNode(s.Properties[k])
			if err != nil {
				return nil, fmt.Errorf("property %q: %w", k, err)
			}
			AddMapping(propsNode, k, propNode)
		}
		AddMapping(node, "properties", propsNode)
	}
	if len(s.Required) > 0 {
		seqNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, r := range s.Required {
			seqNode.Content = append(seqNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: r},
			)
		}
		AddMapping(node, "required", seqNode)
	}
	if s.Items != nil {
		itemNode, err := SchemaToNode(s.Items)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		AddMapping(node, "items", itemNode)
	}
	return node, nil
}
