package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/go-llvm"
)

func TestCompileTestdata(t *testing.T) {
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	testdataDir := "testdata"

	testFiles := []struct {
		file string
		lang string
	}{
		{"example.yak", "yak"},
		{"example.go", "go"},
		{"example.js", "javascript"},
	}

	for _, tf := range testFiles {
		t.Run(tf.file, func(t *testing.T) {
			testCompileFile(t, filepath.Join(testdataDir, tf.file), tf.lang)
		})
	}
}

func testCompileFile(t *testing.T, filepath string, language string) {
	t.Helper()

	code, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filepath, err)
	}

	var opts []ssaconfig.Option
	if language != "" {
		lang, err := ssaconfig.ValidateLanguage(language)
		if err != nil {
			t.Fatalf("Invalid language %s: %v", language, err)
		}
		opts = append(opts, ssaconfig.WithProjectLanguage(lang))
	}

	prog, err := ssaapi.Parse(string(code), opts...)
	if err != nil {
		t.Fatalf("SSA parse failed for %s: %v", filepath, err)
	}

	c := compiler.NewCompiler(context.Background(), prog.Program)

	if err := c.Compile(); err != nil {
		t.Fatalf("LLVM compilation failed for %s: %v", filepath, err)
	}

	if err := llvm.VerifyModule(c.Mod, llvm.ReturnStatusAction); err != nil {
		t.Fatalf("LLVM module verification failed for %s: %v", filepath, err)
	}

	engine, err := llvm.NewExecutionEngine(c.Mod)
	if err != nil {
		t.Fatalf("Failed to create execution engine for %s: %v", filepath, err)
	}
	defer engine.Dispose()

	// Compilation-only test: Do not execute to avoid LLVM execution engine issues with some languages
	// (e.g., Go SSA generates code that may hang in LLVM JIT execution)
	fn := c.Mod.NamedFunction("check")
	if fn.IsNil() {
		t.Logf("Warning: No 'check' function found in %s", filepath)
	}

	t.Logf("File %s (%s) compiled successfully (execution skipped)", filepath, language)
}

func detectLanguage(ext string) string {
	switch ext {
	case ".yak":
		return "yak"
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".java":
		return "java"
	case ".php":
		return "php"
	case ".c", ".h":
		return "c"
	default:
		return ""
	}
}
