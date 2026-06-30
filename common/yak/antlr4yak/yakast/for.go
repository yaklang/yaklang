package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

	uuid "github.com/google/uuid"
)

type forContext struct {
	startCodeIndex       int
	continueScopeCounter int
	breakScopeCounter    int
	forRangeMode         bool
	// loopVarBindings 记录三段式 for 中 `:=` 声明的循环变量在
	// 外层 for-legacy 作用域与本次迭代 block 作用域中的符号对应关系，
	// 用于在 block 正常结束与 continue 时把 block 内的值写回外层符号
	// （Go 1.22 语义：下一轮迭代的新变量从上一轮结束时的值初始化）。
	loopVarBindings []loopVarBinding
}

type loopVarBinding struct {
	name    string
	outerID int
	bodyID  int
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
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("for ")

	// _ 记录一下开始的索引，一般是 continue 的时候
	startIndex := y.GetNextCodeIndex()
	startIndex += 1 // skip new-scope instruction
	y.enterForContext(startIndex)

	var endThirdExpr yak.IForThirdExprContext

	f := y.SwitchSymbolTableInNewScope("for-legacy", uuid.New().String())

	// Go 1.22 语义：仅当初始化语句用 `:=` 声明循环变量时，每次迭代隐式创建
	// 独立的同名变量（块内使用），块结束/continue 时把值写回外层符号，
	// 供条件、第三条语句以及下一轮迭代复制使用。
	// `for i = 0; ...`（赋值）与循环外声明的变量保持共享旧行为，与 Go 一致。
	var loopVars []loopVarBinding

	var toEnds []*yakvm.Code
	var conditionSymbol int
	if e := i.Expression(); e != nil {
		y.VisitExpression(e)
		toEnd := y.pushJmpIfFalse()
		toEnds = append(toEnds, toEnd)
	} else if cond := i.ForStmtCond(); cond != nil {
		condIns := cond.(*yak.ForStmtCondContext)
		if entry := condIns.ForFirstExpr(); entry != nil {
			y.VisitForFirstExpr(entry)
			loopVars = y.collectColonAssignLoopVars(entry)
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
			toEnd := y.pushJmpIfFalse()
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
	if len(loopVars) > 0 {
		forCtx := y.peekForContext()
		y.VisitBlockWithCallbacks(i.Block(), func(y *YakCompiler) {
			// 进入本次迭代的 block 作用域：为每个 `:=` 循环变量建立独立拷贝
			bindings := make([]loopVarBinding, 0, len(loopVars))
			for _, lv := range loopVars {
				bodyID, err := y.currentSymtbl.NewSymbolWithReturn(lv.name)
				if err != nil {
					y.panicCompilerError(forceCreateSymbolFailed, lv.name)
				}
				y.pushRef(lv.outerID)
				y.pushListWithLen(1)
				y.pushLeftRef(bodyID)
				y.pushListWithLen(1)
				y.pushOperator(yakvm.OpAssign)
				bindings = append(bindings, loopVarBinding{name: lv.name, outerID: lv.outerID, bodyID: bodyID})
			}
			if forCtx != nil {
				forCtx.loopVarBindings = bindings
			}
		}, func(y *YakCompiler) {
			// block 正常结束（未 continue/break）：把本次迭代的值写回外层符号
			y.emitLoopVarCopyBack(forCtx)
			if forCtx != nil {
				forCtx.loopVarBindings = nil
			}
		})
	} else {
		y.VisitBlock(i.Block())
	}
	buf := recoverFormatBufferFunc()

	// continue index
	continueIndex := y.GetNextCodeIndex()

	if endThirdExpr != nil {
		if conditionSymbol > 0 {
			y.pushRef(conditionSymbol)
			toEnd := y.pushJmpIfFalse()
			toEnds = append(toEnds, toEnd)
		}
		y.VisitForThirdExpr(endThirdExpr)
		y.writeString(" ")
	}
	y.writeString(buf)
	y.pushJmp().Unary = startIndex
	forEnd := y.GetNextCodeIndex()

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
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("for ")

	recoverFormatBufferFunc := y.switchFormatBuffer()
	expr := i.Expression()
	if expr == nil {
		y.panicCompilerError(compileError, "for-range/in need expression in right value at least")
	}

	recoverSymtbl := y.SwitchSymbolTableInNewScope("for", uuid.New().String())
	defer recoverSymtbl()

	defaultValueSymbol, err := y.currentSymtbl.NewSymbolWithReturn("_")
	if err != nil {
		y.panicCompilerError(compileError, "cannot create `_` variable, reason: "+err.Error())
	}

	/*
		for range: range 后表达式计算后符号化
	*/
	// ! 不应该为迭代的对象创建一个新的符号，这样会导致自修改出现问题，且在迭代后右值所指向的左值被修改，所有后续的自修改失效
	// expressionResultID := y.currentSymtbl.NewSymbolWithoutName()
	// y.pushLeftRef(expressionResultID)
	// enter for-range
	y.VisitExpression(expr)
	buf := recoverFormatBufferFunc()
	// y.pushOperator(yakvm.OpFastAssign)
	defer y.pushOpPop()

	// OpEnterFR 会从栈上pop出来右值创建迭代器，这个值其实不必要被 pop 出，每次 Peek 足够了
	enterFR := y.pushEnterFR()

	// _ 记录一下开始的索引，一般是 continue 的时候无条件跳转的
	startIndex := y.GetNextCodeIndex()
	y.enterForRangeContext(startIndex)

	// 计算出 range 左值的数量
	n := 0
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
	rightAtLeast := 1
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

	// Go 1.22 for-range 语义：循环变量按迭代独立绑定。
	// 这里收集具名的循环变量(标识符左值，跳过 `_` 与成员/下标左值)及其在
	// 外层 for 作用域中的符号 id，稍后在每次迭代的 block 作用域内建立同名拷贝，
	// 使 `go func(){...}` / defer / 闭包捕获到的是"本次迭代"的值，而非所有迭代
	// 共享的同一个外层符号（旧版本需要手写 `val := val` 规避，现在默认即安全）。
	var loopVarNames []string
	var loopVarOuterIDs []int
	if n > 0 {
		if l, ok := i.LeftExpressionList().(*yak.LeftExpressionListContext); ok && l != nil {
			for _, le := range l.AllLeftExpression() {
				name, ok := leftExpressionIdentifierName(le)
				if !ok || name == "" || name == "_" {
					continue
				}
				if id, ok := y.currentSymtbl.GetSymbolByVariableName(name); ok {
					loopVarNames = append(loopVarNames, name)
					loopVarOuterIDs = append(loopVarOuterIDs, id)
				}
			}
		}
	}

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
	if len(loopVarNames) > 0 {
		y.VisitBlockWithCallback(i.Block(), func(y *YakCompiler) {
			// 进入本次迭代的 block 作用域后立即执行，为每个具名循环变量在
			// block 作用域内建立一份独立拷贝并遮蔽外层同名符号。
			// 只发射 opcode，不写格式化文本，避免影响 yakfmt 输出。
			for idx, name := range loopVarNames {
				outerID := loopVarOuterIDs[idx]
				// 右值：读取外层(本次迭代)循环变量的当前值
				y.pushRef(outerID)
				y.pushListWithLen(1)
				// 左值：在 block 符号表中新建同名符号（遮蔽外层）
				newID, err := y.currentSymtbl.NewSymbolWithReturn(name)
				if err != nil {
					y.panicCompilerError(forceCreateSymbolFailed, name)
				}
				y.pushLeftRef(newID)
				y.pushListWithLen(1)
				y.pushOperator(yakvm.OpAssign)
			}
		})
	} else {
		y.VisitBlock(i.Block())
	}

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

// collectColonAssignLoopVars 检查三段式 for 的初始化语句是否为 `:=` 声明，
// 若是则收集其中裸标识符循环变量的名字与其在当前(for-legacy)作用域中的符号 id。
// 仅 `:=` 声明触发 Go 1.22 每迭代独立变量语义；`=` 赋值保持共享行为。
func (y *YakCompiler) collectColonAssignLoopVars(raw yak.IForFirstExprContext) []loopVarBinding {
	i, _ := raw.(*yak.ForFirstExprContext)
	if i == nil {
		return nil
	}
	ae, _ := i.AssignExpression().(*yak.AssignExpressionContext)
	if ae == nil || ae.ColonAssignEq() == nil {
		return nil
	}
	l, _ := ae.LeftExpressionList().(*yak.LeftExpressionListContext)
	if l == nil {
		return nil
	}
	var result []loopVarBinding
	for _, le := range l.AllLeftExpression() {
		name, ok := leftExpressionIdentifierName(le)
		if !ok || name == "" || name == "_" {
			continue
		}
		if id, ok := y.currentSymtbl.GetSymbolByVariableName(name); ok {
			result = append(result, loopVarBinding{name: name, outerID: id})
		}
	}
	return result
}

// emitLoopVarCopyBack 发射把本次迭代 block 内循环变量值写回外层符号的 opcode。
// 在 block 正常结束与 continue 语句处调用，保证条件/第三条语句与下一轮迭代
// 能看到本轮 body 对循环变量的修改（与 Go 1.22 规范一致）。
func (y *YakCompiler) emitLoopVarCopyBack(forCtx *forContext) {
	if forCtx == nil {
		return
	}
	for _, b := range forCtx.loopVarBindings {
		y.pushRef(b.bodyID)
		y.pushListWithLen(1)
		y.pushLeftRef(b.outerID)
		y.pushListWithLen(1)
		y.pushOperator(yakvm.OpAssign)
	}
}

// leftExpressionIdentifierName 从一个 leftExpression 中提取"裸标识符"名字。
// 仅当左值是纯标识符（如 `val`）时返回其名字；成员/下标等复杂左值返回 false。
// 语法为 `leftExpression: Identifier | expression`，因此需同时兼容两种情形。
func leftExpressionIdentifierName(raw yak.ILeftExpressionContext) (string, bool) {
	le, ok := raw.(*yak.LeftExpressionContext)
	if !ok || le == nil {
		return "", false
	}
	if id := le.Identifier(); id != nil {
		return id.GetText(), true
	}
	expr, ok := le.Expression().(*yak.ExpressionContext)
	if !ok || expr == nil {
		return "", false
	}
	// 兜底：expression 顶层恰好是裸标识符
	if id := expr.Identifier(); id != nil && len(expr.AllExpression()) == 0 &&
		expr.MemberCall() == nil && expr.SliceCall() == nil && expr.FunctionCall() == nil {
		return id.GetText(), true
	}
	return "", false
}

func (y *YakCompiler) VisitForThirdExpr(raw yak.IForThirdExprContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.ForThirdExprContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
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
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
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
