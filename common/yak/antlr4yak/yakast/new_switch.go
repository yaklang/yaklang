package yakast

import (
	"fmt"
	"strings"

	uuid "github.com/satori/go.uuid"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

/*
cfg:
	// switch case, just jump
	if a == 1 {
		jump body1
		break
	} else if a==2{
		jump body3
		fallthought
	}else {
		jump body-default
	}

	// body list
	body1 {
		...
		break end-switch (current break counter) // break
		...
	}
	jump end-switch // body1:jump-to-end
	body2 {
		...
		break body-default:start-scope (current break counter) // fallthought
		...
	}
	jump end-switch
	body-default {
		// body-default:start-scope
		...
	}
	jump end-switch
	// end-swich

*/

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
		defaultCodeAddress int
		switchExprIsEmpty  bool
		jmp2Default        *yakvm.Code
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
		y.writeString(" {")
	}

	y.writeNewLine()

	allcases := i.AllCase()
	lenOfAllCases := len(allcases)
	jmpToCase := make([]*yakvm.Code, 0, lenOfAllCases)
	jmpToEnd := make([]*yakvm.Code, 0, lenOfAllCases)
	_fallthrough := make([]*yakvm.Code, 0)
	caseAddress := make([]int, 0, lenOfAllCases)
	conditionbuf := make([]string, 0, lenOfAllCases)

	// save jump to case with index
	pushJumpTrue := func(index int) {
		jmpt := y.pushJmpIfTrue()
		jmpt.Unary = index
		jmpToCase = append(jmpToCase, jmpt)
	}
	// build check list
	for index := range allcases {
		recoverFormatBufferFunc := y.switchFormatBuffer()
		y.writeString("case ")

		if exprs, ok := i.ExpressionList(index).(*yak.ExpressionListContext); ok {
			if len(exprs.AllExpression()) == 1 {
				// only one expression
				y.VisitExpression(exprs.AllExpression()[0])
				if !switchExprIsEmpty {
					y.pushRef(expressionResultID)
					y.pushOperator(yakvm.OpEq)
				}
				pushJumpTrue(index)
			} else {
				// multiple expression
				for i, e := range exprs.AllExpression() {
					y.VisitExpression(e)
					if !switchExprIsEmpty {
						y.pushRef(expressionResultID)
						y.pushOperator(yakvm.OpEq)
					}
					pushJumpTrue(index)
					if i < len(exprs.AllExpression())-1 {
						y.writeString(", ")
					}
				}
			}
		}

		y.writeString(":")
		y.writeNewLine()
		buf := recoverFormatBufferFunc()
		conditionbuf = append(conditionbuf, buf)
	}
	jmp2Default = y.pushJmp()

	// build body list
	for index := range allcases {
		// save case  body address
		stmtAddress := y.GetNextCodeIndex()
		caseAddress = append(caseAddress, stmtAddress)

		// new scope for body
		recoverSymtbl = y.SwitchSymbolTableInNewScope("case", uuid.NewV4().String())

		y.incIndent()
		recoverFormatBufferFunc := y.switchFormatBuffer()

		if stmt, ok := i.StatementList(index).(*yak.StatementListContext); ok {
			allStatement := stmt.AllStatement()
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
						// save in _fallthrough
						b := y.pushBreak()
						b.Unary = index
						_fallthrough = append(_fallthrough, b)
						continue
					}
					y.panicCompilerError(fallthroughError)
				}

				newline := y.VisitStatement(istmt.(*yak.StatementContext))
				if i < lenOfAllStatement-1 && newline {
					y.writeNewLine()
				}
			}
		}

		buf := recoverFormatBufferFunc()
		buf = strings.Trim(buf, "\n")
		y.writeString(conditionbuf[index] + buf)
		y.decIndent()
		y.writeNewLine()

		// end scope
		recoverSymtbl()
		// jump to switch end, when body finish
		jmpToEnd = append(jmpToEnd, y.pushJmp())
	}

	// default
	if i.Default() != nil {
		defaultCodeAddress = y.GetNextCodeIndex()
		// default body scope
		recoverSymtbl = y.SwitchSymbolTableInNewScope("default", uuid.NewV4().String())

		y.writeString("default:")
		y.writeNewLine()
		y.incIndent()

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

		// end scope
		recoverSymtbl()
	}

	// handler jump case
	for _, jmp := range jmpToCase {
		jmp.Unary = caseAddress[jmp.Unary]
	}

	endCodewithScopeEnd := y.GetNextCodeIndex()
	if defaultCodeAddress == 0 {
		defaultCodeAddress = endCodewithScopeEnd
	}

	jmp2Default.Unary = defaultCodeAddress

	// handler fallthough
	// 设置fallthrough跳转到下个statementlist的位置
	for _, b := range _fallthrough {
		// 最后一个的fallthrough应该跳转到default
		if b.Unary == lenOfAllCases-1 {
			b.Unary = defaultCodeAddress
		} else {
			b.Unary = caseAddress[b.Unary+1]
		}
		b.Op2 = yakvm.NewIntValue(2)
	}

	// 设置跳转到switch结尾的位置
	for _, jmp := range jmpToEnd {
		jmp.Unary = endCodewithScopeEnd
	}
	// handler break
	y.exitSwitchContext(endCodewithScopeEnd)

	y.writeString("}")

	return nil
}
