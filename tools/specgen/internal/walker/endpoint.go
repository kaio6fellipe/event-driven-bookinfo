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
		return nil, fmt.Errorf("endpoints var not found in %s", pkg.PkgPath)
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
						return nil, fmt.Errorf("endpoints has no initializer")
					}
					return parseEndpointSlice(pkg, vs.Values[i])
				}
			}
		}
	}
	return nil, fmt.Errorf("endpoints AST node not found")
}

func parseEndpointSlice(pkg *packages.Package, expr ast.Expr) ([]EndpointInfo, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("endpoints initializer is not a composite literal")
	}
	out := make([]EndpointInfo, 0, len(cl.Elts))
	for i, elt := range cl.Elts {
		ec, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("endpoints[%d] is not a composite literal", i)
		}
		ep, err := parseEndpointFields(pkg, ec)
		if err != nil {
			return nil, fmt.Errorf("endpoints[%d]: %w", i, err)
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
			return ep, fmt.Errorf("endpoint field must use Key: Value form")
		}
		name, ok := kv.Key.(*ast.Ident)
		if !ok {
			return ep, fmt.Errorf("endpoint field key is not an identifier")
		}
		if err := setEndpointField(&ep, pkg, name.Name, kv.Value); err != nil {
			return ep, err
		}
	}
	return ep, nil
}

func setEndpointStringField(ep *EndpointInfo, fieldName string, value ast.Expr) error {
	s, err := stringLit(value)
	if err != nil {
		return err
	}
	switch fieldName {
	case "Method":
		ep.Method = s
	case "Path":
		ep.Path = s
	case "Summary":
		ep.Summary = s
	case "EventName":
		ep.EventName = s
	}
	return nil
}

func setEndpointField(ep *EndpointInfo, pkg *packages.Package, fieldName string, value ast.Expr) error {
	switch fieldName {
	case "Method", "Path", "Summary", "EventName":
		return setEndpointStringField(ep, fieldName, value)
	case "Request":
		t, err := namedType(pkg, value)
		if err != nil {
			return fmt.Errorf("request: %w", err)
		}
		ep.RequestType = t
	case "Response":
		named, isSlice, err := typeInfo(pkg, value)
		if err != nil {
			return fmt.Errorf("response: %w", err)
		}
		ep.ResponseType = named
		ep.ResponseIsSlice = isSlice
	case "Errors":
		errs, err := parseErrorSlice(pkg, value)
		if err != nil {
			return fmt.Errorf("errors: %w", err)
		}
		ep.Errors = errs
	}
	return nil
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
				return nil, fmt.Errorf("errorResponse field must use Key: Value form")
			}
			name, ok := kv.Key.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("errorResponse field key is not an identifier")
			}
			switch name.Name {
			case "Status":
				bl, ok := kv.Value.(*ast.BasicLit)
				if !ok {
					return nil, fmt.Errorf("status must be an int literal")
				}
				n, err := strconv.Atoi(bl.Value)
				if err != nil {
					return nil, fmt.Errorf("status: %w", err)
				}
				info.Status = n
			case "Type":
				t, err := namedType(pkg, kv.Value)
				if err != nil {
					return nil, fmt.Errorf("type: %w", err)
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

// typeInfo resolves a composite literal to its named element type and whether
// it represents a slice (e.g. `[]Foo{}`). Returns (named, isSlice, error).
// For a plain struct literal `Foo{}`, isSlice is false.
// For a slice literal `[]Foo{}`, isSlice is true and named is the element type.
func typeInfo(pkg *packages.Package, expr ast.Expr) (*types.Named, bool, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, false, fmt.Errorf("not a composite literal")
	}
	tv, ok := pkg.TypesInfo.Types[cl]
	if !ok {
		return nil, false, fmt.Errorf("no type info for composite literal")
	}
	switch t := tv.Type.(type) {
	case *types.Named:
		return t, false, nil
	case *types.Slice:
		named, ok := t.Elem().(*types.Named)
		if !ok {
			return nil, false, fmt.Errorf("slice element type is not a named type")
		}
		return named, true, nil
	default:
		return nil, false, fmt.Errorf("composite literal type %T is not a named type or slice of named types", tv.Type)
	}
}
