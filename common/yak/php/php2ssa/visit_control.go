package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitBreakStatement(raw phpparser.IBreakStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.BreakStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitReturnStatement(raw phpparser.IReturnStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ReturnStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitYieldExpression(raw phpparser.IYieldExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.YieldExpressionContext)
	if i == nil {
		return nil
	}

	if i.From() != nil {
		// yield from expression
		key := y.VisitExpression(i.Expression(0))
		_ = key
		return nil // NewIterator nil
	}

	key := y.VisitExpression(i.Expression(0))
	if i.DoubleArrow() != nil {
		// yield key => value
		val := y.VisitExpression(i.Expression(1))
		_ = key
		_ = val
		return nil
	}

	// yield key;
	return nil
}

func (y *builder) VisitGotoStatement(raw phpparser.IGotoStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.GotoStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitEmptyStatement(raw phpparser.IEmptyStatement_Context) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.EmptyStatement_Context)
	if i == nil {
		return nil
	}

	return nil
}
