package compiler

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
)

func (c *Compiler) callableContextArgs(inst *ssa.Call, calleeFn *ssa.Function) []contextCallArg {
	argIDs := callframe.BuildCallFrameArgIDs(c.Program, inst, calleeFn)
	args := make([]contextCallArg, 0, len(argIDs))
	for _, argID := range argIDs {
		args = append(args, contextCallArg{
			ssaID:         argID,
			tagPointerArg: c.shouldTagDirectCallArg(inst, argID),
		})
	}
	return args
}

func (c *Compiler) shouldTagDirectCallArg(inst *ssa.Call, argID int64) bool {
	if inst == nil || argID <= 0 {
		return false
	}
	fn := inst.GetFunc()
	if fn == nil {
		return false
	}
	value, ok := fn.GetValueById(argID)
	if !ok || value == nil || value.GetType() == nil {
		return false
	}
	return value.GetType().GetTypeKind() == ssa.StringTypeKind
}
