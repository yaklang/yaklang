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
	if infix.MatchClause() != nil {
		var subject ssa.Value
		if len(infix.AllInfixExpr()) > 0 {
			subject = b.visitInfixExpr(infix.InfixExpr(0))
		} else {
			subject = b.visitPrefixExpr(infix.PrefixExpr())
		}
		return b.visitMatchClause(subject, infix.MatchClause())
	}
	if len(infix.AllInfixExpr()) == 0 {
		return b.visitPrefixExpr(infix.PrefixExpr())
	}
	op := "+"
	if infix.Id() != nil {
		op = infix.Id().GetText()
	}
	if len(infix.AllInfixExpr()) > 1 && (op == "&&" || op == "||") {
		left := infix.InfixExpr(0)
		right := infix.InfixExpr(1)
		return b.BuildShortCircuit(
			func() ssa.Value { return b.visitInfixExpr(left) },
			func() ssa.Value { return b.visitInfixExpr(right) },
			op == "&&",
		)
	}
	left := b.visitInfixExpr(infix.InfixExpr(0))
	if len(infix.AllInfixExpr()) == 1 {
		return left
	}
	right := b.visitInfixExpr(infix.InfixExpr(1))
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
	if !ok || simple == nil || simple.SimpleExprPrimary() == nil {
		return b.EmitConstInstNil()
	}
	value := b.visitSimpleExprPrimary(simple.SimpleExprPrimary())
	for i := 0; i < len(simple.AllSimpleExprSuffix()); i++ {
		suffix, ok := simple.SimpleExprSuffix(i).(*scalaparser.SimpleExprSuffixContext)
		if !ok || suffix == nil {
			continue
		}
		switch {
		case suffix.DOT() != nil && suffix.Id() != nil:
			name := suffix.Id().GetText()
			if name == "asInstanceOf" && i+1 < len(simple.AllSimpleExprSuffix()) {
				if next, ok := simple.SimpleExprSuffix(i + 1).(*scalaparser.SimpleExprSuffixContext); ok && next != nil && next.TypeArgs() != nil {
					typeName := next.TypeArgs().GetText()
					value = b.EmitTypeCast(value, scalaTypeFromText(typeName))
					i++
					continue
				}
			}
			if value != nil && value.GetName() == "this" {
				if field := b.ReadValue(name); field != nil {
					value = field
					continue
				}
			}
			key := b.EmitConstInst(name)
			value = b.ReadMemberCallValue(value, key)
		case suffix.MatchClause() != nil:
			value = b.visitMatchClause(value, suffix.MatchClause())
		case suffix.TypeArgs() != nil:
			// type arguments only affect asInstanceOf, handled above
		case suffix.ArgumentExprs() != nil:
			args := b.visitArgumentExprs(suffix.ArgumentExprs())
			value = b.EmitCall(b.NewCall(value, args))
		}
	}
	return value
}

