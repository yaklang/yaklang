package scala2ssa

import (
	"strconv"
	"strings"

	scalaparser "github.com/yaklang/yaklang/common/yak/scala/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func scalaBinaryOpcode(op string) ssa.BinaryOpcode {
	switch op {
	case "+":
		return ssa.OpAdd
	case "-":
		return ssa.OpSub
	case "*":
		return ssa.OpMul
	case "/":
		return ssa.OpDiv
	case "%":
		return ssa.OpMod
	case "&&":
		return ssa.OpAnd
	case "||":
		return ssa.OpOr
	case "==":
		return ssa.OpEq
	case "!=":
		return ssa.OpNotEq
	case "<":
		return ssa.OpLt
	case ">":
		return ssa.OpGt
	case "<=":
		return ssa.OpLtEq
	case ">=":
		return ssa.OpGtEq
	case "&":
		return ssa.OpAnd
	case "|":
		return ssa.OpOr
	case "^":
		return ssa.OpXor
	case "<<":
		return ssa.OpShl
	case ">>":
		return ssa.OpShr
	}
	return ssa.OpAdd
}

func (b *singleFileBuilder) visitPostfixExpr(raw scalaparser.IPostfixExprContext) ssa.Value {
	postfix, ok := raw.(*scalaparser.PostfixExprContext)
	if !ok || postfix == nil || postfix.InfixExpr() == nil {
		return b.EmitConstInstNil()
	}
	return b.visitInfixExpr(postfix.InfixExpr())
}

func (b *singleFileBuilder) visitInfixExpr(raw scalaparser.IInfixExprContext) ssa.Value {
	infix, ok := raw.(*scalaparser.InfixExprContext)
	if !ok || infix == nil {
		return b.EmitConstInstNil()
	}
	if len(infix.AllInfixExpr()) == 0 {
		return b.visitPrefixExpr(infix.PrefixExpr())
	}
	left := b.visitInfixExpr(infix.InfixExpr(0))
	if len(infix.AllInfixExpr()) == 1 {
		return left
	}
	right := b.visitInfixExpr(infix.InfixExpr(1))
	op := "+"
	if infix.Id() != nil {
		op = infix.Id().GetText()
	}
	if op == "::" || op == ":::" || op == "++" || op == "+:" || op == ":+" {
		op = "+"
	}
	return b.EmitBinOp(scalaBinaryOpcode(op), left, right)
}

func (b *singleFileBuilder) visitPrefixExpr(raw scalaparser.IPrefixExprContext) ssa.Value {
	prefix, ok := raw.(*scalaparser.PrefixExprContext)
	if !ok || prefix == nil || prefix.SimpleExpr() == nil {
		return b.EmitConstInstNil()
	}
	value := b.visitSimpleExpr(prefix.SimpleExpr())
	if prefix.PrefixOperator() == nil {
		return value
	}
	switch prefix.PrefixOperator().GetText() {
	case "-":
		return b.EmitUnOp(ssa.OpNeg, value)
	case "+":
		return b.EmitUnOp(ssa.OpPlus, value)
	case "!":
		return b.EmitUnOp(ssa.OpNot, value)
	case "~":
		return b.EmitUnOp(ssa.OpBitwiseNot, value)
	}
	return value
}

