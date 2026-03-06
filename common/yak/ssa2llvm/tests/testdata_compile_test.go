package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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

func testCompileFile(t *testing.T, filePath string, language string) {
	t.Helper()

	code, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
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
		t.Fatalf("SSA parse failed for %s: %v", filePath, err)
	}

	comp := compiler.NewCompiler(context.Background(), prog.Program)
	defer comp.Dispose()

	if err := comp.Compile(); err != nil {
		t.Fatalf("LLVM compilation failed for %s: %v", filePath, err)
	}
	if err := llvm.VerifyModule(comp.Mod, llvm.ReturnStatusAction); err != nil {
		t.Fatalf("LLVM module verification failed for %s: %v", filePath, err)
	}

	if fn := comp.Mod.NamedFunction("check"); fn.IsNil() {
		t.Logf("Warning: no 'check' function found in %s", filePath)
	}

	t.Logf("File %s (%s) compiled and verified successfully", filePath, language)
}
