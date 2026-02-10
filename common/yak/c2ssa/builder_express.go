package c2ssa

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	cparser "github.com/yaklang/yaklang/common/yak/antlr4c/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *astbuilder) buildExpression(ast *cparser.ExpressionContext, isLeft bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	handlerJumpExpression := func(cond func(string) ssa.Value, trueExpr, falseExpr func() ssa.Value, name string) ssa.Value {
		id := name
		variable := b.CreateVariable(id)
		b.AssignVariable(variable, b.EmitValueOnlyDeclare(id))

		ifb := b.CreateIfBuilder()
		ifb.AppendItem(
			func() ssa.Value {
				return cond(id)
			},
			func() {
				v := trueExpr()
				variable := b.CreateVariable(id)
				b.AssignVariable(variable, v)
			},
		)
		ifb.SetElse(func() {
			v := falseExpr()
			variable := b.CreateVariable(id)
			b.AssignVariable(variable, v)
		})
		ifb.Build()
		// generator phi instruction
		v := b.ReadValue(id)
		v.SetName(ast.GetText())
		return v
	}

	// 1. 一元运算符: unary_op = (Plus | Minus | Not | Caret | Star | And) expression
	if ast.GetUnary_op() != nil && ast.Expression(0) != nil {
		op := ast.GetUnary_op().GetText()
		right, left := b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), false)
		if right != nil {
			switch op {
			case "+":
				return b.EmitUnOp(ssa.OpPlus, right), nil
			case "-":
				return b.EmitUnOp(ssa.OpNeg, right), nil
			case "!":
				return b.EmitUnOp(ssa.OpNot, right), nil
			case "~":
				return b.EmitUnOp(ssa.OpBitwiseNot, right), nil
			case "*":
				if right.GetType().GetTypeKind() == ssa.PointerKind {
					return b.GetOriginValue(right), left
				} else {
					return right, left
				}
			case "&":
				if _, op1Var := b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), true); op1Var != nil {
					return b.EmitConstPointer(op1Var), nil
				}
			}
		}
		return right, nil
	}

	// 2. 乘法/除法/取模/位移/按位与: expression mul_op = (Star | Div | Mod | LeftShift | RightShift | And) expression
	if ast.GetMul_op() != nil && len(ast.AllExpression()) >= 2 {
		op := ast.GetMul_op().GetText()
		op1, _ := b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), false)
		op2, _ := b.buildExpression(ast.Expression(1).(*cparser.ExpressionContext), false)
		if op1 != nil && op2 != nil {
			switch op {
			case "*":
				return b.EmitBinOp(ssa.OpMul, op1, op2), nil
			case "/":
				return b.EmitBinOp(ssa.OpDiv, op1, op2), nil
			case "%":
				return b.EmitBinOp(ssa.OpMod, op1, op2), nil
			case "<<":
				return b.EmitBinOp(ssa.OpShl, op1, op2), nil
			case ">>":
				return b.EmitBinOp(ssa.OpShr, op1, op2), nil
			case "&":
				return b.EmitBinOp(ssa.OpAnd, op1, op2), nil
			}
		}
		return op1, nil
	}

	// 3. 加法/减法/按位或/按位异或: expression add_op = (Plus | Minus | Or | Caret) expression
	if ast.GetAdd_op() != nil && len(ast.AllExpression()) >= 2 {
		op := ast.GetAdd_op().GetText()
		op1, _ := b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), false)
		op2, _ := b.buildExpression(ast.Expression(1).(*cparser.ExpressionContext), false)
		if op1 != nil && op2 != nil {
			switch op {
			case "+":
				return b.EmitBinOp(ssa.OpAdd, op1, op2), nil
			case "-":
				return b.EmitBinOp(ssa.OpSub, op1, op2), nil
			case "|":
				return b.EmitBinOp(ssa.OpOr, op1, op2), nil
			case "^":
				return b.EmitBinOp(ssa.OpXor, op1, op2), nil
			}
		}
		return op1, nil
	}

	// 4. 关系运算符: expression rel_op = (Equal | NotEqual | Less | LessEqual | Greater | GreaterEqual) expression
	if ast.GetRel_op() != nil && len(ast.AllExpression()) >= 2 {
		op := ast.GetRel_op().GetText()
		op1, _ := b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), false)
		op2, _ := b.buildExpression(ast.Expression(1).(*cparser.ExpressionContext), false)
		if op1 != nil && op2 != nil {
			switch op {
			case "==":
				return b.EmitBinOp(ssa.OpEq, op1, op2), nil
			case "!=":
				return b.EmitBinOp(ssa.OpNotEq, op1, op2), nil
			case "<":
				return b.EmitBinOp(ssa.OpLt, op1, op2), nil
			case "<=":
				return b.EmitBinOp(ssa.OpLtEq, op1, op2), nil
			case ">":
				return b.EmitBinOp(ssa.OpGt, op1, op2), nil
			case ">=":
				return b.EmitBinOp(ssa.OpGtEq, op1, op2), nil
			}
		}
		return op1, nil
	}

	// 5. 逻辑与: expression AndAnd expression
	if ast.AndAnd() != nil && len(ast.AllExpression()) >= 2 {
		left, _ := b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), false)
		right, _ := b.buildExpression(ast.Expression(1).(*cparser.ExpressionContext), false)
		if left != nil && right != nil {
			return b.EmitBinOp(ssa.OpLogicAnd, left, right), nil
		}
		return left, nil
	}

	// 6. 逻辑或: expression OrOr expression
	if ast.OrOr() != nil && len(ast.AllExpression()) >= 2 {
		left, _ := b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), false)
		right, _ := b.buildExpression(ast.Expression(1).(*cparser.ExpressionContext), false)
		if left != nil && right != nil {
			return b.EmitBinOp(ssa.OpLogicOr, left, right), nil
		}
		return left, nil
	}

	// 7. 括号表达式: '(' expression ')'
	if ast.LeftParen() != nil && ast.Expression(0) != nil && ast.RightParen() != nil {
		return b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), false)
	}

	// 8. 三元表达式: expression ('?' expression ':' expression)
	if ast.Question() != nil {
		condition, _ := b.buildExpression(ast.Expression(0).(*cparser.ExpressionContext), false)
		value1, _ := b.buildExpression(ast.Expression(1).(*cparser.ExpressionContext), false)
		value2, _ := b.buildExpression(ast.Expression(2).(*cparser.ExpressionContext), false)
		return handlerJumpExpression(
			func(id string) ssa.Value {
				return condition
			},
			func() ssa.Value {
				return value1
			},
			func() ssa.Value {
				return value2
			},
			ssa.AndExpressionVariable,
		), nil
	}

	// 9. 基本表达式: castExpression
	if p := ast.CastExpression(); p != nil {
		return b.buildCastExpression(p.(*cparser.CastExpressionContext), isLeft)
	}

	// 10. 语句表达式: statementsExpression
	if s := ast.StatementsExpression(); s != nil {
		return b.buildStatementsExpression(s.(*cparser.StatementsExpressionContext)), nil
	}

	// 11. 声明表达式: declarationSpecifier
	if d := ast.DeclarationSpecifier(); d != nil {
		ssatype := b.buildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext))
		return ssa.NewTypeValue(ssatype), nil
	}

	return b.EmitConstInst(0), b.CreateVariable("")
}

