package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) compileNext(inst *ssa.Next) error {
	if inst == nil || c.isSSAValueStored(inst.GetId()) {
		return nil
	}
	inNext := uint64(0)
	if inst.InNext {
		inNext = 1
	}

	spec := contextCallSpec{
		inst: inst,
		kind: abi.KindDispatch,
		target: llvm.ConstInt(
			c.LLVMCtx.Int64Type(),
			uint64(abi.IDRuntimeNext),
			false,
		),
		args: []contextCallArg{
			// Resolve the iterator inside emitContextCall, after the builder
			// has been moved to the next instruction's own block. Resolving it
			// here would compute the value at the lazy-compilation call site
			// (often the entry block) where loop-carried slots are still zero.
			{ssaID: inst.Iter, tagPointerArg: true},
			{value: llvm.ConstInt(c.LLVMCtx.Int64Type(), inNext, false)},
			{value: llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(inst.GetId()), false)},
		},
		ctxName:   "yak_next_ctx",
		errPrefix: "emitRuntimeNext",
	}
	return c.lowerResolvedContextCall(spec)
}
