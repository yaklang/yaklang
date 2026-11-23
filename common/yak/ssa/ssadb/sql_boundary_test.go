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

// sqlExecutorFiles are the only files allowed to talk to database/sql or GORM
// execution methods directly. Every other file must use nativeExec,
// nativeQueryContextDB, nativeQueryRowContext, or the catalog helpers in
// database.go so placeholder binding, DM normalization, identifier quoting and
// index DDL stay in one place.
var sqlExecutorFiles = map[string]string{
	"native_read.go":             "sanctioned native SQL executor",
	"secondary_index_suspend.go": "SQLite pinned-connection index maintenance",
	"sqlite_bulk_delete.go":      "SQLite PRAGMA tuning",
}

var rawSQLMethods = map[string]bool{
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
		if _, ok := sqlExecutorFiles[name]; ok {
			continue
		}
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
		"If the file is genuinely dialect- or connection-specific, add it to sqlExecutorFiles with a reason.",
		strings.Join(violations, "\n  "))
}

func inspectSQLCalls(filename string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}
	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		types := collectDBIdentifiers(fn)
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !rawSQLMethods[selector.Sel.Name] {
				return true
			}
			root := rootIdentifier(selector.X)
			if root == nil || !isSQLReceiver(selector.X, root, types) {
				return true
			}
			pos := fset.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: %s", filename, pos.Line, selectorText(selector)))
			return true
		})
		return true
	})
	return violations, nil
}

// collectDBIdentifiers records parameter, receiver, var and simple assignment
// types so the detector does not have to guess from receiver names alone.
func collectDBIdentifiers(fn *ast.FuncDecl) map[string]string {
	types := map[string]string{}
	addFields := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			typ := exprType(field.Type)
			for _, name := range field.Names {
				types[name.Name] = typ
			}
		}
	}
	addFields(fn.Recv)
	addFields(fn.Type.Params)
	addFields(fn.Type.Results)

	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.DeclStmt:
			decl, ok := n.Decl.(*ast.GenDecl)
			if !ok || decl.Tok != token.VAR {
				return true
			}
			for _, spec := range decl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				typ := exprType(valueSpec.Type)
				for i, name := range valueSpec.Names {
					if typ == "" && i < len(valueSpec.Values) {
						typ = inferExpressionType(valueSpec.Values[i], types)
					}
					types[name.Name] = typ
				}
			}
		case *ast.AssignStmt:
			for i, left := range n.Lhs {
				name, ok := left.(*ast.Ident)
				if !ok || i >= len(n.Rhs) {
					continue
				}
				if typ := inferExpressionType(n.Rhs[i], types); typ != "" {
					types[name.Name] = typ
				}
			}
		}
		return true
	})
	return types
}

func inferExpressionType(expr ast.Expr, types map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return types[e.Name]
	case *ast.ParenExpr:
		return inferExpressionType(e.X, types)
	case *ast.CallExpr:
		selector, ok := e.Fun.(*ast.SelectorExpr)
		if !ok {
			return ""
		}
		root := rootIdentifier(selector.X)
		if root == nil {
			return ""
		}
		rootType := strings.ToLower(types[root.Name])
		if !strings.Contains(rootType, "gorm.db") {
			return ""
		}
		switch selector.Sel.Name {
		case "DB":
			return "*sql.DB"
		case "CommonDB":
			return "database/sql.SQLCommon"
		}
	}
	return ""
}

func isSQLReceiver(expr ast.Expr, root *ast.Ident, types map[string]string) bool {
	if isDatabaseValue(types[root.Name], root.Name) {
		return true
	}
	// Handles stored handles such as store.db or store.conn, whose root type
	// is the owning struct rather than a database handle.
	if selector, ok := expr.(*ast.SelectorExpr); ok {
		switch selector.Sel.Name {
		case "db", "DB", "tx", "conn":
			return true
		}
	}
	return false
}

func isDatabaseValue(typ, name string) bool {
	typ = strings.ToLower(strings.TrimSpace(typ))
	if strings.Contains(typ, "gorm.db") ||
		strings.Contains(typ, "sql.db") ||
		strings.Contains(typ, "sql.tx") ||
		strings.Contains(typ, "sql.conn") ||
		strings.Contains(typ, "sqlcommon") {
		return true
	}
	// Unknown types still get flagged for the conventional handle names. This
	// is the fallback that catches a new local variable before someone adds a
	// helper; unrelated structs named something else are ignored.
	switch name {
	case "db", "tx", "conn":
		return typ == ""
	}
	return false
}

func rootIdentifier(expr ast.Expr) *ast.Ident {
	switch e := expr.(type) {
	case *ast.Ident:
		return e
	case *ast.SelectorExpr:
		return rootIdentifier(e.X)
	case *ast.CallExpr:
		return rootIdentifier(e.Fun)
	case *ast.ParenExpr:
		return rootIdentifier(e.X)
	case *ast.StarExpr:
		return rootIdentifier(e.X)
	}
	return nil
}

func exprType(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return b.String()
}

func selectorText(selector *ast.SelectorExpr) string {
	return exprType(selector)
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