func (b *astbuilder) buildCoreExpression(ast *cparser.CoreExpressionContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var right ssa.Value
	if u := ast.CastExpression(); u != nil {
		right, _ = b.buildCastExpression(u.(*cparser.CastExpressionContext), false)
	} else if a := ast.AssignmentExpression(); a != nil {
		right = b.buildAssignmentExpression(a.(*cparser.AssignmentExpressionContext))
	}

	if utils.IsNil(right) {
		right = b.EmitConstInst(0)
	}
	return right
}

func (b *astbuilder) buildAssignmentExpression(ast *cparser.AssignmentExpressionContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var right ssa.Value

	if leftExpr := ast.LeftExpression(); leftExpr != nil {
		if a := ast.AssignmentOperator(); a != nil {
			if e := ast.Expression(); e != nil {
				_, left := b.buildLeftExpression(leftExpr.(*cparser.LeftExpressionContext), true)
				newRight, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
				if utils.IsNil(newRight) {
					newRight = b.EmitUndefined(left.GetName())
				}

				if left != nil {
					right = b.ReadValue(left.GetName())
					if utils.IsNil(right) {
						right = b.EmitUndefined(left.GetName())
					}
				} else {
					right = newRight
				}

				op := a.(*cparser.AssignmentOperatorContext).GetText()
				switch op {
				case "=":
					right = newRight
				case "*=":
					right = b.EmitBinOp(ssa.OpMul, right, newRight)
				case "/=":
					right = b.EmitBinOp(ssa.OpDiv, right, newRight)
				case "%=":
					right = b.EmitBinOp(ssa.OpMod, right, newRight)
				case "+=":
					right = b.EmitBinOp(ssa.OpAdd, right, newRight)
				case "-=":
					right = b.EmitBinOp(ssa.OpSub, right, newRight)
				case "<<=":
					right = b.EmitBinOp(ssa.OpShl, right, newRight)
				case ">>=":
					right = b.EmitBinOp(ssa.OpShr, right, newRight)
				case "&=":
					right = b.EmitBinOp(ssa.OpAnd, right, newRight)
				case "^=":
					right = b.EmitBinOp(ssa.OpXor, right, newRight)
				case "|=":
					right = b.EmitBinOp(ssa.OpOr, right, newRight)
				default:
					right = newRight
					b.NewError(ssa.Warn, TAG, fmt.Sprintf("not find: %s", op))
				}
				if left != nil {
					b.AssignVariable(left, right)
					right.SetType(newRight.GetType())
				}
			}
		}
	}

	if utils.IsNil(right) {
		right = b.EmitConstInst(0)
	}
	return right
}

