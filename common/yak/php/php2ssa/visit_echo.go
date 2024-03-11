package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

func (y *builder) VisitEchoStatement(raw phpparser.IEchoStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.EchoStatementContext)
	if i == nil {
		return nil
	}

	caller := y.ir.ReadOrCreateVariable("echo")
	args := y.VisitExpressionList(i.ExpressionList())
	call := y.ir.NewCall(caller, args)
	y.ir.EmitCall(call)
	return nil
}
