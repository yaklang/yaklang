package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// gorm v2 中返回 error 的“终结”方法。
// 链式方法（Where/Model/Not/Or/Limit/Offset/Order/Select/Preload/Group/Joins/Unscoped/Raw/Distinct/Assign/Attrs）不在此列。
var finisherNames = map[string]bool{
	"Find":          true,
	"First":         true,
	"Last":          true,
	"Take":          true,
	"Count":         true,
	"Pluck":         true,
	"Scan":          true,
	"Create":        true,
	"Save":          true,
	"Update":        true,
	"Updates":       true,
	"Delete":        true,
	"Exec":          true,
	"AutoMigrate":   true,
	"FirstOrCreate": true,
	"FirstOrInit":   true,
}

// Association 相关方法返回 error（在 v2 中 .Append/Replace/Delete/Clear 直接返回 error）
var associationFinishers = map[string]bool{
	"Append":  true,
	"Replace": true,
	"Delete":  true,
	"Clear":   true,
}

func main() {
	var write bool
	var dirs string
	flag.BoolVar(&write, "write", false, "write changes back to files (default: dry-run)")
	flag.StringVar(&dirs, "dirs", "common", "comma-separated directories to process")
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for _, dir := range strings.Split(dirs, ",") {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		absDir := filepath.Join(root, dir)
		if err := filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			// 跳过 vendor / build
			if strings.Contains(path, "/vendor/") || strings.Contains(path, "/build/") || strings.Contains(path, "/.bare/") {
				return nil
			}
			if err := processFile(path, write); err != nil {
				fmt.Fprintf(os.Stderr, "error processing %s: %v\n", path, err)
			}
			return nil
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func processFile(path string, write bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return err
	}

	if !importsGorm(f) {
		return nil
	}

	changed := false

	// 1) 所有 finisher(...).Error 的 .Error 去掉
	astutil.Apply(f, nil, func(c *astutil.Cursor) bool {
		sel, ok := c.Node().(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Error" {
			return true
		}
		call, ok := sel.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isFinisherCall(call) {
			c.Replace(call)
			changed = true
		}
		return true
	})

	// 2) if db := <finisher>(...); db.Error != nil / db.Error == nil -> if err := <finisher>(...); err != nil / err == nil
	astutil.Apply(f, nil, func(c *astutil.Cursor) bool {
		ifStmt, ok := c.Node().(*ast.IfStmt)
		if !ok || ifStmt.Init == nil {
			return true
		}
		assign, ok := ifStmt.Init.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		lhsIdent, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		rhsCall, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || !containsFinisher(rhsCall) {
			return true
		}
		// 条件里必须只用到 lhsIdent.Error 或 lhsIdent.RowsAffected 等字段访问
		if !condOnlyUsesIdentField(ifStmt.Cond, lhsIdent.Name) {
			return true
		}
		newName := makeErrorVarName(lhsIdent.Name)
		// 替换 init 中的变量名
		assign.Lhs[0] = ast.NewIdent(newName)
		// 替换条件中的字段访问为直接使用 newName
		replaceIdentInExpr(ifStmt.Cond, lhsIdent.Name, newName)
		changed = true
		return true
	})

	if !changed {
		return nil
	}

	var buf strings.Builder
	if err := printer.Fprint(&buf, fset, f); err != nil {
		return err
	}
	out := []byte(buf.String())
	if write {
		return os.WriteFile(path, out, 0o644)
	}
	fmt.Printf("--- %s ---\n%s\n", path, out)
	return nil
}

func importsGorm(f *ast.File) bool {
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == "gorm.io/gorm" || path == "gorm.io/gorm" {
			return true
		}
	}
	return false
}

// isFinisherCall 判断一个 CallExpr 是否以 finisher 结尾（最外层调用是 finisher）
func isFinisherCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name := sel.Sel.Name
	if finisherNames[name] {
		return true
	}
	if associationFinishers[name] {
		return true
	}
	return false
}

// containsFinisher 判断一个（可能是链式的）CallExpr 的最后一个调用是否为 finisher
func containsFinisher(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name := sel.Sel.Name
	if finisherNames[name] {
		return true
	}
	if associationFinishers[name] {
		return true
	}
	// 继续往链深处找：例如 db.Where(...).First(&x)
	if inner, ok := sel.X.(*ast.CallExpr); ok {
		return containsFinisher(inner)
	}
	return false
}

// condOnlyUsesIdentField 检查条件是否只使用 identName.Error / identName.RowsAffected 等单字段
func condOnlyUsesIdentField(expr ast.Expr, identName string) bool {
	ok := true
	ast.Inspect(expr, func(n ast.Node) bool {
		sel, okSel := n.(*ast.SelectorExpr)
		if !okSel {
			return true
		}
		id, okId := sel.X.(*ast.Ident)
		if !okId || id.Name != identName {
			// 条件里引用了其他标识符，允许；我们只拒绝再次引用同一 ident 的字段
			return true
		}
		// 只接受 .Error / .RowsAffected
		if sel.Sel.Name != "Error" && sel.Sel.Name != "RowsAffected" {
			ok = false
			return false
		}
		return true
	})
	return ok
}

func replaceIdentInExpr(expr ast.Expr, oldName, newName string) {
	ast.Inspect(expr, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == oldName {
			id.Name = newName
		}
		return true
	})
}

func makeErrorVarName(oldName string) string {
	if oldName == "db" || oldName == "d" || oldName == "tx" {
		return "err"
	}
	return oldName + "Err"
}