func (b *astbuilder) buildUnaryExpression(ast *cparser.UnaryExpressionContext, isLeft bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var right ssa.Value
	var left *ssa.Variable

	// 1. 前缀 ++/--: ('++' | '--') leftExpression
	if ast.PlusPlus() != nil || ast.MinusMinus() != nil {
		if leftExpr := ast.LeftExpression(); leftExpr != nil {
			right, left = b.buildLeftExpression(leftExpr.(*cparser.LeftExpressionContext), true)
			if left != nil {
				right = b.ReadValue(left.GetName())
				if utils.IsNil(right) {
					right = b.EmitConstInst(0)
				}
				if ast.PlusPlus() != nil {
					right = b.EmitBinOp(ssa.OpAdd, right, b.EmitConstInst(1))
				} else if ast.MinusMinus() != nil {
					right = b.EmitBinOp(ssa.OpSub, right, b.EmitConstInst(1))
				}
				b.AssignVariable(left, right)
			}
			return right, left
		}
	}

	// 2. 取地址 &: '&' leftExpression
	if ast.And() != nil {
		if leftExpr := ast.LeftExpression(); leftExpr != nil {
			_, left = b.buildLeftExpression(leftExpr.(*cparser.LeftExpressionContext), true)
			if left != nil {
				return b.EmitConstPointer(left), nil
			}
		}
	}

	// 3. 指针解引用: '*' unaryExpression
	if len(ast.AllStar()) > 0 {
		if u := ast.UnaryExpression(); u != nil {
			right, left = b.buildUnaryExpression(u.(*cparser.UnaryExpressionContext), false)
			if right != nil {
				if right.GetType().GetTypeKind() == ssa.PointerKind {
					if param, ok := ssa.ToParameter(right); ok && !param.IsFreeValue {
						b.ReferenceParameter(right.GetName(), param.FormalParameterIndex, ssa.PointerSideEffect)
						// left = b.GetAndCreateOriginPointer(b.EmitConstPointer(left))
					} else {
						if isLeft {
							left = b.GetAndCreateOriginPointer(right)
						}
						right = b.GetOriginValue(right)
					}
				}
			}
		}
	}

	// 3. ('sizeof' | '_Alignof') '(' typeName ')'
	if t := ast.TypeName(); t != nil && ast.Sizeof() != nil {
		ssatype := b.buildTypeName(t.(*cparser.TypeNameContext))
		return b.EmitCall(b.NewCall(b.EmitUndefined(ast.Sizeof().GetText()), ssa.Values{b.GetDefaultValue(ssatype)})), nil
	}

	// 4. '&&' unaryExpression - GCC 标签地址扩展
	if ast.AndAnd() != nil {
		if u := ast.UnaryExpression(); u != nil {
			// 尝试从 unaryExpression 中提取标签名
			// 通常 &&label 中的 label 是一个标识符
			if postfix := u.(*cparser.UnaryExpressionContext).PostfixExpression(); postfix != nil {
				if primary := postfix.(*cparser.PostfixExpressionContext).PrimaryExpression(); primary != nil {
					if id := primary.(*cparser.PrimaryExpressionContext).Identifier(); id != nil {
						labelName := id.GetText()
						// 获取或创建标签
						labelBuilder := b.GetLabelByName(labelName)
						_ = labelBuilder
						// 标签地址通常作为指针值返回
						// 这里我们返回标签的引用（作为特殊值）
						labelValue := b.EmitUndefined(fmt.Sprintf("&&%s", labelName))
						labelValue.SetType(ssa.NewPointerType())
						return labelValue, nil
					}
				}
			}
			// 如果无法提取标签名，递归处理
			return b.buildUnaryExpression(u.(*cparser.UnaryExpressionContext), isLeft)
		}
	}

	// 5. postfixExpression
	if p := ast.PostfixExpression(); p != nil {
		right, left = b.buildPostfixExpression(p.(*cparser.PostfixExpressionContext), isLeft)
	}

	return right, left
}

