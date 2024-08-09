package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitDeferStmt(raw yak.IDeferStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeferStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("defer ")

	finished := y.SwitchCodes()
	if s := i.CallExpr(); s != nil {
		y.VisitCallExpr(s)
	} else if s := i.RecoverStmt(); s != nil {
		y.VisitRecoverStmt(s)
	} else if s := i.PanicStmt(); s != nil {
		y.VisitPanicStmt(s)
	}

	// defer recover only
	if len(y.codes) == 1 && y.codes[0].Opcode == yakvm.OpRecover {
		y.codes[0].Op1 = yakvm.NewValue("bool", true, `true`)
	}

	// 保证平栈
	y.pushOpPop()
	funcCode := make([]*yakvm.Code, len(y.codes))
	copy(funcCode, y.codes)
	finished()

	// defer 是一个语句，在结束之后在执行的
	y.pushDefer(funcCode)

	return nil
}
