package cpp2ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/cfamily/cliteral"
	cppparser "github.com/yaklang/yaklang/common/yak/cpp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// evalNode evaluates any expression rule node.
func (b *singleFileBuilder) evalNode(node antlr.Tree) ssa.Value {
	switch n := node.(type) {
	case *cppparser.ExpressionContext:
		var value ssa.Value
		for _, assignment := range n.AllAssignmentExpression() {
			value = b.evalNode(assignment)
		}
		if value == nil {
			return b.EmitConstInstNil()
		}
		return value
	case *cppparser.AssignmentExpressionContext:
		return b.visitAssignmentExpression(n)
	case *cppparser.ConditionalExpressionContext:
		return b.visitConditionalExpression(n)
	case *cppparser.LogicalOrExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.LogicalAndExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.InclusiveOrExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.ExclusiveOrExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.AndExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.EqualityExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.RelationalExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.ShiftExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.AdditiveExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.MultiplicativeExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.PointerMemberExpressionContext:
		return b.foldChain(n, b.evalNode)
	case *cppparser.CastExpressionContext:
		return b.visitCastExpression(n)
	case *cppparser.UnaryExpressionContext:
		return b.visitUnaryExpression(n)
	case *cppparser.PostfixExpressionContext:
		return b.visitPostfixExpression(n)
	case *cppparser.PrimaryExpressionContext:
		return b.visitPrimaryExpression(n)
	case *cppparser.LiteralContext:
		return b.visitLiteral(n)
	case *cppparser.IdExpressionContext:
		return b.visitIdExpression(n)
	case *cppparser.InitializerClauseContext:
		if n.BracedInitList() != nil {
			return b.visitBracedInitList(n.BracedInitList(), nil)
		}
		if assignment := n.AssignmentExpression(); assignment != nil {
			return b.evalNode(assignment)
		}
	case *cppparser.ConstantExpressionContext:
		return b.evalNode(n.ConditionalExpression())
	case *cppparser.BracedInitListContext:
		return b.visitBracedInitList(n, nil)
	case *cppparser.ThrowExpressionContext:
		if assignment := n.AssignmentExpression(); assignment != nil {
			b.evalNode(assignment)
		}
		return b.EmitUndefined("throw")
	}
	return b.EmitConstInstNil()
}

// foldChain left-folds a binary expression chain. The grammar nesting already
// fixes precedence, so folding each level left to right is correct.
func (b *singleFileBuilder) foldChain(ctx antlr.ParserRuleContext, eval func(antlr.Tree) ssa.Value) ssa.Value {
	var left ssa.Value
	operator := ""
	for _, child := range ctx.GetChildren() {
		if terminal, ok := child.(antlr.TerminalNode); ok {
			operator = terminal.GetText()
			continue
		}
		if shift, ok := child.(*cppparser.ShiftOperatorContext); ok && shift != nil {
			operator = shift.GetText()
			continue
		}
		value := eval(child)
		if left == nil {
			left = value
			continue
		}
		left = b.EmitBinOp(cppBinaryOpcode(operator), left, value)
		operator = ""
	}
	if left == nil {
		return b.EmitConstInstNil()
	}
	return left
}

func cppBinaryOpcode(operator string) ssa.BinaryOpcode {
	switch operator {
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
		return ssa.OpLogicAnd
	case "||":
		return ssa.OpLogicOr
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
	case ".*", "->*":
		return ssa.OpNone
	}
	return ssa.OpNone
}

func (b *singleFileBuilder) visitAssignmentExpression(expression *cppparser.AssignmentExpressionContext) ssa.Value {
	if conditional := expression.ConditionalExpression(); conditional != nil {
		return b.evalNode(conditional)
	}
	if expression.ThrowExpression() != nil {
		return b.evalNode(expression.ThrowExpression())
	}
	left, variable := b.evalLValue(expression.LogicalOrExpression())
	right := b.evalNode(expression.InitializerClause())
	if right == nil {
		right = b.EmitConstInstNil()
	}
	operator := "="
	if assignmentOperator := expression.AssignmentOperator(); assignmentOperator != nil {
		operator = assignmentOperator.GetText()
	}
	if operator != "=" {
		baseOp := strings.TrimSuffix(operator, "=")
		right = b.EmitBinOp(cppBinaryOpcode(baseOp), left, right)
	}
	if variable != nil {
		b.AssignVariable(variable, right)
	}
	return right
}