func (b *astbuilder) buildPostfixExpression(ast *cparser.PostfixExpressionContext, isLeft bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var right ssa.Value
	var left *ssa.Variable

	// log.Infof("exp = %s\n", ast.GetText())

	// 1. primaryExpression postfixSuffix*
	if p := ast.PrimaryExpression(); p != nil {
		right, left = b.buildPrimaryExpression(p.(*cparser.PrimaryExpressionContext), isLeft)
		// 处理 postfixSuffix*
		for _, suffix := range ast.AllPostfixSuffix() {
			right, left = b.buildPostfixSuffix(suffix.(*cparser.PostfixSuffixContext), right, left, isLeft)
		}
		return right, left
	}

	// 2. '__extension__'? '(' typeName ')' '{' initializerList ','? '}' postfixSuffix*
	if t := ast.TypeName(); t != nil {
		ssatype := b.buildTypeName(t.(*cparser.TypeNameContext))
		_ = ssatype
		if i := ast.InitializerList(); i != nil {
			right = b.buildInitializerList(i.(*cparser.InitializerListContext))
		}
		// 处理 postfixSuffix*
		for _, suffix := range ast.AllPostfixSuffix() {
			right, left = b.buildPostfixSuffix(suffix.(*cparser.PostfixSuffixContext), right, left, isLeft)
		}
		return right, left
	}

	// 3. leftExpression '++' | leftExpression '--'
	if ast.PlusPlus() != nil || ast.MinusMinus() != nil {
		if leftExpr := ast.LeftExpression(); leftExpr != nil {
			right, left = b.buildLeftExpression(leftExpr.(*cparser.LeftExpressionContext), true)
			if left != nil {
				right = b.ReadValue(left.GetName())
				if utils.IsNil(right) {
					right = b.EmitConstInst(0)
				}
				oldRight := right
				if ast.PlusPlus() != nil {
					right = b.EmitBinOp(ssa.OpAdd, right, b.EmitConstInst(1))
				} else if ast.MinusMinus() != nil {
					right = b.EmitBinOp(ssa.OpSub, right, b.EmitConstInst(1))
				}
				b.AssignVariable(left, right)
				// 后缀操作符返回旧值
				return oldRight, left
			}
		}
	}

	return right, left
}

// buildPostfixSuffix 处理后缀操作（数组下标、函数调用、成员访问）
func (b *astbuilder) buildPostfixSuffix(ast *cparser.PostfixSuffixContext, right ssa.Value, left *ssa.Variable, isLeft bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	// 1. 数组下标：'[' expression ']'
	if ast.LeftBracket() != nil && ast.RightBracket() != nil {
		if e := ast.Expression(); e != nil {
			index, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
			right = b.ReadMemberCallValue(right, index)
			return right, left
		}
	}

	// 2. 函数调用：'(' argumentExpressionList? ')'
	if ast.LeftParen() != nil && ast.RightParen() != nil {
		var args ssa.Values
		if a := ast.ArgumentExpressionList(); a != nil {
			args = b.buildArgumentExpressionList(a.(*cparser.ArgumentExpressionListContext))
		}

		if right != nil && right.GetName() == "malloc" {
			if len(args) > 0 {
				if tv, ok := ssa.ToTypeValue(args[0]); ok {
					right = b.EmitMakeBuildWithType(tv.GetType(), nil, nil)
				} else if c, ok := ssa.ToConstInst(args[0]); ok {
					index, _ := strconv.Atoi(c.String())
					newtype := ssa.NewSliceType(ssa.CreateByteType())
					newtype.Len = index
					right = b.EmitMakeBuildWithType(newtype, nil, nil)
				}
			}
			return right, left
		}
		right = b.EmitCall(b.NewCall(right, args))
		return right, left
	}

	// 3. 结构体成员：'.' Identifier 或 '->' Identifier
	if ast.Dot() != nil || ast.Arrow() != nil {
		isPointer := ast.Arrow() != nil
		if id := ast.Identifier(); id != nil {
			key := id.GetText()
			if right != nil {
				if t := right.GetType(); t != nil && t.GetTypeKind() == ssa.PointerKind {
					right = b.GetOriginValue(right)
				}
				right = b.ReadMemberCallValue(right, b.EmitConstInst(key))
			}
			if left != nil {
				member := b.ReadValue(left.GetName())
				// 处理指针类型的解引用
				if t := member.GetType(); t != nil && t.GetTypeKind() == ssa.PointerKind {
					member = b.GetOriginValue(member)
				}
				// 创建成员访问变量
				left = b.CreateMemberCallVariable(member, b.EmitConstInst(key))
				// 如果是通过指针访问参数成员，注册副作用
				if isPointer {
					if p, ok := ssa.ToParameter(member); ok && !p.IsFreeValue {
						b.ReferenceParameter(left.GetName(), p.FormalParameterIndex, ssa.PointerSideEffect)
					}
				}
			}
		}
	}

	return right, left
}

