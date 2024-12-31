package java2ssa

import (
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

type JavaSwitchLabel string

const (
	CASE    JavaSwitchLabel = "case"
	DEFAULT                 = "default"
)

func (y *builder) VisitBlock(raw javaparser.IBlockContext, syntaxBlocks ...bool) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.BlockContext)
	if i == nil {
		return nil
	}
	syntaxBlock := false
	if len(syntaxBlocks) > 0 {
		syntaxBlock = syntaxBlocks[0]
	}
	if syntaxBlock {
		if ret := i.BlockStatementList(); ret != nil {
			y.BuildSyntaxBlock(func() {
				y.VisitBlockStatementList(ret)
			})
		}
	} else {
		if ret := i.BlockStatementList(); ret != nil {
			y.VisitBlockStatementList(ret)
		}
	}

	return nil
}

func (y *builder) VisitBlockStatement(raw javaparser.IBlockStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
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
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	var opcode ssa.BinaryOpcode
	var unaryOpcode ssa.UnaryOpcode
	var handlerJumpExpression = func(cond func(string) ssa.Value, trueExpr, falseExpr func() ssa.Value) ssa.Value {
		// 为了聚合产生Phi指令
		id := uuid.NewString()
		variable := y.CreateVariable(id)
		y.AssignVariable(variable, y.EmitValueOnlyDeclare(id))
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
	case *javaparser.SliceCallExpressionContext:
		// 处理切片调用表达式
		expr := y.VisitExpression(ret.Expression(0))
		key := y.VisitExpression(ret.Expression(1))
		if utils.IsNil(expr) {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), "slice call expression left side is empty")
			return y.EmitUndefined(raw.GetText())
		}
		if utils.IsNil(key) {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignRightSideEmpty())
			return y.EmitUndefined(raw.GetText())
		}
		return y.ReadMemberCallValue(expr, key)
	case *javaparser.MemberCallExpressionContext:
		// 处理成员调用表达式，如通过点操作符访问成员
		obj := y.VisitExpression(ret.Expression())
		if utils.IsNil(obj) {
			return y.EmitUndefined(raw.GetText())
		}
		var key ssa.Value
		var res ssa.Value
		if id := ret.Identifier(); id != nil {
			key = y.EmitConstInst(id.GetText())
		} else if method := ret.MethodCall(); method != nil {
			res = y.VisitMethodCall(method, obj)
		} else if this := ret.THIS(); this != nil {
			key = y.EmitConstInst(this.GetText())
		} else if super := ret.SUPER(); super != nil {
			// todo: 访问父类成员
			key = y.EmitConstInst(super.GetText())
		} else if creator := ret.InnerCreator(); creator != nil {
			if ret.NonWildcardTypeArguments() != nil {
				// todo:泛型
			}
			res = y.VisitInnerCreator(ret.InnerCreator(), ret.Expression().GetText())
		} else if explicit := ret.ExplicitGenericInvocation(); explicit != nil {
			//todo : 显式泛型调用
			key = y.EmitConstInst(explicit.GetText())
		}
		if utils.IsNil(res) {
			res = y.ReadMemberCallValue(obj, key)
			if utils.IsNil(res) {
				return y.EmitUndefined(raw.GetText())
			}
		}
		resTyp := res.GetType()
		if resTyp != nil && len(resTyp.GetFullTypeNames()) != 0 {
			return res
		}

		t := obj.GetType()
		if ftName := t.GetFullTypeNames(); len(ftName) != 0 {
			newTyp := y.MergeFullTypeNameForType(ftName, res.GetType())
			res.SetType(newTyp)
		}
		return res
	case *javaparser.FunctionCallExpressionContext:
		// 处理函数调用表达式
		if s := ret.MethodCall(); s != nil {
			return y.VisitMethodCall(s, nil)
		}
		return y.EmitUndefined(raw.GetText())
	case *javaparser.MethodReferenceExpressionContext:
		// 处理方法引用表达式
		// todo: 方法引用表达式
		return y.EmitUndefined(raw.GetText())
	case *javaparser.ConstructorReferenceExpressionContext:
		// 处理构造器引用表达式
		// todo: 构造器引用表达式
		return y.EmitUndefined(raw.GetText())
	case *javaparser.Java17SwitchExpressionContext:
		// 处理 Java 17 的 switch 表达式
		value := y.VisitSwitchExpression(ret.SwitchExpression(), true)
		return value
	case *javaparser.PostfixExpression1Context:
		// 处理后缀表达式，如自增、自减操作
		var variable *ssa.Variable
		var value ssa.Value

		obj := y.VisitExpression(ret.Expression())
		if lsc := ret.LeftSliceCall(); lsc != nil {
			variable = y.VisitLeftSliceCall(lsc, obj)
		} else if lmc := ret.LeftMemberCall(); lmc != nil {
			variable = y.VisitLeftMemberCall(lmc, obj)
		}

		if variable == nil {
			//log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignLeftSideEmpty())
			return y.EmitUndefined(raw.GetText())
		}
		if postfix := ret.GetPostfix().GetText(); postfix == "++" {
			value = y.EmitBinOp(ssa.OpAdd, y.ReadValueByVariable(variable), y.EmitConstInst(1))
		} else if postfix == "--" {
			value = y.EmitBinOp(ssa.OpSub, y.ReadValueByVariable(variable), y.EmitConstInst(1))
		}
		y.AssignVariable(variable, value)
		return value

	case *javaparser.PostfixExpression2Context:
		var variable *ssa.Variable
		var value ssa.Value
		if s := ret.Identifier(); s != nil {
			text := s.GetText()
			variable = y.CreateVariable(text)
		}
		if variable == nil {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignLeftSideEmpty())
			return y.EmitUndefined(raw.GetText())
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
		var value ssa.Value
		if ret.Expression() != nil {
			value = y.VisitExpression(ret.Expression())
		} else {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignRightSideEmpty())
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignRightSideEmpty())
			// return nil
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
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.UnaryOperatorNotSupport(ret.GetText()))
		}
		return y.EmitUnOp(unaryOpcode, value)
	case *javaparser.PrefixBinayExpression1Context:
		// 处理前缀表达式中的"--"和"++"
		var variable *ssa.Variable
		var value ssa.Value
		obj := y.VisitExpression(ret.Expression())
		if lsc := ret.LeftSliceCall(); lsc != nil {
			variable = y.VisitLeftSliceCall(lsc, obj)
		} else if lmc := ret.LeftMemberCall(); lmc != nil {
			variable = y.VisitLeftMemberCall(lmc, obj)
		}
		if variable == nil {
			//log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignLeftSideEmpty())
			return y.EmitUndefined(raw.GetText())
		}

		value = y.ReadValueByVariable(variable)
		if prefix := ret.GetPrefix().GetText(); prefix == "++" {
			y.AssignVariable(variable, y.EmitBinOp(ssa.OpAdd, value, y.EmitConstInst(1)))
		} else if prefix == "--" {
			y.AssignVariable(variable, y.EmitBinOp(ssa.OpSub, value, y.EmitConstInst(1)))
		}
		return value

	case *javaparser.PrefixBinayExpression2Context:
		var variable *ssa.Variable
		var value ssa.Value
		if s := ret.Identifier(); s != nil {
			text := s.GetText()
			variable = y.CreateVariable(text)
		}
		if variable == nil {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignLeftSideEmpty())
			return y.EmitUndefined(raw.GetText())
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
		var castType ssa.Type
		if len(ret.AllBITAND()) == 0 {
			castType = y.VisitTypeType(ret.TypeType(0))
			castType = y.SetCastTypeFlag(castType)
		} else {
			// TODO:处理类型交集语句
		}

		v := y.VisitExpression(ret.Expression())
		if utils.IsNil(v) {
			return y.EmitUndefined(raw.GetText())
		}
		if castType != nil {
			v.SetType(castType)
		}
		return v
	case *javaparser.NewCreatorExpressionContext:
		// 处理创建对象的表达式
		obj, call := y.VisitCreator(ret.Creator())
		if call != nil {
			return call
		}
		return obj
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
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
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
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.ShiftExpressionContext:
		// 处理位移表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		ltNum := len(ret.AllLT())
		rtNum := len(ret.AllGT())
		if ltNum != 0 && rtNum != 0 {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
		}
		if ltNum == 2 {
			opcode = ssa.OpShl
		} else if rtNum == 2 || rtNum == 3 {
			opcode = ssa.OpShr
		} else {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
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
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
		}
		return y.EmitBinOp(opcode, op1, op2)

	case *javaparser.InstanceofExpressionContext:
		// 处理 instanceof 表达式
		// todo instanceof 表达式
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
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.BitwiseAndExpressionContext:
		// 处理按位与表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		if bop := ret.GetBop().GetText(); bop == "&" {
			opcode = ssa.OpAnd
		} else {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.BitwiseXORExpressionContext:
		// 处理按位异或表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		if bop := ret.GetBop().GetText(); bop == "^" {
			opcode = ssa.OpXor
		} else {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
		}
		return y.EmitBinOp(opcode, op1, op2)
	case *javaparser.BitwiseORExpressionContext:
		// 处理按位或表达式
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		if bop := ret.GetBop().GetText(); bop == "|" {
			opcode = ssa.OpOr
		} else {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
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
		builder := y.CreateIfBuilder()
		allExpr := ret.AllExpression()
		if allExpr != nil {
			builder.AppendItem(func() ssa.Value {
				return y.VisitExpression(ret.Expression(0))
			},
				func() { y.VisitExpression(ret.Expression(1)) })
			builder.SetElse(func() { y.VisitExpression(ret.Expression(2)) })
			builder.Build()
		}
		if y.VisitExpression(ret.Expression(0)) == y.EmitConstInst(true) {
			return y.VisitExpression(ret.Expression(1))
		} else {
			return y.VisitExpression(ret.Expression(2))
		}
	case *javaparser.AssignmentExpression1Context:
		// 处理赋值表达式，包括所有赋值运算符
		var variable *ssa.Variable
		var value ssa.Value
		obj := y.VisitExpression(ret.Expression(0))
		if lsc := ret.LeftSliceCall(); lsc != nil {
			variable = y.VisitLeftSliceCall(lsc, obj)
		} else if lmc := ret.LeftMemberCall(); lmc != nil {
			variable = y.VisitLeftMemberCall(lmc, obj)
		}
		if variable == nil {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignLeftSideEmpty())
			return y.EmitUndefined(raw.GetText())
		}
		v := y.VisitExpression(ret.Expression(1))
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
			opcode = ssa.OpShr
		case "&=":
			opcode = ssa.OpAnd
		case "|=":
			opcode = ssa.OpOr
		case "^=":
			opcode = ssa.OpXor
		default:
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
		}
		value = y.EmitBinOp(opcode, y.ReadValueByVariable(variable), v)
		y.AssignVariable(variable, value)
		return value

	case *javaparser.AssignmentExpression2Context:
		// 处理赋值表达式，包括所有赋值运算符
		var variable *ssa.Variable
		var value ssa.Value
		variable = y.CreateVariable(ret.Identifier().GetText())
		if variable == nil {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.AssignLeftSideEmpty())
			return y.EmitUndefined(raw.GetText())
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
			opcode = ssa.OpShr
		case "&=":
			opcode = ssa.OpAnd
		case "|=":
			opcode = ssa.OpOr
		case "^=":
			opcode = ssa.OpXor
		default:
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.BinaryOperatorNotSupport(ret.GetText()))
			return y.EmitUndefined(raw.GetText())
		}
		value = y.EmitBinOp(opcode, y.ReadValueByVariable(variable), v)
		y.AssignVariable(variable, value)
		return value

	case *javaparser.AssignmentEqExpression1Context:
		// 处理赋值表达式的等于号
		var variable *ssa.Variable
		var value ssa.Value
		obj := y.VisitExpression(ret.Expression(0))
		if lsc := ret.LeftSliceCall(); lsc != nil {
			variable = y.VisitLeftSliceCall(lsc, obj)
		} else if lmc := ret.LeftMemberCall(); lmc != nil {
			variable = y.VisitLeftMemberCall(lmc, obj)
		}
		if variable == nil {
			return y.EmitUndefined(raw.GetText())
		}

		if id := ret.Identifier(); id != nil {
			value = y.ReadValue(id.GetText())
		} else if expr := ret.Expression(1); expr != nil {
			value = y.VisitExpression(expr)
		}
		y.AssignVariable(variable, value)
		return value
	case *javaparser.AssignmentEqExpression2Context:
		// 处理赋值表达式的等于号
		var variable *ssa.Variable
		var value ssa.Value
		s := ret.Identifier(0)
		if s != nil {
			variable, _ = y.VisitIdentifier(s, true)
		}
		if id := ret.Identifier(1); id != nil {
			_, value = y.VisitIdentifier(id)
		} else if expr := ret.Expression(); expr != nil {
			value = y.VisitExpression(expr)
		}
		y.AssignVariable(variable, value)
		return value
	case *javaparser.Java8LambdaExpressionContext:
		// 处理 Java 8 的 lambda 表达式
		return y.VisitLambdaExpression(ret.LambdaExpression())
	default:
		// 默认情况，可能是不支持的表达式类型
		log.Errorf("unsupported expression type: %T", ret)
		panic("unsupported expression type")
	}
	return y.EmitConstInstNil()
}

