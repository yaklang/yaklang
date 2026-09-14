package client

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMCPClientProductionCodeDoesNotWriteStdout reserves process stdout for
// an enclosing stdio MCP transport. This is especially important when these
// clients are used by the external-MCP bridge inside another MCP server.
func TestMCPClientProductionCodeDoesNotWriteStdout(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read client package: %v", err)
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(".", name)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}

			if pkg.Name == "os" && selector.Sel.Name == "Stdout" {
				t.Errorf("%s: MCP client production code must not write os.Stdout", fset.Position(selector.Pos()))
			}
			if pkg.Name == "fmt" && (selector.Sel.Name == "Print" || selector.Sel.Name == "Printf" || selector.Sel.Name == "Println") {
				t.Errorf("%s: MCP client production code must not call fmt.%s", fset.Position(selector.Pos()), selector.Sel.Name)
			}
			return true
		})
	}
}
