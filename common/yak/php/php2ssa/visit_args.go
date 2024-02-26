package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitArguments(raw phpparser.IArgumentsContext) ([]ssa.Value, bool) {
	if y == nil || raw == nil {
		return nil, false
	}

	i, _ := raw.(*phpparser.ArgumentsContext)
	if i == nil {
		return nil, false
	}

	var ret []ssa.Value

	var ellipsis bool
	for _, arg := range i.AllActualArgument() {
		value, b := y.VisitActualArgument(arg)
		if b {
			ellipsis = true
		}
		ret = append(ret, value)
	}

	return ret, ellipsis
}

func (y *builder) VisitActualArgument(raw phpparser.IActualArgumentContext) (ssa.Value, bool) {
	if y == nil || raw == nil {
		return nil, false
	}

	i, _ := raw.(*phpparser.ActualArgumentContext)
	if i == nil {
		return nil, false
	}

	if i.Expression() != nil {
		val := y.VisitExpression(i.Expression())
		return val, i.Ellipsis() != nil
	} else if i.Ampersand() != nil {
		return y.VisitChain(i.Chain()), false
	}
	return nil, false
}
