package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitWhileStatement(raw phpparser.IWhileStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.WhileStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitDoWhileStatement(raw phpparser.IDoWhileStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.DoWhileStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitForStatement(raw phpparser.IForStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ForStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitContinueStatement(raw phpparser.IContinueStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ContinueStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitForeachStatement(raw phpparser.IForeachStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ForeachStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
