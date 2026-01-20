package compiler

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"tinygo.org/x/go-llvm"
)

func TestCompileCall(t *testing.T) {
	// Test calling a function

	// 1. Init Program
	prog := ssa.NewTmpProgram("test_call_prog")
	ctx := context.Background()
	prog.Cache = ssa.NewDBCache(ctx, prog, ssa.ProgramCacheMemory, 0)

	// Init Compiler
	c := NewCompiler(ctx, prog)
	defer c.Dispose()

	// 2. Define callee function "add"
	// func add(x, y int64) int64 { return x + y }
	addFuncType := llvm.FunctionType(c.LLVMCtx.Int64Type(), []llvm.Type{
		c.LLVMCtx.Int64Type(),
		c.LLVMCtx.Int64Type(),
	}, false)
	addFn := llvm.AddFunction(c.Mod, "add", addFuncType)

	// Create entry block for add
	addEntry := c.LLVMCtx.AddBasicBlock(addFn, "entry")
	c.Builder.SetInsertPointAtEnd(addEntry)

	// Get params and add
	paramX := addFn.Param(0)
	paramY := addFn.Param(1)
	sum := c.Builder.CreateAdd(paramX, paramY, "sum")
	c.Builder.CreateRet(sum)

	// 3. Define main function
	mainFuncType := llvm.FunctionType(c.LLVMCtx.Int64Type(), []llvm.Type{}, false)
	mainFn := llvm.AddFunction(c.Mod, "main", mainFuncType)
	mainEntry := c.LLVMCtx.AddBasicBlock(mainFn, "entry")
	c.Builder.SetInsertPointAtEnd(mainEntry)

	// Create const 10 and 20
	const10 := llvm.ConstInt(c.LLVMCtx.Int64Type(), 10, false)
	const20 := llvm.ConstInt(c.LLVMCtx.Int64Type(), 20, false)

	// Register these in Values (mock IDs)
	c.Values[100] = const10
	c.Values[101] = const20

	// 4. Call add(10, 20) directly via LLVM Builder
	// This tests the LLVM call instruction generation

	// Call add(10, 20)
	callResult := c.Builder.CreateCall(addFuncType, addFn, []llvm.Value{const10, const20}, "call_result")
	c.Builder.CreateRet(callResult)

	// 5. Verify IR
	ir := c.Mod.String()
	t.Logf("Generated IR:\n%s", ir)

	if !strings.Contains(ir, "call i64 @add") {
		t.Error("IR should contain call i64 @add")
	}
	if !strings.Contains(ir, "define i64 @add") {
		t.Error("IR should contain define i64 @add")
	}
	if !strings.Contains(ir, "define i64 @main") {
		t.Error("IR should contain define i64 @main")
	}
}