func (b *astbuilder) buildInitializerList(ast *cparser.InitializerListContext, ssatype ...ssa.Type) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var keys, values ssa.Values
	for _, d := range ast.AllDesignation() {
		keys = append(keys, b.buildDesignation(d.(*cparser.DesignationContext))...)
	}
	if len(ssatype) > 0 && len(keys) == 0 {
		if objType, ok := ssa.ToObjectType(ssatype[0]); ok {
			keys = append(keys, objType.Keys...)
		}
	}

	for _, i := range ast.AllInitializer() {
		values = append(values, b.buildInitializer(i.(*cparser.InitializerContext), ssatype...))
	}
	obj := b.InterfaceAddFieldBuild(len(values), func(i int) ssa.Value {
		if i >= len(keys) {
			return b.EmitConstInst(i)
		}
		return keys[i]
	}, func(i int) ssa.Value {
		return values[i]
	})
	if len(ssatype) > 0 {
		coverType(obj.GetType(), ssatype[0])
	}
	return obj
}

func (b *astbuilder) buildInitializer(ast *cparser.InitializerContext, ssatype ...ssa.Type) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var value ssa.Value
	if a := ast.Expression(); a != nil {
		value, _ = b.buildExpression(a.(*cparser.ExpressionContext), false)
	} else if i := ast.InitializerList(); i != nil {
		value = b.buildInitializerList(i.(*cparser.InitializerListContext), ssatype...)
	}
	if utils.IsNil(value) {
		value = b.EmitConstInst(0)
	}

	return value
}

func (b *astbuilder) buildDesignation(ast *cparser.DesignationContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DesignatorList(); d != nil {
		return b.buildDesignatorList(d.(*cparser.DesignatorListContext))
	}
	return nil
}

func (b *astbuilder) buildDesignatorList(ast *cparser.DesignatorListContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var ret ssa.Values
	for _, d := range ast.AllDesignator() {
		ret = append(ret, b.buildDesignator(d.(*cparser.DesignatorContext)))
	}
	return ret
}

func (b *astbuilder) buildDesignator(ast *cparser.DesignatorContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var value ssa.Value
	if e := ast.Expression(); e != nil {
		value, _ = b.buildExpression(e.(*cparser.ExpressionContext), false)
	} else if id := ast.Identifier(); id != nil {
		value = b.EmitConstInst(id.GetText())
	}
	if utils.IsNil(value) {
		value = b.EmitConstInst(0)
	}

	return value
}

func (b *astbuilder) buildCastExpression(ast *cparser.CastExpressionContext, isLeft bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var right ssa.Value
	var left *ssa.Variable
	if t := ast.TypeName(); t != nil {
		ssatype := b.buildTypeName(t.(*cparser.TypeNameContext))
		if c := ast.CastExpression(); c != nil {
			right, left = b.buildCastExpression(c.(*cparser.CastExpressionContext), isLeft)
		}
		if right != nil && !isLeft && ssatype != nil {
			existingType := right.GetType()
			if utils.IsNil(existingType) || !ssa.TypeEqual(existingType, ssatype) {
				if casted := b.EmitTypeCast(right, ssatype); casted != nil {
					right = casted
				}
			}
		}
	} else if u := ast.UnaryExpression(); u != nil {
		right, left = b.buildUnaryExpression(u.(*cparser.UnaryExpressionContext), isLeft)
	} else if d := ast.DigitSequence(); d != nil {
		// DigitSequence 在 castExpression 中可能用于数字字面量
		// 尝试解析为整数常量
		text := d.GetText()
		if val, err := strconv.ParseInt(text, 10, 64); err == nil {
			right = b.EmitConstInst(val)
		} else {
			// 如果解析失败，作为字符串常量处理
			right = b.EmitConstInst(text)
		}
	}

	return right, left
}

func (b *astbuilder) buildLeftExpression(ast *cparser.LeftExpressionContext, isLeft bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	// log.Infof("exp = %s\n", ast.GetText())

	// 1. '*' unaryExpression - 解引用
	if ast.Star() != nil {
		if u := ast.UnaryExpression(); u != nil {
			right, left := b.buildUnaryExpression(u.(*cparser.UnaryExpressionContext), false)
			if right != nil && right.GetType().GetTypeKind() == ssa.PointerKind {
				if p, ok := ssa.ToParameter(right); ok {
					b.ReferenceParameter(right.GetName(), p.FormalParameterIndex, ssa.PointerSideEffect)
				} else {
					if isLeft {
						left = b.GetAndCreateOriginPointer(right)
					}
					right = b.GetOriginValue(right)
				}
			}
			return right, left
		}
	}

	// 2. postfixExpressionLvalue
	if p := ast.PostfixExpressionLvalue(); p != nil {
		return b.buildPostfixExpressionLvalue(p.(*cparser.PostfixExpressionLvalueContext), isLeft)
	}

	// 3. '(' leftExpression ')'
	if ast.LeftParen() != nil && ast.RightParen() != nil {
		if leftExpr := ast.LeftExpression(); leftExpr != nil {
			return b.buildLeftExpression(leftExpr.(*cparser.LeftExpressionContext), isLeft)
		}
	}

	return b.EmitConstInst(0), b.CreateVariable("")
}