func (y *builder) VisitMethodCall(raw javaparser.IMethodCallContext, object ssa.Value) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.MethodCallContext)
	if i == nil {
		return nil
	}

	if utils.IsNil(object) {
		var v ssa.Value
		if ret := i.Identifier(); ret != nil {
			_, v = y.VisitIdentifier(ret)
		} else if ret := i.THIS(); ret != nil {
			v = y.ReadValue(ret.GetText())
		} else if ret = i.SUPER(); ret != nil {
			v = y.ReadValue(ret.GetText())
		}

		var args []ssa.Value
		if argument := i.Arguments(); argument != nil {
			args = y.VisitArguments(i.Arguments())
			c := y.NewCall(v, args)
			return y.EmitCall(c)
		}
	} else {
		var memberKey ssa.Value
		if ret := i.Identifier(); ret != nil {
			memberKey = y.EmitConstInst(ret.GetText())
		} else if ret := i.THIS(); ret != nil {
			memberKey = y.EmitConstInst(ret.GetText())
			// get clazz
		} else if ret = i.SUPER(); ret != nil {
			memberKey = y.EmitConstInst(ret.GetText())
			// get parent class
		}

		methodCall := y.ReadMemberCallMethod(object, memberKey)
		var args []ssa.Value
		if argument := i.Arguments(); argument != nil {
			args = y.VisitArguments(i.Arguments())
			c := y.EmitCall(y.NewCall(methodCall, args))
			y.HookMemberCallMethod(object, memberKey, args...)
			return c
		}
	}

	return y.EmitUndefined(raw.GetText())

}

