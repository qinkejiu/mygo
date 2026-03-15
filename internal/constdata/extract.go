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
	Name   string      // Array name (e.g., "test_data")
	Values []int64     // Constant values
	Type   string      // Element type (e.g., "int32")
	Length int         // Array length
}

// ExtractConstants extracts constant array initializers from a Go source file
func ExtractConstants(filePath string) ([]ArrayConstant, error) {
	// Debug output
	fmt.Fprintf(os.Stderr, "[DEBUG] ExtractConstants called for: %s\n", filePath)

	// Read the source file
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read source file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[DEBUG] Read %d bytes from source\n", len(src))

	// Parse the source file - use parser.AllErrors to get more information
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse source file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[DEBUG] Parsed %d declarations\n", len(f.Decls))

	constants := []ArrayConstant{}
	genDeclCount := 0
	valueSpecCount := 0
	compositeLitCount := 0
	arrayTypeCount := 0

	// Walk the AST to find array declarations with constant initializers
	ast.Inspect(f, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok {
			return true
		}
		genDeclCount++
		if genDecl.Tok != token.VAR {
			return true
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Names) == 0 {
				continue
			}
			valueSpecCount++

			name := valueSpec.Names[0].Name
			if len(valueSpec.Values) == 0 {
				continue
			}

			// Check if this is an array literal with constant values
			compositeLit, ok := valueSpec.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			compositeLitCount++

			// Assume any variable with a composite literal containing multiple elements is an array
			// Extract the element type from the type if available
			elemType := "int32" // default
			if valueSpec.Type != nil {
				if arrayType, ok := valueSpec.Type.(*ast.ArrayType); ok {
					arrayTypeCount++
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
					// Skip arrays with non-constant elements
					fmt.Fprintf(os.Stderr, "[DEBUG] Skipping %s due to non-constant element: %v\n", name, err)
					return false
				}
				values = append(values, val)
			}

			// Only include arrays that are fully constant
			// Only include test input data (e.g., test_data), not expected outputs or working storage
			if len(values) > 0 && name == "test_data" {
				fmt.Fprintf(os.Stderr, "[DEBUG] Found constant array: %s with %d values (type %s)\n", name, len(values), elemType)
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
	fmt.Fprintf(os.Stderr, "[DEBUG] AST walk: %d GenDecls, %d ValueSpecs, %d CompositeLits, %d ArrayTypes\n",
		genDeclCount, valueSpecCount, compositeLitCount, arrayTypeCount)

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
