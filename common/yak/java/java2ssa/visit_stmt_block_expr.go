package java2ssa

import (
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

func (y *builder) VisitExpression(raw javaparser.IExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	switch ret := raw.(type) {
	case *javaparser.PrimaryExpressionContext:
		// 处理主要表达式
		return y.VisitPrimary(ret.Primary())
	case *javaparser.SliceCallExpressionContext:
		// 处理切片调用表达式
	case *javaparser.MemberCallExpressionContext:
		// 处理成员调用表达式，如通过点操作符访问成员
	case *javaparser.FunctionCallExpressionContext:
		// 处理函数调用表达式
	case *javaparser.MethodReferenceExpressionContext:
		// 处理方法引用表达式
	case *javaparser.ConstructorReferenceExpressionContext:
		// 处理构造器引用表达式
	case *javaparser.Java17SwitchExpressionContext:
		// 处理 Java 17 的 switch 表达式
	case *javaparser.PostfixExpressionContext:
		// 处理后缀表达式，如自增、自减操作
		if ret.INC() != nil { //++
			variable := y.VisitExpression(ret.Expression()).(*ssa.Variable)
			if variable == nil {
				y.NewError(ssa.Error, "javaast", yak2ssa.AssignLeftSideEmpty())
				return nil
			}
			value := y.EmitBinOp(ssa.OpAdd, y.ReadValueByVariable(variable), y.EmitConstInst(1))
			y.AssignVariable(variable, value)
			return variable
		}
		if ret.DEC() != nil { //--
			variable := y.VisitExpression(ret.Expression()).(*ssa.Variable)
			if variable == nil {
				y.NewError(ssa.Error, "javaast", yak2ssa.AssignLeftSideEmpty())
				return nil
			}
			value := y.EmitBinOp(ssa.OpSub, y.ReadValueByVariable(variable), y.EmitConstInst(1))
			y.AssignVariable(variable, value)
			return variable
		}
	case *javaparser.PrefixExpressionContext:
		// 处理前缀表达式，如正负号、逻辑非等
	case *javaparser.CastExpressionContext:
		// 处理类型转换表达式
	case *javaparser.NewCreatorExpressionContext:
		// 处理创建对象的表达式
	case *javaparser.MultiplicativeExpressionContext:
		// 处理乘法、除法、模运算表达式
	case *javaparser.AdditiveExpressionContext:
		// 处理加法和减法表达式
	case *javaparser.ShiftExpressionContext:
		// 处理位移表达式
	case *javaparser.RelationalExpressionContext:
		// 处理关系运算表达式，如大于、小于等
	case *javaparser.InstanceofExpressionContext:
		// 处理 instanceof 表达式
	case *javaparser.EqualityExpressionContext:
		// 处理等于和不等于表达式
	case *javaparser.BitwiseAndExpressionContext:
		// 处理按位与表达式
	case *javaparser.BitwiseXORExpressionContext:
		// 处理按位异或表达式
	case *javaparser.BitwiseORExpressionContext:
		// 处理按位或表达式
	case *javaparser.LogicANDExpressionContext:
		// 处理逻辑与表达式
	case *javaparser.LogicORExpressionContext:
		// 处理逻辑或表达式
	case *javaparser.TernaryExpressionContext:
		// 处理三元运算符表达式
	case *javaparser.AssignmentExpressionContext:
		// 处理赋值表达式，包括所有赋值运算符
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

	return nil
}

func (y *builder) VisitPrimary(raw javaparser.IPrimaryContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.PrimaryContext)
	if i == nil {
		return nil
	}
	if ret := i.Literal(); ret != nil {
		literal := ret.(*javaparser.LiteralContext)
		return literal.GetText()
	}
	if ret := i.Identifier(); ret != nil {
		text := ret.GetText()
		return y.CreateVariable(text)
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

	id := i.Identifier()
	if id != nil {
		varName := id.GetText()
		variable := y.CreateLocalVariable(varName)
		exprResult := y.VisitExpression(i.Expression())
		value := y.EmitConstInst(exprResult)
		y.AssignVariable(variable, value)
		return nil
	}
	return nil
}
