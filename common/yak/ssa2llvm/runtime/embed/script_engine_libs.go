package embed

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type scriptEngineLibRegistry struct {
	Modules map[string]scriptEngineExport
	Globals []scriptEngineExport
}

type scriptEngineExport struct {
	Expr    string
	Imports []goImportSpec
	Keys    map[string]struct{}
}

func scriptEngineRegistryForRuntimeDir(runtimeDir string) (*scriptEngineLibRegistry, error) {
	root, err := sourceRootForRuntimeDir(runtimeDir)
	if err != nil {
		return nil, err
	}
	return loadScriptEngineLibRegistry(root)
}

func scriptEngineRegistryFromLocalSource() (*scriptEngineLibRegistry, error) {
	root, err := localGoModuleRoot()
	if err != nil {
		return nil, err
	}
	return loadScriptEngineLibRegistry(root)
}

func sourceRootForRuntimeDir(runtimeDir string) (string, error) {
	dir := strings.TrimSpace(runtimeDir)
	if dir != "" {
		dir = filepath.Clean(dir)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir, nil
			}
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}
	return localGoModuleRoot()
}

func loadScriptEngineLibRegistry(sourceRoot string) (*scriptEngineLibRegistry, error) {
	sourceRoot = strings.TrimSpace(sourceRoot)
	if sourceRoot == "" {
		return nil, fmt.Errorf("load yak script engine libs failed: empty source root")
	}
	modulePath, err := modulePathFromGoMod(sourceRoot)
	if err != nil {
		return nil, err
	}
	reg := &scriptEngineLibRegistry{
		Modules: make(map[string]scriptEngineExport),
	}
	files := []string{
		filepath.Join(sourceRoot, "common", "yak", "script_engine.go"),
		filepath.Join(sourceRoot, "common", "yak", "irify_libs.go"),
	}
	var parsed int
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("load yak script engine libs failed: stat %s: %w", file, err)
		}
		if err := parseScriptEngineLibFile(reg, sourceRoot, modulePath, file); err != nil {
			return nil, err
		}
		parsed++
	}
	if parsed == 0 {
		return nil, fmt.Errorf("load yak script engine libs failed: script engine sources not found under %s", sourceRoot)
	}
	return reg, nil
}

func modulePathFromGoMod(sourceRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(sourceRoot, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("load yak script engine libs failed: read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "module "); ok {
			modulePath := strings.TrimSpace(after)
			if modulePath != "" {
				return modulePath, nil
			}
		}
	}
	return "", fmt.Errorf("load yak script engine libs failed: module path not found in go.mod")
}

func parseScriptEngineLibFile(reg *scriptEngineLibRegistry, sourceRoot, modulePath, filePath string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return fmt.Errorf("load yak script engine libs failed: parse %s: %w", filePath, err)
	}
	imports := collectFileImports(file)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		locals := collectLocalExportSlices(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if module, expr, ok := yaklangImportCall(call); ok {
				export, ok := buildScriptEngineExport(fset, sourceRoot, modulePath, imports, locals, expr)
				if ok {
					reg.Modules[module] = export
				}
				return true
			}
			if expr, ok := importGlobalCall(call); ok {
				export, ok := buildScriptEngineExport(fset, sourceRoot, modulePath, imports, locals, expr)
				if ok {
					reg.Globals = append(reg.Globals, export)
				}
				return true
			}
			return true
		})
	}
	return nil
}

func collectFileImports(file *ast.File) map[string]goImportSpec {
	out := make(map[string]goImportSpec, len(file.Imports))
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath == "" {
			continue
		}
		alias := ""
		if spec.Name != nil {
			alias = spec.Name.Name
		} else {
			alias = path.Base(importPath)
		}
		if alias == "" || alias == "_" || alias == "." {
			continue
		}
		out[alias] = goImportSpec{Alias: alias, Path: importPath}
	}
	return out
}

func collectLocalExportSlices(body *ast.BlockStmt) map[string][]ast.Expr {
	out := make(map[string][]ast.Expr)
	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range stmt.Lhs {
				if i >= len(stmt.Rhs) {
					continue
				}
				ident, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				if elts, ok := exportSliceElements(stmt.Rhs[i]); ok {
					out[ident.Name] = elts
				}
			}
		case *ast.ValueSpec:
			for i, name := range stmt.Names {
				if i >= len(stmt.Values) || name == nil {
					continue
				}
				if elts, ok := exportSliceElements(stmt.Values[i]); ok {
					out[name.Name] = elts
				}
			}
		}
		return true
	})
	return out
}