func (y *builder) VisitPrimary(raw javaparser.IPrimaryContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.PrimaryContext)
	if i == nil {
		return nil
	}

	if ret := i.Expression(); ret != nil {
		return y.VisitExpression(ret)
	}

	if ret := i.THIS(); ret != nil {
		text := ret.GetText()
		if value := y.PeekValue(text); value != nil {
			return value
		}
		return y.EmitConstInst(text)
	}

	if ret := i.SUPER(); ret != nil {
		text := ret.GetText()
		parent := y.PeekValue(text)
		if parent == nil {
			parent = y.EmitConstInst(text)
		}
		cls := y.MarkedThisClassBlueprint.GetSuperClass()
		if parent != nil {
			parent.SetType(cls)
		}
		return parent
	}

	if ret := i.Literal(); ret != nil {
		return y.VisitLiteral(ret)
	}

	if ret := i.Identifier(); ret != nil {
		_, v := y.VisitIdentifier(ret)
		return v
	}

	if ret := i.TypeTypeOrVoid(); ret != nil {
		typ := y.VisitTypeTypeOrVoid(ret)
		// TODO:  if not found class, not return any, create undefine class
		return y.EmitTypeValue(typ)
	}
	return y.EmitUndefined(raw.GetText())
}

func (y *builder) VisitBlockOrState(raw javaparser.IBlockOrStateContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.BlockOrStateContext)
	if i == nil {
		return
	}
	if ret := i.Block(); ret != nil {
		y.VisitBlock(ret)
	} else if ret := i.Statement(); ret != nil {
		y.VisitStatement(ret)
	}
}

func (y *builder) VisitStatement(raw javaparser.IStatementContext) interface{} {
	if y.IsBlockFinish() {
		return nil
	}
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}

	recoverRange := y.SetRange(raw)
	defer recoverRange()
	y.AppendBlockRange()

	switch ret := raw.(type) {
	case *javaparser.BlockLabelStatementContext:
		return y.VisitBlock(ret.Block())
	case *javaparser.AssertStatementContext:
		// 处理断言语句
		getExpr := func(i int) ssa.Value {
			if expr := ret.Expression(i); expr != nil {
				return y.VisitExpression(expr)
			}
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.UnexpectedAssertStmt())
			return y.EmitUndefined(raw.GetText())
		}
		exprs := ret.AllExpression()
		lenExprs := len(exprs)

		var cond, msgV ssa.Value

		cond = getExpr(0)
		if lenExprs > 1 {
			msgV = getExpr(1)
		}
		return y.EmitAssert(cond, msgV, exprs[0].GetText())
	case *javaparser.IfStatementContext:
		// 处理 if 语句
		y.VisitIfStmt(ret.Ifstmt())
		return nil
	case *javaparser.ForStatementContext:
		//处理 for 语句
		if ret.ForControl() != nil {
			loop := y.VisitForControl(ret.ForControl())
			//设置循环体
			loop.SetBody(func() {
				if state := ret.BlockOrState(); state != nil {
					y.VisitBlockOrState(state)
				}
			})
			loop.Finish()
		}
		return nil
	case *javaparser.WhileStatementContext:
		// 处理 while 语句
		loop := y.CreateLoopBuilder()
		parExpr := ret.ParExpression().(*javaparser.ParExpressionContext)

		if parExpr != nil {
			expr := parExpr.Expression()
			loop.SetCondition(func() ssa.Value {
				condition := y.VisitExpression(expr)
				if condition == nil {
					condition = y.EmitConstInst(true)
				} else {
					condition = y.VisitExpression(expr)
				}
				return condition
			})
		}
		loop.SetBody(func() {
			if state := ret.BlockOrState(); state != nil {
				y.VisitBlockOrState(state)
			}
		})
		loop.Finish()
		return nil
	case *javaparser.DoWhileStatementContext:
		// 处理 do while 语句
		loop := y.CreateLoopBuilder()

		loop.SetCondition(
			func() ssa.Value {
				return y.EmitConstInst(true)
			})
		parExprs := ret.ParExpressionList().(*javaparser.ParExpressionListContext)
		if parExprs != nil {
			exprs := parExprs.ExpressionList()
			loop.SetThird(func() []ssa.Value {
				return y.VisitExpressionList(exprs)
			})
		}

		loop.SetBody(func() {
			if block := ret.Block(); block != nil {
				y.VisitBlock(block)
			}
		})

		loop.Finish()
		return nil
	case *javaparser.TryStatementContext:
		// 处理 try 语句
		if ret.TRY() != nil {
			tryBuilder := y.BuildTry()

			tryBuilder.BuildTryBlock(func() {
				if ret := ret.Block(); ret != nil {
					y.VisitBlock(ret)
				}
			})
			for _, catch := range ret.AllCatchClause() {
				catchClause := catch.(*javaparser.CatchClauseContext)
				tryBuilder.BuildErrorCatch(func() string {
					return catchClause.Identifier().GetText()
				}, func() {
					if block := catchClause.Block(); block != nil {
						y.VisitBlock(block)
					}
				})
			}
			if finallyBlock := ret.FinallyBlock(); finallyBlock != nil {
				tryBuilder.BuildFinally(func() {
					y.VisitBlock(finallyBlock.(*javaparser.FinallyBlockContext).Block())
				})
			}
			tryBuilder.Finish()
		}
		return nil
	case *javaparser.TryWithResourcesStatementContext:
		// 处理 try with resources 语句
		if ret.TRY() != nil {
			tryBuilder := y.BuildTry()
			var shouldClosedValue []ssa.Value
			tryBuilder.BuildTryBlock(func() {
				if r := ret.ResourceSpecification(); r != nil {
					shouldClosedValue = y.VisitResourceSpecification(r)
				}
				if b := ret.Block(); ret != nil {
					y.VisitBlock(b)
				}
			})
			for _, catch := range ret.AllCatchClause() {
				catchClause := catch.(*javaparser.CatchClauseContext)
				tryBuilder.BuildErrorCatch(func() string {
					return catchClause.Identifier().GetText()
				}, func() {
					if block := catchClause.Block(); block != nil {
						y.VisitBlock(block)
					}
				})
			}
			if finallyBlock := ret.FinallyBlock(); finallyBlock != nil {
				tryBuilder.BuildFinally(func() {
					y.VisitBlock(finallyBlock.(*javaparser.FinallyBlockContext).Block())
					key := y.EmitConstInst("close")
					if shouldClosedValue != nil {
						for _, value := range shouldClosedValue {
							y.ReadMemberCallValue(value, key)
						}
					}
				})
			} else {
				tryBuilder.BuildFinally(func() {
					key := y.EmitConstInst("close")
					if shouldClosedValue != nil {
						for _, value := range shouldClosedValue {
							y.ReadMemberCallMethod(value, key)
						}
					}
				})
			}
			tryBuilder.Finish()
		}

		return nil
	case *javaparser.PureSwitchStatementContext:
		y.VisitSwitchStatement(ret.SwitchStatement())
	case *javaparser.SynchronizedStatementContext:
		// 处理 synchronized 语句
		return nil
	case *javaparser.ReturnStatementContext:
		// 处理 return 语句
		if ret.Expression() != nil {
			value := y.VisitExpression(ret.Expression())
			if utils.IsNil(value) {
				return nil
			}
			if funcTyp := y.GetCurrentReturnType(); funcTyp != nil {
				value.SetType(funcTyp)
			}
			y.HookReturn(value)
			y.EmitReturn([]ssa.Value{value})
		} else {
			y.EmitReturn(nil)
		}
		return nil
	case *javaparser.ThrowStatementContext:
		// 处理 throw 语句
		return nil
	case *javaparser.BreakStatementContext:
		// 处理 break 语句
		// todo break使用标签
		if !y.Break() {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.UnexpectedBreakStmt())
		}
		return nil
	case *javaparser.ContinueStatementContext:
		// 处理 continue 语句
		// todo continue使用标签
		if !y.Continue() {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), yak2ssa.UnexpectedContinueStmt())
		}
		return nil
	case *javaparser.YieldStatementContext:
		// 处理 yield 语句
		return y.VisitExpression(ret.Expression())
	case *javaparser.ExpressionStatementContext:
		// 处理表达式语句
		return y.VisitExpression(ret.Expression())
	case *javaparser.SwitchArrowExpressionContext:
		// 处理 switch 箭头语句
		_ = y.VisitSwitchExpression(ret.SwitchExpression(), false)
		return nil
	case *javaparser.IdentifierLabelStatementContext:
		// 处理标识符标签语句
		return nil
	default:
		return nil
	}
	return nil
}

