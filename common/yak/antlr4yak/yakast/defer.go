package yakast

import (
	yak "yaklang/common/yak/antlr4yak/parser"
	"yaklang/common/yak/antlr4yak/yakvm"
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
	y.VisitExpression(i.Expression())
	// 保证平栈
	y.pushOpPop()
	funcCode := make([]*yakvm.Code, len(y.codes))
	copy(funcCode, y.codes)
	finished()

	// defer 是一个语句，在结束之后在执行的
	y.pushDefer(funcCode)

	return nil
}
