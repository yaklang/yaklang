package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitIfStatement(raw phpparser.IIfStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.IfStatementContext)
	if i == nil {
		return nil
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