func (b *singleFileBuilder) visitSimpleExprPrimary(raw scalaparser.ISimpleExprPrimaryContext) ssa.Value {
	simple, ok := raw.(*scalaparser.SimpleExprPrimaryContext)
	if !ok || simple == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case simple.SimpleRef() != nil:
		return b.visitSimpleRef(simple.SimpleRef())
	case simple.Literal() != nil:
		return b.visitLiteral(simple.Literal())
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
	if !ok || args == nil {
		return nil
	}
	if args.BlockExpr() != nil {
		return []ssa.Value{b.visitBlockExprValue(args.BlockExpr())}
	}
	if args.ParArgumentExprs() == nil {
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
		value := b.visitPostfixExpr(expr.PostfixExpr())
		if expr.COLON() != nil && expr.Type_() != nil {
			value = b.EmitTypeCast(value, scalaTypeFromText(expr.Type_().GetText()))
		}
		return value
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

func scalaTypeFromText(text string) ssa.Type {
	text = strings.TrimSpace(text)
	for strings.HasPrefix(text, "[") || strings.HasPrefix(text, "(") {
		text = strings.TrimSpace(text[1:])
	}
	for strings.HasSuffix(text, "]") || strings.HasSuffix(text, ")") {
		text = strings.TrimSpace(text[:len(text)-1])
	}
	if index := strings.IndexAny(text, "<[("); index >= 0 {
		text = text[:index]
	}
	switch text {
	case "Double", "Float":
		return ssa.CreateFloatType()
	case "Int", "Long", "Short", "Byte":
		return ssa.CreateNumberType()
	case "String", "Char":
		return ssa.CreateStringType()
	case "Boolean":
		return ssa.CreateBooleanType()
	default:
		return ssa.CreateAnyType()
	}
}

func (b *singleFileBuilder) visitBlockExprValue(raw scalaparser.IBlockExprContext) ssa.Value {
	expr, ok := raw.(*scalaparser.BlockExprContext)
	if !ok || expr == nil {
		return nil
	}
	if expr.Block() != nil {
		return b.visitBlockResult(expr.Block())
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitLambdaBlockResult(result *scalaparser.BlockResultContext) ssa.Value {
	if result == nil || result.Block() == nil {
		return nil
	}
	previousSuppress := b.suppressValueBlock
	b.suppressValueBlock = false
	defer func() { b.suppressValueBlock = previousSuppress }()
	previous := b.FunctionBuilder
	previousSupport := previous.SupportClosure
	previous.SupportClosure = true
	fn := previous.NewFunc("")
	b.FunctionBuilder = previous.PushFunction(fn)
	b.SupportClosure = true
	b.SetForceCapture(true)
	if params := result.FunParams(); params != nil {
		if p, ok := params.(*scalaparser.FunParamsContext); ok && p != nil {
			if p.Bindings() != nil {
				if bindings, ok := p.Bindings().(*scalaparser.BindingsContext); ok && bindings != nil {
					for _, rawBinding := range bindings.AllBinding() {
						binding, ok := rawBinding.(*scalaparser.BindingContext)
						if !ok || binding == nil {
							continue
						}
						if binding.USCORE() != nil {
							b.NewParam("_")
						} else if binding.Id() != nil {
							b.NewParam(binding.Id().GetText())
						}
					}
				}
			} else if p.USCORE() != nil {
				b.NewParam("_")
			} else if p.Id() != nil {
				b.NewParam(p.Id().GetText())
			}
		}
	}
	if value := b.visitBlockResult(result.Block()); value != nil {
		b.EmitReturn([]ssa.Value{value})
	}
	b.Finish()
	b.SetForceCapture(false)
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	previous.SupportClosure = previousSupport
	return fn
}

func (b *singleFileBuilder) visitMatchClause(subject ssa.Value, raw scalaparser.IMatchClauseContext) ssa.Value {
	clause, ok := raw.(*scalaparser.MatchClauseContext)
	if !ok || clause == nil || clause.CaseClauses() == nil {
		return b.EmitConstInstNil()
	}
	clauses, ok := clause.CaseClauses().(*scalaparser.CaseClausesContext)
	if !ok || clauses == nil {
		return b.EmitConstInstNil()
	}
	if subject == nil {
		subject = b.EmitConstInstNil()
	}
	cases := make([]ssa.MatchCase, 0, len(clauses.AllCaseClause()))
	for _, rawCase := range clauses.AllCaseClause() {
		caseClause, ok := rawCase.(*scalaparser.CaseClauseContext)
		if !ok || caseClause == nil {
			continue
		}
		condition, bindings := b.scalaPatternCondition(subject, caseClause.Pattern())
		guard := caseClause.Guard_()
		body := caseClause.Block()
		cases = append(cases, ssa.MatchCase{
			Condition: func() ssa.Value {
				for _, name := range bindings {
					b.AssignVariable(b.CreateVariable(name), subject)
				}
				result := condition
				if result == nil {
					result = b.EmitConstInst(true)
				}
				if guard != nil {
					if guardCtx, ok := guard.(*scalaparser.Guard_Context); ok && guardCtx.PostfixExpr() != nil {
						if value := b.visitPostfixExpr(guardCtx.PostfixExpr()); value != nil {
							result = b.EmitBinOp(ssa.OpAnd, result, value)
						}
					}
				}
				return result
			},
			Body: func() ssa.Value {
				if body == nil {
					return b.EmitConstInstNil()
				}
				return b.visitBlockResult(body)
			},
		})
	}
	return b.BuildMatchValue(cases, nil)
}

func (b *singleFileBuilder) scalaPatternCondition(subject ssa.Value, raw scalaparser.IPatternContext) (ssa.Value, []string) {
	pattern, ok := raw.(*scalaparser.PatternContext)
	if !ok || pattern == nil || len(pattern.AllPattern1()) == 0 {
		return b.EmitConstInst(true), nil
	}
	var result ssa.Value
	var bindings []string
	for _, rawPart := range pattern.AllPattern1() {
		part, ok := rawPart.(*scalaparser.Pattern1Context)
		if !ok || part == nil {
			continue
		}
		condition, names := b.scalaPattern1Condition(subject, part)
		bindings = append(bindings, names...)
		if result == nil {
			result = condition
		} else {
			result = b.EmitBinOp(ssa.OpOr, result, condition)
		}
	}
	if result == nil {
		result = b.EmitConstInst(true)
	}
	return result, bindings
}

func (b *singleFileBuilder) scalaPattern1Condition(subject ssa.Value, part *scalaparser.Pattern1Context) (ssa.Value, []string) {
	if part.SimpleLiteral() != nil {
		if value := b.scalaSimpleLiteralValue(part.SimpleLiteral()); value != nil {
			return b.EmitBinOp(ssa.OpEq, subject, value), nil
		}
		return b.EmitConstInst(true), nil
	}
	if part.PatVar() != nil {
		variable, ok := part.PatVar().(*scalaparser.PatVarContext)
		if !ok || variable == nil {
			return b.EmitConstInst(true), nil
		}
		name := "_"
		if variable.Varid() != nil {
			name = variable.Varid().GetText()
		}
		if name == "_" {
			return b.EmitConstInst(true), nil
		}
		if part.COLON() != nil && part.RefinedType() != nil {
			condition := b.emitScalaIsType(subject, part.RefinedType().GetText())
			return condition, []string{name}
		}
		return b.EmitConstInst(true), []string{name}
	}
	if part.Pattern2() != nil {
		return b.scalaPattern2Condition(subject, part.Pattern2())
	}
	return b.EmitConstInst(true), nil
}

func (b *singleFileBuilder) scalaPattern2Condition(subject ssa.Value, raw scalaparser.IPattern2Context) (ssa.Value, []string) {
	pattern, ok := raw.(*scalaparser.Pattern2Context)
	if !ok || pattern == nil {
		return b.EmitConstInst(true), nil
	}
	if pattern.AT() != nil && pattern.Id() != nil {
		return b.EmitConstInst(true), []string{pattern.Id().GetText()}
	}
	if pattern.InfixPattern() != nil {
		return b.scalaSimplePatternCondition(subject, pattern.InfixPattern())
	}
	return b.EmitConstInst(true), nil
}

func (b *singleFileBuilder) scalaSimplePatternCondition(subject ssa.Value, raw scalaparser.IInfixPatternContext) (ssa.Value, []string) {
	infix, ok := raw.(*scalaparser.InfixPatternContext)
	if !ok || infix == nil || len(infix.AllSimplePattern()) == 0 {
		return b.EmitConstInst(true), nil
	}
	var result ssa.Value
	var bindings []string
	for _, rawSimple := range infix.AllSimplePattern() {
		part, ok := rawSimple.(*scalaparser.SimplePatternContext)
		if !ok || part == nil {
			continue
		}
		condition, names := b.scalaSimplePatternOneCondition(subject, part)
		bindings = append(bindings, names...)
		if result == nil {
			result = condition
		} else {
			result = b.EmitBinOp(ssa.OpOr, result, condition)
		}
	}
	if result == nil {
		result = b.EmitConstInst(true)
	}
	return result, bindings
}

func (b *singleFileBuilder) scalaSimplePatternOneCondition(subject ssa.Value, simple *scalaparser.SimplePatternContext) (ssa.Value, []string) {
	if simple.Literal() != nil {
		if value := b.visitLiteral(simple.Literal()); value != nil {
			return b.EmitBinOp(ssa.OpEq, subject, value), nil
		}
		return b.EmitConstInst(true), nil
	}
	if simple.PatVar() != nil {
		variable, ok := simple.PatVar().(*scalaparser.PatVarContext)
		if !ok || variable == nil {
			return b.EmitConstInst(true), nil
		}
		name := "_"
		if variable.Varid() != nil {
			name = variable.Varid().GetText()
		}
		if name == "_" {
			return b.EmitConstInst(true), nil
		}
		return b.EmitConstInst(true), []string{name}
	}
	return b.EmitConstInst(true), nil
}

func (b *singleFileBuilder) emitScalaIsType(value ssa.Value, typeName string) ssa.Value {
	if value == nil {
		value = b.EmitUndefined("value")
	}
	callee := b.ReadValue("is")
	if callee == nil {
		callee = b.EmitUndefined("is")
	}
	result := b.EmitCall(b.NewCall(callee, []ssa.Value{value, b.EmitConstInst(typeName)}))
	if result != nil {
		result.SetType(ssa.CreateBooleanType())
	}
	return result
}

func (b *singleFileBuilder) scalaSimpleLiteralValue(raw scalaparser.ISimpleLiteralContext) ssa.Value {
	simple, ok := raw.(*scalaparser.SimpleLiteralContext)
	if !ok || simple == nil {
		return nil
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
	return nil
}
