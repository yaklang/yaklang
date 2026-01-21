package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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

	ctx := context.Background()
	progBundle, err := ssaapi.Parse(code)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ssaProg := progBundle.Program
	c := compiler.NewCompiler(ctx, ssaProg)

	// Compile to LLVM IR
	c.Compile()

	// Verify module
	if err := llvm.VerifyModule(c.Mod, llvm.PrintMessageAction); err != nil {
		c.Dispose()
		t.Fatalf("Module verification failed: %v\nIR:\n%s", err, c.Mod.String())
	}

	// JIT Execution
	engine, err := llvm.NewExecutionEngine(c.Mod)
	if err != nil {
		c.Dispose()
		t.Fatalf("Failed to create execution engine: %v", err)
	}
	defer engine.Dispose()

	// Determine entry point: "check" or "main"
	targetFunc := "check"
	fn := c.Mod.NamedFunction(targetFunc)
	if fn.IsNil() {
		// Fallback to "main"?
		// If user code is `check = () => ...`, SSA might compile it as a function named "check"
		// if it's a global function assignment.
		// If it's `main`, try that.
		targetFunc = "main"
		fn = c.Mod.NamedFunction(targetFunc)
	}

	if fn.IsNil() {
		t.Fatalf("No executable function 'check' or 'main' found in module:\n%s", c.Mod.String())
	}

	// Prepare result check
	result := engine.RunFunction(fn, []llvm.GenericValue{})

	compareResult(t, expected, result)
}

// checkEx compiles and runs the given code with extra config options.
func checkEx(t *testing.T, code string, language string, expected interface{}) {
	t.Helper()

	ctx := context.Background()
	// Detect options based on language
	var opts []ssaconfig.Option
	if language != "" {
		lang, err := ssaconfig.ValidateLanguage(language)
		if err != nil {
			t.Fatalf("Invalid language %s: %v", language, err)
		}
		opts = append(opts, ssaconfig.WithProjectLanguage(lang))
	}

	progBundle, err := ssaapi.Parse(code, opts...)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ssaProg := progBundle.Program
	c := compiler.NewCompiler(ctx, ssaProg)

	// Compile to LLVM IR
	c.Compile()

	// Verify module
	if err := llvm.VerifyModule(c.Mod, llvm.PrintMessageAction); err != nil {
		c.Dispose()
		t.Fatalf("Module verification failed: %v\nIR:\n%s", err, c.Mod.String())
	}

	// JIT Execution
	engine, err := llvm.NewExecutionEngine(c.Mod)
	if err != nil {
		c.Dispose()
		t.Fatalf("Failed to create execution engine: %v", err)
	}
	defer engine.Dispose()
	defer c.Builder.Dispose()
	// Determine entry point: "check" or "main"
	targetFunc := "check"
	fn := c.Mod.NamedFunction(targetFunc)
	if fn.IsNil() {
		targetFunc = "main"
		fn = c.Mod.NamedFunction(targetFunc)
	}

	if fn.IsNil() {
		t.Fatalf("No executable function 'check' or 'main' found in module:\n%s", c.Mod.String())
	}

	// Prepare result check
	result := engine.RunFunction(fn, []llvm.GenericValue{})

	// Compare result based on expectation type
	compareResult(t, expected, result)
}

func compareResult(t *testing.T, expected interface{}, result llvm.GenericValue) {
	t.Helper()
	switch expect := expected.(type) {
	case int:
		val := result.Int(true) // sign extend
		if int64(val) != int64(expect) {
			t.Errorf("Result check failed. Expected: %d, Got: %d", expect, val)
		}
	case int64:
		val := result.Int(true)
		if int64(val) != expect {
			t.Errorf("Result check failed. Expected: %d, Got: %d", expect, val)
		}
	case string:
		val := result.Int(true)
		if fmt.Sprintf("%d", val) != expect {
			t.Errorf("Result check failed. Expected: %s, Got: %d", expect, val)
		}
	default:
		t.Fatalf("Unsupported expected value type: %T", expected)
	}
}

// runJIT executes a function in the compiled module using LLVM MCJIT and returns the result.
// funcName is the name of the function to call.
// args are the uint64 arguments to pass to the function.
func runJIT(t *testing.T, c *compiler.Compiler, funcName string, args ...uint64) uint64 {
	t.Helper()

	// Verify module
	if err := llvm.VerifyModule(c.Mod, llvm.PrintMessageAction); err != nil {
		t.Fatalf("Module verification failed: %v\nIR:\n%s", err, c.Mod.String())
	}

	// Create execution engine
	engine, err := llvm.NewExecutionEngine(c.Mod)
	if err != nil {
		t.Fatalf("Failed to create execution engine: %v", err)
	}
	defer engine.Dispose()

	// Find function
	fn := c.Mod.NamedFunction(funcName)
	if fn.IsNil() {
		t.Fatalf("Function %s not found in module", funcName)
	}

	// Prepare arguments
	llvmArgs := make([]llvm.GenericValue, len(args))
	for i, arg := range args {
		llvmArgs[i] = llvm.NewGenericValueFromInt(c.LLVMCtx.Int64Type(), arg, false)
	}

	// Run function
	result := engine.RunFunction(fn, llvmArgs)

	// Get result as uint64
	return result.Int(false)
}

// compileAndVerify compiles the module and verifies it's valid LLVM IR.
func compileAndVerify(t *testing.T, c *compiler.Compiler) {
	t.Helper()
	if err := llvm.VerifyModule(c.Mod, llvm.PrintMessageAction); err != nil {
		t.Fatalf("Module verification failed: %v\nIR:\n%s", err, c.Mod.String())
	}
}
