package java2ssa

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

func (y *builder) VisitBlock(raw javaparser.IBlockContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.BlockContext)
	if i == nil {
		return nil
	}

	for _, stmt := range i.AllBlockStatement() {
		y.VisitBlockStatement(stmt)
	}

	return nil
}

func (y *builder) VisitBlockStatement(raw javaparser.IBlockStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.BlockStatementContext)
	if i == nil {
		return nil
	}

	if ret := i.LocalVariableDeclaration(); ret != nil {
		y.VisitLocalVariableDeclaration(ret)
	} else if ret := i.LocalTypeDeclaration(); ret != nil {
		y.VisitLocalTypeDeclaration(ret)
	} else if ret := i.Statement(); ret != nil {
		y.VisitStatement(ret)
	}

	return nil
}

func (y *builder) VisitExpression(raw javaparser.IExpressionContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	var opcode ssa.BinaryOpcode
	var unaryOpcode ssa.UnaryOpcode
	var variable *ssa.Variable
	var value ssa.Value
	var handlerJumpExpression = func(cond func(string) ssa.Value, trueExpr, falseExpr func() ssa.Value) ssa.Value {
		// 为了聚合产生Phi指令
		id := uuid.NewString()
		variable := y.CreateVariable(id)
		y.AssignVariable(variable, y.EmitConstInstAny())
		// 只需要使用b.WriteValue设置value到此ID，并最后调用b.ReadValue可聚合产生Phi指令，完成语句预期行为
		ifb := y.CreateIfBuilder()
		ifb.AppendItem(
			func() ssa.Value {
				return cond(id)
			},
			func() {
				v := trueExpr()
				variable := y.CreateVariable(id)
				y.AssignVariable(variable, v)
			},
		)
		ifb.SetElse(func() {
			v := falseExpr()
			variable := y.CreateVariable(id)
			y.AssignVariable(variable, v)
		})
		ifb.Build()
		// generator phi instruction
		v := y.ReadValue(id)
		v.SetName(raw.GetText())
		return v
	}

	switch ret := raw.(type) {
	case *javaparser.PrimaryExpressionContext:
		// 处理主要表达式
		if ret.Primary() != nil {
			return y.VisitPrimary(ret.Primary())
		}
		return nil
	case *javaparser.SliceCallExpressionContext:
		// 处理切片调用表达式
		expr := y.VisitExpression(ret.Expression(0))
		key := y.VisitExpression(ret.Expression(1))
		if key == nil {
			y.NewError(ssa.Error, "javaast", yak2ssa.AssignRightSideEmpty())
			return nil
		}
		return y.ReadMemberCallVariable(expr, key)
	case *javaparser.MemberCallExpressionContext:
		// 处理成员调用表达式，如通过点操作符访问成员
	case *javaparser.FunctionCallExpressionContext:
		// 处理函数调用表达式
		if s := ret.MethodCall(); s != nil {
			return y.VisitMethodCall(s)
		}
		return nil
	case *javaparser.MethodReferenceExpressionContext:
		// 处理方法引用表达式
	case *javaparser.ConstructorReferenceExpressionContext:
		// 处理构造器引用表达式
	case *javaparser.Java17SwitchExpressionContext:
		// 处理 Java 17 的 switch 表达式
	case *javaparser.PostfixExpressionContext:
		// 处理后缀表达式，如自增、自减操作
		if s := ret.Identifier(); s != nil {
			text := s.GetText()
			variable = y.CreateVariable(text)
		}

		if variable == nil {
			y.NewError(ssa.Error, "javaast", yak2ssa.AssignLeftSideEmpty())
			return nil
		}

		if postfix := ret.GetPostfix().GetText(); postfix == "++" {
			value = y.EmitBinOp(ssa.OpAdd, y.ReadValueByVariable(variable), y.EmitConstInst(1))
		} else if postfix == "--" {
			value = y.EmitBinOp(ssa.OpSub, y.ReadValueByVariable(variable), y.EmitConstInst(1))
		}

		y.AssignVariable(variable, value)
		return value

	case *javaparser.PrefixUnaryExpressionContext:
		// 处理前缀表达式，如正负号、逻辑非等
		if ret.Expression() != nil {
			value = y.VisitExpression(ret.Expression())
		} else {
			y.NewError(ssa.Error, "javaast", yak2ssa.AssignRightSideEmpty())
		}
		switch ret.GetPrefix().GetText() {
		case "+":
			unaryOpcode = ssa.OpPlus
		case "-":
			unaryOpcode = ssa.OpNeg
		case "~":
			unaryOpcode = ssa.OpBitwiseNot
		case "!":
			unaryOpcode = ssa.OpNot
		default:
			y.NewError(ssa.Error, "javaast", yak2ssa.UnaryOperatorNotSupport(ret.GetText()))
		}
		return y.EmitUnOp(unaryOpcode, value)
	case *javaparser.PrefixBinayExpressionContext:
		// 处理前缀表达式中的"--"和"++"
		if s := ret.Identifier(); s != nil {
			text := s.GetText()
			variable = y.CreateVariable(text)
		}
		if variable == nil {
			y.NewError(ssa.Error, "javaast", yak2ssa.AssignLeftSideEmpty())
			return nil
		}

		value = y.ReadValueByVariable(variable)
		if prefix := ret.GetPrefix().GetText(); prefix == "++" {
			y.AssignVariable(variable, y.EmitBinOp(ssa.OpAdd, value, y.EmitConstInst(1)))
		} else if prefix == "--" {
			y.AssignVariable(variable, y.EmitBinOp(ssa.OpSub, value, y.EmitConstInst(1)))
		}
		return value
	case *javaparser.CastExpressionContext:
		// 处理类型转换表达式
	case *javaparser.NewCreatorExpressionContext:
		// 处理创建对象的表达式
	case *javaparser.MultiplicativeExpressionContext:
		// 处理乘法、除法、模运算表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		switch ret.GetBop().GetText() {
		case "*":
			opcode = ssa.OpMul
		case "/":
			opcode = ssa.OpDiv
		case "%":
			opcode = ssa.OpMod
		default:
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.AdditiveExpressionContext:
		// 处理加法和减法表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		switch ret.GetBop().GetText() {
		case "+":
			opcode = ssa.OpAdd
		case "-":
			opcode = ssa.OpSub
		default:
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.ShiftExpressionContext:
		// 处理位移表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		switch ret.GetBop().GetText() {
		case "<<":
			opcode = ssa.OpShl
		case ">>":
			opcode = ssa.OpShr
		case ">>>":
			//todo: 无符号右移运算符
			opcode = ssa.OpShr
		default:
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.RelationalExpressionContext:
		// 处理关系运算表达式，如大于、小于等
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		switch ret.GetBop().GetText() {
		case "<":
			opcode = ssa.OpLt
		case ">":
			opcode = ssa.OpGt
		case "<=":
			opcode = ssa.OpLtEq
		case ">=":
			opcode = ssa.OpGtEq
		default:
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		return y.EmitBinOp(opcode, op1, op2)

	case *javaparser.InstanceofExpressionContext:
		// 处理 instanceof 表达式
	case *javaparser.EqualityExpressionContext:
		// 处理等于和不等于表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		switch ret.GetBop().GetText() {
		case "==":
			opcode = ssa.OpEq
		case "!=":
			opcode = ssa.OpNotEq
		default:
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.BitwiseAndExpressionContext:
		// 处理按位与表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		if bop := ret.GetBop().GetText(); bop == "&" {
			opcode = ssa.OpAnd
		} else {
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.BitwiseXORExpressionContext:
		// 处理按位异或表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		if bop := ret.GetBop().GetText(); bop == "^" {
			opcode = ssa.OpXor
		} else {
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.BitwiseORExpressionContext:
		// 处理按位或表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		if bop := ret.GetBop().GetText(); bop == "|" {
			opcode = ssa.OpOr
		} else {
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.LogicANDExpressionContext:
		// 处理逻辑与表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		return handlerJumpExpression(
			func(id string) ssa.Value {
				return op1
			},
			func() ssa.Value {
				return op2
			},
			func() ssa.Value {
				return op1
			},
		)
	case *javaparser.LogicORExpressionContext:
		// 处理逻辑或表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		return handlerJumpExpression(
			func(id string) ssa.Value {
				return op1
			},
			func() ssa.Value {
				return op1
			},
			func() ssa.Value {
				return op2
			},
		)
	case *javaparser.TernaryExpressionContext:
		// 处理三元运算符表达式
	case *javaparser.AssignmentExpressionContext:
		// 处理赋值表达式，包括所有赋值运算符
		variable = y.CreateVariable(ret.Identifier().GetText())
		if variable == nil {
			y.NewError(ssa.Error, "javaast", yak2ssa.AssignLeftSideEmpty())
			return nil
		}
		v := y.VisitExpression(ret.Expression())
		switch ret.GetBop().GetText() {
		case "+=":
			opcode = ssa.OpAdd
		case "-=":
			opcode = ssa.OpSub
		case "*=":
			opcode = ssa.OpMul
		case "/=":
			opcode = ssa.OpDiv
		case "%=":
			opcode = ssa.OpMod
		case "<<=":
			opcode = ssa.OpShl
		case ">>=":
			opcode = ssa.OpShr
		case ">>>=":
			opcode = ssa.OpShr //todo: 无符号右移运算符
		case "&=":
			opcode = ssa.OpAnd
		case "|=":
			opcode = ssa.OpOr
		case "^=":
			opcode = ssa.OpXor
		default:
			y.NewError(ssa.Error, "javaast", yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return nil
		}
		value := y.EmitBinOp(opcode, y.ReadValueByVariable(variable), v)
		y.AssignVariable(variable, value)
		return value
	case *javaparser.AssignmentEqExpressionContext:
		// 处理赋值表达式的等于号
		leftVariable := y.CreateVariable(ret.Identifier(0).GetText())
		rightVariable := y.CreateVariable(ret.Identifier(1).GetText())
		if leftVariable == nil || rightVariable == nil {
			return nil
		}
		value := y.ReadValueByVariable(rightVariable)
		y.AssignVariable(leftVariable, value)
		return value
	case *javaparser.Java8LambdaExpressionContext:
		// 处理 Java 8 的 lambda 表达式
	default:
		// 默认情况，可能是不支持的表达式类型
		log.Errorf("unsupported expression type: %T", ret)
		panic("unsupported expression type")
	}

	return y.EmitUndefined("_")
}

