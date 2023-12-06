package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitGlobalStatement(raw phpparser.IGlobalStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.GlobalStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