func (y *builder) VisitLocalTypeDeclaration(raw javaparser.ILocalTypeDeclarationContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.LocalTypeDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitLocalVariableDeclaration(raw javaparser.ILocalVariableDeclarationContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.LocalVariableDeclarationContext)
	if i == nil {
		return
	}

	if ret := i.Identifier(); ret != nil {
		//log.Infof("visit local variable declaration: %v", ret.GetText())
		variable := y.CreateLocalVariable(ret.GetText())
		value := y.VisitExpression(i.Expression())
		y.AssignVariable(variable, value)
	} else if ret := i.VariableDeclarators(); ret != nil {
		var typ ssa.Type
		if i.TypeType() != nil {
			typ = y.VisitTypeType(i.TypeType())
		}
		//log.Infof("visit local variable declaration: %v,type:%v", ret.GetText(), typName)
		decls := ret.(*javaparser.VariableDeclaratorsContext)
		for _, decl := range decls.AllVariableDeclarator() {
			y.VisitVariableDeclarator(decl, typ)
		}
	}
	return
}

func (y *builder) OnlyVisitVariableDeclaratorName(raw javaparser.IVariableDeclaratorContext) string {
	name := uuid.NewString()[:4]
	if y == nil || raw == nil || y.IsStop() {
		return name
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.VariableDeclaratorContext)
	if i == nil {
		return name
	}

	id, ok := i.VariableDeclaratorId().(*javaparser.VariableDeclaratorIdContext)
	if !ok {
		return name
	}
	name = id.Identifier().GetText()
	return name
}
func (y *builder) VisitVariableDeclarator(raw javaparser.IVariableDeclaratorContext, typ ssa.Type) (name string, value ssa.Value) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.VariableDeclaratorContext)
	if i == nil {
		return
	}

	var variable *ssa.Variable
	if ret := i.VariableDeclaratorId(); ret != nil {
		name, variable = y.VisitVariableDeclaratorId(ret, true)
	}
	if variable == nil {
		return
	}

	if i.VariableInitializer() != nil {
		value := y.VisitVariableInitializer(i.VariableInitializer())
		if utils.IsNil(value) {
			return name, nil
		} else {
			rightValTyp := value.GetType()
			rightValTypName := rightValTyp.GetFullTypeNames()
			// 如果有类型转换，就用转换后的typeName
			if len(rightValTypName) != 0 && y.HaveCastType(rightValTyp) {
				newTyp := y.RemoveCastTypeFlag(rightValTyp)
				value.SetType(newTyp)
			} else {
				// 没有类型转换，就使用在右值的typeName加上typeType的typeName
				if typ != nil {
					newTyp := y.MergeFullTypeNameForType(typ.GetFullTypeNames(), rightValTyp)
					value.SetType(newTyp)
				}
			}
		}
		y.AssignVariable(variable, value)
		return name, value
	} else {
		value := y.EmitValueOnlyDeclare(name)
		y.AssignVariable(variable, value)
		return name, value
	}
}

func (y *builder) VisitVariableInitializer(raw javaparser.IVariableInitializerContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.VariableInitializerContext)
	if i == nil {
		return nil
	}

	if ret := i.Expression(); ret != nil {
		return y.VisitExpression(ret)
	} else if ret := i.ArrayInitializer(); ret != nil {
		return y.VisitArrayInitializer(ret)
	}
	return nil
}

func (y *builder) VisitArguments(raw javaparser.IArgumentsContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.ArgumentsContext)
	if i == nil {
		return nil
	}

	var args []ssa.Value
	if ret := i.ExpressionList(); ret != nil {
		args = y.VisitExpressionList(ret)
	}
	return args
}

func (y *builder) VisitExpressionList(raw javaparser.IExpressionListContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.ExpressionListContext)
	if i == nil {
		return nil
	}
	values := make([]ssa.Value, 0, len(i.AllExpression()))
	for _, expr := range i.AllExpression() {
		if v := y.VisitExpression(expr); !utils.IsNil(v) {
			values = append(values, v)
		}
	}
	return values
}

func (y *builder) VisitStatementList(raw javaparser.IStatementListContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.StatementListContext)
	if i == nil {
		return
	}

	for _, stmt := range i.AllStatement() {
		y.VisitStatement(stmt)
	}
}