func exportSliceElements(expr ast.Expr) ([]ast.Expr, bool) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || len(lit.Elts) == 0 {
		return nil, false
	}
	return append([]ast.Expr(nil), lit.Elts...), true
}

func yaklangImportCall(call *ast.CallExpr) (string, ast.Expr, bool) {
	if len(call.Args) < 2 {
		return "", nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Import" {
		return "", nil, false
	}
	root, ok := sel.X.(*ast.Ident)
	if !ok || root.Name != "yaklang" {
		return "", nil, false
	}
	module, ok := stringLiteral(call.Args[0])
	if !ok {
		return "", nil, false
	}
	return module, call.Args[1], true
}

func importGlobalCall(call *ast.CallExpr) (ast.Expr, bool) {
	if len(call.Args) != 1 {
		return nil, false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != "importGlobal" {
		return nil, false
	}
	return call.Args[0], true
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

func buildScriptEngineExport(fset *token.FileSet, sourceRoot, modulePath string, imports map[string]goImportSpec, locals map[string][]ast.Expr, expr ast.Expr) (scriptEngineExport, bool) {
	expr = expandVariadicLocalExportSlice(expr, locals)
	roots := make(map[string]struct{})
	collectExprImportRoots(expr, roots)
	specs := make([]goImportSpec, 0, len(roots))
	for root := range roots {
		spec, ok := imports[root]
		if !ok {
			return scriptEngineExport{}, false
		}
		specs = append(specs, spec)
	}
	text, err := exprString(fset, expr)
	if err != nil {
		return scriptEngineExport{}, false
	}
	return scriptEngineExport{
		Expr:    text,
		Imports: specs,
		Keys:    exportKeysFromExpr(sourceRoot, modulePath, imports, expr),
	}, true
}

func expandVariadicLocalExportSlice(expr ast.Expr, locals map[string][]ast.Expr) ast.Expr {
	call, ok := expr.(*ast.CallExpr)
	if !ok || !call.Ellipsis.IsValid() || len(call.Args) == 0 {
		return expr
	}
	last, ok := call.Args[len(call.Args)-1].(*ast.Ident)
	if !ok {
		return expr
	}
	elts, ok := locals[last.Name]
	if !ok {
		return expr
	}
	clone := *call
	clone.Args = append([]ast.Expr{}, call.Args[:len(call.Args)-1]...)
	clone.Args = append(clone.Args, elts...)
	clone.Ellipsis = token.NoPos
	return &clone
}

func exprString(fset *token.FileSet, expr ast.Expr) (string, error) {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, expr); err != nil {
		return "", err
	}
	return b.String(), nil
}

func collectExprImportRoots(expr ast.Expr, roots map[string]struct{}) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.Ident:
		if e.Name != "" && e.Name != "nil" && e.Name != "true" && e.Name != "false" {
			roots[e.Name] = struct{}{}
		}
	case *ast.SelectorExpr:
		if root, ok := leftmostSelectorIdent(e); ok {
			roots[root] = struct{}{}
		}
	case *ast.CallExpr:
		collectExprImportRoots(e.Fun, roots)
		for _, arg := range e.Args {
			collectExprImportRoots(arg, roots)
		}
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			collectExprImportRoots(elt, roots)
		}
	case *ast.KeyValueExpr:
		collectExprImportRoots(e.Value, roots)
	case *ast.ParenExpr:
		collectExprImportRoots(e.X, roots)
	case *ast.UnaryExpr:
		collectExprImportRoots(e.X, roots)
	case *ast.StarExpr:
		collectExprImportRoots(e.X, roots)
	case *ast.IndexExpr:
		collectExprImportRoots(e.X, roots)
		collectExprImportRoots(e.Index, roots)
	case *ast.IndexListExpr:
		collectExprImportRoots(e.X, roots)
		for _, idx := range e.Indices {
			collectExprImportRoots(idx, roots)
		}
	case *ast.SliceExpr:
		collectExprImportRoots(e.X, roots)
		collectExprImportRoots(e.Low, roots)
		collectExprImportRoots(e.High, roots)
		collectExprImportRoots(e.Max, roots)
	case *ast.Ellipsis:
		collectExprImportRoots(e.Elt, roots)
	}
}

