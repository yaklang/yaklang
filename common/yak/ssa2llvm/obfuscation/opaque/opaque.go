package opaque

/*
#include <llvm-c/Core.h>
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

// opaquePredicateObfuscator inserts opaque predicates—conditional branches
// whose outcome is always true or always false at runtime but appears
// non-trivial to static analysis.
type opaquePredicateObfuscator struct{}

func init() {
	core.Register(opaquePredicateObfuscator{})
}

func (opaquePredicateObfuscator) Name() string { return "opaque" }

func (opaquePredicateObfuscator) Kind() core.Kind { return core.KindLLVM }

func (opaquePredicateObfuscator) Apply(ctx *core.Context) error {
	if ctx == nil || ctx.Stage != core.StageLLVM {
		return nil
	}
	module := ctx.LLVM
	if module.C == nil {
		return nil
	}

	builder := module.NewBuilder()
	defer builder.Dispose()

	for function := module.FirstFunction(); !function.IsNil(); function = function.NextFunction() {
		// Skip declarations (no body).
		if function.FirstBasicBlock().IsNil() {
			continue
		}
		// Need at least 2 blocks to have meaningful branches.
		if countBlocks(function) < 2 {
			continue
		}
		insertOpaquePredicates(module, builder, function)
	}

	return nil
}

func countBlocks(function llvm.Value) int {
	count := 0
	for block := function.FirstBasicBlock(); !block.IsNil(); block = block.NextBasicBlock() {
		count++
	}
	return count
}

// insertOpaquePredicates scans unconditional branches and replaces a subset
// with opaque conditional branches that always take the original target.
//
// Opaque predicate: (x * x) >= 0 — always true for any integer.
func insertOpaquePredicates(module llvm.Module, builder llvm.Builder, function llvm.Value) {
	llvmCtx := module.Context()
	i64 := llvmCtx.Int64Type()
	zero := llvm.ConstInt(i64, 0, false)

	blockIndex := 0
	for block := function.FirstBasicBlock(); !block.IsNil(); block = block.NextBasicBlock() {
		term := lastInstruction(block)
		if term.IsNil() {
			continue
		}

		// Only transform unconditional branches.
		if !isUnconditionalBr(term) {
			continue
		}

		// Only transform every other block to keep some real branches.
		blockIndex++
		if blockIndex%2 == 0 {
			continue
		}

		target := term.Operand(0)
		targetBB := valueAsBasicBlock(target)

		// Create a bogus block that is never reached.
		bogusName := fmt.Sprintf("opaque_bogus_%d", blockIndex)
		bogusBlock := llvmCtx.AddBasicBlock(function, bogusName)
		builder.SetInsertPointAtEnd(bogusBlock)
		buildUnreachable(builder)

		// Replace the unconditional branch with an opaque conditional.
		builder.SetInsertPointBefore(term)
		seed := llvm.ConstInt(i64, uint64(blockIndex*7+3), false)
		squared := builder.CreateMul(seed, seed, "opaque_sq")
		cond := builder.CreateICmp(llvm.IntSGE, squared, zero, "opaque_cond")
		builder.CreateCondBr(cond, targetBB, bogusBlock)

		eraseInstruction(term)
	}
}

func lastInstruction(block llvm.BasicBlock) llvm.Value {
	var last llvm.Value
	for inst := block.FirstInstruction(); !inst.IsNil(); inst = inst.NextInstruction() {
		last = inst
	}
	return last
}

func isUnconditionalBr(inst llvm.Value) bool {
	if inst.IsNil() {
		return false
	}
	cv := (C.LLVMValueRef)(unsafe.Pointer(inst.C))
	if C.LLVMGetInstructionOpcode(cv) != C.LLVMBr {
		return false
	}
	// Unconditional branch has exactly 1 operand; conditional has 3.
	return inst.NumOperands() == 1
}

func eraseInstruction(inst llvm.Value) {
	if inst.IsNil() {
		return
	}
	cv := (C.LLVMValueRef)(unsafe.Pointer(inst.C))
	C.LLVMInstructionEraseFromParent(cv)
}

func buildUnreachable(builder llvm.Builder) {
	cb := (C.LLVMBuilderRef)(unsafe.Pointer(builder.C))
	C.LLVMBuildUnreachable(cb)
}

func valueAsBasicBlock(v llvm.Value) llvm.BasicBlock {
	cv := (C.LLVMValueRef)(unsafe.Pointer(v.C))
	bb := C.LLVMValueAsBasicBlock(cv)
	var ret llvm.BasicBlock
	*(*unsafe.Pointer)(unsafe.Pointer(&ret)) = unsafe.Pointer(bb)
	return ret
}
