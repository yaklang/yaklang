package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func getCurrentBlueprint(v any) []*ssa.Blueprint {
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
		return nil
	}

	var rets []*ssa.Blueprint
	_ = values.Recursive(func(operator sfvm.ValueOperator) error {
		if ret, ok := operator.(*Value); ok {
			if bp := getBlueprint(ret); bp != nil {
				rets = append(rets, bp)
			}
		}
		return nil
	})
	return rets
}
