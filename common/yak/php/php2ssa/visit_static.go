package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitStaticVariableStatement(raw phpparser.IStaticVariableStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.StaticVariableStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
