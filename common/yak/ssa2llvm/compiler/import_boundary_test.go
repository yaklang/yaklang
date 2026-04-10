package compiler

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompilerDoesNotImportConcreteObfuscators(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	compilerDir := filepath.Dir(file)
	files, err := filepath.Glob(filepath.Join(compilerDir, "*.go"))
	require.NoError(t, err)

	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		require.NoError(t, err, path)

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			switch importPath {
			case "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize",
				"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/callret",
				"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/addsub",
				"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/llvmxor",
				"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/mba",
				"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/opaque",
				"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/builtin",
				"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/policy":
				t.Fatalf("%s imports concrete obfuscator package %s", filepath.Base(path), importPath)
			}
		}
	}
}
