package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) compileNext(inst *ssa.Next) error {
	iterVal, err := c.getValue(inst, inst.Iter)
	if err != nil {
		return fmt.Errorf("compileNext: failed to resolve iterator: %w", err)
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
			{value: iterVal, tagPointerArg: true},
			{value: llvm.ConstInt(c.LLVMCtx.Int64Type(), inNext, false)},
			{value: llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(inst.GetId()), false)},
		},
		ctxName:   "yak_next_ctx",
		errPrefix: "emitRuntimeNext",
	}
	return c.lowerResolvedContextCall(spec)
}
