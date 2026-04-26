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
	Topic       string
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
		Dir:   moduleDir,
		Tests: false,
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
						return nil, fmt.Errorf("exposed slice has no initializer")
					}
					out, err := parseDescriptorSlice(pkg, vs.Values[i])
					if err != nil {
						return nil, fmt.Errorf("extracting Exposed: %w", err)
					}
					return out, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("exposed slice not found in %s", pkg.PkgPath)
}

func parseDescriptorSlice(pkg *packages.Package, expr ast.Expr) ([]DescriptorInfo, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("exposed slice initializer is not a composite literal")
	}
	out := make([]DescriptorInfo, 0, len(cl.Elts))
	for i, elt := range cl.Elts {
		ec, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("exposed[%d] is not a composite literal", i)
		}
		d, err := parseDescriptorFields(pkg, ec)
		if err != nil {
			return nil, fmt.Errorf("exposed[%d]: %w", i, err)
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
			return d, fmt.Errorf("descriptor field must use Key: Value form")
		}
		name, ok := kv.Key.(*ast.Ident)
		if !ok {
			return d, fmt.Errorf("descriptor field key is not an identifier")
		}
		if err := setDescriptorField(&d, pkg, name.Name, kv.Value); err != nil {
			return d, err
		}
	}
	return d, nil
}

// ConsumedInfo is the walker's view of one element in
// `var Consumed []events.ConsumedDescriptor`.
type ConsumedInfo struct {
	Name            string
	SourceService   string
	SourceEventName string
	CEType          string
	Description     string
}

// LoadConsumed loads the package at moduleDir/<importPath> and returns
// its `var Consumed []events.ConsumedDescriptor` contents.
// Returns (nil, nil) when the package has no Consumed slice (most
// services don't consume events).
func LoadConsumed(moduleDir, importPath string) ([]ConsumedInfo, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps |
			packages.NeedImports,
		Dir:   moduleDir,
		Tests: false,
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
					if name.Name != "Consumed" {
						continue
					}
					if i >= len(vs.Values) {
						return nil, fmt.Errorf("consumed slice has no initializer")
					}
					out, err := parseConsumedSlice(vs.Values[i])
					if err != nil {
						return nil, fmt.Errorf("extracting Consumed: %w", err)
					}
					return out, nil
				}
			}
		}
	}
	return nil, nil // No Consumed slice — that's fine
}

func parseConsumedSlice(expr ast.Expr) ([]ConsumedInfo, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("consumed slice initializer is not a composite literal")
	}
	out := make([]ConsumedInfo, 0, len(cl.Elts))
	for i, elt := range cl.Elts {
		ec, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("consumed[%d] is not a composite literal", i)
		}
		c, err := parseConsumedFields(ec)
		if err != nil {
			return nil, fmt.Errorf("consumed[%d]: %w", i, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func parseConsumedFields(cl *ast.CompositeLit) (ConsumedInfo, error) {
	var c ConsumedInfo
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return c, fmt.Errorf("ConsumedDescriptor field must use Key: Value form")
		}
		name, ok := kv.Key.(*ast.Ident)
		if !ok {
			return c, fmt.Errorf("ConsumedDescriptor field key is not an identifier")
		}
		s, err := stringLit(kv.Value)
		if err != nil {
			return c, fmt.Errorf("field %s: %w", name.Name, err)
		}
		switch name.Name {
		case "Name":
			c.Name = s
		case "SourceService":
			c.SourceService = s
		case "SourceEventName":
			c.SourceEventName = s
		case "CEType":
			c.CEType = s
		case "Description":
			c.Description = s
		}
	}
	return c, nil
}

func setDescriptorStringField(d *DescriptorInfo, fieldName string, value ast.Expr) error {
	s, err := stringLit(value)
	if err != nil {
		return err
	}
	switch fieldName {
	case "Name":
		d.Name = s
	case "ExposureKey":
		d.ExposureKey = s
	case "Topic":
		d.Topic = s
	case "CEType":
		d.CEType = s
	case "CESource":
		d.CESource = s
	case "Version":
		d.Version = s
	case "ContentType":
		d.ContentType = s
	case "Description":
		d.Description = s
	}
	return nil
}

func setDescriptorField(d *DescriptorInfo, pkg *packages.Package, fieldName string, value ast.Expr) error {
	switch fieldName {
	case "Name", "ExposureKey", "Topic", "CEType", "CESource", "Version", "ContentType", "Description":
		return setDescriptorStringField(d, fieldName, value)
	case "Payload":
		t, err := namedType(pkg, value)
		if err != nil {
			return fmt.Errorf("payload: %w", err)
		}
		d.PayloadType = t
	}
	return nil
}