func (b *astbuilder) buildPostfixExpressionLvalue(ast *cparser.PostfixExpressionLvalueContext, isLeft bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var right ssa.Value
	var left *ssa.Variable

	// 1. primaryExpression postfixSuffixLvalue*
	if p := ast.PrimaryExpression(); p != nil {
		right, left = b.buildPrimaryExpression(p.(*cparser.PrimaryExpressionContext), isLeft)
		// 处理 postfixSuffixLvalue*
		for _, suffix := range ast.AllPostfixSuffixLvalue() {
			right, left, _ = b.buildPostfixSuffixLvalue(suffix.(*cparser.PostfixSuffixLvalueContext), right, left, isLeft)
		}
		return right, left
	}

	// 2. '__extension__'? '(' typeName ')' '{' initializerList ','? '}' postfixSuffixLvalue*
	if t := ast.TypeName(); t != nil {
		ssatype := b.buildTypeName(t.(*cparser.TypeNameContext))
		_ = ssatype
		if i := ast.InitializerList(); i != nil {
			right = b.buildInitializerList(i.(*cparser.InitializerListContext))
		}
		// 处理 postfixSuffixLvalue*
		for _, suffix := range ast.AllPostfixSuffixLvalue() {
			right, left, _ = b.buildPostfixSuffixLvalue(suffix.(*cparser.PostfixSuffixLvalueContext), right, left, isLeft)
		}
		return right, left
	}

	for _, suffix := range ast.AllPostfixSuffixLvalue() {
		right, left, _ = b.buildPostfixSuffixLvalue(suffix.(*cparser.PostfixSuffixLvalueContext), right, left, isLeft)
	}

	return right, left
}

// buildPostfixSuffixLvalue 处理左值后缀操作（数组下标、成员访问，不包括函数调用）
func (b *astbuilder) buildPostfixSuffixLvalue(ast *cparser.PostfixSuffixLvalueContext, right ssa.Value, left *ssa.Variable, isLeft bool) (ssa.Value, *ssa.Variable, bool) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	setReference := false
	// log.Infof("postfix = %s\n", ast.GetText())

	if p := ast.PostfixSuffixLvalue(); p != nil {
		right, left, setReference = b.buildPostfixSuffixLvalue(p.(*cparser.PostfixSuffixLvalueContext), right, left, isLeft)
	}

	// 1. 数组下标：'[' expression ']'
	if ast.LeftBracket() != nil && ast.RightBracket() != nil {
		if e := ast.Expression(); e != nil {
			index, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
			RefParameterIndex := -1
			if p, ok := ssa.ToParameter(right); ok && !p.IsFreeValue {
				RefParameterIndex = p.FormalParameterIndex
			} else if p, ok := ssa.ToParameterMember(right); ok {
				RefParameterIndex = p.FormalParameterIndex
			}

			right = b.ReadMemberCallValue(right, index)
			if isLeft && left != nil {
				left = b.CreateMemberCallVariable(b.ReadValue(left.GetName()), index)
			}
			if RefParameterIndex != -1 {
				b.ReferenceParameter(left.GetName(), RefParameterIndex, ssa.PointerSideEffect)
			}
			return right, left, false
		}
	}

	// 2. 结构体成员：'.' Identifier 或 '->' Identifier
	if ast.Dot() != nil || ast.Arrow() != nil {
		isPointer := ast.Arrow() != nil
		if id := ast.Identifier(); id != nil {
			key := id.GetText()
			if right != nil {
				if t := right.GetType(); isPointer && t != nil && t.GetTypeKind() == ssa.PointerKind {
					right = b.GetOriginValue(right)
				}
				right = b.ReadMemberCallValue(right, b.EmitConstInst(key))
			}
			if left != nil {
				member := b.PeekValue(left.GetName())

				checkParameter := func(target string) {
					RefParameterIndex := -1
					if p, ok := ssa.ToParameter(member); ok && !p.IsFreeValue {
						RefParameterIndex = p.FormalParameterIndex
					} else if p, ok := ssa.ToParameterMember(member); ok {
						RefParameterIndex = p.FormalParameterIndex
					}

					if ref, ok := b.RefParameter[member.GetName()]; ok {
						RefParameterIndex = ref.Index
						delete(b.RefParameter, member.GetName())
					}

					if RefParameterIndex != -1 {
						b.ReferenceParameter(target, RefParameterIndex, ssa.PointerSideEffect)
					}
				}

				if t := member.GetType(); isPointer && t != nil && t.GetTypeKind() == ssa.PointerKind {
					member = b.GetOriginValue(member)
					setReference = true
				}

				left = b.CreateMemberCallVariable(member, b.EmitConstInst(key))

				if setReference {
					checkParameter(left.GetName())
					setReference = true
				}
			}
		}
	}

	return right, left, setReference
}

