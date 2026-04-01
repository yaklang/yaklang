package compiler

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
)

func (c *Compiler) callableContextArgs(inst *ssa.Call, calleeFn *ssa.Function) []contextCallArg {
	return ssaArgs(callframe.BuildCallFrameArgIDs(c.Program, inst, calleeFn), false)
}