func (y *builder) VisitMethodCall(raw javaparser.IMethodCallContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.MethodCallContext)
	if i == nil {
		return nil
	}

	var v ssa.Value
	if ret := i.Identifier(); ret != nil {
		v = y.ReadValue(ret.GetText())
	} else if ret := i.THIS(); ret != nil {
		v = y.ReadValue(ret.GetText())
	} else if ret = i.SUPER(); ret != nil {
		v = y.ReadValue(ret.GetText())
	}

	args := y.VisitArguments(i.Arguments())
	c := y.NewCall(v, args)
	return y.EmitCall(c)
}

func (y *builder) VisitPrimary(raw javaparser.IPrimaryContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.PrimaryContext)
	if i == nil {
		return nil
	}

	if ret := i.Literal(); ret != nil {
		return y.VisitLiteral(ret)
	}

	if ret := i.Identifier(); ret != nil {
		text := ret.GetText()
		if text == "_" {
			y.NewError(ssa.Warn, "javaast", "cannot use _ as value")
			return nil
		}
		v := y.ReadValue(text)
		return v
	}
	return nil
}

func (y *builder) VisitStatement(raw javaparser.IStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.StatementContext)
	if i == nil {
		return nil
	}

	if ret := i.Expression(0); ret != nil {
		log.Infof("visit expression: %v", ret.GetText())
		y.VisitExpression(ret)
	}

	return nil
}

