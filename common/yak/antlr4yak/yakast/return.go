package yakast

import (
	yak "yaklang.io/yaklang/common/yak/antlr4yak/parser"
	"yaklang.io/yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitReturnStmt(raw yak.IReturnStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.ReturnStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("return")

	// 这是一个压栈操作，虚拟机要记录返回值，所以需要 return 作为 OPCODE 去操作栈
	if list := i.ExpressionList(); list != nil {
		y.writeString(" ")
		y.VisitExpressionList(list)
	}
	if y.tryDepthStack.Len() > 0 {
		y.pushOperator(yakvm.OpStopCatchError)
	}
	y.pushOperator(yakvm.OpReturn)

	return nil
}
