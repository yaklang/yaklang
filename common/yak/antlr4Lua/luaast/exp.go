package luaast

import (
	lua "yaklang/common/yak/antlr4Lua/parser"
	"yaklang/common/yak/antlr4yak/yakvm"
)

func (l *LuaTranslator) VisitExp(raw lua.IExpContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.ExpContext)
	if i == nil {
		return nil
	}

	if s := i.Nil(); s != nil {
		l.pushUndefined()
		return nil
	}

	if s := i.False(); s != nil {
		l.pushBool(false)
		return nil
	}

	if s := i.True(); s != nil {
		l.pushBool(true)
		return nil
	}

	if s := i.Number(); s != nil {
		l.VisitNumber(s)
		return nil
	}

	if s := i.String_(); s != nil {
		l.VisitString(s)
		return nil
	}

	if s := i.Ellipsis(); s != nil {
		l.VisitVariadicEllipsis(false)
		return nil
	}

	if s := i.Functiondef(); s != nil {
		l.VisitFunctionDef(s)
		return nil
	}

	if s := i.Prefixexp(); s != nil {
		l.VisitPrefixExp(s)
		return nil
	}

	if s := i.Tableconstructor(); s != nil {
		l.VisitTableConstructor(s)
		return nil
	}

	// implement `^` operator with built-in func `pow`
	if s := i.OperatorPower(); s != nil {
		expLeft, expRight := i.Exp(0), i.Exp(1)
		l.pushIdentifierName("@pow")
		l.VisitExp(expRight)
		l.VisitExp(expLeft)
		l.pushCall(2)
		return nil
	}

	if s := i.OperatorUnary(); s != nil {
		opStr := s.GetText()
		switch opStr {
		case "not":
			l.VisitExp(i.Exp(0))
			l.pushOperator(yakvm.OpNot) // logically
		case "#":
			//TODO: meta table with __len
			l.pushIdentifierName("@getlen")
			l.VisitExp(i.Exp(0))
			l.pushCall(1)
		case "-":
			l.VisitExp(i.Exp(0))
			l.pushOperator(yakvm.OpNeg)
		case "~": // waiting master branch to add opCode bitWise not
			l.VisitExp(i.Exp(0))
			//l.pushOperator(yakvm.OpBitwiseNot)
		default:
			l.panicCompilerError(notImplemented, opStr)
		}
		return nil
	}

	if s := i.AllExp(); s != nil && len(s) == 2 {
		// general binary operation
		if operator := i.OperatorMulDivMod(); operator != nil {
			opStr := operator.GetText()
			switch opStr {
			case "*":
				l.VisitExp(s[0])
				l.VisitExp(s[1])
				l.pushOperator(yakvm.OpMul)
			case "/":
				l.VisitExp(s[0])
				l.VisitExp(s[1])
				l.pushOperator(yakvm.OpDiv)
			case "%":
				l.VisitExp(s[0])
				l.VisitExp(s[1])
				l.pushOperator(yakvm.OpMod)
			case "//":
				l.pushIdentifierName("@floor")
				l.VisitExp(s[0])
				l.VisitExp(s[1])
				l.pushCall(2)
			default:
				panic("multiplicative error")
			}
			return nil
		} else if operator := i.OperatorAddSub(); operator != nil {
			l.VisitExp(s[0])
			l.VisitExp(s[1])
			opStr := operator.GetText()
			switch opStr {
			case "+":
				l.pushOperator(yakvm.OpAdd)
			case "-":
				l.pushOperator(yakvm.OpSub)
			}
			return nil
		} else if i.OperatorStrcat() != nil {
			l.pushIdentifierName("@strcat")
			l.VisitExp(s[0])
			l.VisitExp(s[1])
			l.pushCall(2)
			return nil
		} else if condition := i.OperatorComparison(); condition != nil {
			l.VisitExp(s[0])
			l.VisitExp(s[1])
			switch condition.GetText() {
			case `>`:
				l.pushOperator(yakvm.OpGt)
			case `<`:
				l.pushOperator(yakvm.OpLt)
			case `<=`:
				l.pushOperator(yakvm.OpLtEq)
			case `>=`:
				l.pushOperator(yakvm.OpGtEq)
			case `~=`:
				l.pushOperator(yakvm.OpNotEq)
			case `==`:
				l.pushOperator(yakvm.OpEq)
			}
			return nil
		} else if operator := i.OperatorAnd(); operator != nil {
			l.VisitExp(s[0])
			jmptop := l.pushJmpIfFalseOrPop()
			l.VisitExp(s[1])
			jmptop.Unary = l.GetNextCodeIndex()
			return nil
		} else if operator := i.OperatorOr(); operator != nil {
			l.VisitExp(s[0])
			jmptop := l.pushJmpIfTrueOrPop()
			l.VisitExp(s[1])
			jmptop.Unary = l.GetNextCodeIndex()
			return nil
		} else if operator := i.OperatorBitwise(); operator != nil { // bit binary op
			l.VisitExp(s[0])
			l.VisitExp(s[1])
			opStr := operator.GetText()
			switch opStr {
			case "&":
				l.pushOperator(yakvm.OpAnd)
			case "|":
				l.pushOperator(yakvm.OpOr)
			case "~":
				l.pushOperator(yakvm.OpXor)
			case ">>":
				l.pushOperator(yakvm.OpShr)
			case "<<":
				l.pushOperator(yakvm.OpShl)
			default:
				panic("bitBinary error")
			}
			return nil
		}
	}

	panic("Not Implement")
	return nil
}

func (l *LuaTranslator) VisitPrefixExp(raw lua.IPrefixexpContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.PrefixexpContext)
	if i == nil {
		return nil
	}

	l.VisitVarOrExp(false, i.VarOrExp())

	args := i.AllNameAndArgs()

	// 多个agr 例如 a:add(10):add(20).x 是闭包函数返回调用的情况 链式调用
	for _, arg := range args { // at least one since it include LParen and RParen
		l.VisitNameAndArgs(arg)
	}
	return nil

}
