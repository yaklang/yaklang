package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitPanicStmt(raw yak.IPanicStmtContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*yak.PanicStmtContext)
	if i == nil {
		return
	}

	if op := i.Panic(); op != nil {
		y.writeString(op.GetText() + "(")
		y.VisitExpression(i.Expression())
		y.writeString(")")
		y.pushOperator(yakvm.OpPanic)
	}
}
