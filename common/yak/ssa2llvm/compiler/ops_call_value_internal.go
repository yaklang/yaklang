package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) resolveSSAValueAsInt64(contextInst ssa.Instruction, valueID int64, ptrName string) (llvm.Value, error) {
	argVal, err := c.getValue(contextInst, valueID)
	if err == nil {
		return c.coerceToInt64(argVal), nil
	}

	fn := contextInst.GetFunc()
	if fn == nil {
		return llvm.Value{}, err
	}
	value, ok := fn.GetValueById(valueID)
	if !ok || value == nil {
		return llvm.Value{}, err
	}
	if param, ok := ssa.ToParameter(value); ok && param != nil && param.GetDefault() != nil {
		value = param.GetDefault()
	}
	ssaFn, ok := ssa.ToFunction(value)
	if !ok || ssaFn == nil {
		return llvm.Value{}, err
	}
	llvmFn, _ := c.getOrDeclareLLVMFunction(ssaFn)
	return c.Builder.CreatePtrToInt(llvmFn, c.LLVMCtx.Int64Type(), ptrName), nil
}
