package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitIfStatement(raw phpparser.IIfStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.IfStatementContext)
	if i == nil {
		return nil
	}

	if i.Colon() == nil {
		// classicIf
		/*
			if (true) echo "abc";
			if (true) echo "abc"; else if true 1+1;
			if (true) echo "abc"; else if true 1+1; else "abc"."ccc";
		*/
		i := y.main.BuildIf()
		i.BuildCondition(func() ssa.Value {
			return y.VisitExpression(i.Expression())
		})
		y.VisitParentheses(i.Parentheses())
		y.VisitStatement(i.Statement())
	} else {
		// tag if
	}

	return nil
}

func (y *builder) VisitSwitchStatement(raw phpparser.ISwitchStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.SwitchStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
