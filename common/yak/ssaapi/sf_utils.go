package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func _SearchValues(values Values, mod ssadb.MatchMode, handler func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values
	for _, value := range values {
		result := _SearchValue(value, mod, handler, opt...)
		newValue = append(newValue, result...)
	}

	return lo.UniqBy(newValue, func(v *Value) int { return int(v.GetId()) })
}

func _SearchValue(value *Value, mod ssadb.MatchMode, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	newValue := make([]*Value, 0)
	newValue = append(newValue, SearchWithValue(value, mod, compare, opt...)...)
	newValue = append(newValue, SearchWithCFG(value, mod, compare, opt...)...)
	newValue = append(newValue, Values(newValue).ExpandPhiClosure()...)
	return newValue
}

func _SearchValuesByOpcode(values Values, opcode string, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values
	if len(values) == 0 {
		return newValue
	}

	for _, value := range values {
		if value.GetOpcode() == opcode {
			_ = value.AppendPredecessor(value, opt...)
			newValue = append(newValue, value)
		}
	}
	return newValue
}

func ToSFVMValues(values Values) sfvm.Values {
	if len(values) == 0 {
		return sfvm.NewEmptyValues()
	}
	list := make([]sfvm.ValueOperator, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		list = append(list, value)
	}
	return sfvm.NewValues(list)
}

func FromSFVMValues(values sfvm.Values) Values {
	var rets Values
	for _, value := range values {
		if utils.IsNil(value) {
			continue
		}
		if ret, ok := value.(*Value); ok {
			rets = append(rets, ret)
			continue
		}
		log.Warnf("cannot handle type: %T", value)
	}
	return rets
}

// SyntaxFlowVariableToValues extracts *Value leaves from sfvm atomic values.
func SyntaxFlowVariableToValues(vs ...sfvm.ValueOperator) Values {
	var rets Values
	for _, v := range vs {
		if utils.IsNil(v) {
			continue
		}
		if ret, ok := v.(*Value); ok {
			rets = append(rets, ret)
			continue
		}
		log.Warnf("cannot handle type: %T", v)
	}
	return rets
}
