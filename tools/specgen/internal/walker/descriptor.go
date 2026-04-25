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
						return nil, fmt.Errorf("Exposed has no initializer")
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
