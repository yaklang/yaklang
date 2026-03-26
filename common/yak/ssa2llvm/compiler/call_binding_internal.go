package compiler

import (
	"sort"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

type orderedFreeValueBinding struct {
	name  string
	ssaID int64
}

func orderedFreeValueBindings(fn *ssa.Function) []orderedFreeValueBinding {
	if fn == nil || len(fn.FreeValues) == 0 {
		return nil
	}

	bindings := make([]orderedFreeValueBinding, 0, len(fn.FreeValues))
	for variable, ssaID := range fn.FreeValues {
		if variable == nil || ssaID <= 0 {
			continue
		}
		bindings = append(bindings, orderedFreeValueBinding{
			name:  variable.GetName(),
			ssaID: ssaID,
		})
	}

	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].name == bindings[j].name {
			return bindings[i].ssaID < bindings[j].ssaID
		}
		return bindings[i].name < bindings[j].name
	})
	return bindings
}

func (c *Compiler) callableContextArgs(inst *ssa.Call, calleeFn *ssa.Function) []contextCallArg {
	args := ssaArgs(c.callArgIDs(inst), false)
	if inst == nil || calleeFn == nil {
		return args
	}

	for _, binding := range orderedFreeValueBindings(calleeFn) {
		arg := contextCallArg{}
		if actualID, ok := inst.Binding[binding.name]; ok && actualID > 0 {
			arg.ssaID = actualID
		}
		args = append(args, arg)
	}
	return args
}
