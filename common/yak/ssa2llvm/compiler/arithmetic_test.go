package compiler

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"tinygo.org/x/go-llvm"
)

// Helper internal method exposed via export_test.go usually,
// but here we are in same package 'tests' relative to root or we import compiler.
// Since compiler package is separate, we can't test private methods easily.
// But we want to test 'compileBinOp' which is internal to 'compiler' package?
// Wait, 'ops.go' is in 'compiler' package.
// If I write test in 'compiler' package I can access it.
// So I will write this test file in 'compiler' package but placed in 'compiler/compiler_test.go' path.

// Wait, the user asked to place it in `ssa2llvm/compiler/compiler_test.go`.
// That file will naturally be in package `compiler`.
// But I am writing to `tests/arithmetic_test.go` as per my plan?
// Actually, to test internal methods like `compileBinOp` (if not exported),
// I must be in package `compiler`.
// `compiler.go` defines `CompileFunction`. I can test `CompileFunction` if I can construct `ssa.Function`.
// The user suggestion was to mock `ssa.Function`.
// However, `ssa` structs are complex.
// User suggestion: "Use manual LLVM IR verification" and "compileBinOp" direct call strategy.

// I will create `compiler/arithmetic_test.go` (package compiler) to access internals.

func TestCompileAdd(t *testing.T) {
	// 1. Init Compiler
	prog := &ssa.Program{Name: "test_module"}
	// We need context
	ctx := context.Background()
	c := NewCompiler(ctx, prog)
	defer c.Dispose()

	// 2. Setup manual function context for testing
	// func test_manual(p1, p2) int64
	funcType := llvm.FunctionType(c.LLVMCtx.Int64Type(), []llvm.Type{c.LLVMCtx.Int64Type(), c.LLVMCtx.Int64Type()}, false)
	llvmFn := llvm.AddFunction(c.Mod, "test_manual", funcType)
	bb := c.LLVMCtx.AddBasicBlock(llvmFn, "entry")
	c.Builder.SetInsertPointAtEnd(bb)

	// 3. Inject Values manually
	p1 := llvmFn.Param(0)
	p2 := llvmFn.Param(1)
	c.Values[10] = p1
	c.Values[11] = p2

	// 4. Create ssa.BinOp using exported constructors to ensure proper initialization
	// We need dummy values to satisfy NewBinOp signature
	dummy1 := ssa.NewUndefined("d1")
	dummy1.SetId(10) // Set ID so NewBinOp reads 10
	dummy2 := ssa.NewUndefined("d2")
	dummy2.SetId(11) // Set ID so NewBinOp reads 11

	// NewBinOp(op, x, y)
	// It will read x.GetId() and y.GetId()
	binOp := ssa.NewBinOp(ssa.OpAdd, dummy1, dummy2)
	binOp.SetId(30)

	// 5. Invoke compileInstruction via internal method
	// Since we are in package compiler, we can call c.compileInstruction(binOp)
	err := c.compileInstruction(binOp)
	if err != nil {
		t.Fatalf("compileInstruction failed: %v", err)
	}

	// 6. Return instruction
	// NewReturn takes a slice of Values (Values type)
	// Values is []Value
	// We want to return the result of binOp (ID 30)
	// We can reuse binOp as the value because BinOp implements Value
	retOp := ssa.NewReturn(ssa.Values{binOp})
	retOp.SetId(40)

	err = c.compileInstruction(retOp)
	if err != nil {
		t.Fatalf("compileReturn failed: %v", err)
	}

	// 7. Verify IR
	ir := c.Mod.String()
	t.Logf("Generated IR:\n%s", ir)

	if !strings.Contains(ir, "add i64") {
		t.Error("Expected 'add i64' instruction")
	}
	if !strings.Contains(ir, "ret i64") {
		t.Error("Expected 'ret i64' instruction")
	}
}
