package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func getCurrentBlueprint(v sfvm.ValueOperator) []*ssa.Blueprint {
	getBlueprint := func(v *Value) *ssa.Blueprint {
		if v == nil {
			return nil
		}
		bp, isBlueprint := ssa.ToClassBluePrintType(v.innerValue.GetType())
		if isBlueprint {
			return bp
		}
		fun := v.GetFunction()
		if fun == nil {
			return nil
		}
		funIns, ok := ssa.ToFunction(fun.innerValue)
		if !ok {
			return nil
		}
		return funIns.GetCurrentBlueprint()
	}

	var rets []*ssa.Blueprint
	v.Recursive(func(operator sfvm.ValueOperator) error {
		switch ret := operator.(type) {
		case *Value:
			if bp := getBlueprint(ret); bp != nil {
				rets = append(rets, bp)
			}
		case Values:
			ret.ForEach(func(value *Value) {
				if bp := getBlueprint(value); bp != nil {
					rets = append(rets, bp)
				}
			})
		case *sfvm.ValueList:
			values, err := SFValueListToValues(ret)
			if err == nil {
				values.ForEach(func(value *Value) {
					if bp := getBlueprint(value); bp != nil {
						rets = append(rets, bp)
					}
				})
			}
		default:
			return nil
		}
		return nil
	})
	return rets
}
