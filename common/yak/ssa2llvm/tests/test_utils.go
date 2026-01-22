package tests

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/go-llvm"
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

// checkPrint compiles and runs the code using JIT (with hook) and Binary (captured output).
// It verifies that println(expectedVal) was called.
func checkPrint(t *testing.T, code string, expectedVals ...int64) {
	t.Helper()

	// 1. JIT Test with Hook
	teardown := SetupJITHook()

	opts := compiler.RunOptions{
		SourceCode:   code,
		Language:     "yak",
		FunctionName: "check",
		ExternalHooks: map[string]unsafe.Pointer{
			"yak_internal_print_int": getHookAddr(),
		},
	}

	// Run JIT
	_, err := compiler.RunViaJIT(opts)
	vals := teardown()

	if err != nil {
		t.Fatalf("JIT execution failed: %v", err)
	}

	// Check collected values
	if len(vals) != len(expectedVals) {
		t.Errorf("JIT hook mismatch. Expected %d calls, got %d. Got: %v, Expected: %v",
			len(expectedVals), len(vals), vals, expectedVals)
	} else {
		for i, v := range vals {
			if v != expectedVals[i] {
				t.Errorf("JIT hook mismatch at index %d. Expected %d, got %d", i, expectedVals[i], v)
			}
		}
	}

	// 2. Binary Test (Integration)
	// Create temporary source file
	tmpFile, err := os.CreateTemp("", "test_print_*.yak")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(code); err != nil {
		t.Fatalf("Failed to write code: %v", err)
	}
	tmpFile.Close()

	tmpBin := tmpFile.Name() + ".bin"
	defer os.Remove(tmpBin)

	compOpts := compiler.CompileOptions{
		SourceFile:        tmpFile.Name(),
		OutputFile:        tmpBin,
		Language:          "yak",
		EntryFunctionName: "check", // Always use check for tests
	}

	if err := compiler.CompileToExecutable(compOpts); err != nil {
		t.Fatalf("Binary compilation failed: %v", err)
	}

	cmd := exec.Command(tmpBin)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Binary execution failed: %v\nOutput: %s", err, output)
	}

	// Build expected output string
	var expectedBuffer strings.Builder
	for _, v := range expectedVals {
		expectedBuffer.WriteString(fmt.Sprintf("%d\n", v))
	}
	expectedStr := expectedBuffer.String()

	if string(output) != expectedStr {
		t.Errorf("Binary output mismatch. Expected '%s', got '%s'", expectedStr, output)
	}
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
