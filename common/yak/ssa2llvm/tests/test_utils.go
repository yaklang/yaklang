package tests

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"tinygo.org/x/go-llvm"
)

func init() {
	// Initialize LLVM native target for JIT execution
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()
}

// check compiles and runs the given Yaklang code, then asserts the result matches expected.
// It assumes the code defines a function named "check" to be executed, or runs "main" if not found.
// expected can be:
// - int/int64: expects return value to match (as int64)
// - string: (future) check console output or string return ?? For now assuming return value check.
func check(t *testing.T, code string, expected interface{}) {
	t.Helper()
	checkEx(t, code, "yak", expected)
}

// checkEx compiles and runs the given code with extra config options.
func checkEx(t *testing.T, code string, language string, expected interface{}) {
	t.Helper()

	opts := compiler.RunOptions{
		SourceCode: code,
		Language:   language,
	}

	result, err := compiler.RunViaJIT(opts)
	if err != nil {
		t.Fatalf("JIT execution failed: %v", err)
	}

	// Compare result based on expectation type
	compareResult(t, expected, result)
}

func compareResult(t *testing.T, expected interface{}, result int64) {
	t.Helper()
	switch expect := expected.(type) {
	case int:
		if result != int64(expect) {
			t.Errorf("Result check failed. Expected: %d, Got: %d", expect, result)
		}
	case int64:
		if result != expect {
			t.Errorf("Result check failed. Expected: %d, Got: %d", expect, result)
		}
	case string:
		if fmt.Sprintf("%d", result) != expect {
			t.Errorf("Result check failed. Expected: %s, Got: %d", expect, result)
		}
	default:
		t.Fatalf("Unsupported expected value type: %T", expected)
	}
}
