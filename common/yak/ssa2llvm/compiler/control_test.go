package compiler

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"tinygo.org/x/go-llvm"
)

func TestCompileLoop(t *testing.T) {
	// Loop: Sum 1..N (let N=5)
	// Entry -> LoopHeader -> (LoopBody, Exit)
	// LoopBody -> LoopHeader

	// 1. Init Valid SSA Program
	prog := ssa.NewTmpProgram("test_loop_prog")
	ctx := context.Background()
	// NewTmpProgram doesn't init Cache, but NewFunction needs it. Init manually.
	prog.Cache = ssa.NewDBCache(ctx, prog, ssa.ProgramCacheMemory, 0)

	// Create Function
	fn := prog.NewFunction("test_loop")

	// Create Blocks using SSA API
	entrySsa := fn.NewBasicBlock("entry")
	headerSsa := fn.NewBasicBlock("header")
	// bodySsa := fn.NewBasicBlock("body")
	// exitSsa := fn.NewBasicBlock("exit")

	// Init Compiler
	c := NewCompiler(ctx, prog)
	defer c.Dispose()

	// 2. Setup LLVM Function manually (since we are not calling CompileFunction full flow)
	// func test_loop(n int64) int64
	funcType := llvm.FunctionType(c.LLVMCtx.Int64Type(), []llvm.Type{c.LLVMCtx.Int64Type()}, false)
	llvmFn := llvm.AddFunction(c.Mod, "test_loop", funcType)

	// Register LLVM Blocks in Compiler
	entryBB := c.LLVMCtx.AddBasicBlock(llvmFn, "entry")
	headerBB := c.LLVMCtx.AddBasicBlock(llvmFn, "header")

	c.Blocks[entrySsa.GetId()] = entryBB
	c.Blocks[headerSsa.GetId()] = headerBB

	// --- Block: Entry ---
	c.Builder.SetInsertPointAtEnd(entryBB)

	// Jump to Header
	jump1 := ssa.NewJump(headerSsa)

	if err := c.compileJump(jump1); err != nil {
		t.Fatalf("compileJump entry failed: %v", err)
	}

	// --- Block: Header ---
	c.Builder.SetInsertPointAtEnd(headerBB)

	// Phi i
	phiI := ssa.NewPhi(headerSsa, "i")
	// Manually set Edge for test
	val0 := ssa.NewUndefined("const0")
	valNext := ssa.NewUndefined("i_next")

	phiI.Edge = []int64{val0.GetId(), valNext.GetId()}

	// Pass 1: Compile Phi
	if err := c.compilePhi(phiI); err != nil {
		t.Fatalf("compilePhi failed: %v", err)
	}

	// Verify IR has PHI and BR
	ir := c.Mod.String()

	if !strings.Contains(ir, "phi i64") {
		t.Error("IR should contain phi instruction")
	}
	if !strings.Contains(ir, "br label") {
		t.Error("IR should contain br instruction")
	}
}