func (b *singleFileBuilder) visitConditionalExpression(expression *cppparser.ConditionalExpressionContext) ssa.Value {
	condition := b.evalNode(expression.LogicalOrExpression())
	if expression.Question() == nil {
		return condition
	}
	name := fmt.Sprintf("ternary_%d", expression.GetStart().GetStart())
	// Register the result in the enclosing scope before branching. A variable
	// first created inside an if arm is not visible to the merge, and the join
	// would have no phi for the ternary result.
	b.CreateVariable(name)
	var trueValue, falseValue ssa.Value
	builder := b.CreateIfBuilder()
	builder.SetCondition(func() ssa.Value {
		return condition
	}, func() {
		if value := b.evalNode(expression.Expression()); value != nil {
			trueValue = value
			b.AssignVariable(b.CreateVariable(name), value)
		}
	})
	if elseExpression := expression.AssignmentExpression(); elseExpression != nil {
		builder.SetElse(func() {
			if value := b.evalNode(elseExpression); value != nil {
				falseValue = value
				b.AssignVariable(b.CreateVariable(name), value)
			}
		})
	}
	builder.Build()
	if value := b.PeekValue(name); !utils.IsNil(value) {
		if phi, ok := ssa.ToPhi(value); ok {
			// The if merge kept the variable name but the phi value itself is
			// unnamed; name it so the result is traceable in SSA dumps.
			phi.SetName(name)
		}
		return value
	}
	// IfStmt.BuildFinish shadows the join scope with CoverBy, which drops a
	// variable that did not exist before the if. Merge the two branch values
	// explicitly in that case so the ternary still has a phi result.
	if !utils.IsNil(trueValue) && !utils.IsNil(falseValue) {
		return b.EmitPhi(name, ssa.Values{trueValue, falseValue})
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitCastExpression(expression *cppparser.CastExpressionContext) ssa.Value {
	if expression == nil {
		return b.EmitConstInstNil()
	}
	if unary := expression.UnaryExpression(); unary != nil {
		return b.evalNode(unary)
	}
	// C-style cast: (type-id) cast-expression.
	if typeID := expression.TheTypeId(); typeID != nil && expression.CastExpression() != nil {
		value := b.evalNode(expression.CastExpression())
		if utils.IsNil(value) {
			return b.EmitConstInstNil()
		}
		return b.EmitTypeCast(value, b.typeFromTypeIDText(typeID.GetText()))
	}
	if inner := expression.CastExpression(); inner != nil {
		return b.evalNode(inner)
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitUnaryExpression(expression *cppparser.UnaryExpressionContext) ssa.Value {
	switch {
	case expression.PostfixExpression() != nil:
		return b.visitPostfixExpression(expression.PostfixExpression().(*cppparser.PostfixExpressionContext))
	case expression.PlusPlus() != nil || expression.MinusMinus() != nil:
		operand := expression.UnaryExpression()
		if operand == nil {
			return b.EmitConstInstNil()
		}
		value, variable := b.evalLValue(operand)
		one := b.EmitConstInst(1)
		if expression.PlusPlus() != nil {
			value = b.EmitBinOp(ssa.OpAdd, value, one)
		} else {
			value = b.EmitBinOp(ssa.OpSub, value, one)
		}
		if variable != nil {
			b.AssignVariable(variable, value)
		}
		return value
	case expression.UnaryOperator() != nil:
		operand := expression.UnaryExpression()
		if operand == nil {
			return b.EmitConstInstNil()
		}
		switch expression.UnaryOperator().GetText() {
		case "*":
			return b.derefValue(operand)
		case "&":
			if _, variable := b.evalLValue(operand); variable != nil {
				return b.EmitConstPointer(variable)
			}
			return b.evalNode(operand)
		case "!":
			return b.EmitUnOp(ssa.OpNot, b.evalNode(operand))
		case "-":
			return b.EmitUnOp(ssa.OpNeg, b.evalNode(operand))
		case "+":
			return b.EmitUnOp(ssa.OpPlus, b.evalNode(operand))
		case "~":
			return b.EmitUnOp(ssa.OpBitwiseNot, b.evalNode(operand))
		}
		return b.evalNode(operand)
	case expression.Sizeof() != nil || expression.Alignof() != nil:
		// sizeof and alignof are compile-time constants; the frontend does not
		// model object layout, so the value is 0 rather than a guess.
		return b.EmitConstInst(0)
	case expression.NewExpression_() != nil:
		return b.visitNewExpression(expression.NewExpression_().(*cppparser.NewExpression_Context))
	case expression.DeleteExpression() != nil:
		return b.visitDeleteExpression(expression.DeleteExpression().(*cppparser.DeleteExpressionContext))
	case expression.NoExceptExpression() != nil:
		return b.EmitConstInst(true)
	}
	return b.EmitConstInstNil()
}

// derefValue reads the value behind a pointer. EmitConstPointer stores the origin
// name under "@pointer", so a plain dereference can recover the origin variable.
func (b *singleFileBuilder) derefValue(operand antlr.Tree) ssa.Value {
	value := b.evalNode(operand)
	if value == nil {
		return b.EmitConstInstNil()
	}
	if origin := b.GetOriginValue(value); origin != nil && !utils.IsNil(origin) {
		return origin
	}
	return value
}

func (b *singleFileBuilder) visitNewExpression(expression *cppparser.NewExpression_Context) ssa.Value {
	typeText := ""
	if newType := expression.NewTypeId(); newType != nil {
		typeText = newType.GetText()
	} else if typeID := expression.TheTypeId(); typeID != nil {
		typeText = typeID.GetText()
	}
	typeText = normalizeTypeText(typeText)
	typ := b.typeFromText(typeText)
	if strings.Contains(typeText, "[") {
		typ = ssa.NewSliceType(typ)
	}
	value := b.EmitUndefined("new " + typeText)
	if typ != nil {
		value.SetType(typ)
	}
	args := b.visitNewInitializer(expression.NewInitializer_())
	if blueprint, ok := typ.(*ssa.Blueprint); ok && !utils.IsNil(blueprint) {
		if constructor := blueprint.GetMagicMethod(ssa.Constructor, b.FunctionBuilder); !utils.IsNil(constructor) {
			b.EmitCall(b.NewCall(constructor, args))
		}
	}
	return value
}

func (b *singleFileBuilder) visitNewInitializer(raw cppparser.INewInitializer_Context) []ssa.Value {
	initializer, ok := raw.(*cppparser.NewInitializer_Context)
	if !ok || initializer == nil {
		return nil
	}
	if list := initializer.ExpressionList(); list != nil {
		return b.visitExpressionList(list)
	}
	if braced := initializer.BracedInitList(); braced != nil {
		if value := b.visitBracedInitList(braced, nil); value != nil {
			return []ssa.Value{value}
		}
	}
	return nil
}

func (b *singleFileBuilder) visitDeleteExpression(expression *cppparser.DeleteExpressionContext) ssa.Value {
	if cast := expression.CastExpression(); cast != nil {
		b.evalNode(cast)
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitPostfixExpression(expression *cppparser.PostfixExpressionContext) ssa.Value {
	if expression == nil {
		return b.EmitConstInstNil()
	}
	// Each node carries exactly one suffix alternative; the left-recursive part
	// is the nested PostfixExpression.
	switch {
	case expression.PrimaryExpression() != nil:
		return b.visitPrimaryExpression(expression.PrimaryExpression().(*cppparser.PrimaryExpressionContext))
	case expression.PostfixExpression() != nil:
		base := b.visitPostfixExpression(expression.PostfixExpression().(*cppparser.PostfixExpressionContext))
		switch {
		case expression.LeftParen() != nil:
			args := b.visitExpressionList(expression.ExpressionList())
			if name := postfixCalleeName(expression); name != "" {
				base = b.resolveCallableValue(name, len(args))
			}
			return b.EmitCall(b.NewCall(base, args))
		case expression.LeftBracket() != nil:
			if target := expression.Expression(); target != nil {
				return b.ReadMemberCallValue(base, b.evalNode(target))
			}
			return base
		case expression.Dot() != nil || expression.Arrow() != nil:
			name := ""
			if id := expression.IdExpression(); id != nil {
				name = id.GetText()
			}
			if name == "" {
				return base
			}
			object := base
			if expression.Arrow() != nil {
				if origin := b.GetOriginValue(base); origin != nil && !utils.IsNil(origin) {
					object = origin
				}
			}
			if method := b.memberMethodForValue(object, name); !utils.IsNil(method) {
				return method
			}
			return b.ReadMemberCallValue(object, b.EmitConstInst(name))
		case expression.PlusPlus() != nil || expression.MinusMinus() != nil:
			// The postfix form also updates the lvalue; treating it as a pure
			// BinOp would leave `i++` without an assignment and the loop
			// variable would keep its pre-loop value.
			value, variable := b.evalLValue(expression.PostfixExpression())
			if utils.IsNil(value) {
				value = base
			}
			one := b.EmitConstInst(1)
			var updated ssa.Value
			if expression.PlusPlus() != nil {
				updated = b.EmitBinOp(ssa.OpAdd, value, one)
			} else {
				updated = b.EmitBinOp(ssa.OpSub, value, one)
			}
			if variable != nil {
				b.AssignVariable(variable, updated)
			}
			return updated
		}
		return base
	case expression.SimpleTypeSpecifier() != nil || expression.TypeNameSpecifier() != nil:
		// The grammar also matches `name(args)` as a function-style cast on a
		// type specifier. For an unknown plain name this is a call, not a
		// temporary: `println("x")` must reach println, not a discarded value.
		spelling := ""
		if simple := expression.SimpleTypeSpecifier(); simple != nil {
			spelling = simple.GetText()
			if specifier, ok := simple.(*cppparser.SimpleTypeSpecifierContext); ok && specifier != nil && specifier.SimpleTemplateId() != nil {
				if templateID, ok := specifier.SimpleTemplateId().(*cppparser.SimpleTemplateIdContext); ok && templateID != nil && templateID.TemplateName() != nil {
					spelling = templateID.TemplateName().GetText()
				}
			}
		}
		// An explicit template call `twice<int>(3)` names the function before
		// the angle brackets. Monomorphization is not performed; the call binds
		// to the template function built with placeholder types.
		callableSpelling, templateArguments := splitTemplateSpelling(spelling)
		if len(templateArguments) == 0 {
			callableSpelling = spelling
		}
		if expression.LeftParen() != nil && isPlainCppIdentifier(callableSpelling) && !b.isKnownTypeSpelling(callableSpelling) {
			args := b.visitExpressionList(expression.ExpressionList())
			if callee := b.resolveCallableValue(callableSpelling, len(args)); !utils.IsNil(callee) {
				return b.EmitCall(b.NewCall(callee, args))
			}
		}
		typeText := expression.GetText()
		if expression.LeftParen() != nil && isPlainCppIdentifier(spelling) && b.isKnownTypeSpelling(spelling) {
			// Functional cast: the type is the spelling before the parenthesis.
			typeText = spelling
		}
		typ := b.typeFromText(typeText)
		value := b.EmitUndefined("value-of " + typeText)
		value.SetType(typ)
		if braced := expression.BracedInitList(); braced != nil {
			if initialized := b.visitBracedInitList(braced, typ); initialized != nil {
				return initialized
			}
		}
		if expression.LeftParen() != nil {
			args := b.visitExpressionList(expression.ExpressionList())
			if len(args) == 1 && !utils.IsNil(args[0]) {
				// Functional cast: `int(x)` and `Counter(x)`.
				return b.EmitTypeCast(args[0], typ)
			}
		}
		return value
	case expression.Dynamic_cast() != nil || expression.Static_cast() != nil ||
		expression.Reinterpret_cast() != nil || expression.Const_cast() != nil:
		// All four named casts lower to the dedicated typecast instruction;
		// the cast flavour changes diagnostics, not SSA dataflow.
		if castValue := expression.Expression(); castValue != nil {
			value := b.evalNode(castValue)
			if typeID := expression.TheTypeId(); typeID != nil && !utils.IsNil(value) {
				return b.EmitTypeCast(value, b.typeFromTypeIDText(typeID.GetText()))
			}
			return value
		}
	case expression.TypeIdOfTheTypeId() != nil:
		if castValue := expression.Expression(); castValue != nil {
			return b.evalNode(castValue)
		}
		if typeID := expression.TheTypeId(); typeID != nil {
			value := b.EmitUndefined(typeID.GetText())
			value.SetType(b.typeFromText(typeID.GetText()))
			return value
		}
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitPrimaryExpression(expression *cppparser.PrimaryExpressionContext) ssa.Value {
	if expression == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case expression.IdExpression() != nil:
		return b.visitIdExpression(expression.IdExpression().(*cppparser.IdExpressionContext))
	case expression.This() != nil:
		if value := b.ReadValue("this"); value != nil {
			return value
		}
		return b.EmitUndefined("this")
	case expression.Expression() != nil:
		return b.evalNode(expression.Expression())
	case expression.LambdaExpression() != nil:
		return b.visitLambdaExpression(expression.LambdaExpression().(*cppparser.LambdaExpressionContext))
	case len(expression.AllLiteral()) > 0:
		var value ssa.Value
		for _, literal := range expression.AllLiteral() {
			value = b.evalNode(literal)
		}
		return value
	}
	return b.EmitConstInstNil()
}

// visitLambdaExpression builds the lambda body as an anonymous function.
// Explicit captures become free-value parameters, matching the Go/yak closure
// model; default captures (`[&]`, `[=]`) are accepted but not expanded yet.
func (b *singleFileBuilder) visitLambdaExpression(lambda *cppparser.LambdaExpressionContext) ssa.Value {
	if lambda == nil || lambda.CompoundStatement() == nil {
		return b.EmitConstInstNil()
	}
	name := fmt.Sprintf("lambda_%d", lambda.GetStart().GetStart())
	fn := b.NewFunc(name)
	previous := b.FunctionBuilder
	b.FunctionBuilder = b.PushFunction(fn)
	if declarator, ok := lambda.LambdaDeclarator().(*cppparser.LambdaDeclaratorContext); ok && declarator != nil {
		if clause, ok := declarator.ParameterDeclarationClause().(*cppparser.ParameterDeclarationClauseContext); ok && clause != nil {
			if list, ok := clause.ParameterDeclarationList().(*cppparser.ParameterDeclarationListContext); ok && list != nil {
				for _, rawDeclaration := range list.AllParameterDeclaration() {
					declaration, ok := rawDeclaration.(*cppparser.ParameterDeclarationContext)
					if !ok || declaration == nil {
						continue
					}
					parameter := b.parseParameterDeclaration(declaration)
					param := b.NewParam(parameter.name)
					if parameter.typ != nil {
						param.SetType(parameter.typ)
					}
				}
			}
		}
	}
	for _, capture := range lambdaCaptureNames(lambda) {
		b.BuildFreeValue(capture)
	}
	b.visitCompoundStatement(lambda.CompoundStatement())
	b.Finish()
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	return fn
}

// lambdaCaptureNames returns the identifiers in a lambda capture list.
// Default captures and `this` contribute no identifier.
func lambdaCaptureNames(lambda *cppparser.LambdaExpressionContext) []string {
	if lambda == nil {
		return nil
	}
	introducer, ok := lambda.LambdaIntroducer().(*cppparser.LambdaIntroducerContext)
	if !ok || introducer == nil {
		return nil
	}
	capture, ok := introducer.LambdaCapture().(*cppparser.LambdaCaptureContext)
	if !ok || capture == nil {
		return nil
	}
	list, ok := capture.CaptureList().(*cppparser.CaptureListContext)
	if !ok || list == nil {
		return nil
	}
	var names []string
	seen := make(map[string]struct{})
	for _, raw := range list.AllCapture() {
		item, ok := raw.(*cppparser.CaptureContext)
		if !ok || item == nil {
			continue
		}
		name := ""
		if simple, ok := item.SimpleCapture().(*cppparser.SimpleCaptureContext); ok && simple != nil && simple.Identifier() != nil {
			name = simple.Identifier().GetText()
		}
		if init, ok := item.InitCapture().(*cppparser.InitCaptureContext); ok && init != nil && init.Identifier() != nil {
			name = init.Identifier().GetText()
		}
		if name == "" || !isPlainCppIdentifier(name) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func (b *singleFileBuilder) visitIdExpression(expression *cppparser.IdExpressionContext) ssa.Value {
	if expression == nil {
		return b.EmitConstInstNil()
	}
	text := strings.TrimPrefix(expression.GetText(), "::")
	if base, args := splitTemplateSpelling(text); len(args) > 0 {
		// `twice<int>` names the template function twice; monomorphization is
		// not performed, so resolve the base name.
		text = base
	}
	// PeekValue does not create an Undefined for an unknown name. Creating one
	// here would shadow a sibling function exported in the program table.
	if value := b.PeekValue(text); !utils.IsNil(value) {
		return value
	}
	// Function definitions are exported by their qualified name. A function
	// body or a sibling function resolves the name through this table; the
	// lexical scope only holds locals.
	if value := b.GetProgram().GetExportValue(text); !utils.IsNil(value) {
		return value
	}
	if name, scope := splitQualifiedName(text); len(scope) > 0 {
		if value := b.PeekValue(name); !utils.IsNil(value) {
			return value
		}
		if value := b.GetProgram().GetExportValue(name); !utils.IsNil(value) {
			return value
		}
		if blueprint := b.ensureClassBlueprint(strings.Join(scope, "::")); blueprint != nil && !utils.IsNil(blueprint.Container()) {
			if value := b.ReadMemberCallValue(blueprint.Container(), b.EmitConstInst(name)); value != nil {
				return value
			}
		}
	}
	for _, using := range b.usingNamespaces {
		if value := b.PeekValue(using + "::" + text); !utils.IsNil(value) {
			return value
		}
		if value := b.GetProgram().GetExportValue(using + "::" + text); !utils.IsNil(value) {
			return value
		}
	}
	return b.ReadValue(text)
}

func (b *singleFileBuilder) visitLiteral(literal *cppparser.LiteralContext) ssa.Value {
	if literal == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case literal.IntegerLiteral() != nil:
		if parsed, err := cliteral.ParseInt(literal.GetText()); err == nil {
			return b.EmitConstInst(parsed)
		}
	case literal.FloatingLiteral() != nil:
		if parsed, err := cliteral.ParseFloat(literal.GetText()); err == nil {
			return b.EmitConstInst(parsed)
		}
	case literal.CharacterLiteral() != nil:
		return b.EmitConstInst(cliteral.ParseChar(literal.GetText()))
	case literal.StringLiteral() != nil:
		if text, ok := cliteral.ParseString(literal.GetText()); ok {
			return b.EmitConstInst(text)
		}
	case literal.BooleanLiteral() != nil:
		return b.EmitConstInst(strings.EqualFold(literal.GetText(), "true"))
	case literal.PointerLiteral() != nil:
		return b.EmitConstInstNil()
	case literal.UserDefinedLiteral() != nil:
		if parsed, err := cliteral.ParseInt(literal.GetText()); err == nil {
			return b.EmitConstInst(parsed)
		}
	}
	return b.EmitConstInstNil()
}

// evalLValue resolves the variable an assignment writes to. It returns the
// current value and the variable; either may be nil when the target is not a
// simple variable or member.
func (b *singleFileBuilder) evalLValue(node antlr.Tree) (ssa.Value, *ssa.Variable) {
	switch n := node.(type) {
	case *cppparser.PostfixExpressionContext:
		return b.postfixLValue(n)
	case *cppparser.PrimaryExpressionContext:
		if id := n.IdExpression(); id != nil {
			name := strings.TrimPrefix(id.GetText(), "::")
			return b.ReadValue(name), b.CreateVariable(name)
		}
		if inner := n.Expression(); inner != nil {
			return b.evalLValue(inner)
		}
	case *cppparser.IdExpressionContext:
		name := strings.TrimPrefix(n.GetText(), "::")
		return b.ReadValue(name), b.CreateVariable(name)
	case *cppparser.UnaryExpressionContext:
		if n.UnaryOperator() != nil && n.UnaryOperator().GetText() == "*" {
			pointer := b.evalNode(n.UnaryExpression())
			if pointer != nil {
				if origin := b.GetOriginValue(pointer); origin != nil && !utils.IsNil(origin) {
					name := b.GetOriginPointerName(pointer)
					if name == "" {
						name = origin.GetName()
					}
					return origin, b.CreateVariable(name)
				}
			}
			return pointer, nil
		}
		if n.PostfixExpression() != nil {
			return b.postfixLValue(n.PostfixExpression().(*cppparser.PostfixExpressionContext))
		}
	case *cppparser.CastExpressionContext:
		if n.CastExpression() != nil {
			return b.evalLValue(n.CastExpression())
		}
		if n.UnaryExpression() != nil {
			return b.evalLValue(n.UnaryExpression())
		}
	}
	value := b.evalNode(node)
	if value == nil {
		return nil, nil
	}
	name := value.GetName()
	if name == "" {
		// Constants carry no name. Recover a plain lvalue from the source text
		// so `value += 2` still updates `value`; member and pointer targets are
		// handled by the branches above.
		if titled, ok := node.(interface{ GetText() string }); ok {
			if text := strings.TrimPrefix(titled.GetText(), "::"); isPlainCppIdentifier(text) {
				name = text
			}
		}
	}
	if name == "" {
		return value, nil
	}
	return value, b.CreateVariable(name)
}

func (b *singleFileBuilder) postfixLValue(expression *cppparser.PostfixExpressionContext) (ssa.Value, *ssa.Variable) {
	if expression == nil {
		return nil, nil
	}
	if expression.Dot() != nil || expression.Arrow() != nil {
		if nested := expression.PostfixExpression(); nested != nil {
			object := b.visitPostfixExpression(nested.(*cppparser.PostfixExpressionContext))
			if expression.Arrow() != nil {
				if origin := b.GetOriginValue(object); origin != nil && !utils.IsNil(origin) {
					object = origin
				}
			}
			name := ""
			if id := expression.IdExpression(); id != nil {
				name = id.GetText()
			}
			if name != "" && object != nil {
				key := b.EmitConstInst(name)
				value := b.ReadMemberCallValue(object, key)
				return value, b.CreateMemberCallVariable(object, key, true)
			}
			return object, nil
		}
	}
	if expression.LeftBracket() != nil && expression.PostfixExpression() != nil {
		base := b.visitPostfixExpression(expression.PostfixExpression().(*cppparser.PostfixExpressionContext))
		var index ssa.Value = b.EmitConstInstNil()
		if target := expression.Expression(); target != nil {
			index = b.evalNode(target)
		}
		if base != nil {
			return b.ReadMemberCallValue(base, index), b.CreateMemberCallVariable(base, index, true)
		}
	}
	if nested := expression.PostfixExpression(); nested != nil {
		return b.evalLValue(nested)
	}
	if primary := expression.PrimaryExpression(); primary != nil {
		if ctx, ok := primary.(*cppparser.PrimaryExpressionContext); ok && ctx != nil {
			return b.evalLValue(ctx)
		}
	}
	return nil, nil
}

// memberMethodForValue returns the blueprint method a member access names, so a
// call binds to the function definition instead of a detached member value.
func (b *singleFileBuilder) memberMethodForValue(object ssa.Value, name string) ssa.Value {
	if utils.IsNil(object) || name == "" {
		return nil
	}
	typ := object.GetType()
	if obj, ok := ssa.ToObjectType(typ); ok && obj != nil && obj.Kind == ssa.PointerKind && obj.FieldType != nil {
		typ = obj.FieldType
	}
	blueprint, ok := ssa.ToClassBluePrintType(typ)
	if !ok || blueprint == nil {
		return nil
	}
	if method := blueprint.GetNormalMethod(name); !utils.IsNil(method) {
		return method
	}
	if method := blueprint.GetStaticMethod(name); !utils.IsNil(method) {
		return method
	}
	return nil
}

// postfixCalleeName returns the identifier a call is made through, when the
// callee is a plain name (`add(1, 2)`); member and pointer calls return "".
func postfixCalleeName(expression *cppparser.PostfixExpressionContext) string {
	if expression == nil {
		return ""
	}
	nested := expression.PostfixExpression()
	if nested == nil {
		return ""
	}
	postfix, ok := nested.(*cppparser.PostfixExpressionContext)
	if !ok || postfix == nil || postfix.PrimaryExpression() == nil {
		return ""
	}
	primary, ok := postfix.PrimaryExpression().(*cppparser.PrimaryExpressionContext)
	if !ok || primary == nil || primary.IdExpression() == nil {
		return ""
	}
	text := strings.TrimPrefix(primary.IdExpression().GetText(), "::")
	if base, args := splitTemplateSpelling(text); len(args) > 0 {
		text = base
	}
	if !isPlainCppIdentifier(text) {
		return ""
	}
	return text
}