func leftmostSelectorIdent(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name, x.Name != ""
	case *ast.SelectorExpr:
		return leftmostSelectorIdent(x.X)
	case *ast.IndexExpr:
		return leftmostSelectorIdent(x.X)
	case *ast.IndexListExpr:
		return leftmostSelectorIdent(x.X)
	default:
		return "", false
	}
}

func exportKeysFromExpr(sourceRoot, modulePath string, imports map[string]goImportSpec, expr ast.Expr) map[string]struct{} {
	keys := make(map[string]struct{})
	collectExportKeys(sourceRoot, modulePath, imports, expr, keys)
	if len(keys) == 0 {
		return nil
	}
	return keys
}

func collectExportKeys(sourceRoot, modulePath string, imports map[string]goImportSpec, expr ast.Expr, keys map[string]struct{}) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.SelectorExpr:
		alias, exportName, ok := simpleSelector(e)
		if !ok {
			return
		}
		spec, ok := imports[alias]
		if !ok {
			return
		}
		for key := range packageMapKeys(sourceRoot, modulePath, spec.Path, exportName) {
			keys[key] = struct{}{}
		}
	case *ast.CallExpr:
		for _, arg := range e.Args {
			collectExportKeys(sourceRoot, modulePath, imports, arg, keys)
		}
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			collectExportKeys(sourceRoot, modulePath, imports, elt, keys)
		}
	case *ast.KeyValueExpr:
		collectExportKeys(sourceRoot, modulePath, imports, e.Value, keys)
	case *ast.ParenExpr:
		collectExportKeys(sourceRoot, modulePath, imports, e.X, keys)
	case *ast.UnaryExpr:
		collectExportKeys(sourceRoot, modulePath, imports, e.X, keys)
	case *ast.StarExpr:
		collectExportKeys(sourceRoot, modulePath, imports, e.X, keys)
	case *ast.IndexExpr:
		collectExportKeys(sourceRoot, modulePath, imports, e.X, keys)
	case *ast.IndexListExpr:
		collectExportKeys(sourceRoot, modulePath, imports, e.X, keys)
	case *ast.SliceExpr:
		collectExportKeys(sourceRoot, modulePath, imports, e.X, keys)
	case *ast.Ellipsis:
		collectExportKeys(sourceRoot, modulePath, imports, e.Elt, keys)
	}
}

func simpleSelector(expr *ast.SelectorExpr) (alias, name string, ok bool) {
	if expr == nil || expr.Sel == nil {
		return "", "", false
	}
	root, ok := expr.X.(*ast.Ident)
	if !ok || root.Name == "" {
		return "", "", false
	}
	return root.Name, expr.Sel.Name, true
}

func packageMapKeys(sourceRoot, modulePath, importPath, exportName string) map[string]struct{} {
	out := make(map[string]struct{})
	if sourceRoot == "" || modulePath == "" || importPath == "" || exportName == "" {
		return out
	}
	rel, ok := strings.CutPrefix(importPath, modulePath+"/")
	if !ok {
		return out
	}
	dir := filepath.Join(sourceRoot, filepath.FromSlash(rel))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	fset := token.NewFileSet()
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		filePath := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, filePath, nil, 0)
		if err != nil {
			continue
		}
		collectMapKeysFromFile(file, exportName, out)
	}
	return out
}

func collectMapKeysFromFile(file *ast.File, exportName string, keys map[string]struct{}) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name == nil || name.Name != exportName || i >= len(vs.Values) {
					continue
				}
				collectMapKeysFromExpr(vs.Values[i], keys)
			}
		}
	}
}

func collectMapKeysFromExpr(expr ast.Expr, keys map[string]struct{}) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := stringLiteral(kv.Key)
		if ok && key != "" {
			keys[key] = struct{}{}
		}
	}
}

func (r *scriptEngineLibRegistry) module(name string) (scriptEngineExport, bool) {
	if r == nil {
		return scriptEngineExport{}, false
	}
	export, ok := r.Modules[strings.TrimSpace(name)]
	return export, ok
}

func (r *scriptEngineLibRegistry) globalForMethod(method string) (scriptEngineExport, bool) {
	if r == nil {
		return scriptEngineExport{}, false
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return scriptEngineExport{}, false
	}
	for _, export := range r.Globals {
		if len(export.Keys) == 0 {
			continue
		}
		if _, ok := export.Keys[method]; ok {
			return export, true
		}
	}
	return scriptEngineExport{}, false
}
