package constdata

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
)

// ArrayConstant holds the name and values of a constant array
type ArrayConstant struct {
	Name   string  // Array name (e.g., "test_data")
	Values []int64 // Constant values
	Type   string  // Element type (e.g., "int32")
	Length int     // Array length
}

// ExtractConstants extracts constant array initializers from a Go source file
func ExtractConstants(filePath string) ([]ArrayConstant, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read source file: %w", err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse source file: %w", err)
	}

	constants := []ArrayConstant{}

	ast.Inspect(f, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok {
			return true
		}
		if genDecl.Tok != token.VAR {
			return true
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Names) == 0 {
				continue
			}

			name := valueSpec.Names[0].Name
			if len(valueSpec.Values) == 0 {
				continue
			}

			// Check if this is an array literal with constant values
			compositeLit, ok := valueSpec.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}

			elemType := "int32" // default
			if valueSpec.Type != nil {
				if arrayType, ok := valueSpec.Type.(*ast.ArrayType); ok {
					if ident, ok := arrayType.Elt.(*ast.Ident); ok {
						elemType = ident.Name
					}
				} else if ident, ok := valueSpec.Type.(*ast.Ident); ok {
					elemType = ident.Name
				}
			}

			// Extract constant values
			values := []int64{}
			for _, elt := range compositeLit.Elts {
				val, err := extractConstantValue(elt)
				if err != nil {
					return false
				}
				values = append(values, val)
			}

			if len(values) > 0 && name == "test_data" {
				constants = append(constants, ArrayConstant{
					Name:   name,
					Values: values,
					Type:   elemType,
					Length: len(values),
				})
			}
		}
		return true
	})

	return constants, nil
}

// extractConstantValue extracts a constant integer value from an AST expression
func extractConstantValue(expr ast.Expr) (int64, error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.INT {
			// Handle hex (0x) and decimal formats
			val, err := strconv.ParseInt(e.Value, 0, 64)
			if err != nil {
				return 0, fmt.Errorf("parse int constant: %w", err)
			}
			return val, nil
		}
	case *ast.UnaryExpr:
		if e.Op == token.SUB {
			val, err := extractConstantValue(e.X)
			if err != nil {
				return 0, err
			}
			return -val, nil
		}
	}
	return 0, fmt.Errorf("not a constant value")
}
