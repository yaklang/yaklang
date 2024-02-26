package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
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
	var args []ssa.Value
	for _, expr := range i.ExpressionList().(*phpparser.ExpressionListContext).AllExpression() {
		val := y.VisitExpression(expr)
		args = append(args, val)
	}

	call := y.ir.NewCall(caller, args)
	y.ir.EmitCall(call)
	return nil
}