func (b *singleFileBuilder) visitSimpleExpr(raw scalaparser.ISimpleExprContext) ssa.Value {
	simple, ok := raw.(*scalaparser.SimpleExprContext)
	if !ok || simple == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case simple.SimpleRef() != nil:
		return b.visitSimpleRef(simple.SimpleRef())
	case simple.Literal() != nil:
		return b.visitLiteral(simple.Literal())
	case simple.SimpleExpr() != nil:
		base := b.visitSimpleExpr(simple.SimpleExpr())
		switch {
		case simple.DOT() != nil && simple.Id() != nil:
			key := b.EmitConstInst(simple.Id().GetText())
			return b.ReadMemberCallValue(base, key)
		case simple.ArgumentExprs() != nil:
			args := b.visitArgumentExprs(simple.ArgumentExprs())
			return b.EmitCall(b.NewCall(base, args))
		}
		return base
	case simple.ExprsInParens() != nil:
		if values, ok := simple.ExprsInParens().(*scalaparser.ExprsInParensContext); ok && values != nil && len(values.AllExprInParens()) > 0 {
			return b.visitExprInParens(values.ExprInParens(0))
		}
	case simple.BlockExpr() != nil:
		if block, ok := simple.BlockExpr().(*scalaparser.BlockExprContext); ok && block != nil && block.Block() != nil {
			return b.visitBlockResult(block.Block())
		}
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitSimpleRef(raw scalaparser.ISimpleRefContext) ssa.Value {
	ref, ok := raw.(*scalaparser.SimpleRefContext)
	if !ok || ref == nil {
		return b.EmitConstInstNil()
	}
	if ref.THIS() != nil {
		return b.ReadValue("this")
	}
	if ref.SUPER() != nil {
		return b.EmitUndefined("super")
	}
	ids := ref.AllId()
	if len(ids) == 0 {
		return b.EmitConstInstNil()
	}
	value := b.ReadValue(ids[0].GetText())
	for i := 1; i < len(ids); i++ {
		key := b.EmitConstInst(ids[i].GetText())
		value = b.ReadMemberCallValue(value, key)
	}
	return value
}

func (b *singleFileBuilder) visitArgumentExprs(raw scalaparser.IArgumentExprsContext) []ssa.Value {
	args, ok := raw.(*scalaparser.ArgumentExprsContext)
	if !ok || args == nil || args.ParArgumentExprs() == nil {
		return nil
	}
	par, ok := args.ParArgumentExprs().(*scalaparser.ParArgumentExprsContext)
	if !ok || par == nil {
		return nil
	}
	if par.ExprsInParens() == nil {
		return nil
	}
	expressions, ok := par.ExprsInParens().(*scalaparser.ExprsInParensContext)
	if !ok || expressions == nil {
		return nil
	}
	result := make([]ssa.Value, 0, len(expressions.AllExprInParens()))
	for _, rawExpression := range expressions.AllExprInParens() {
		result = append(result, b.visitExprInParens(rawExpression))
	}
	return result
}

func (b *singleFileBuilder) visitExprInParens(raw scalaparser.IExprInParensContext) ssa.Value {
	expr, ok := raw.(*scalaparser.ExprInParensContext)
	if !ok || expr == nil {
		return b.EmitConstInstNil()
	}
	if expr.Expr() != nil {
		return b.VisitExpr(expr.Expr())
	}
	if expr.PostfixExpr() != nil {
		return b.visitPostfixExpr(expr.PostfixExpr())
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitLiteral(raw scalaparser.ILiteralContext) ssa.Value {
	literal, ok := raw.(*scalaparser.LiteralContext)
	if !ok || literal == nil {
		return b.EmitConstInstNil()
	}
	if literal.NullLiteral() != nil {
		return b.EmitConstInstNil()
	}
	if literal.InterpolatedStringLiteral() != nil {
		text := strings.Trim(literal.InterpolatedStringLiteral().GetText(), "\"")
		return b.EmitConstInst(text)
	}
	if literal.SimpleLiteral() == nil {
		return b.EmitConstInstNil()
	}
	simple, ok := literal.SimpleLiteral().(*scalaparser.SimpleLiteralContext)
	if !ok || simple == nil {
		return b.EmitConstInstNil()
	}
	negative := simple.Negation() != nil
	switch {
	case simple.IntegerLiteral() != nil:
		text := strings.ReplaceAll(simple.IntegerLiteral().GetText(), "_", "")
		if strings.HasSuffix(text, "L") || strings.HasSuffix(text, "l") {
			text = text[:len(text)-1]
		}
		if value, err := strconv.ParseInt(text, 0, 64); err == nil {
			if negative {
				value = -value
			}
			return b.EmitConstInst(value)
		}
	case simple.FloatingPointLiteral() != nil:
		text := strings.ReplaceAll(simple.FloatingPointLiteral().GetText(), "_", "")
		if value, err := strconv.ParseFloat(text, 64); err == nil {
			if negative {
				value = -value
			}
			return b.EmitConstInst(value)
		}
	case simple.BooleanLiteral() != nil:
		return b.EmitConstInst(simple.BooleanLiteral().GetText() == "true")
	case simple.CharacterLiteral() != nil:
		return b.EmitConstInst(strings.Trim(simple.CharacterLiteral().GetText(), "'"))
	case simple.StringLiteral() != nil:
		return b.EmitConstInst(strings.Trim(simple.StringLiteral().GetText(), "\""))
	}
	return b.EmitConstInstNil()
}
