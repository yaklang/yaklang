package ssaapi

import (
	"strings"

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

// valueOperatorToSSAValue maps SFVM carriers (including <getCfg>'s *CfgCtxValue) into *ssaapi.Value so
// SyntaxFlowResult.GetValues, persistence, and yakurl listings see a normal value row (string const, no range).
func valueOperatorToSSAValue(value sfvm.ValueOperator) (*Value, bool) {
	if utils.IsNil(value) {
		return nil, false
	}
	if ret, ok := value.(*Value); ok {
		return ret, true
	}
	if c, ok := value.(*CfgCtxValue); ok {
		if c == nil || c.IsEmpty() {
			return nil, true
		}
		prog := c.prog
		if prog == nil {
			prog = NewTmpProgram("")
		}
		return prog.NewConstValue(c.String(), nil), true
	}
	return nil, false
}

// IsCfgCtxURLDisplayString matches the stable text form of [CfgCtxValue.String] (used when bridging to const).
func IsCfgCtxURLDisplayString(s string) bool {
	return strings.HasPrefix(s, "cfg(func=")
}

func FromSFVMValues(values sfvm.Values) Values {
	var rets Values
	for _, value := range values {
		if v, ok := valueOperatorToSSAValue(value); ok {
			if v != nil {
				rets = append(rets, v)
			}
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
		if val, ok := valueOperatorToSSAValue(v); ok {
			if val != nil {
				rets = append(rets, val)
			}
			continue
		}
		log.Warnf("cannot handle type: %T", v)
	}
	return rets
}
