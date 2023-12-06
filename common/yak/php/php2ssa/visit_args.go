package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitArguments(raw phpparser.IArgumentsContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ArgumentsContext)
	if i == nil {
		return nil
	}

	for _, arg := range i.AllActualArgument() {
		y.VisitActualArgument(arg)
	}

	return nil
}

func (y *builder) VisitActualArgument(raw phpparser.IActualArgumentContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ActualArgumentContext)
	if i == nil {
		return nil
	}

	if i.Expression() != nil {
		return y.VisitExpression(i.Expression())
	} else if i.Ampersand() != nil {

	} else if i.YieldExpression() != nil {
		return y.VisitYieldExpression(i.YieldExpression())
	}
	return nil
}
