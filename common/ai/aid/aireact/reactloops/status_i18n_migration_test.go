package reactloops

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestProductionStatusCallsUseI18n(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	aidRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	fset := token.NewFileSet()
	var violations []string

	err := filepath.WalkDir(aidRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			var callName string
			switch function := call.Fun.(type) {
			case *ast.SelectorExpr:
				callName = function.Sel.Name
			case *ast.Ident:
				callName = function.Name
			default:
				return true
			}
			position := fset.Position(call.Lparen)
			switch callName {
			case "EmitStatus", "LoadingStatus":
				violations = append(violations, fmt.Sprintf(
					"%s:%d uses legacy %s; use EmitStatusI18n/UserStatus with separate translations",
					path, position.Line, callName,
				))
			case "EmitStatusI18n":
				if len(call.Args) >= 3 && (isEmptyStringLiteral(call.Args[1]) || isEmptyStringLiteral(call.Args[2])) {
					violations = append(violations, fmt.Sprintf(
						"%s:%d passes an empty translation to EmitStatusI18n",
						path, position.Line,
					))
				}
			case "UserStatus", "planUserStatus", "loopInfraStatus":
				if len(call.Args) >= 2 && (isEmptyStringLiteral(call.Args[0]) || isEmptyStringLiteral(call.Args[1])) {
					violations = append(violations, fmt.Sprintf(
						"%s:%d passes an empty translation to %s",
						path, position.Line, callName,
					))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("status i18n migration violations:\n%s", strings.Join(violations, "\n"))
	}
}

func isEmptyStringLiteral(expression ast.Expr) bool {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(literal.Value)
	return err == nil && strings.TrimSpace(value) == ""
}
