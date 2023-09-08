package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

	uuid "github.com/satori/go.uuid"
)

type forContext struct {
	startCodeIndex       int
	continueScopeCounter int
	breakScopeCounter    int
	forRangeMode         bool
}

func (y *YakCompiler) enterForContext(start int) {
	y.forDepthStack.Push(&forContext{
		startCodeIndex: start,
	})
}

func (y *YakCompiler) enterForRangeContext(start int) {
	y.forDepthStack.Push(&forContext{
		startCodeIndex: start,
		forRangeMode:   true,
	})
}

func (y *YakCompiler) peekForContext() *forContext {
	raw, ok := y.forDepthStack.Peek().(*forContext)
	if ok {
		return raw
	} else {
		return nil
	}
}

func (y *YakCompiler) exitForContext(end int, continueIndex int) {
	start := y.peekForStartIndex()
	if start < 0 {
		return
	}

	for _, c := range y.codes[start:] {
		if c.Opcode == yakvm.OpBreak && c.Unary <= 0 {
			// 设置 for 开始到结尾的所有语句的 Break Code 的跳转值
			c.Unary = end
			if y.peekForContext().forRangeMode {
				c.Op2 = yakvm.NewIntValue(1) // for range mode
			}
		}

		if c.Opcode == yakvm.OpContinue && c.Unary <= 0 {
			if !y.peekForContext().forRangeMode {
				c.Op1.Value = c.Op1.Value.(int) - 1
			}
			c.Unary = continueIndex
		}
	}

	y.forDepthStack.Pop()
}

func (y *YakCompiler) VisitForStmt(raw yak.IForStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.ForStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("for ")

	// _ 记录一下开始的索引，一般是 continue 的时候
	startIndex := y.GetNextCodeIndex()
	startIndex += 1 // skip new-scope instruction
	y.enterForContext(startIndex)

	var endThirdExpr yak.IForThirdExprContext

	f := y.SwitchSymbolTableInNewScope("for-legacy", uuid.NewV4().String())

	var toEnds []*yakvm.Code
	var conditionSymbol int
	if e := i.Expression(); e != nil {
		y.VisitExpression(e)
		var toEnd = y.pushJmpIfFalse()
		toEnds = append(toEnds, toEnd)
	} else if cond := i.ForStmtCond(); cond != nil {
		condIns := cond.(*yak.ForStmtCondContext)
		if entry := condIns.ForFirstExpr(); entry != nil {
			y.VisitForFirstExpr(entry)
		}
		y.writeString("; ")

		startIndex = y.GetNextCodeIndex()
		if condIns.Expression() != nil {
			conditionSymbol = y.currentSymtbl.NewSymbolWithoutName()
			y.pushLeftRef(conditionSymbol)
			y.VisitExpression(condIns.Expression())
			// 为了后面可以根据条件判断是否执行第三条语句，我们需要把结果缓存到中间符号中
			y.pushOperator(yakvm.OpFastAssign)
			// 条件应该是 forEnd 不是 blockEnd
			var toEnd = y.pushJmpIfFalse()
			toEnds = append(toEnds, toEnd)
		}
		y.writeString("; ")

		if e := condIns.ForThirdExpr(); e != nil {
			endThirdExpr = e
		}
	}
	// for 执行体结束之后应该无条件跳转回开头，重新判断
	// 但是三语句 for ;; 应该是 block 执行解释后执行第三条语句
	recoverFormatBufferFunc := y.switchFormatBuffer()
	y.VisitBlock(i.Block())
	buf := recoverFormatBufferFunc()

	// continue index
	continueIndex := y.GetNextCodeIndex()

	if endThirdExpr != nil {
		if conditionSymbol > 0 {
			y.pushRef(conditionSymbol)
			var toEnd = y.pushJmpIfFalse()
			toEnds = append(toEnds, toEnd)
		}
		y.VisitForThirdExpr(endThirdExpr)
		y.writeString(" ")
	}
	y.writeString(buf)
	y.pushJmp().Unary = startIndex
	var forEnd = y.GetNextCodeIndex()

	f()
	// 设置解析的 block 中没有设置过的 break
	y.exitForContext(forEnd+1, continueIndex)

	// 设置条件自带的 toEnd 位置
	for _, toEnd := range toEnds {
		if toEnd != nil {
			toEnd.Unary = forEnd
		}
	}

	return nil
}

