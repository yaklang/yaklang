package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) coerceToI1(val llvm.Value, name string) llvm.Value {
	if val.Type().IntTypeWidth() == 1 {
		return val
	}
	zero := llvm.ConstInt(val.Type(), 0, false)
	return c.Builder.CreateICmp(llvm.IntNE, val, zero, name)
}

func (c *Compiler) resolveCalleeName(fn *ssa.Function, methodID int64) string {
	if fn != nil {
		if calleeVal, ok := fn.GetValueById(methodID); ok && calleeVal != nil {
			if ssaFn, ok := ssa.ToFunction(calleeVal); ok {
				return ssaFn.GetName()
			}
			if name := calleeVal.GetName(); name != "" {
				return name
			}
		}
	}
	return fmt.Sprintf("func_%d", methodID)
}
