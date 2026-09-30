package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitStatementList(raw yak.IStatementListContext, _ ...bool) interface{} {
	if raw == nil {
		return nil
	}
	i := raw.(*yak.StatementListContext)
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()
	for _, s := range i.AllStatement() {
		y.VisitStatement(s.(*yak.StatementContext))
	}
	return nil
}

// VisitStatement compiles one statement. Its boolean result is retained for
// source compatibility with callers of the former formatting visitor.
func (y *YakCompiler) VisitStatement(i *yak.StatementContext) (_ bool) {
	defer func() { _ = recover() }()

	if i == nil {
		return true
	}

	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	if s := i.LineCommentStmt(); s != nil {
		return false
	}

	if s := i.DeclareVariableExpressionStmt(); s != nil {
		y.VisitDeclareVariableExpressionStmt(s)
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
	if s := i.SelectStmt(); s != nil {
		y.VisitSelectStmt(s)
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
		// continue 跳过 block 剩余部分直接进入下一轮，需要先把 `:=` 循环变量
		// 的本轮值写回外层符号（Go 1.22 语义：下一轮变量从本轮结束值初始化）
		y.emitLoopVarCopyBack(y.peekForContext())
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
		y.VisitBlock(s)
		return false
	}

	if s := i.ReturnStmt(); s != nil {
		y.VisitReturnStmt(s)
		return true
	}

	if s := i.DeferStmt(); s != nil {
		y.VisitDeferStmt(s)
		return false
	}

	if s := i.Empty(); s != nil {
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
