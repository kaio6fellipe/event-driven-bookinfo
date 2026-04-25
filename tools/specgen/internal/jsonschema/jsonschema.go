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