func (b *astbuilder) buildPrimaryExpression(ast *cparser.PrimaryExpressionContext, isLeft bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	// 1. Identifier
	if id := ast.Identifier(); id != nil {
		text := id.GetText()

		left := b.CreateVariable(text)
		right := b.PeekValue(text)
		if right != nil {
			return right, left
		}
		if right, ok := b.getSpecialValue(text); ok {
			return right, left
		}
		if fun, ok := b.GetFunc(text, ""); ok {
			right = fun
		} else {
			b.NewError(ssa.Warn, TAG, fmt.Sprintf("not find variable %s in current scope", text))
			right = b.ReadValue(text)
		}

		return right, left
	}
	// 2. Constant
	if c := ast.Constant(); c != nil {
		text := c.GetText()

		if len(text) > 0 {
			if text[0] == '\'' || (len(text) > 1 && text[0] == 'L' && text[1] == '\'') {
				val := parseCChar(text)
				return b.EmitConstInst(val), nil
			}
			if isCIntLiteral(text) {
				val, _ := parseCInt(text)
				return b.EmitConstInst(val), nil
			}
			if isCFloatLiteral(text) {
				val, _ := parseCFloat(text)
				return b.EmitConstInst(val), nil
			}
		}
	}
	// 3. stringLiteralExpression
	if sle := ast.StringLiteralExpression(); sle != nil {
		stringLitExpr := sle.(*cparser.StringLiteralExpressionContext)
		var sb strings.Builder
		for _, litNode := range stringLitExpr.AllStringLiteral() {
			lit := litNode.GetText()
			if len(lit) >= 2 && lit[0] == '"' && lit[len(lit)-1] == '"' {
				unquoted, err := strconv.Unquote(lit)
				if err == nil {
					sb.WriteString(unquoted)
				} else {
					sb.WriteString(lit[1 : len(lit)-1])
				}
			} else {
				sb.WriteString(lit)
			}
		}
		str := sb.String()
		str = strings.ReplaceAll(str, "%", "%%")
		return b.EmitConstInst(str), nil
	}
	// 4. '(' expression ')'
	if ast.LeftParen() != nil && ast.Expression() != nil && ast.RightParen() != nil {
		return b.buildExpression(ast.Expression().(*cparser.ExpressionContext), isLeft)
	}
	// 5. genericSelection
	if g := ast.GenericSelection(); g != nil {

	}
	// 6. __extension__? '(' compoundStatement ')'
	if ast.Extension() != nil && ast.LeftParen() != nil && ast.CompoundStatement() != nil && ast.RightParen() != nil {
		b.buildCompoundStatement(ast.CompoundStatement().(*cparser.CompoundStatementContext))

	}
	// 7. __builtin_va_arg '(' unaryExpression ',' typeName ')'
	if ast.BuiltinVaArg() != nil {

	}
	// 8. __builtin_offsetof '(' typeName ',' unaryExpression ')'
	if ast.BuiltinOffsetof() != nil {

	}
	// 9. macroCallExpression
	if mce := ast.MacroCallExpression(); mce != nil {
		right := b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
		return right, nil
	}

	return b.EmitConstInst(0), b.CreateVariable("")
}

func (b *astbuilder) buildArgumentExpressionList(ast *cparser.ArgumentExpressionListContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var ret ssa.Values
	// 新语法使用 macroArgument 而不是 expression
	for _, a := range ast.AllMacroArgument() {
		right := b.buildMacroArgument(a.(*cparser.MacroArgumentContext))
		if right != nil {
			ret = append(ret, right)
		}
	}
	return ret
}