func (y *builder) VisitForControl(raw javaparser.IForControlContext) *ssa.LoopBuilder {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.ForControlContext)
	if i == nil {
		return nil
	}

	var cond javaparser.IExpressionContext
	loop := y.CreateLoopBuilder()
	if i.EnhancedForControl() != nil {
		//处理增强for循环形式(for each)
		enhanced := i.EnhancedForControl().(*javaparser.EnhancedForControlContext)
		var value ssa.Value
		loop.SetFirst(func() []ssa.Value {
			value = y.VisitExpression(enhanced.Expression())
			return []ssa.Value{value}
		})

		loop.SetCondition(func() ssa.Value {
			var variable *ssa.Variable
			if decl := enhanced.VariableDeclaratorId(); decl != nil {
				text := decl.(*javaparser.VariableDeclaratorIdContext).Identifier().GetText()
				variable = y.CreateVariable(text)
			}
			_, field, ok := y.EmitNext(value, false)
			y.AssignVariable(variable, field)
			return ok
		})
		return loop
	} else {
		// 处理标准for循环形式
		// 设置第一个参数
		if first := i.ForInit(); first != nil {
			loop.SetFirst(func() []ssa.Value { return y.VisitForInit(first) })
		}
		// 设置第二个参数
		if expr := i.Expression(); expr != nil {
			cond = expr
		}
		// 设置第三个参数
		if third := i.GetForUpdate(); third != nil {
			loop.SetThird(func() []ssa.Value { return y.VisitExpressionList(third) })
		}
	}
	// 设置循环条件
	loop.SetCondition(func() ssa.Value {
		var condition ssa.Value
		if cond == nil {
			condition = y.EmitConstInst(true)
		} else {
			condition = y.VisitExpression(cond)
		}
		return condition
	})
	return loop
}

func (y *builder) VisitForInit(raw javaparser.IForInitContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.ForInitContext)
	if i == nil {
		return nil
	}
	// for循环first为局部变量声明
	// for(int a=1;;){}
	var value []ssa.Value
	if ret := i.LocalVariableDeclaration(); ret != nil {
		y.VisitLocalVariableDeclaration(ret)
		// 访问expressionlist获取变量名的Value
		if name := ret.(*javaparser.LocalVariableDeclarationContext).Identifier(); name != nil {
			text := name.GetText()
			value = append(value, y.ReadValue(text))
		} else if name := ret.(*javaparser.LocalVariableDeclarationContext).VariableDeclarators(); name != nil {
			// 访问localVariableDeclaration，定义变量，并获取变量名的value
			y.VisitLocalVariableDeclaration(ret)
			// 获取所有定义变量的变量名
			decls := name.(*javaparser.VariableDeclaratorsContext)
			for _, decl := range decls.AllVariableDeclarator() {
				if decl != nil {
					variableDeclaratorId := decl.(*javaparser.VariableDeclaratorContext).VariableDeclaratorId()
					text := variableDeclaratorId.(*javaparser.VariableDeclaratorIdContext).Identifier().GetText()
					value = append(value, y.ReadValue(text))
				}
			}
		}
	}
	return value
}

func (y *builder) VisitIfStmt(raw javaparser.IIfstmtContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	builder := y.CreateIfBuilder()

	var build func(raw javaparser.IIfstmtContext) func()
	build = func(raw javaparser.IIfstmtContext) func() {
		if raw == nil {
			return nil
		}
		i, _ := raw.(*javaparser.IfstmtContext)
		if i == nil {
			return nil
		}

		if parExpr := i.ParExpression(); parExpr != nil {
			expr := parExpr.(*javaparser.ParExpressionContext).Expression()
			if state := i.BlockOrState(); state != nil {
				builder.AppendItem(
					func() ssa.Value { return y.VisitExpression(expr) },
					func() {
						y.VisitBlockOrState(state)
					})
			} else {
				// 没有block的情况
				builder.AppendItem(
					func() ssa.Value { return y.VisitExpression(expr) },
					func() {})
			}

		}

		for _, elseIfBlock := range i.AllElseIfBlock() {
			if elseIfBlock != nil {
				elseIfStmt := elseIfBlock.(*javaparser.ElseIfBlockContext)
				state := elseIfStmt.BlockOrState()
				parExpr := elseIfStmt.ParExpression()
				expr := parExpr.(*javaparser.ParExpressionContext).Expression()
				builder.AppendItem(
					func() ssa.Value {
						return (y.VisitExpression(expr))
					},
					func() {
						y.VisitBlockOrState(state)
					},
				)
			}
		}
		elseStmt := i.ElseBlock()
		if elseStmt != nil {
			if elseState := elseStmt.(*javaparser.ElseBlockContext).BlockOrState(); elseState != nil {
				return func() { y.VisitBlockOrState(elseState) }
			}
		}
		return nil
	}
	elseBlock := build(raw)
	builder.SetElse(elseBlock)
	builder.Build()
	return nil
}

