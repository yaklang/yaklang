package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func walkCurrentBlueprint(v any, handle func(src *Value, bp *ssa.Blueprint)) {
	getBlueprint := func(v *Value) *ssa.Blueprint {
		if v == nil {
			return nil
		}
		bp, isBlueprint := ssa.ToClassBluePrintType(v.getValue().GetType())
		if isBlueprint {
			return bp
		}
		fun := v.GetFunction()
		if fun == nil {
			return nil
		}
		funIns, ok := ssa.ToFunction(fun.getValue())
		if !ok {
			return nil
		}
		return funIns.GetCurrentBlueprint()
	}

	var values sfvm.Values
	switch ret := v.(type) {
	case sfvm.Values:
		values = ret
	case sfvm.ValueOperator:
		values = sfvm.ValuesOf(ret)
	default:
		return
	}

	_ = values.Recursive(func(operator sfvm.ValueOperator) error {
		if ret, ok := operator.(*Value); ok {
			if bp := getBlueprint(ret); bp != nil {
				handle(ret, bp)
			}
		}
		return nil
	})
}

func getCurrentBlueprint(v any) []*ssa.Blueprint {
	var rets []*ssa.Blueprint
	walkCurrentBlueprint(v, func(_ *Value, bp *ssa.Blueprint) {
		rets = append(rets, bp)
	})
	return rets
}
