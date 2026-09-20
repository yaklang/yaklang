// Command source_audit inventories Go symbols and forbids acquisition capabilities.
// It is an audit executable outside the SCA runtime dependency closure.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Symbol struct {
	Name string `json:"name"`
	Line int    `json:"line"`
}
type File struct {
	Path    string   `json:"path"`
	SHA256  string   `json:"sha256"`
	Symbols []Symbol `json:"symbols"`
	Imports []string `json:"imports"`
	Test    bool     `json:"test"`
}

func main() {
	root := "common/sca"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	var files []File
	var problems []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			if strings.HasSuffix(path, ".s") || strings.HasSuffix(path, ".c") {
				problems = append(problems, "native source: "+path)
			}
			return nil
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		node, e := parser.ParseFile(fset, path, raw, parser.ParseComments)
		if e != nil {
			return e
		}
		hash := sha256.Sum256(raw)
		r := File{Path: filepath.ToSlash(path), SHA256: hex.EncodeToString(hash[:]), Test: strings.HasSuffix(path, "_test.go")}
		aliases := map[string]string{}
		for _, im := range node.Imports {
			v, _ := strconv.Unquote(im.Path.Value)
			r.Imports = append(r.Imports, v)
			alias := filepath.Base(v)
			if im.Name != nil {
				alias = im.Name.Name
			}
			aliases[alias] = v
			if !r.Test && (v == "C" || v == "unsafe" || v == "syscall" || v == "os/exec" || v == "plugin" || v == "database/sql" || v == "net" || strings.HasPrefix(v, "net/") && v != "net/textproto" || v == "reflect" && !strings.Contains(r.Path, "internal/testcheck")) {
				problems = append(problems, path+": forbidden import "+v)
			}
		}
		for _, decl := range node.Decls {
			switch v := decl.(type) {
			case *ast.FuncDecl:
				name := v.Name.Name
				if v.Recv != nil {
					switch t := v.Recv.List[0].Type.(type) {
					case *ast.Ident:
						name = t.Name + "." + name
					case *ast.StarExpr:
						if id, ok := t.X.(*ast.Ident); ok {
							name = id.Name + "." + name
						}
					}
				}
				r.Symbols = append(r.Symbols, Symbol{name, fset.Position(v.Pos()).Line})
			case *ast.GenDecl:
				for _, sp := range v.Specs {
					if t, ok := sp.(*ast.TypeSpec); ok {
						r.Symbols = append(r.Symbols, Symbol{t.Name.Name, fset.Position(t.Pos()).Line})
					}
				}
			}
		}
		if !r.Test {
			for _, cg := range node.Comments {
				for _, c := range cg.List {
					if strings.Contains(c.Text, "go:linkname") || strings.Contains(c.Text, "#cgo") {
						problems = append(problems, path+": forbidden directive")
					}
				}
			}
			ast.Inspect(node, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if aliases[id.Name] == "os" {
					allowed := map[string]bool{"Open": true, "DirFS": true, "SameFile": true, "FileMode": true, "O_RDONLY": true, "IsNotExist": true}
					if !allowed[sel.Sel.Name] {
						problems = append(problems, fmt.Sprintf("%s:%d forbidden os.%s", path, fset.Position(n.Pos()).Line, sel.Sel.Name))
					}
				}
				return true
			})
		}
		sort.Strings(r.Imports)
		files = append(files, r)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	out := struct {
		Passed     bool     `json:"passed"`
		Files      []File   `json:"files"`
		Violations []string `json:"violations"`
	}{len(problems) == 0, files, problems}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
	if len(problems) > 0 {
		os.Exit(1)
	}
}