func (y *builder) VisitSwitchExpression(raw javaparser.ISwitchExpressionContext, isExpression bool) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}

	i, _ := raw.(*javaparser.SwitchExpressionContext)
	if i == nil {
		return nil
	}

	switchBuilder := y.BuildSwitch()
	switchBuilder.AutoBreak = false

	parExpr := i.ParExpression().(*javaparser.ParExpressionContext)
	expr := parExpr.Expression()
	if expr != nil {
		switchBuilder.BuildCondition(func() ssa.Value {
			return y.VisitExpression(expr)
		})
	} else {
		recoverRange := y.SetRangeFromTerminalNode(i.SWITCH())
		y.NewError(ssa.Warn, "javaast", "switch expression is nil")
		recoverRange()
	}

	switchLabels := i.AllSwitchLabeledRule()
	caseNum := len(switchLabels)
	//得到case后面参数的value
	getCaseValue := func(i int) []ssa.Value {
		switchStmt := switchLabels[i].(*javaparser.SwitchLabeledRuleContext)
		if switchStmt.ExpressionList() != nil {
			return y.VisitExpressionList(switchStmt.ExpressionList())
		} else if switchStmt.NULL_LITERAL() != nil {
			return []ssa.Value{y.EmitConstInstNil()}
		} else if switchStmt.GuardedPattern() != nil {
			return []ssa.Value{y.EmitConstInstNil()} // todo: 处理guarded pattern
		} else {
			return nil
		}
	}

	switchBuilder.BuildCaseSize(caseNum)
	switchBuilder.SetCase(func(i int) []ssa.Value {
		return getCaseValue(i)
	})

	switchBuilder.BuildBody(func(i int) {
		switchStmt := switchLabels[i].(*javaparser.SwitchLabeledRuleContext)
		if switchRuleOutCome := switchStmt.SwitchRuleOutcome(); switchRuleOutCome != nil {
			s := switchRuleOutCome.(*javaparser.SwitchRuleOutcomeContext)
			if s.Block() != nil {
				y.VisitBlock(s.Block())
			}
			for _, stmt := range s.AllBlockStatement() {
				y.VisitBlockStatement(stmt)
			}
		}
	})

	if i.DefaultLabeledRule() != nil {
		switchBuilder.BuildDefault(func() {
			if defaultStmt := i.DefaultLabeledRule().(*javaparser.DefaultLabeledRuleContext); defaultStmt != nil {
				switchRuleOutCome := defaultStmt.SwitchRuleOutcome()
				s := switchRuleOutCome.(*javaparser.SwitchRuleOutcomeContext)
				if s.Block() != nil {
					y.VisitBlock(s.Block())
				}
				for _, stmt := range s.AllBlockStatement() {
					y.VisitBlockStatement(stmt)
				}
			}
		})
	}

	switchBuilder.Finish()
	// switch 作为expression
	if isExpression {
		// 当switch作为expression的时候需要返回ssa.Value
		// 得到blockStatement的ssa.Value
		// 因为blockStatement并不所有的语句都会返回ssa.Value
		// 而switch作为expression的时候需要返回ssa.Value
		// todo 处理yeild语句
		getBlockValue := func(stmt javaparser.IBlockContext) []ssa.Value {
			if stmt == nil {
				return nil
			}
			block := stmt.(*javaparser.BlockContext)
			if blockStmtList := block.BlockStatementList(); blockStmtList != nil {
				blockStmts := blockStmtList.(*javaparser.BlockStatementListContext)
				for _, blockStmt := range blockStmts.AllBlockStatement() {
					blockStatement := blockStmt.(*javaparser.BlockStatementContext)
					if blockStatement.Statement() != nil {
						statement := blockStatement.Statement()
						switch ret := statement.(type) {
						case *javaparser.YieldStatementContext:
							return []ssa.Value{y.VisitExpression(ret.Expression())}
						}
					}
				}
			}

			return nil
		}

		getBlockStmtValue := func(stmt javaparser.IBlockStatementContext) []ssa.Value {
			if stmt == nil {
				return nil
			}
			blockStatement := stmt.(*javaparser.BlockStatementContext)
			if blockStatement.Statement() != nil {
				statement := blockStatement.Statement()
				switch ret := statement.(type) {
				case *javaparser.ExpressionStatementContext:
					return []ssa.Value{y.VisitExpression(ret.Expression())}
				}
			}
			return nil
		}
		// 遍历case的switchRuleOutcome的block和blockStatement
		getSwitchOutcomeValue := func(i int) ssa.Value {
			var value []ssa.Value
			switchStmt := switchLabels[i].(*javaparser.SwitchLabeledRuleContext)
			if switchRuleOutCome := switchStmt.SwitchRuleOutcome(); switchRuleOutCome != nil {
				s := switchRuleOutCome.(*javaparser.SwitchRuleOutcomeContext)
				if s.Block() != nil {
					block := s.Block().(*javaparser.BlockContext)
					value = append(value, getBlockValue(block)...)

				}
				for _, blockStmt := range s.AllBlockStatement() {
					value = append(value, getBlockStmtValue(blockStmt)...)
				}
			}
			// switch 作为参数的时候只能返回一个value
			if len(value) > 1 {
				y.NewError(ssa.Warn, "javaast", "switch as expression can only return one value")
				return nil
			} else {
				return value[0]
			}
		}
		// 遍历default的switchRuleOutcome的block和blockStatement
		getDefalutOutCome := func() ssa.Value {
			var value []ssa.Value
			defaultStmt := i.DefaultLabeledRule().(*javaparser.DefaultLabeledRuleContext)
			if switchRuleOutCome := defaultStmt.SwitchRuleOutcome(); switchRuleOutCome != nil {
				s := switchRuleOutCome.(*javaparser.SwitchRuleOutcomeContext)
				if s.Block() != nil {
					block := s.Block().(*javaparser.BlockContext)
					value = append(value, getBlockValue(block)...)

				}
				for _, blockStmt := range s.AllBlockStatement() {
					value = append(value, getBlockStmtValue(blockStmt)...)
				}
			}
			if len(value) > 1 {
				y.NewError(ssa.Warn, "javaast", "switch as expression can only return one value")
				return nil
			} else if len(value) == 1 {
				return value[0]
			} else {
				return y.EmitConstInstNil()
			}
		}
		// switch参数的value
		value1 := y.VisitExpression(expr)
		for i := 0; i < caseNum; i++ {
			// case参数的value
			value2 := getCaseValue(i)
			for _, v := range value2 {
				if value1.String() == v.String() {
					return getSwitchOutcomeValue(i)
				}
			}
		}
		if i.DefaultLabeledRule() != nil {
			return getDefalutOutCome()
		} else {
			return nil
		}
	} else {
		return nil
	}

}

func (y *builder) VisitGuardedPattern(raw javaparser.IGuardedPatternContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.GuardedPatternContext)
	if i == nil {
		return nil
	}
	return nil

}

func (y *builder) VisitBlockStatementList(raw javaparser.IBlockStatementListContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.BlockStatementListContext)
	if i == nil {
		return
	}
	for _, stmt := range i.AllBlockStatement() {
		if stmt != nil {
			y.VisitBlockStatement(stmt)
		}
	}
}

func (y *builder) VisitInnerCreator(raw javaparser.IInnerCreatorContext, outClassVariable string) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	i, _ := raw.(*javaparser.InnerCreatorContext)
	if i == nil {
		return nil
	}
	// todo 类声明的泛型
	if nonWildcard := i.NonWildcardTypeArgumentsOrDiamond(); nonWildcard != nil {
	}
	outClassObj := y.ReadOrCreateVariable(outClassVariable)
	outClassType := outClassObj.GetType()
	outClassName := outClassType.String()
	var builder strings.Builder
	builder.WriteString(outClassName)
	builder.WriteString(".")
	builder.WriteString(i.Identifier().GetText())
	className := builder.String()

	class := y.GetBluePrint(className)
	if class == nil {
		return nil
	}

	obj := y.EmitMakeWithoutType(nil, nil)
	obj.SetType(class)

	constructor := class.Constructor
	if constructor == nil {
		return obj
	}

	args := []ssa.Value{obj}
	arguments := y.VisitClassCreatorRest(i.ClassCreatorRest(), className)
	args = append(args, arguments...)
	c := y.NewCall(constructor, args)
	y.EmitCall(c)
	return obj

}

func (y *builder) VisitCreator(raw javaparser.ICreatorContext) (obj ssa.Value, constructorCall ssa.Value) {
	if y == nil || raw == nil || y.IsStop() {
		return nil, nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.CreatorContext)
	if i == nil {
		return nil, nil
	}

	// todo 类声明的泛型
	if nonWildcard := i.NonWildcardTypeArguments(); nonWildcard != nil {
	}

	var (
		p           ssa.Type
		createdName []string
		nameValues  []ssa.Value
	)
	if ret := i.CreatedName(); ret != nil {
		p, createdName, nameValues = y.VisitCreatedName(ret)
	}

	fixChainCreator := func() ssa.Value {
		var object ssa.Value
		if len(createdName) != len(nameValues) {
			return nil
		}
		for i, v := range nameValues {
			if i == 0 {
				newTyp := y.AddFullTypeNameFromMap(createdName[0], v.GetType())
				v.SetType(newTyp)
				object = v
			} else {
				key := y.EmitConstInst(createdName[i])
				object = y.ReadMemberCallValue(object, key)
				newTyp := y.AddFullTypeNameFromMap(createdName[0], object.GetType())
				object.SetType(newTyp)
			}
		}
		return object
	}

	className := strings.Join(createdName, ".")
	if ret := i.ClassCreatorRest(); ret != nil {
		class := y.GetBluePrint(className)
		obj := y.EmitUndefined(className)
		if class == nil {
			log.Warnf("class %v instantiation failed. maybe the origin (package) is not loaded? (dependency missed) ", className)
			variable := y.CreateVariable(className)
			defaultClassFullback := fixChainCreator()
			if utils.IsNil(defaultClassFullback) {
				log.Errorf("fixChainCreator failed.")
				return obj, nil
			}
			y.AssignVariable(variable, defaultClassFullback)
			var newCallTyp ssa.Type
			args := []ssa.Value{obj}
			arguments := y.VisitClassCreatorRest(ret, className)
			args = append(args, arguments...)
			call := y.EmitCall(y.NewCall(defaultClassFullback, args))
			newCallTyp = y.AddFullTypeNameFromMap(className, call.GetType())

			call.SetType(newCallTyp)
			return obj, call
		}
		obj.SetType(class)
		args := []ssa.Value{obj}
		arguments := y.VisitClassCreatorRest(ret, className)
		args = append(args, arguments...)
		return nil, y.ClassConstructor(class, args)
		//return obj, y.EmitCall(y.NewCall(constructor, args))
	}
	//array init
	if ret := i.ArrayCreatorRest(); ret != nil {
		return y.VisitArrayCreatorRest(ret, p), nil
	}
	log.Errorf("array  init failed.")
	obj = y.EmitMakeWithoutType(nil, nil)
	obj.SetType(ssa.CreateAnyType())
	return obj, nil
}

