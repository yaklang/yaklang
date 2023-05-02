package yakast

import (
	yak "yaklang.io/yaklang/common/yak/antlr4yak/parser"
	"yaklang.io/yaklang/common/yak/antlr4yak/yakvm"

	"github.com/google/uuid"
)

func (y *YakCompiler) PreviewStatementList(raw yak.IStatementListContext) (int, *yak.StatementContext) {
	if raw == nil {
		return -1, nil
	}
	i := raw.(*yak.StatementListContext)
	istmts := i.AllStatement()
	var firstStmt *yak.StatementContext
	if len(istmts) > 0 {
		firstStmt = istmts[0].(*yak.StatementContext)
	}
	return len(istmts), firstStmt
}

func (y *YakCompiler) VisitStatementList(raw yak.IStatementListContext, inline ...bool) interface{} {
	if raw == nil {
		return nil
	}
	i := raw.(*yak.StatementListContext)
	newLine := false
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	allStatement := i.AllStatement()
	lenOfAllStatement := len(allStatement)
	for index, s := range allStatement {
		if index == 0 && len(inline) > 0 && inline[0] {
		} else {
			y.writeIndent()
		}
		y.keepCommentLine(allStatement, index)

		newLine = y.VisitStatement(s.(*yak.StatementContext))
		if index < lenOfAllStatement-1 && newLine {
			y.writeNewLine()
		}
	}

	return nil
}

func (y *YakCompiler) VisitStatement(i *yak.StatementContext) (newLine bool) {
	defer func() {
		if e := recover(); e != nil {

		}
	}()
	defer y.writeEOS(i.Eos())

	if i == nil {
		return true
	}

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	if s := i.LineCommentStmt(); s != nil {
		y.VisitLineCommentStmt(s.(*yak.LineCommentStmtContext))
		return false
	}

	if s := i.DeclearVariableExpressionStmt(); s != nil {
		y.VisitDeclearVariableExpressionStmt(s)
		return false
	}

	if s := i.ExpressionStmt(); s != nil {
		y.VisitExpressionStmt(s)
		return false
	}

	if s := i.AssignExpressionStmt(); s != nil {
		y.VisitAssignExpressionStmt(s)
		return false
	}

	if s := i.IncludeStmt(); s != nil {
		y.VisitIncludeStmt(s)
		return true
	}

	//if s := i.FunctionDeclareStmt(); s != nil {
	//	y.VisitFunctionDeclareStmt(s)
	//	return nil
	//}

	if s := i.IfStmt(); s != nil {
		y.VisitIfStmt(s)
		return true
	}

	if s := i.SwitchStmt(); s != nil {
		y.VisitSwitchStmt(s)
		return true
	}

	if s := i.ForStmt(); s != nil {
		y.VisitForStmt(s)
		return true
	}

	if s := i.ForRangeStmt(); s != nil {
		y.VisitForRangeStmt(s)
		return true
	}

	if s := i.ContinueStmt(); s != nil {
		if !y.NowInFor() {
			y.panicCompilerError(continueError)
		}
		y.writeString("continue")
		y.writeEOS(i.Eos())
		var tryStart = -1
		if y.tryDepthStack.Len() > 0 {
			tryStart = y.tryDepthStack.Peek().(int)
			var start int = -1
			if y.NowInFor() {
				nearestForContext := y.forDepthStack.Peek().(*forContext)
				start = nearestForContext.startCodeIndex
			}
			if start != -1 && start < tryStart {
				y.pushOperator(yakvm.OpStopCatchError)
			}
		}
		y.pushContinue()
		return true
	}

	if s := i.BreakStmt(); s != nil {
		// break 目前应该出现在两个地方
		// 一个是 for 一个是 switch
		// 这两个流程本质上是相同的，但是实际操作的时候，公用一个关键字
		// 在 for 循环结束的时候
		// 给 for 循环没有设置过 break 的设置 break 的位置
		// 类似的的 switch 结束的时候，switch 范围内的也应该设置位置
		//
		// 如何在判断 for 还是 switch 内？其实不要紧，无需判断，for / switch 语句自己解决
		//
		if !y.NowInFor() && !y.NowInSwitch() {
			y.panicCompilerError(breakError)
		}
		y.writeString("break")
		y.writeEOS(i.Eos())

		var tryStart = -1
		if y.tryDepthStack.Len() > 0 {
			tryStart = y.tryDepthStack.Peek().(int)
			var start int = -1
			if y.NowInFor() {
				nearestForContext := y.forDepthStack.Peek().(*forContext)
				start = nearestForContext.startCodeIndex
			}
			if y.NowInSwitch() {
				nearestSwitchContext := y.switchDepthStack.Peek().(*switchContext)
				if start == -1 || start > nearestSwitchContext.startCodeIndex {
					start = nearestSwitchContext.startCodeIndex
				}
			}
			if start != -1 && start < tryStart {
				y.pushOperator(yakvm.OpStopCatchError)
			}
		}
		y.pushBreak()
		// fmt.Printf("debug : %#v\n", i.Eos().GetText())
		return true
	}
	//y.panicCompilerError(breakError)

	// fallthrough 实现在switch中,进行了特殊处理
	// 这里遇到fallthrough直接panic
	if s := i.FallthroughStmt(); s != nil {
		y.panicCompilerError(fallthroughError)
	}

	if s := i.GoStmt(); s != nil {
		y.VisitGoStmt(s)
		return true
	}

	if s := i.Block(); s != nil {
		tableRecover := y.SwitchSymbolTableInNewScope("block", uuid.New().String())
		y.VisitBlock(s)
		tableRecover()
		y.writeEOS(i.Eos())
		return false
	}

	if s := i.ReturnStmt(); s != nil {
		y.VisitReturnStmt(s)
		return true
	}

	if s := i.DeferStmt(); s != nil {
		y.VisitDeferStmt(s)
		return true
	}

	if s := i.Empty(); s != nil {
		y.writeEosWithText(s.GetText())
		return false
	}
	if s := i.AssertStmt(); s != nil {
		y.VisitAssertStmt(s)
		return false
	}

	if s := i.TryStmt(); s != nil {
		y.VisitTryStmt(s)
		return true
	}

	return true
}