// buildMacroArgument 处理宏参数（可以是表达式、类型名、数字序列或操作符）
func (b *astbuilder) buildMacroArgument(ast *cparser.MacroArgumentContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	// 1. expression
	if e := ast.Expression(); e != nil {
		right, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
		return right
	}

	// 2. typeName
	if t := ast.TypeName(); t != nil {
		ssatype := b.buildTypeName(t.(*cparser.TypeNameContext))
		return ssa.NewTypeValue(ssatype)
	}

	// 3. DigitSequence Identifier?
	if d := ast.DigitSequence(); d != nil {
		text := d.GetText()
		if id := ast.Identifier(); id != nil {
			// 处理像 3d_array 这样的标识符
			text = text + id.GetText()
		}
		if val, err := strconv.ParseInt(text, 10, 64); err == nil {
			return b.EmitConstInst(val)
		}
		return b.EmitConstInst(text)
	}

	// 4. 单个操作符
	// 操作符作为宏参数时，通常作为字符串常量处理
	if ast.Less() != nil {
		return b.EmitConstInst("<")
	}
	if ast.Greater() != nil {
		return b.EmitConstInst(">")
	}
	if ast.LessEqual() != nil {
		return b.EmitConstInst("<=")
	}
	if ast.GreaterEqual() != nil {
		return b.EmitConstInst(">=")
	}
	if ast.Equal() != nil {
		return b.EmitConstInst("==")
	}
	if ast.NotEqual() != nil {
		return b.EmitConstInst("!=")
	}
	if ast.Plus() != nil {
		return b.EmitConstInst("+")
	}
	if ast.Minus() != nil {
		return b.EmitConstInst("-")
	}
	if ast.Star() != nil {
		return b.EmitConstInst("*")
	}
	if ast.Div() != nil {
		return b.EmitConstInst("/")
	}
	if ast.Mod() != nil {
		return b.EmitConstInst("%")
	}
	if ast.LeftShift() != nil {
		return b.EmitConstInst("<<")
	}
	if ast.RightShift() != nil {
		return b.EmitConstInst(">>")
	}
	if ast.And() != nil {
		return b.EmitConstInst("&")
	}
	if ast.Or() != nil {
		return b.EmitConstInst("|")
	}
	if ast.Caret() != nil {
		return b.EmitConstInst("^")
	}
	if ast.AndAnd() != nil {
		return b.EmitConstInst("&&")
	}
	if ast.OrOr() != nil {
		return b.EmitConstInst("||")
	}
	if ast.Tilde() != nil {
		return b.EmitConstInst("~")
	}
	if ast.Not() != nil {
		return b.EmitConstInst("!")
	}
	if ast.PlusPlus() != nil {
		return b.EmitConstInst("++")
	}
	if ast.MinusMinus() != nil {
		return b.EmitConstInst("--")
	}

	return b.EmitConstInst(0)
}

func parseCChar(text string) int32 {
	// 简单处理：去除前后引号，处理转义
	if len(text) >= 2 && text[0] == '\'' && text[len(text)-1] == '\'' {
		body := text[1 : len(text)-1]
		if len(body) == 1 {
			return int32(body[0])
		}
		if body[0] == '\\' {
			// 处理转义字符
			switch body[1] {
			case 'n':
				return '\n'
			case 't':
				return '\t'
			case 'r':
				return '\r'
			case '\\':
				return '\\'
			case '\'':
				return '\''
				// 可扩展更多
			}
		}
	}
	return 0
}

func isCIntLiteral(text string) bool {
	// 简单判断：全为数字或0x/0X/0b/0B开头
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") || strings.HasPrefix(text, "0b") || strings.HasPrefix(text, "0B") {
		return true
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

func parseCInt(text string) (int64, error) {
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		return strconv.ParseInt(text[2:], 16, 64)
	}
	if strings.HasPrefix(text, "0b") || strings.HasPrefix(text, "0B") {
		return strconv.ParseInt(text[2:], 2, 64)
	}
	if strings.HasPrefix(text, "0") && len(text) > 1 {
		return strconv.ParseInt(text, 8, 64)
	}
	return strconv.ParseInt(text, 10, 64)
}

func isCFloatLiteral(text string) bool {
	return strings.Contains(text, ".") || strings.ContainsAny(text, "eE")
}

func parseCFloat(text string) (float64, error) {
	return strconv.ParseFloat(text, 64)
}

func coverType(ityp, iwantTyp ssa.Type) {
	typ, ok := ityp.(*ssa.ObjectType)
	if !ok {
		return
	}
	wantTyp, ok := iwantTyp.(*ssa.ObjectType)
	if !ok {
		return
	}

	typ.SetTypeKind(wantTyp.GetTypeKind())
	switch wantTyp.GetTypeKind() {
	case ssa.SliceTypeKind:
		typ.FieldType = wantTyp.FieldType
	case ssa.MapTypeKind:
		typ.FieldType = wantTyp.FieldType
		typ.KeyTyp = wantTyp.KeyTyp
	case ssa.StructTypeKind:
		typ.FieldType = wantTyp.FieldType
		typ.KeyTyp = wantTyp.KeyTyp
		wantTyp.RangeMethod(func(s string, f *ssa.Function) {
			typ.AddMethod(s, f)
		})
	}
	for n, a := range wantTyp.AnonymousField {
		typ.AnonymousField[n] = a
	}
}
