package ssadb

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// sqlBoundaryAllow is the per-function escape hatch. Exceptions live next to
// the code that needs them instead of in a filename allowlist in this test.
const sqlBoundaryAllow = "sql-boundary:allow"

// sqlExecutionMethods is the database/sql and GORM raw-execution API surface.
// This is API vocabulary, not an exception list: any use anywhere in the
// package is reported unless the enclosing function carries an allow comment.
var sqlExecutionMethods = map[string]bool{
	"Raw":             true,
	"Exec":            true,
	"Query":           true,
	"QueryRow":        true,
	"QueryContext":    true,
	"QueryRowContext": true,
	"ExecContext":     true,
	"Prepare":         true,
	"PrepareContext":  true,
}

func TestSQLBoundaryUsesNativeExecutor(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)

	var violations []string
	for _, name := range files {
		fileViolations, err := inspectSQLCalls(name)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		violations = append(violations, fileViolations...)
	}
	if len(violations) == 0 {
		return
	}
	sort.Strings(violations)
	t.Fatalf("direct SQL execution outside the sanctioned executor:\n  %s\n\n"+
		"Use nativeExec/nativeQueryContextDB/nativeQueryRowContext (or a catalog helper in database.go). "+
		"If a function must issue raw SQL, add a // %s <reason> comment above it.",
		strings.Join(violations, "\n  "), sqlBoundaryAllow)
}

func inspectSQLCalls(filename string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || hasSQLBoundaryAllowance(fn) {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !sqlExecutionMethods[selector.Sel.Name] {
				return true
			}
			pos := fset.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: %s", filename, pos.Line, selectorText(selector)))
			return true
		})
	}
	return violations, nil
}

func hasSQLBoundaryAllowance(fn *ast.FuncDecl) bool {
	if fn.Doc == nil {
		return false
	}
	return strings.Contains(fn.Doc.Text(), sqlBoundaryAllow)
}

func selectorText(selector *ast.SelectorExpr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), selector); err != nil {
		return selector.Sel.Name
	}
	return b.String()
}

func TestSQLBoundaryDetectorCatchesRawQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	source := `package sample

import "github.com/yaklang/gorm"

func broken(db *gorm.DB) {
	db.Raw("SELECT 1").Row()
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	violations, err := inspectSQLCalls(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 {
		t.Fatalf("detector did not flag db.Raw, got %v", violations)
	}
}

func TestSQLBoundaryAnnotationAllowsExecutor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	source := `package sample

import "github.com/yaklang/gorm"

// sql-boundary:allow native SQL executor
func allowed(db *gorm.DB) {
	db.Raw("SELECT 1").Row()
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	violations, err := inspectSQLCalls(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("annotated function was still flagged: %v", violations)
	}
}
