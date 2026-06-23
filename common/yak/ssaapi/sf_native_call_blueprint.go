package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func resolveGetObject(val *Value) *Value {
	if val == nil || val.IsNil() {
		return nil
	}
	raw := val.getValue()
	if raw == nil {
		return nil
	}
	if fun, ok := ssa.ToFunction(raw); ok && fun.IsMethod() {
		if bp := fun.GetCurrentBlueprint(); bp != nil && bp.Container() != nil {
			return val.NewValue(bp.Container())
		}
	}
	for _, pair := range ssa.GetObjectKeyPairs(raw) {
		if pair.Object == nil {
			continue
		}
		if _, ok := ssa.ToBluePrintType(pair.Object.GetType()); ok {
			return val.NewValue(pair.Object)
		}
	}
	return val.GetObject()
}

func walkCurrentBlueprint(v any, handle func(src *Value, bp *ssa.Blueprint)) {
	getBlueprint := func(v *Value) *ssa.Blueprint {
		if v == nil {
			return nil
		}
		bp, isBlueprint := ssa.ToBluePrintType(v.getValue().GetType())
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
