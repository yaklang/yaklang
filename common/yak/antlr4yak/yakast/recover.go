package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitRecoverStmt(raw yak.IRecoverStmtContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*yak.RecoverStmtContext)
	if i == nil {
		return
	}

	if op := i.Recover(); op != nil {
		y.writeString(op.GetText() + "()")
		y.pushOperator(yakvm.OpRecover)
	}
}
