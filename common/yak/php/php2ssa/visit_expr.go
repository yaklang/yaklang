package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitExpressionStatement(raw phpparser.IExpressionStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ExpressionStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
