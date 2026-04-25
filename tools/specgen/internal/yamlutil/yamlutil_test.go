package yamlutil_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/jsonschema"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/yamlutil"
)

func TestMapping_ReturnsMappingNode(t *testing.T) {
	t.Parallel()
	node := yamlutil.Mapping()
	if node.Kind != yaml.MappingNode {
		t.Errorf("got Kind=%v, want MappingNode", node.Kind)
	}
	if len(node.Content) != 0 {
		t.Errorf("got %d children, want 0", len(node.Content))
	}
}

func TestAddScalar_AppendsKeyValuePair(t *testing.T) {
	t.Parallel()
	node := yamlutil.Mapping()
	yamlutil.AddScalar(node, "key", "value")
	if len(node.Content) != 2 {
		t.Fatalf("got %d children, want 2", len(node.Content))
	}
	if node.Content[0].Value != "key" {
		t.Errorf("key node value = %q, want %q", node.Content[0].Value, "key")
	}
	if node.Content[0].Kind != yaml.ScalarNode {
		t.Errorf("key node kind = %v, want ScalarNode", node.Content[0].Kind)
	}
	if node.Content[1].Value != "value" {
		t.Errorf("value node value = %q, want %q", node.Content[1].Value, "value")
	}
	if node.Content[1].Kind != yaml.ScalarNode {
		t.Errorf("value node kind = %v, want ScalarNode", node.Content[1].Kind)
	}
}

func TestAddMapping_AppendsKeyAndNestedNode(t *testing.T) {
	t.Parallel()
	parent := yamlutil.Mapping()
	child := yamlutil.Mapping()
	yamlutil.AddScalar(child, "nested-key", "nested-value")
	yamlutil.AddMapping(parent, "section", child)

	if len(parent.Content) != 2 {
		t.Fatalf("got %d children, want 2", len(parent.Content))
	}
	if parent.Content[0].Value != "section" {
		t.Errorf("key = %q, want %q", parent.Content[0].Value, "section")
	}
	if parent.Content[1] != child {
		t.Error("value node is not the child node that was passed")
	}
}

func TestAnyToNode_SortsMapKeysAlphabetically(t *testing.T) {
	t.Parallel()
	input := map[string]any{
		"z": "last",
		"a": "first",
		"m": "middle",
	}
	node, err := yamlutil.AnyToNode(input)
	if err != nil {
		t.Fatalf("AnyToNode: %v", err)
	}
	if node.Kind != yaml.MappingNode {
		t.Fatalf("got Kind=%v, want MappingNode", node.Kind)
	}
	// MappingNode Content: key, value, key, value, key, value
	if len(node.Content) != 6 {
		t.Fatalf("got %d content nodes, want 6", len(node.Content))
	}
	wantKeys := []string{"a", "m", "z"}
	for i, want := range wantKeys {
		got := node.Content[i*2].Value
		if got != want {
			t.Errorf("key[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestAnyToNode_HandlesScalarTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   any
		wantTag string
		wantVal string
	}{
		{"string", "hello", "!!str", "hello"},
		{"bool true", true, "!!bool", "true"},
		{"bool false", false, "!!bool", "false"},
		{"int", 42, "!!int", "42"},
		{"nil", nil, "!!null", "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			node, err := yamlutil.AnyToNode(tt.input)
			if err != nil {
				t.Fatalf("AnyToNode(%v): %v", tt.input, err)
			}
			if node.Tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", node.Tag, tt.wantTag)
			}
			if node.Value != tt.wantVal {
				t.Errorf("value = %q, want %q", node.Value, tt.wantVal)
			}
		})
	}
}

func TestSchemaToNode_SortsPropertiesAlphabetically(t *testing.T) {
	t.Parallel()
	s := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"zebra": {Type: "string"},
			"apple": {Type: "integer"},
			"mango": {Type: "boolean"},
		},
	}
	node, err := yamlutil.SchemaToNode(s)
	if err != nil {
		t.Fatalf("SchemaToNode: %v", err)
	}
	if node.Kind != yaml.MappingNode {
		t.Fatalf("got Kind=%v, want MappingNode", node.Kind)
	}

	// Find the "properties" key in the node content.
	var propsNode *yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "properties" {
			propsNode = node.Content[i+1]
			break
		}
	}
	if propsNode == nil {
		t.Fatal("properties key not found in schema node")
	}

	// Properties are key/value pairs; keys should be sorted.
	wantOrder := []string{"apple", "mango", "zebra"}
	for i, want := range wantOrder {
		idx := i * 2
		if idx >= len(propsNode.Content) {
			t.Fatalf("propsNode has fewer children than expected (want key at index %d)", idx)
		}
		got := propsNode.Content[idx].Value
		if got != want {
			t.Errorf("property key[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestSchemaToNode_NilReturnsNullNode(t *testing.T) {
	t.Parallel()
	node, err := yamlutil.SchemaToNode(nil)
	if err != nil {
		t.Fatalf("SchemaToNode(nil): %v", err)
	}
	if node.Tag != "!!null" {
		t.Errorf("tag = %q, want !!null", node.Tag)
	}
}
