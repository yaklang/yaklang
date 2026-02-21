package ssaapi

import (
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func isCFGExactLabel(label string) bool {
	if !strings.HasPrefix(label, "search-exact:") {
		return false
	}
	target := strings.TrimPrefix(label, "search-exact:")
	switch target {
	case "throws":
		return true
	default:
		return false
	}
}

func _SearchValues(values Values, mod int, handler func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values
	for _, value := range values {
		result := _SearchValue(value, mod, handler, opt...)
		newValue = append(newValue, result...)
	}

	return lo.UniqBy(newValue, func(v *Value) int { return int(v.GetId()) })
	// return newValue
}

func _SearchValue(value *Value, mod int, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	ctx := sfvm.NewDefaultAnalysisContext()
	for _, apply := range opt {
		apply(ctx)
	}
	label := ctx.Label
	skipValueSearch := false
	skipCFGSearch := false
	if label != "" && strings.HasPrefix(label, "search-exact:") {
		if isCFGExactLabel(label) {
			// CFG keywords should be resolved by CFG edges directly.
			skipValueSearch = true
		} else if shouldUseCFGSearch(value) {
			skipValueSearch = true
		} else {
			skipCFGSearch = true
		}
	}
	newValue := make([]*Value, 0)
	if !skipValueSearch {
		name := "sf.SearchWithValue"
		if label != "" {
			name += ":" + label
		}
		_ = diagnostics.TrackLow(name, func() error {
			newValue = append(newValue, SearchWithValue(value, mod, compare, opt...)...)
			return nil
		})
	}

	if !skipCFGSearch {
		name := "sf.SearchWithCFG"
		if label != "" {
			name += ":" + label
		}
		_ = diagnostics.TrackLow(name, func() error {
			newValue = append(newValue, SearchWithCFG(value, mod, compare, opt...)...)
			return nil
		})
	}

	return newValue
}

func shouldUseCFGSearch(value *Value) bool {
	if value == nil {
		return false
	}
	inst := value.getInstruction()
	if inst == nil {
		return false
	}
	if ssa.IsControlInstruction(inst) {
		return true
	}
	switch inst.GetOpcode() {
	case ssa.SSAOpcodeFunction, ssa.SSAOpcodeErrorCatch:
		return true
	default:
		return false
	}
}

func _SearchValuesByOpcode(values Values, opcode string, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values
	if len(values) == 0 {
		return newValue
	}

	for _, value := range values {
		if value.GetOpcode() == opcode {
			value.AppendPredecessor(value, opt...)
			newValue = append(newValue, value)
		}
	}
	return newValue
}

// SyntaxFlowVariableToValues 将 sfvm.ValueOperator 转换为 ssaapi.Values
func SyntaxFlowVariableToValues(vs ...sfvm.ValueOperator) Values {
	var rets Values
	for _, v := range vs {
		if utils.IsNil(v) {
			continue
		}
		err := v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Value:
				rets = append(rets, ret)
			case *sfvm.ValueList:
				// Values 内部可能包含 *Value，递归提取
				ret.Recursive(func(vo sfvm.ValueOperator) error {
					if val, ok := vo.(*Value); ok {
						rets = append(rets, val)
					}
					return nil
				})
			default:
				log.Warnf("cannot handle type: %T", operator)
			}
			return nil
		})
		if err != nil {
			log.Warnf("SyntaxFlowToValues: %v", err)
		}
	}
	return rets
}

// ValuesToSFValueList 将 ssaapi.Values 转换为 sfvm.ValueList
func ValuesToSFValueList(values Values) *sfvm.ValueList {
	out := make([]sfvm.ValueOperator, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return &sfvm.ValueList{Values: out}
}

// MergeSFValueOperator 合并多个 sfvm.ValueOperator 为一个 sfvm.ValueList
func MergeSFValueOperator(sfv ...sfvm.ValueOperator) sfvm.ValueOperator {
	ret := []sfvm.ValueOperator{}
	values := make(Values, 0)
	for _, item := range sfv {
		item.Recursive(func(vo sfvm.ValueOperator) error {
			switch v := vo.(type) {
			case *Program:
				ret = append(ret, v)
			case *Value:
				values = append(values, v)
			}
			return nil
		})
	}
	// 合并重复的 Value
	for _, v := range MergeValues(values) {
		ret = append(ret, v)
	}
	return &sfvm.ValueList{Values: ret}
}