func (y *YakCompiler) VisitForRangeStmt(raw yak.IForRangeStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.ForRangeStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("for ")

	recoverFormatBufferFunc := y.switchFormatBuffer()
	expr := i.Expression()
	if expr == nil {
		y.panicCompilerError(compileError, "for-range/in need expression in right value at least")
	}

	recoverSymtbl := y.SwitchSymbolTableInNewScope("for", uuid.NewV4().String())
	defer recoverSymtbl()

	var defaultValueSymbol, err = y.currentSymtbl.NewSymbolWithReturn("_")
	if err != nil {
		y.panicCompilerError(compileError, "cannot create `_` variable, reason: "+err.Error())
	}

	/*
		for range: range 后表达式计算后符号化
	*/
	// 为 for range 表达式创建一个新符号，这个然后赋值给这个表达式
	expressionResultID := y.currentSymtbl.NewSymbolWithoutName()
	y.pushLeftRef(expressionResultID)
	// enter for-range
	y.VisitExpression(expr)
	buf := recoverFormatBufferFunc()
	y.pushOperator(yakvm.OpFastAssign)
	defer y.pushOpPop()

	// OpEnterFR 会从栈上pop出来右值创建迭代器，这个值其实不必要被 pop 出，每次 Peek 足够了
	enterFR := y.pushEnterFR()

	// _ 记录一下开始的索引，一般是 continue 的时候无条件跳转的
	startIndex := y.GetNextCodeIndex()
	y.enterForRangeContext(startIndex)

	// 计算出 range 左值的数量
	var n = 0
	if l := i.LeftExpressionList(); l != nil { // 访问左值
		n = len(l.(*yak.LeftExpressionListContext).AllLeftExpression())
		// 一般来说左值有两个，有一个和两个的情况赋值不一样，这个要看具体实现
		// 但是在for-range下不能大于 2, 这是有问题的
		if n > 2 && i.Range() != nil {
			y.panicCompilerError(compileError, "`for ... range` accept up to tow left expression value")
		}
	}

	// peek ExpressionResultID 使用 RangeNext 或者 InNext 进行迭代计算
	// 迭代计算之后，应该是一个 list，可以作为赋值的右值
	var rightAtLeast = 1
	if n > 1 {
		rightAtLeast = n
	}

	var nextCode *yakvm.Code
	if i.In() != nil {
		nextCode = y.pushInNext(rightAtLeast)
	} else {
		nextCode = y.pushRangeNext(rightAtLeast) // 遍历前面的expression
	}

	// 如果左值没有的话，应该保留一些值吗？当然，应该给 _ 赋值成当前循环的次数或者第一个值
	if n <= 0 {
		y.pushLeftRef(defaultValueSymbol)
		y.pushListWithLen(1)
	} else {
		n = y.VisitLeftExpressionList(true, i.LeftExpressionList())
		if n == -1 {
			y.panicCompilerError(compileError, "invalid left expression list")
		}
	}
	y.pushOperator(yakvm.OpAssign) // 赋值给左边的变量
	if op, op2 := i.In(), i.Range(); op != nil || op2 != nil {
		if op != nil {
			y.writeStringWithWhitespace(op.GetText())
		} else {
			eq, ceq := i.AssignEq(), i.ColonAssignEq()
			if eq != nil {
				y.writeStringWithWhitespace(eq.GetText())
			} else if ceq != nil {
				y.writeStringWithWhitespace(ceq.GetText())
			}
			y.writeString(op2.GetText() + " ")
		}
	}
	y.writeString(buf + " ")
	y.VisitBlock(i.Block())

	exitFR := y.GetNextCodeIndex()
	// exit for-range

	// 设置next code的跳转位置,用于关闭的管道
	nextCode.Op1 = yakvm.NewIntValue(exitFR)

	forEnd := y.GetNextCodeIndex()

	// 设置解析的 block 中没有设置过的 break
	y.exitForContext(forEnd+2, forEnd)

	// 设置enterFR的跳转,如果为空则直接跳转
	enterFR.Unary = forEnd + 1
	y.pushExitFR(startIndex)

	// for range 起始位置
	// 循环次数通过 _ 变量赋值，退出条件为 range
	// 不一样的是，for range 需要支持三种情况至少
	//   1. 针对一个 slice 的 range
	//   2. 针对一个 map 的 range
	//   3. 针对一个整数的 range，这种情况 golang 没有
	//  	预期为 for range 4 { println(1) } 将会打印 4 个 1\n，等价于 for range [0,1,2,3] {}...

	return nil
}

func (y *YakCompiler) VisitForThirdExpr(raw yak.IForThirdExprContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.ForThirdExprContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	if ae := i.AssignExpression(); ae != nil {
		// 复制表达式是平栈的，不需要额外 pop
		y.VisitAssignExpression(ae)
		return nil
	}

	if e := i.Expression(); e != nil {
		y.VisitExpression(e)
		y.pushOpPop()
		return nil
	}

	y.panicCompilerError(compileError, "visit first for expr failed")

	return nil
}

func (y *YakCompiler) VisitForFirstExpr(raw yak.IForFirstExprContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.ForFirstExprContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	if ae := i.AssignExpression(); ae != nil {
		// 复制表达式是平栈的，不需要额外 pop
		y.VisitAssignExpression(ae)
		return nil
	}

	if e := i.Expression(); e != nil {
		y.VisitExpression(e)
		y.pushOpPop()
		return nil
	}

	y.panicCompilerError(compileError, "visit first for expr failed")
	return nil
}
