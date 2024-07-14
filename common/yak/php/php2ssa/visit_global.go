package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitGlobalStatement(raw phpparser.IGlobalStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.GlobalStatementContext)
	if i == nil {
		return nil
	}
	return nil
}
func (y *builder) VisitGlobalVar(raw phpparser.IGlobalVarContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.GlobalVarContext)
	if i == nil {
		return nil
	}

	return nil
}