func (y *builder) VisitClassCreatorRest(raw javaparser.IClassCreatorRestContext, oldClassName string) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.ClassCreatorRestContext)
	if i == nil {
		return nil
	}

	var args []ssa.Value
	if i.Arguments() != nil {
		exprList := i.Arguments().(*javaparser.ArgumentsContext).ExpressionList()
		args = y.VisitExpressionList(exprList)
	}
	if i.ClassBody() != nil {
		// 匿名类
		className := uuid.NewString()
		class := y.CreateBluePrint(className)
		if oldClassName != "" {
			class.AddParentClass(y.GetBluePrint(oldClassName))
		}
		y.VisitClassBody(i.ClassBody(), class)
	}
	return args
}

func (y *builder) VisitArrayCreatorRest(raw javaparser.IArrayCreatorRestContext, p ssa.Type) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.ArrayCreatorRestContext)
	if i == nil {
		return nil
	}
	// 数组声明
	if ret := i.ArrayInitializer(); ret != nil {
		return y.VisitArrayInitializer(ret)
	}
	allExpr := i.AllExpression()
	var slice ssa.Value
	if allExpr == nil {
		slice = y.EmitMakeBuildWithType(ssa.NewSliceType(ssa.CreateAnyType()),
			y.EmitConstInst(0), y.EmitConstInst(0))
	}
	slice = y.InterfaceAddFieldBuild(len(allExpr),
		func(i int) ssa.Value { return y.EmitConstInst(i) },
		func(i int) ssa.Value { return y.VisitExpression(allExpr[i]) },
	)
	if utils.IsNil(slice) {
		return nil
	} else {
		slice.SetType(p)
		return slice
	}

}

func (y *builder) VisitCreatedName(raw javaparser.ICreatedNameContext) (typ ssa.Type, createdName []string, createdNameValue []ssa.Value) {
	if y == nil || raw == nil || y.IsStop() {
		return nil, nil, nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.CreatedNameContext)
	if i == nil {
		return nil, nil, nil
	}

	if ret := i.PrimitiveType(); ret != nil {
		typ = y.VisitPrimitiveType(ret)
	} else {
		typ = ssa.CreateAnyType()
	}
	for _, name := range i.AllIdentifier() {
		text := name.GetText()
		if v := y.PeekValue(text); v != nil {
			createdNameValue = append(createdNameValue, v)
		} else {
			createdNameValue = append(createdNameValue, y.ReadValue(text))
		}
		createdName = append(createdName, text)
	}
	return typ, createdName, createdNameValue
}

func (y *builder) VisitLambdaExpression(raw javaparser.ILambdaExpressionContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.LambdaExpressionContext)
	if i == nil {
		return nil
	}
	newFunc := y.NewFunc("")
	y.FunctionBuilder = y.PushFunction(newFunc)
	{
		y.VisitLambdaParameters(i.LambdaParameters())
		y.VisitLamdaBody(i.LambdaBody())
		y.Finish()
	}
	y.FunctionBuilder = y.PopFunction()
	return newFunc
}

func (y *builder) VisitLambdaParameters(raw javaparser.ILambdaParametersContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}

	switch ret := raw.(type) {
	case *javaparser.SingleLambdaParameterContext:
		y.NewParam(ret.Identifier().GetText())
	case *javaparser.FormalLambdaParametersContext:
		y.VisitFormalParameterList(ret.FormalParameterList())
	case *javaparser.MultiLambdaParametersContext:
		for _, id := range ret.AllIdentifier() {
			y.NewParam(id.GetText())
		}
	case *javaparser.LambdaLVTIParametersContext:
		if ret.LambdaLVTIList() != nil {
			y.VisitLambdaLVTIList(ret.LambdaLVTIList())
		}
	}
}

func (y *builder) VisitLamdaBody(raw javaparser.ILambdaBodyContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	i := raw.(*javaparser.LambdaBodyContext)
	if i == nil {
		return
	}

	if i.Expression() != nil {
		y.VisitExpression(i.Expression())
	} else if i.Block() != nil {
		y.VisitBlock(i.Block())
	}
}

func (y *builder) VisitLambdaLVTIList(raw javaparser.ILambdaLVTIListContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}

	i := raw.(*javaparser.LambdaLVTIListContext)
	if i == nil {
		return
	}
	for _, lv := range i.AllLambdaLVTIParameter() {
		y.VisitLambdaLVTIParameter(lv)
	}
}

func (y *builder) VisitLambdaLVTIParameter(raw javaparser.ILambdaLVTIParameterContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	i := raw.(*javaparser.LambdaLVTIParameterContext)
	if i == nil {
		return
	}

	var insCallbacks []func(ssa.Value)
	for _, modifier := range i.AllVariableModifier() {
		_, insCallback := y.VisitVariableModifier(modifier)
		insCallbacks = append(insCallbacks, insCallback)
	}
	param := y.NewParam(i.Identifier().GetText())
	for _, insCallback := range insCallbacks {
		insCallback(param)
	}
}

