package visitors

import (
	"fmt"
	"strconv"
	"strings"

	nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (c *Compiler) VisitExpressionSequence(i nasl.IExpressionSequenceContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	expseq, ok := i.(*nasl.ExpressionSequenceContext)
	if ok {
		for _, exp := range expseq.AllSingleExpression() {
			c.VisitSingleExpression(exp)
		}
	}
}

func (c *Compiler) VisitSingleExpression(i nasl.ISingleExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	if memberIndexExpression, ok := i.(*nasl.MemberIndexExpressionContext); ok {
		c.VisitMemberIndexExpression(memberIndexExpression)
	}
	if unaryPlusExpression, ok := i.(*nasl.UnaryPlusExpressionContext); ok {
		c.VisitUnaryPlusExpression(unaryPlusExpression)
	}
	if unaryMinusExpression, ok := i.(*nasl.UnaryMinusExpressionContext); ok {
		c.VisitUnaryMinusExpression(unaryMinusExpression)
	}
	if bitNotExpression, ok := i.(*nasl.BitNotExpressionContext); ok {
		c.VisitBitNotExpression(bitNotExpression)
	}
	if notExpression, ok := i.(*nasl.NotExpressionContext); ok {
		c.VisitNotExpression(notExpression)
	}
	if multiplicativeExpression, ok := i.(*nasl.MultiplicativeExpressionContext); ok {
		c.VisitMultiplicativeExpression(multiplicativeExpression)
	}
	if additiveExpression, ok := i.(*nasl.AdditiveExpressionContext); ok {
		c.VisitAdditiveExpression(additiveExpression)
	}
	if bitShiftExpression, ok := i.(*nasl.BitShiftExpressionContext); ok {
		c.VisitBitShiftExpression(bitShiftExpression)
	}
	if relationalExpression, ok := i.(*nasl.RelationalExpressionContext); ok {
		c.VisitRelationalExpression(relationalExpression)
	}
	if equalityExpression, ok := i.(*nasl.EqualityExpressionContext); ok {
		c.VisitEqualityExpression(equalityExpression)
	}
	if bitAndExpression, ok := i.(*nasl.BitAndExpressionContext); ok {
		c.VisitBitAndExpression(bitAndExpression)
	}
	if bitXOrExpression, ok := i.(*nasl.BitXOrExpressionContext); ok {
		c.VisitBitXOrExpression(bitXOrExpression)
	}
	if bitOrExpression, ok := i.(*nasl.BitOrExpressionContext); ok {
		c.VisitBitOrExpression(bitOrExpression)
	}
	if logicalAndExpression, ok := i.(*nasl.LogicalAndExpressionContext); ok {
		c.VisitLogicalAndExpression(logicalAndExpression)
	}
	if logicalOrExpression, ok := i.(*nasl.LogicalOrExpressionContext); ok {
		c.VisitLogicalOrExpression(logicalOrExpression)
	}
	if identifierExpression, ok := i.(*nasl.IdentifierExpressionContext); ok {
		c.VisitIdentifierExpression(identifierExpression)
	}
	if literalExpression, ok := i.(*nasl.LiteralExpressionContext); ok {
		c.VisitLiteralExpression(literalExpression)
	}
	if arrayLiteralExpression, ok := i.(*nasl.ArrayLiteralExpressionContext); ok {
		c.VisitArrayLiteralExpression(arrayLiteralExpression)
	}
	if callExpression, ok := i.(*nasl.CallExpressionContext); ok {
		c.VisitCallExpression(callExpression)
	}
	if parenthesizedExpression, ok := i.(*nasl.ParenthesizedExpressionContext); ok {
		c.VisitParenthesizedExpression(parenthesizedExpression)
	}
	if postIncrementExpression, ok := i.(*nasl.PostIncrementExpressionContext); ok {
		c.VisitPostIncrementExpression(postIncrementExpression)
	}
	if postDecreaseExpression, ok := i.(*nasl.PostDecreaseExpressionContext); ok {
		c.VisitPostDecreaseExpression(postDecreaseExpression)
	}
	if preIncrementExpression, ok := i.(*nasl.PreIncrementExpressionContext); ok {
		c.VisitPreIncrementExpression(preIncrementExpression)
	}
	if preDecreaseExpression, ok := i.(*nasl.PreDecreaseExpressionContext); ok {
		c.VisitPreDecreaseExpression(preDecreaseExpression)
	}
	if assignmentExpression, ok := i.(*nasl.AssignmentExpressionContext); ok {
		c.VisitAssignmentExpression(assignmentExpression)
	}
	if xExpression, ok := i.(*nasl.XExpressionContext); ok {
		c.VisitXExpression(xExpression)
	}
	if memberDotExpression, ok := i.(*nasl.MemberDotExpressionContext); ok {
		c.VisitMemberDotExpression(memberDotExpression)
	}

}
func (c *Compiler) VisitXExpression(i *nasl.XExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	/***
	伪代码:
	for(i=0;i<t;i++){
		expression
	}
	*/
	// 创建变量i=0
	iId := c.symbolTable.NewSymbolWithoutName()
	code := c.pushOpcodeFlag(yakvm.OpPushLeftRef)
	code.Unary = iId
	c.pushInt(0)
	c.pushAssigin()
	c.pushOpcodeFlag(yakvm.OpPop)

	//创建变量t为循环次数
	tId := c.symbolTable.NewSymbolWithoutName()
	code = c.pushOpcodeFlag(yakvm.OpPushLeftRef)
	code.Unary = tId
	c.VisitSingleExpression(i.SingleExpression(1))
	c.pushAssigin()
	c.pushOpcodeFlag(yakvm.OpPop)

	//判断i<t
	startP := c.GetCodePostion()
	code = c.pushOpcodeFlag(yakvm.OpPushRef)
	code.Unary = iId
	code = c.pushOpcodeFlag(yakvm.OpPushRef)
	code.Unary = tId
	c.pushOpcodeFlag(yakvm.OpLt)

	//if false 就结束循环
	jmpF := c.pushJmpIfFalse()
	c.VisitSingleExpression(i.SingleExpression(0))
	//i++ 并继续循环
	code = c.pushOpcodeFlag(yakvm.OpPushLeftRef)
	code.Unary = iId
	c.pushOpcodeFlag(yakvm.OpPlusPlus)
	jmp := c.pushJmp()
	jmp.Unary = startP
	jmpF.Unary = c.GetCodePostion()
}
func (c *Compiler) VisitMemberIndexExpression(i *nasl.MemberIndexExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.pushRef("get_array_elem")
	c.VisitSingleExpression(i.SingleExpression(0))
	c.VisitSingleExpression(i.SingleExpression(1))
	c.pushCall(2)
}
func (c *Compiler) VisitPostIncrementExpression(i *nasl.PostIncrementExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	exp := i.SingleExpression()
	name := ""
	if v, ok := exp.(*nasl.IdentifierExpressionContext); ok {
		name = v.GetText()
		c.pushLeftRef(name)
	} else {
		panic("post increment expression must be identifier")
	}
	c.pushOpcodeFlag(yakvm.OpPlusPlus)
	c.pushRef(name)
	c.NeedPop(false)
}
func (c *Compiler) VisitPostDecreaseExpression(i *nasl.PostDecreaseExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	exp := i.SingleExpression()
	name := ""
	if v, ok := exp.(*nasl.IdentifierExpressionContext); ok {
		name = v.GetText()
		c.pushLeftRef(name)
		c.pushLeftRef(name)
	} else {
		panic("post decrease expression must be identifier")
	}
	c.pushRef(name)
	c.pushOpcodeFlag(yakvm.OpMinusMinus)
	c.pushRef(name)
	c.NeedPop(false)
}
func (c *Compiler) VisitPreIncrementExpression(i *nasl.PreIncrementExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	exp := i.SingleExpression()
	name := ""
	if v, ok := exp.(*nasl.IdentifierExpressionContext); ok {
		name = v.GetText()
		c.pushLeftRef(name)
	} else {
		panic("pre increment expression must be identifier")
	}
	c.pushOpcodeFlag(yakvm.OpPlusPlus)
	c.pushRef(name)
	c.NeedPop(false)
}
func (c *Compiler) VisitPreDecreaseExpression(i *nasl.PreDecreaseExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	exp := i.SingleExpression()
	name := ""
	if v, ok := exp.(*nasl.IdentifierExpressionContext); ok {
		name = v.GetText()
		c.pushLeftRef(name)
	} else {
		panic("pre decrease expression must be identifier")
	}

	c.pushOpcodeFlag(yakvm.OpMinusMinus)
	c.pushRef(name)
	c.NeedPop(false)
}
func (c *Compiler) VisitUnaryPlusExpression(i *nasl.UnaryPlusExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression())
	c.pushOpcodeFlag(yakvm.OpPlus)
}
func (c *Compiler) VisitUnaryMinusExpression(i *nasl.UnaryMinusExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression())
	c.pushOpcodeFlag(yakvm.OpNeg)
}
func (c *Compiler) VisitBitNotExpression(i *nasl.BitNotExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.pushRef("BitNot")
	c.VisitSingleExpression(i.SingleExpression())
	c.pushCall(1)
}
func (c *Compiler) VisitNotExpression(i *nasl.NotExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression())
	c.pushOpcodeFlag(yakvm.OpNot)
}
func (c *Compiler) VisitMultiplicativeExpression(i *nasl.MultiplicativeExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	if i.Pow() != nil {
		c.pushRef("__pow")
		c.VisitSingleExpression(i.SingleExpression(0))
		c.VisitSingleExpression(i.SingleExpression(1))
		c.pushCall(2)
		return
	}
	c.VisitSingleExpression(i.SingleExpression(0))
	c.VisitSingleExpression(i.SingleExpression(1))
	if i.Multiply() != nil {
		c.pushOpcodeFlag(yakvm.OpMul)
	} else if i.Divide() != nil {
		c.pushOpcodeFlag(yakvm.OpDiv)
	} else if i.Modulus() != nil {
		c.pushOpcodeFlag(yakvm.OpMod)
	}
}
func (c *Compiler) VisitAdditiveExpression(i *nasl.AdditiveExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression(0))
	c.VisitSingleExpression(i.SingleExpression(1))
	if i.Plus() != nil {
		c.pushOpcodeFlag(yakvm.OpAdd)
	} else {
		c.pushOpcodeFlag(yakvm.OpSub)
	}
}
func (c *Compiler) VisitBitShiftExpression(i *nasl.BitShiftExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	if i.RightShiftLogical() != nil { // 没有这个运算符
		c.pushRef("RightShiftLogical")
		c.VisitSingleExpression(i.SingleExpression(0))
		c.VisitSingleExpression(i.SingleExpression(1))
		c.pushCall(2)
		return
	} else if i.LeftShiftLogical() != nil {
		c.pushRef("LeftShiftLogical")
		c.VisitSingleExpression(i.SingleExpression(0))
		c.VisitSingleExpression(i.SingleExpression(1))
		c.pushCall(2)
		return
	} else {
		c.VisitSingleExpression(i.SingleExpression(0))
		c.VisitSingleExpression(i.SingleExpression(1))
		if i.RightShiftArithmetic() != nil {
			c.pushOpcodeFlag(yakvm.OpShr)
		} else if i.LeftShiftArithmetic() != nil {
			c.pushOpcodeFlag(yakvm.OpShl)
		}
	}

}
func (c *Compiler) VisitRelationalExpression(i *nasl.RelationalExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression(0))
	c.VisitSingleExpression(i.SingleExpression(1))
	if i.LessThan() != nil {
		c.pushOpcodeFlag(yakvm.OpLt)
	} else if i.LessThanEquals() != nil {
		c.pushOpcodeFlag(yakvm.OpLtEq)
	} else if i.MoreThan() != nil {
		c.pushOpcodeFlag(yakvm.OpGt)
	} else if i.GreaterThanEquals() != nil {
		c.pushOpcodeFlag(yakvm.OpGtEq)
	} else {
		panic("unknown relational operator")
	}
}
func (c *Compiler) VisitEqualityExpression(i *nasl.EqualityExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	if i.EqualsRe() != nil {
		c.pushRef("reEqual")
		c.VisitSingleExpression(i.SingleExpression(0))
		c.VisitSingleExpression(i.SingleExpression(1))
		c.pushCall(2)
		return
	} else if i.NotLong() != nil {
		c.pushRef("reEqual")
		c.VisitSingleExpression(i.SingleExpression(0))
		c.VisitSingleExpression(i.SingleExpression(1))
		c.pushCall(2)
		c.pushOpcodeFlag(yakvm.OpNot)
		return
	} else if i.MTLT() != nil {
		c.pushRef("strIn")
		c.VisitSingleExpression(i.SingleExpression(1))
		c.VisitSingleExpression(i.SingleExpression(0))
		c.pushCall(2)
		return
	} else if i.MTNotLT() != nil {
		c.pushRef("strIn")
		c.VisitSingleExpression(i.SingleExpression(1))
		c.VisitSingleExpression(i.SingleExpression(0))
		c.pushCall(2)
		c.pushOpcodeFlag(yakvm.OpNot)
		return
	}
	c.VisitSingleExpression(i.SingleExpression(0))
	c.VisitSingleExpression(i.SingleExpression(1))
	if i.Equals_() != nil {
		c.pushOpcodeFlag(yakvm.OpEq)
	} else if i.NotEquals() != nil {
		c.pushOpcodeFlag(yakvm.OpNotEq)
	}
}
func (c *Compiler) VisitBitAndExpression(i *nasl.BitAndExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression(0))
	c.VisitSingleExpression(i.SingleExpression(1))
	c.pushBitAnd()
}
func (c *Compiler) VisitBitXOrExpression(i *nasl.BitXOrExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression(0))
	c.VisitSingleExpression(i.SingleExpression(1))
	c.pushOpcodeFlag(yakvm.OpXor)
}
func (c *Compiler) VisitBitOrExpression(i *nasl.BitOrExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression(0))
	c.VisitSingleExpression(i.SingleExpression(1))
	c.pushBitOr()
}
func (c *Compiler) VisitLogicalAndExpression(i *nasl.LogicalAndExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression(0))
	code := c.pushJmpIfFalse()
	c.VisitSingleExpression(i.SingleExpression(1))
	code.Unary = len(c.codes)
}
func (c *Compiler) VisitLogicalOrExpression(i *nasl.LogicalOrExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression(0))
	code := c.pushJmpIfTrue()
	c.VisitSingleExpression(i.SingleExpression(1))
	code.Unary = len(c.codes)
}
func (c *Compiler) VisitIdentifier(i *nasl.IdentifierContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	text := i.GetText()
	if text != "" {
		if _, ok := c.symbolTable.GetSymbolByVariableName(text); !ok && c.checkId {
			if _, ok := c.extVarNames[text]; !ok {
				c.AddError(fmt.Errorf("undefined variable: %s", text))
			}
		}
		c.pushRef(text)
	}
}
func (c *Compiler) VisitIdentifierExpression(i *nasl.IdentifierExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	c.VisitIdentifier(i.Identifier().(*nasl.IdentifierContext))
}
func (c *Compiler) VisitLiteralExpression(i *nasl.LiteralExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	iliteral := i.Literal()
	if iliteral == nil {
		return
	}
	if lit, ok := iliteral.(*nasl.LiteralContext); ok {
		if slit := lit.StringLiteral(); slit != nil {
			var res string
			//var err error
			s := slit.GetText()
			res = s[1 : len(s)-1]
			if s[0] == '\'' || s[0] == '"' {
				//res = strings.ReplaceAll(res, `\'`, `\\'`)
				//res = strings.ReplaceAll(res, `"`, `\"`)
				//res, err = strconv.Unquote(`"` + res + `"`)
				//if err != nil {
				//	panic(utils.Errorf("parse single-quote string literal error: %v", err))
				//}
				//res = strings.ReplaceAll(res, `\'`, `'`)
				escapeMap := map[string]string{
					`\\`: `\`,
					`\'`: `'`,
					`\"`: `"`,
					`\a`: "\a",
					`\b`: "\b",
					`\f`: "\f",
					`\n`: "\n",
					`\r`: "\r",
					`\0`: "\x00",
				}
				r := ""
				for i := 0; i < len(res); i++ {
					if res[i] == '\\' {
						if i+1 < len(res) {
							if v, ok := escapeMap[res[i:i+2]]; ok {
								r += v
								i++
								continue
							}
						}
						if i+3 < len(res) {
							if res[i:i+2] == "\\x" {
								if v, err := strconv.ParseInt(res[i+2:i+4], 16, 8); err == nil {
									r += string(v)
									i += 3
									continue
								}
							}
						}
					}
					r += string(res[i])
				}
				res = r
			}
			c.pushString(res)
		}
		if numlit := lit.NumericLiteral(); numlit != nil {
			if numlit_, ok := numlit.(*nasl.NumericLiteralContext); ok {
				if ilit := numlit_.IntegerLiteral(); ilit != nil {
					i, err := strconv.Atoi(ilit.GetText())
					if err != nil {
						panic("invalid integer literal")
					}
					c.pushInt(i)
				}
				if flit := numlit_.FloatLiteral(); flit != nil {
					f, err := strconv.ParseFloat(flit.GetText(), 64)
					if err != nil {
						panic("invalid float literal")
					}
					c.pushFloat(f)
				}
				if hexlit := numlit_.HexLiteral(); hexlit != nil {
					hexStr := hexlit.GetText()[2:]
					i, err := strconv.ParseInt(hexStr, 16, 64)
					if err != nil {
						panic("invalid hex literal")
					}
					c.pushInt(int(i))
				}
			}
		}
		if blit := lit.BooleanLiteral(); blit != nil {
			c.pushBool(strings.ToLower(blit.GetText()) == "true")
		}
		if iplit := lit.IpLiteral(); iplit != nil {
			c.pushString(iplit.GetText())
		}
		if nullLit := lit.NULLLiteral(); nullLit != nil {
			c.pushValue(yakvm.GetUndefined())
		}
	}

}
func (c *Compiler) VisitArrayLiteralExpression(i *nasl.ArrayLiteralExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	iarrayLit := i.ArrayLiteral()
	if iarrayLit == nil {
		return
	}
	arrayLit := iarrayLit.(*nasl.ArrayLiteralContext)
	eleList := arrayLit.ElementList()
	if eleList == nil {
		c.pushList(0)
	} else {
		eles := eleList.(*nasl.ElementListContext)
		allEles := eles.AllArrayElement()
		for _, ele := range allEles {
			arrayEle := ele.(*nasl.ArrayElementContext)
			if exp := arrayEle.SingleExpression(); exp != nil {
				c.VisitSingleExpression(exp)
			}
			if id := arrayEle.Identifier(); id != nil {
				c.pushRef(id.GetText())
			}
		}
		c.pushList(len(allEles))
	}
}
func (c *Compiler) VisitCallExpression(i *nasl.CallExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	funcName := i.SingleExpression().GetText()
	paramsLen := 0
	if _, ok := c.naslLib[funcName]; ok {
		c.pushRef("__method_proxy__")
		c.pushInt(c.GetSymbolId(i.SingleExpression().GetText()))
		paramsLen++
	} else {
		c.pushRef("__function__" + funcName)
	}
	iarguments := i.ArgumentList()
	var argLen int
	if arguments, ok := iarguments.(*nasl.ArgumentListContext); ok {
		allArg := arguments.AllArgument()
		for _, iargument := range allArg {
			argument := iargument.(*nasl.ArgumentContext)
			id := argument.Identifier()
			if argument.Colon() != nil {
				id := c.GetSymbolId(id.GetText())
				code := c.pushOpcodeFlag(yakvm.OpPushLeftRef)
				code.Unary = id
			}
			c.VisitSingleExpression(argument.SingleExpression())
		}
		argLen = len(allArg)
	}
	code := c.pushCall(paramsLen + argLen)
	if paramsLen != 0 {
		code.Op1 = yakvm.NewAutoValue(true)
	}
}
func (c *Compiler) VisitAssignmentExpression(i *nasl.AssignmentExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	if i.OpenBracket() != nil {
		if id := i.Identifier(0); id != nil {
			name := id.GetText()
			if id, _ := c.symbolTable.GetSymbolByVariableName(name); !c.symbolTable.IdIsInited(id) {
				c.pushLeftRef(name)
				idValue, _ := c.symbolTable.GetSymbolByVariableName(name)
				delete(c.symbolTable.InitedId, idValue)
				c.VisitSingleExpression(i.SingleExpression(0))
				c.pushGenList(2)
				c.VisitSingleExpression(i.SingleExpression(1))
				//code := c.pushOpcodeFlag(yakvm.OpNewMap)
				//code.Unary = 1
				//c.pushGenList(1)

				//c.pushGenList(1)
				c.pushAssigin()
				return
			}
		}
	}
	pushLeft := func() {
		id := i.Identifier(0).(*nasl.IdentifierContext)

		if i.OpenBracket() != nil {
			c.VisitIdentifier(id)
			c.VisitSingleExpression(i.SingleExpression(0))
			c.pushGenList(2)
		} else if i.Dot() != nil {
			c.VisitIdentifier(id)
			c.pushString(i.Identifier(1).GetText())
			c.pushGenList(2)
		} else {
			code := c.pushLeftRef(id.GetText())
			c.symbolTable.SetIdIsInited(code.Unary)
			//if id, ok := id.(*nasl.IdentifierExpressionContext); ok {
			//	code := c.pushLeftRef(id.GetText())
			//	c.symbolTable.SetIdIsInited(code.Unary)
			//} else {
			//	c.VisitSingleExpression(exp)
			//}
		}

	}
	pushLeftRef := func() {
		id := i.Identifier(0).(*nasl.IdentifierContext)
		if i.OpenBracket() != nil {
			c.VisitIdentifier(id)
			c.VisitSingleExpression(i.SingleExpression(0))
			c.pushGenList(2)
		} else if i.Dot() != nil {
			c.VisitIdentifier(id)
			c.pushString(i.Identifier(1).GetText())
			c.pushGenList(2)
		} else {
			c.pushRef(id.GetText())
		}

	}
	_ = pushLeftRef
	pushRight := func() {
		if i.OpenBracket() != nil {
			c.VisitSingleExpression(i.SingleExpression(1))
		} else {
			c.VisitSingleExpression(i.SingleExpression(0))
		}
	}
	if i.AssignmentOperator().GetText() == "=" {
		pushLeft()
		pushRight()
		//c.pushGenList(1)

		//c.pushGenList(1)
		c.pushAssigin()
		return
	}
	switch i.AssignmentOperator().GetText() {
	case "+=":
		pushRight()
		pushLeft()
		c.pushOpcodeFlag(yakvm.OpPlusEq)
	case "-=":
		pushRight()
		pushLeft()
		c.pushOpcodeFlag(yakvm.OpMinusEq)
	case "*=":
		pushRight()
		pushLeft()
		c.pushOpcodeFlag(yakvm.OpMulEq)
	case "/=":
		pushRight()
		pushLeft()
		c.pushOpcodeFlag(yakvm.OpDivEq)
	case "%=":
		pushRight()
		pushLeft()
		c.pushOpcodeFlag(yakvm.OpModEq)
	case ">>=":
		pushRight()
		pushLeft()
		c.pushOpcodeFlag(yakvm.OpShrEq)
	case "<<=":
		pushRight()
		pushLeft()
		c.pushOpcodeFlag(yakvm.OpShlEq)
	case ">>>=":
		pushLeft()
		c.pushRef("RightShiftLogical")
		pushLeftRef()
		pushRight()
		c.pushCall(2)
		c.pushAssigin()
	case "<<<=":
		pushLeft()
		c.pushRef("LeftShiftLogical")
		pushLeftRef()
		pushRight()
		c.pushCall(2)
		c.pushAssigin()
	}

}
func (c *Compiler) VisitParenthesizedExpression(i *nasl.ParenthesizedExpressionContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitExpressionSequence(i.ExpressionSequence())
}
func (c *Compiler) VisitMemberDotExpression(i *nasl.MemberDotExpressionContext) {
	panic("implement me")
	if i == nil {
		return
	}
	c.visitHook(c, i)
	c.VisitSingleExpression(i.SingleExpression())
	id := i.Identifier().GetText()
	c.pushString(id)
	c.pushOpcodeFlag(yakvm.OpMemberCall)
}
