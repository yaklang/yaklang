// Record test entry points from the files selected by go list's build context.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if err := recordNames(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func recordNames() error {
	var pkg struct {
		Dir                       string
		TestGoFiles, XTestGoFiles []string
	}
	if err := json.NewDecoder(os.Stdin).Decode(&pkg); err != nil {
		return err
	}
	names := make(map[string]bool)
	for _, name := range append(pkg.TestGoFiles, pkg.XTestGoFiles...) {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(pkg.Dir, name), nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name == "TestMain" {
				continue
			}
			name := fn.Name.Name
			if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Example") || strings.HasPrefix(name, "Fuzz") {
				names[name] = true
			}
		}
	}
	var sorted []string
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	output := ""
	if len(sorted) > 0 {
		output = strings.Join(sorted, "\n") + "\n"
	}
	return os.WriteFile(os.Args[1], []byte(output), 0644)
}