func (y *builder) VisitLocalTypeDeclaration(raw javaparser.ILocalTypeDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.LocalTypeDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitLocalVariableDeclaration(raw javaparser.ILocalVariableDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.LocalVariableDeclarationContext)
	if i == nil {
		return nil
	}

	if ret := i.Identifier(); ret != nil {
		log.Infof("visit local variable declaration: %v", ret.GetText())
		variable := y.CreateLocalVariable(ret.GetText())
		value := y.VisitExpression(i.Expression())
		y.AssignVariable(variable, value)
		return nil
	} else if ret := i.VariableDeclarators(); ret != nil {
		decls := ret.(*javaparser.VariableDeclaratorsContext)
		for _, decl := range decls.AllVariableDeclarator() {
			y.VisitVariableDeclarator(decl)
		}
	}

	return nil
}

func (y *builder) VisitVariableDeclarator(raw javaparser.IVariableDeclaratorContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.VariableDeclaratorContext)
	if i == nil {
		return nil
	}

	return nil

}

func (y *builder) VisitArguments(raw javaparser.IArgumentsContext) []ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.ArgumentsContext)
	if i == nil {
		return nil
	}

	var args []ssa.Value
	if ret := i.ExpressionList(); ret != nil {
		exprs := ret.(*javaparser.ExpressionListContext)
		for _, expr := range exprs.AllExpression() {
			args = append(args, y.VisitExpression(expr))
		}
	}
	return args
}