func (y *builder) VisitIdentifier(raw javaparser.IIdentifierContext, wantVariable ...bool) (variable *ssa.Variable, value ssa.Value) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.IdentifierContext)
	if i == nil {
		return
	}
	name := i.GetText()
	class := y.MarkedThisClassBlueprint
	// create variable
	if len(wantVariable) != 0 && wantVariable[0] == true {
		if class == nil {
			return
		}
		if class.GetNormalMember(name) != nil {
			obj := y.PeekValue("this")
			if obj != nil {
				variable = y.CreateMemberCallVariable(obj, y.EmitConstInst(name))
				return variable, nil
			}
		}
		if variable = y.GetOuterClassFieldVariable(name); variable != nil {
			return variable, nil
		}
		variable = y.CreateVariable(name)
		return variable, nil
	}
	// get value
	//set full type name for value
	defer func() {
		if utils.IsNil(value) {
			return
		}
		t := value.GetType()
		if t != nil && len(t.GetFullTypeNames()) == 0 {
			newType := y.AddFullTypeNameFromMap(name, value.GetType())
			value.SetType(newType)
		}
	}()

	if value = y.PeekValue(name); value != nil {
		// found
		return nil, value
	}
	//if in this class, return
	if class != nil {
		if method := class.GetStaticMethod(name); !utils.IsNil(method) {
			value = method
			return nil, method
		}
		if class.GetNormalMember(name) != nil {
			obj := y.PeekValue("this")
			if obj != nil {
				if value = y.ReadMemberCallValue(obj, y.EmitConstInst(name)); value != nil {
					return nil, value
				}
			}
		}
		value = y.ReadSelfMember(name)
		if value != nil {
			return nil, value
		}
	}

	var ok bool
	if value, ok = y.ReadConst(name); ok {
		return nil, value
	}
	if importValue, b := y.GetProgram().ReadImportValue(name); b {
		value = importValue
		return nil, importValue
	}
	value = y.ReadValue(name)
	return
}

func (y *builder) VisitResourceSpecification(raw javaparser.IResourceSpecificationContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ResourceSpecificationContext)
	if i == nil {
		return nil
	}
	if ret := i.Resources(); ret != nil {
		return y.VisitResources(ret)
	}
	return nil
}

func (y *builder) VisitResources(raw javaparser.IResourcesContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ResourcesContext)
	if i == nil {
		return nil
	}
	var values []ssa.Value
	for _, res := range i.AllResource() {
		values = append(values, y.VisitResource(res))
	}
	return values
}

func (y *builder) VisitResource(raw javaparser.IResourceContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ResourceContext)
	if i == nil {
		return nil
	}
	var variable *ssa.Variable
	var value ssa.Value
	if i.Expression() != nil {
		value = y.VisitExpression(i.Expression())
	}
	if value == nil {
		return nil
	}

	if ret := i.Identifier(); ret != nil {
		variable = y.CreateLocalVariable(ret.GetText())
	} else if ret := i.VariableDeclaratorId(); ret != nil {
		var typ ssa.Type
		if cls := i.ClassOrInterfaceType(); cls != nil {
			typ = y.VisitClassOrInterfaceType(cls)
		}
		if i.VariableDeclaratorId() != nil {
			name := i.VariableDeclaratorId().(*javaparser.VariableDeclaratorIdContext).Identifier().GetText()
			variable = y.CreateVariable(name)
			rightValTyp := value.GetType()
			rightValTypName := rightValTyp.GetFullTypeNames()
			if len(rightValTypName) != 0 && y.HaveCastType(rightValTyp) {
				newTyp := y.RemoveCastTypeFlag(rightValTyp)
				value.SetType(newTyp)
			} else {
				if typ != nil {
					newTyp := y.MergeFullTypeNameForType(typ.GetFullTypeNames(), rightValTyp)
					value.SetType(newTyp)
				}
			}
		}
	}
	y.AssignVariable(variable, value)
	return value
}

func (y *builder) VisitSwitchStatement(raw javaparser.ISwitchStatementContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.SwitchStatementContext)
	if i == nil {
		return
	}

	SwitchBuilder := y.BuildSwitch()
	SwitchBuilder.AutoBreak = false
	var cond ssa.Value
	parExpr := i.ParExpression().(*javaparser.ParExpressionContext)
	if expr := parExpr.Expression(); expr != nil {
		SwitchBuilder.BuildCondition(func() ssa.Value {
			cond = y.VisitExpression(expr)
			return cond
		})
	} else {
		recoverRange := y.SetRangeFromTerminalNode(i.SWITCH())
		y.NewError(ssa.Warn, "javaast", "switch expression is nil")
		recoverRange()
	}

	var defaultStatement func()
	caseLen := 0
	caseValueMap := make(map[int]ssa.Values)
	caseStatementMap := make(map[int]func())

	for _, s := range i.AllSwitchBlockStatementGroup() {
		labelType, labelValues, visitStatement := y.VisitSwitchBlockStatementGroup(s)
		if labelType == CASE {
			caseValueMap[caseLen] = labelValues
			caseStatementMap[caseLen] = visitStatement
			caseLen++
		}
		if labelType == DEFAULT {
			defaultStatement = visitStatement
		}
	}
	SwitchBuilder.BuildCaseSize(caseLen)
	SwitchBuilder.SetCase(func(i int) []ssa.Value {
		if v, ok := caseValueMap[i]; ok {
			return v
		}
		return nil
	})
	SwitchBuilder.BuildBody(func(i int) {
		if f, ok := caseStatementMap[i]; ok {
			f()
		}
	})
	if defaultStatement != nil {
		SwitchBuilder.BuildDefault(defaultStatement)
	}
	SwitchBuilder.Finish()
}

func (y *builder) VisitSwitchBlockStatementGroup(raw javaparser.ISwitchBlockStatementGroupContext) (labelType JavaSwitchLabel, labelValues ssa.Values, visitStatement func()) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.SwitchBlockStatementGroupContext)
	if i == nil {
		return
	}
	if ret := i.SwitchLabel(); ret != nil {
		labelType, labelValues = y.VisitSwitchLabel(ret)
	}
	visitStatement = func() {
		if ret := i.StatementList(); ret != nil {
			y.VisitStatementList(ret)
		}
	}
	return
}

func (y *builder) VisitSwitchLabel(raw javaparser.ISwitchLabelContext) (JavaSwitchLabel, ssa.Values) {
	if y == nil || raw == nil || y.IsStop() {
		return "", nil
	}

	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.SwitchLabelContext)
	if i == nil {
		return "", nil
	}
	if i.CASE() != nil {
		return CASE, y.VisitExpressionList(i.ExpressionList())
	}
	if i.DEFAULT() != nil {
		return DEFAULT, y.VisitExpressionList(i.ExpressionList())
	}
	return "", nil
}

func (y *builder) VisitLeftSliceCall(raw javaparser.ILeftSliceCallContext, object ssa.Value) *ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.LeftSliceCallContext)
	if i == nil {
		return nil
	}
	index := y.VisitExpression(i.Expression())
	return y.CreateMemberCallVariable(object, index)
}

func (y *builder) VisitLeftMemberCall(raw javaparser.ILeftMemberCallContext, object ssa.Value) *ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.LeftMemberCallContext)
	if i == nil {
		return nil
	}
	name := i.Identifier().GetText()
	return y.CreateMemberCallVariable(object, y.EmitConstInst(name))
}

func (y *builder) GetOuterClassFieldVariable(name string) *ssa.Variable {
	bp := y.MarkedThisClassBlueprint
	if bp == nil {
		return nil
	}
	s := strings.Split(bp.Name, INNER_CLASS_SPLIT)
	if len(s) != 2 {
		return nil
	}
	bp = y.GetBluePrint(s[0])
	if bp == nil {
		return nil
	}
	var variable *ssa.Variable
	if ret := bp.GetNormalMember(name); ret != nil {
		obj := y.PeekValue("this")
		if obj != nil {
			variable = y.CreateMemberCallVariable(obj, y.EmitConstInst(name))
			return variable
		}
	}
	return nil
}
