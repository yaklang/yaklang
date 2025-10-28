//go:build !no_language
// +build !no_language

package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

func (y *builder) VisitEchoStatement(raw phpparser.IEchoStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.EchoStatementContext)
	if i == nil {
		return nil
	}

	caller := y.ReadOrCreateVariable("echo")
	args := y.VisitExpressionList(i.ExpressionList())
	call := y.NewCall(caller, args)
	y.EmitCall(call)
	return nil
}
