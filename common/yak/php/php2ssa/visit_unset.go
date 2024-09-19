package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitUnsetStatement(raw phpparser.IUnsetStatementContext) interface{} {
	if y == nil || raw == nil || y.isStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.UnsetStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
