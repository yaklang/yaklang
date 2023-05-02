package yakast

import (
	"fmt"
	"strings"
	yak "yaklang.io/yaklang/common/yak/antlr4yak/parser"
	"yaklang.io/yaklang/common/yak/antlr4yak/yakvm"

	uuid "github.com/satori/go.uuid"
)

type switchContext struct {
	startCodeIndex          int
	switchBreakScopeCounter int
}

func (y *YakCompiler) enterSwitchContext(start int) {
	y.switchDepthStack.Push(&switchContext{
		startCodeIndex: start,
	})
}

func (y *YakCompiler) peekSwitchContext() *switchContext {
	raw, ok := y.switchDepthStack.Peek().(*switchContext)
	if ok {
		return raw
	} else {
		return nil
	}
}

func (y *YakCompiler) exitSwitchContext(end int) {
	start := y.peekSwitchStartIndex()
	if start <= 0 {
		return
	}

	for _, c := range y.codes[start:] {
		if c.Opcode == yakvm.OpBreak && c.Unary <= 0 {
			// 设置 for 开始到结尾的所有语句的 Break Code 的跳转值
			c.Unary = end
		}
	}

	y.switchDepthStack.Pop()
}

func (y *YakCompiler) VisitSwitchStmt(raw yak.ISwitchStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.SwitchStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("switch ")

	var (
		defaultCodeIndex  int
		switchExprIsEmpty bool
	)

	recoverSymtbl := y.SwitchSymbolTableInNewScope("switch", uuid.NewV4().String())
	defer recoverSymtbl()

	startIndex := y.GetNextCodeIndex()
	y.enterSwitchContext(startIndex)

	symbolName := fmt.Sprintf("$switch:%v$", y.GetNextCodeIndex())
	expressionResultID, err := y.currentSymtbl.NewSymbolWithReturn(symbolName)
	if err != nil {
		y.panicCompilerError(CreateSymbolError, symbolName)
	}

	// 将表达式的值放入符号中
	if e := i.Expression(); e != nil {
		y.VisitExpression(i.Expression())
		y.pushListWithLen(1)
		// 设置左值，这个左值是一个新建的符号！
		y.pushLeftRef(expressionResultID)
		y.pushListWithLen(1)
		// 为右值创建一个符号，这个符号为 rightExpressionSymbol
		y.pushOperator(yakvm.OpAssign)
		y.writeString(" {")
	} else {
		//y.pushUndefined()
		switchExprIsEmpty = true
		y.writeString("{")
	}

	y.writeNewLine()

	allcases := i.AllCase()
	lenOfAllCases := len(allcases)
	jmpToEnd := make([]*yakvm.Code, 0, lenOfAllCases)
	jmpToNextCase := make([]*yakvm.Code, 0, lenOfAllCases)
	jmpFallthrough := make([]*yakvm.Code, 0)
	nextCaseIndexs := make([]int, 0, lenOfAllCases)
	nextStmtIndexs := make([]int, 0, lenOfAllCases)

	for index := range allcases {
		recoverSymtbl = y.SwitchSymbolTableInNewScope("case", uuid.NewV4().String())
		y.writeString("case ")

		var jmpToStmt []*yakvm.Code
		// 获取下个case的位置
		caseIndex := y.GetNextCodeIndex()
		nextCaseIndexs = append(nextCaseIndexs, caseIndex)

		// if 判断
		iExprs := i.ExpressionList(index)
		if iExprs != nil {
			exprs := iExprs.(*yak.ExpressionListContext)
			lenOfExprs := len(exprs.AllExpression())
			// 如果只有一个表达式,则直接用eq判断
			if lenOfExprs == 1 {
				y.VisitExpression(exprs.AllExpression()[0])
				if !switchExprIsEmpty {
					y.pushRef(expressionResultID)
					y.pushOperator(yakvm.OpEq)
				}
			} else { // 如果多个表达式,要短路处理

				for i, e := range exprs.AllExpression() {
					y.VisitExpression(e)
					if !switchExprIsEmpty {
						y.pushRef(expressionResultID)
						y.pushOperator(yakvm.OpEq)
					}
					jmpToStmt = append(jmpToStmt, y.pushJmpIfTrue())
					if i < lenOfExprs-1 {
						y.writeString(", ")
					}
				}
				// 最后补一个false,用于下面的jmpToNextCase条件判断
				y.pushBool(false)
			}
		}
		y.writeString(":")
		y.writeNewLine()
		y.incIndent()

		// 如果不相等，跳转到下个case
		jmpToNextCase = append(jmpToNextCase, y.pushJmpIfFalse())

		// 获取下个statementlist的位置
		stmtIndex := y.GetNextCodeIndex()
		nextStmtIndexs = append(nextStmtIndexs, stmtIndex)

		// 设置条件短路
		for _, jmp := range jmpToStmt {
			jmp.Unary = stmtIndex
		}

		// 执行case中的语句,由于有fallthrough需要获取上下文,不能直接用VisitStatementList和VisitStatement
		recoverFormatBufferFunc := y.switchFormatBuffer()
		iStmts := i.StatementList(index)
		if iStmts != nil {
			stmts := iStmts.(*yak.StatementListContext)
			allStatement := stmts.AllStatement()
			lenOfAllStatement := len(allStatement)
			for i, istmt := range allStatement {
				if istmt == nil {
					continue
				}
				stmt := istmt.(*yak.StatementContext)
				// 忽略开头的empty
				if i == 0 && stmt.Empty() != nil {
					continue
				}

				y.writeIndent()

				if s := stmt.FallthroughStmt(); s != nil {
					if y.NowInSwitch() {
						y.writeString("fallthrough")
						y.writeEOS(stmt.Eos())
						jmp := y.pushJmp()
						// 暂时设置为index, 后面会设置为跳转到下一个statementlist的位置
						jmp.Unary = index
						jmpFallthrough = append(jmpFallthrough, jmp)
						continue
					}
					y.panicCompilerError(fallthroughError)
				} else {
				}

				newline := y.VisitStatement(istmt.(*yak.StatementContext))
				if i < lenOfAllStatement-1 && newline {
					y.writeNewLine()
				}
			}
		}
		buf := recoverFormatBufferFunc()
		buf = strings.Trim(buf, "\n")
		y.writeString(buf)
		y.decIndent()
		y.writeNewLine()

		// 跳转到switch结尾
		jmpToEnd = append(jmpToEnd, y.pushJmp())
		recoverSymtbl()
	}

	// 访问default statementlist
	if i.Default() != nil {
		recoverSymtbl = y.SwitchSymbolTableInNewScope("default", uuid.NewV4().String())
		y.writeString("default:")
		y.writeNewLine()
		y.incIndent()

		defaultCodeIndex = y.GetNextCodeIndex()

		recoverFormatBufferFunc := y.switchFormatBuffer()
		stmts := i.StatementList(len(allcases)).(*yak.StatementListContext)
		// y.VisitStatementList(stmts)
		allStatement := stmts.AllStatement()
		lenOfAllStatement := len(allStatement)
		for i, istmt := range allStatement {
			if istmt == nil {
				continue
			}
			stmt := istmt.(*yak.StatementContext)
			// 忽略开头的empty
			if i == 0 && stmt.Empty() != nil {
				continue
			}

			y.writeIndent()
			newline := y.VisitStatement(istmt.(*yak.StatementContext))
			if i < lenOfAllStatement-1 && newline {
				y.writeNewLine()
			}
		}

		buf := recoverFormatBufferFunc()
		buf = strings.Trim(buf, "\n")
		y.writeString(buf)
		y.decIndent()
		y.writeNewLine()
		recoverSymtbl()
	}

	endCodewithScopeEnd := y.GetNextCodeIndex()
	// 如果没有default,则跳转到switch结尾
	if defaultCodeIndex == 0 {
		defaultCodeIndex = endCodewithScopeEnd
	}

	endCode := y.GetNextCodeIndex()

	// 设置fallthrough跳转到下个statementlist的位置

	for _, jmp := range jmpFallthrough {
		// 最后一个的fallthrough应该跳转到default
		if jmp.Unary == lenOfAllCases-1 {
			jmp.Unary = defaultCodeIndex
		} else {
			jmp.Unary = nextStmtIndexs[jmp.Unary+1]
		}
	}

	// 设置跳转到下个case的位置
	for index, jmp := range jmpToNextCase[:len(jmpToNextCase)-1] {
		jmp.Unary = nextCaseIndexs[index+1]
	}

	// 设置最后一个case跳转到default
	jmpToNextCase[len(jmpToNextCase)-1].Unary = defaultCodeIndex

	// 设置跳转到switch结尾的位置
	for _, jmp := range jmpToEnd {
		jmp.Unary = endCodewithScopeEnd
	}
	// 设置break跳转到switch结尾的位置,没有scopeEnd,因为break自带了scopeEnd
	y.exitSwitchContext(endCode + 1)

	y.writeString("}")

	return nil
}
