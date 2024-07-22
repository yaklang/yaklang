package php2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitExpressionStatement(raw phpparser.IExpressionStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ExpressionStatementContext)
	if i == nil {
		return nil
	}
	va := y.VisitExpression(i.Expression())
	return va
}

func (y *builder) VisitParentheses(raw phpparser.IParenthesesContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ParenthesesContext)
	if i == nil {
		return nil
	}

	return y.VisitExpression(i.Expression())
}

func (y *builder) VisitExpression(raw phpparser.IExpressionContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if raw.GetText() == "" {
		return y.EmitConstInst("")
	}

	switch ret := raw.(type) {
	case *phpparser.CloneExpressionContext:
		// 浅拷贝，一个对象
		// 如果类定义了 __clone，就执行 __clone
		target := y.VisitExpression(ret.Expression())
		val, ok := target.GetStringMember("__clone")
		if !ok {
			return target
		}
		return y.EmitCall(y.NewCall(val, nil))
	case *phpparser.VariableExpressionContext:
		return y.VisitRightValue(ret.FlexiVariable())
	case *phpparser.MemerCallExpressionContext:
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitMemberCallKey(ret.MemberCallKey())
		return y.ReadMemberCallVariable(obj, key)
	//case *phpparser.CodeExecExpressionContext:
	//	var code string
	//	value := y.VisitExpression(ret.Expression())
	//	if value.GetType().GetTypeKind() == ssa.StringTypeKind {
	//		if unquote, err := strconv.Unquote(value.String()); err != nil {
	//			code = value.String()
	//		} else {
	//			code = unquote
	//		}
	//		// 应该考虑更多情况
	//		code = `<?php ` + code + ";"
	//		if err := y.GetProgram().Build("Exec-"+uuid.NewString(), memedit.NewMemEditor(code), y.FunctionBuilder); err != nil {
	//			log.Errorf("execute code %v failed", code)
	//		}
	//	} else {
	//		var execFunction string
	//		if ret.Assert() != nil {
	//			execFunction = "assert"
	//		} else {
	//			execFunction = "eval"
	//		}
	//		readValue := y.ReadValue(execFunction)
	//		call := y.NewCall(readValue, []ssa.Value{value})
	//		return y.EmitCall(call)
	//	}
	//	return y.EmitConstInstNil()
	case *phpparser.KeywordNewExpressionContext:
		return y.VisitNewExpr(ret.NewExpr())
	case *phpparser.IndexCallExpressionContext: // $a[1]
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitIndexMemberCallKey(ret.IndexMemberCallKey())
		if key == nil {
			return obj
		}
		return y.ReadMemberCallVariable(obj, key)
	case *phpparser.IndexLegacyCallExpressionContext: // $a{1}
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitIndexMemberCallKey(ret.IndexMemberCallKey())
		if key == nil {
			return obj
		}
		return y.ReadMemberCallVariable(obj, key)
	case *phpparser.FunctionCallExpressionContext:
		tmp := y.isFunction
		y.isFunction = true
		defer func() {
			y.isFunction = tmp
		}()
		fname := y.VisitExpression(ret.Expression())
		if ret, isConst := ssa.ToConst(fname); isConst {
			if ret != nil {
				funcName := ret.VarString()
				fname = y.ReadValue(funcName)
			}
		}
		args, ellipsis := y.VisitArguments(ret.Arguments())
		callInst := y.NewCall(fname, args)
		if ellipsis {
			callInst.IsEllipsis = true
		}
		return y.EmitCall(callInst)
	case *phpparser.CastExpressionContext:
		target := y.VisitExpression(ret.Expression())
		return y.EmitTypeCast(target, y.VisitCastOperation(ret.CastOperation()))
	case *phpparser.UnaryOperatorExpressionContext:
		/*
			| ('~' | '@') expression                                      # UnaryOperatorExpression
			| ('!' | '+' | '-') expression                                # UnaryOperatorExpression
		*/
		val := y.VisitExpression(ret.Expression())
		switch {
		case ret.Bang() != nil:
			return y.EmitUnOp(ssa.OpNot, val)
		case ret.Plus() != nil:
			return y.EmitUnOp(ssa.OpPlus, val)
		case ret.Minus() != nil:
			return y.EmitUnOp(ssa.OpNeg, val)
		case ret.Tilde() != nil:
			return y.EmitUnOp(ssa.OpBitwiseNot, val)
		case ret.SuppressWarnings() != nil:
			/*
				TODO:
				  var a;
				  try {
				    $a = exec expr;
				  }
			*/
			return val
		}
	case *phpparser.PrefixIncDecExpressionContext:
		// variable := y.variable
		// val := y.VisitExpression(ret.Expression())
		variable := y.VisitLeftVariable(ret.FlexiVariable())
		val := y.ReadValueByVariable(variable)
		if ret.Inc() != nil {
			after := y.EmitBinOp(ssa.OpAdd, val, y.EmitConstInst(1))
			y.AssignVariable(variable, after)
			// y.EmitUpdate(val, after)
			return after
		} else if ret.Dec() != nil {
			after := y.EmitBinOp(ssa.OpSub, val, y.EmitConstInst(1))
			y.AssignVariable(variable, after)
			return after
		}
		return y.EmitConstInstNil()
	case *phpparser.PostfixIncDecExpressionContext:
		variable := y.VisitLeftVariable(ret.FlexiVariable())
		val := y.ReadValueByVariable(variable)
		if ret.Inc() != nil {
			after := y.EmitBinOp(ssa.OpAdd, val, y.EmitConstInst(1))
			y.AssignVariable(variable, after)
			return val
		} else if ret.Dec() != nil {
			after := y.EmitBinOp(ssa.OpSub, val, y.EmitConstInst(1))
			y.AssignVariable(variable, after)
			return val
		}
		return y.EmitConstInstNil()
	case *phpparser.PrintExpressionContext:
		caller := y.ReadValue("print")
		args := y.VisitExpression(ret.Expression())
		callInst := y.NewCall(caller, []ssa.Value{args})
		return y.EmitCall(callInst)
	case *phpparser.ArrayCreationExpressionContext:
		creation, i := y.VisitArrayCreation(ret.ArrayCreation())
		obj := y.EmitMakeWithoutType(y.EmitConstInst(i), y.EmitConstInst(i))
		for _, values := range creation {
			key, value := values[0], values[1]
			variable := y.ReadOrCreateMemberCallVariable(obj, key).GetLastVariable()
			y.AssignVariable(variable, value)
		}
		return obj
	case *phpparser.ScalarExpressionContext: // constant / string / label / php literal
		if i := ret.Constant(); i != nil {
			return y.VisitConstant(i)
		} else if i := ret.String_(); i != nil {
			return y.VisitString_(i)
		} else if ret.Label() != nil {
			return y.EmitConstInst(i.GetText())
		} else {
			log.Warnf("PHP Scalar Expr Failed: %s", ret.GetText())
		}
	case *phpparser.BackQuoteStringExpressionContext:
		r := ret.GetText()
		if len(r) >= 2 {
			r = r[1 : len(r)-1]
		}
		return y.EmitConstInst(r)
	case *phpparser.ParenthesisExpressionContext:
		return y.VisitExpression(ret.Expression())
	case *phpparser.SpecialWordExpressionContext:
		if i := ret.Yield(); i != nil {
			return y.EmitConstInstNil()
		} else if i := ret.List(); i != nil {
		} else if i := ret.IsSet(); i != nil {
			for _, chain := range ret.ChainList().(*phpparser.ChainListContext).AllChain() {
				visitChain := y.VisitChain(chain)
				undefine, ok := ssa.ToUndefined(visitChain)
				if visitChain == nil || (ok && undefine.Kind == ssa.UndefinedValueInValid) {
					return y.EmitConstInst(false)
				}
			}
			return y.EmitConstInst(true)
		} else if i := ret.Empty(); i != nil {
			return y.VisitChain(ret.Chain())
		} else if i := ret.Throw(); i != nil {
			return y.VisitExpression(ret.Expression())
		} else if ret.Die() != nil || ret.Exit() != nil {
			return y.VisitExpression(ret.Expression())
		}
		return y.EmitConstInstNil()
	case *phpparser.IncludeExpressionContext:
		if ret.Expression() != nil {
			value := y.VisitExpression(ret.Expression())
			y.AddIncludePath(value.String())
			return y.EmitConstInst(true)
		}
		return y.VisitIncludeExpression(ret.Include())
	case *phpparser.LambdaFunctionExpressionContext:
		return y.VisitLambdaFunctionExpr(ret.LambdaFunctionExpr())
	case *phpparser.MatchExpressionContext:
	case *phpparser.ArithmeticExpressionContext:
		exprs := ret.AllExpression()
		if len(exprs) == 0 {
			log.Error("Arithmetic Expression need 2 ops")
		}
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		var o ssa.BinaryOpcode
		opStr := ret.GetOp().GetText()
		switch opStr {
		case "**":
			o = ssa.OpPow
		case "+":
			o = ssa.OpAdd
		case "-":
			o = ssa.OpSub
		case "*":
			o = ssa.OpMul
		case "/":
			o = ssa.OpDiv
		case "%":
			o = ssa.OpMod
		case ".":
			o = ssa.OpAdd
		default:
			log.Errorf("unexpected op: %v", opStr)
			return y.EmitUndefined("")
		}
		return y.EmitBinOp(o, op1, op2)
	case *phpparser.InstanceOfExpressionContext:
		// instanceof
		log.Error("InstanceOfExpressionContext unfinished")
		log.Error("InstanceOfExpressionContext unfinished")
		log.Error("InstanceOfExpressionContext unfinished")
		y.EmitUndefined("")
	case *phpparser.ComparisonExpressionContext:
		switch ret.GetOp().GetText() {
		case "<<":
			return y.EmitBinOp(ssa.OpShl, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">>":
			return y.EmitBinOp(ssa.OpShr, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "<":
			return y.EmitBinOp(ssa.OpLt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">":
			return y.EmitBinOp(ssa.OpGt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "<=":
			return y.EmitBinOp(ssa.OpLtEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">=":
			return y.EmitBinOp(ssa.OpGtEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "==":
			return y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "===":
			return y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "!=":
			return y.EmitBinOp(ssa.OpNotEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "!==":
			return y.EmitBinOp(ssa.OpNotEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		default:
			log.Errorf("unhandled comparison expression: %v", ret.GetText())
		}
		return y.EmitConstInstNil()
	case *phpparser.BitwiseExpressionContext:
		switch ret.GetOp().GetText() {
		case "&&":
			id := uuid.NewString()
			v1 := y.VisitExpression(ret.Expression(0))
			y.AssignVariable(y.CreateVariable(id), y.EmitValueOnlyDeclare(id))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, v1, y.EmitConstInst(true))
			}, func() {
				v2 := y.VisitExpression(ret.Expression(1))
				y.AssignVariable(y.CreateVariable(id), y.EmitBinOp(ssa.OpEq, v2, y.EmitConstInst(true)))
			}).SetElse(func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(false))
			}).Build()
			return y.ReadValue(id)
		case "||":
			id := uuid.NewString()
			v1 := y.VisitExpression(ret.Expression(0))
			y.AssignVariable(y.CreateVariable(id), y.EmitValueOnlyDeclare(id))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, v1, y.EmitConstInst(true))
			}, func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(true))
			}).SetElse(func() {
				v2 := y.VisitExpression(ret.Expression(1))
				y.AssignVariable(y.CreateVariable(id), y.EmitBinOp(ssa.OpEq, v2, y.EmitConstInst(true)))
			}).Build()
			return y.ReadValue(id)
		case "|":
			return y.EmitBinOp(ssa.OpOr, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "^":
			return y.EmitBinOp(ssa.OpXor, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "&":
			return y.EmitBinOp(ssa.OpAnd, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		default:
			return y.EmitConstInstNil()
		}
	case *phpparser.ConditionalExpressionContext:
		v1 := y.VisitExpression(ret.Expression(0))
		exprCount := len(ret.AllExpression())
		var result ssa.Value
		y.CreateIfBuilder().AppendItem(func() ssa.Value {
			t1 := y.EmitBinOp(ssa.OpNotEq, v1, y.EmitConstInstNil())
			t2 := y.EmitBinOp(ssa.OpNotEq, v1, y.EmitConstInst(0))
			t3 := y.EmitBinOp(ssa.OpNotEq, v1, y.EmitConstInst(false))
			return y.EmitBinOp(ssa.OpLogicAnd, t1, y.EmitBinOp(ssa.OpLogicAnd, t2, t3))
		}, func() {
			if exprCount == 2 {
				result = v1
			} else {
				// exprCount == 3
				result = y.VisitExpression(ret.Expression(1))
			}
		}).SetElse(func() {
			if exprCount == 2 {
				result = y.VisitExpression(ret.Expression(1))
			} else {
				result = y.VisitExpression(ret.Expression(2))
			}
		}).Build()
		return result
	case *phpparser.NullCoalescingExpressionContext:
		if leftValue := y.VisitExpression(ret.Expression(0)); leftValue.IsUndefined() {
			return y.VisitExpression(ret.Expression(1)) // 如果是undefined就返回1
		} else {
			return leftValue
		}
	case *phpparser.DefinedOrScanDefinedExpressionContext:
		return y.VisitDefineExpr(ret.DefineExpr())
	case *phpparser.SpaceshipExpressionContext:
		var result ssa.Value
		y.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		}, func() {
			result = y.EmitConstInst(0)
		}).SetElse(func() {
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpLt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
			}, func() {
				result = y.EmitConstInst(-1)
			}).SetElse(func() {
				result = y.EmitConstInst(1)
			})
		})
		return result
	case *phpparser.ArrayCreationUnpackExpressionContext:
		// [$1, $2, $3] = $arr;
		// unpacking
		creation := y.VisitLeftArrayCreation(ret.LeftArrayCreation())
		expression := y.VisitExpression(ret.Expression())
		for _, variable := range creation {
			y.AssignVariable(variable, expression) //连接上数据流
		}
		return y.EmitConstInstNil()
	case *phpparser.OrdinaryAssignmentExpressionContext:
		variable := y.VisitLeftVariable(ret.FlexiVariable())
		rightValue := y.VisitExpression(ret.Expression())
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), variable, rightValue)
		y.AssignVariable(variable, rightValue)
		return rightValue

	case *phpparser.LogicalExpressionContext:
		id := uuid.NewString()
		y.AssignVariable(y.CreateVariable(id), y.EmitValueOnlyDeclare(id))
		if ret.LogicalXor() != nil {
			v1 := y.VisitExpression(ret.Expression(0))
			v2 := y.VisitExpression(ret.Expression(1))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, v1, v2)
			}, func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(true))
			}).SetElse(func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(false))
			}).Build()
		}
		if ret.LogicalOr() != nil {
			value := y.VisitExpression(ret.Expression(0))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, value, y.EmitConstInst(true))
			}, func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(true))
			}).SetElse(func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(1)), y.EmitConstInst(true)))
			}).Build()
		}
		if ret.LogicalAnd() != nil {
			value := y.VisitExpression(ret.Expression(0))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, value, y.EmitConstInst(true))
			}, func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(1)), y.EmitConstInst(true)))
			}).SetElse(func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(false))
			}).Build()
		}
		return y.ReadValue(id)

	case *phpparser.ShortQualifiedNameExpressionContext:
		// 因为涉及到函数，先peek 如果没有读取到说明是一个常量 （define定义的常量会出现问题）
		var unquote string
		_unquote, err := yakunquote.Unquote(ret.Identifier().GetText())
		if err != nil {
			unquote = ret.Identifier().GetText()
		} else {
			unquote = _unquote
		}
		// 先在常量表中查询
		if !y.isFunction {
			if s, ok := y.ReadConst(unquote); ok {
				return s
			}
			log.Warnf("const map not found %v", unquote)
		}
		_value := y.VisitIdentifier(ret.Identifier())
		if _, exit := y.GetProgram().ExternInstance[strings.ToLower(y.VisitIdentifier(ret.Identifier()))]; exit {
			_value = strings.ToLower(_value)
			return y.ReadValue(_value)
		} else if value := y.PeekValue(y.VisitIdentifier(ret.Identifier())); value != nil {
			return value
		} else {
			return y.EmitConstInst(y.VisitIdentifier(ret.Identifier()))
		}

	// TODO: static class member
	// 静态方法调用
	case *phpparser.StaticClassAccessExpressionContext:
		return y.VisitStaticClassExpr(ret.StaticClassExpr())

	case *phpparser.StaticClassMemberCallAssignmentExpressionContext:
		variable, _, _ := y.VisitStaticClassExprVariableMember(ret.StaticClassExprVariableMember())
		//return y.ReadValueByVariable(member)
		rightValue := y.VisitExpression(ret.Expression())
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), variable, rightValue)
		y.AssignVariable(variable, rightValue)
		return rightValue
	}
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	return y.EmitConstInstNil()
}

func (y *builder) VisitChainList(raw phpparser.IChainListContext) []ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainListContext)
	if i == nil {
		return nil
	}
	var tmpValue []ssa.Value
	for _, chain := range i.AllChain() {
		tmpValue = append(tmpValue, y.VisitChain(chain))
	}
	return tmpValue
}

func (y *builder) VisitChainLeft(raw phpparser.IChainContext) *ssa.Variable {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainContext)
	if i == nil {
		return nil
	}
	if i.FlexiVariable() != nil {
		return y.VisitLeftVariable(i.FlexiVariable())
	}
	if i.StaticClassExprVariableMember() != nil {
		member, _, _ := y.VisitStaticClassExprVariableMember(i.StaticClassExprVariableMember())
		return member
	}
	return nil
}

func (y *builder) VisitChain(raw phpparser.IChainContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainContext)
	if i == nil {
		return nil
	}
	return y.VisitRightValue(i.FlexiVariable())
}

func (y *builder) VisitMemberAccess(origin ssa.Value, raw phpparser.IMemberAccessContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.MemberAccessContext)
	if i == nil {
		return nil
	}

	fieldName := y.VisitKeyedFieldName(i.KeyedFieldName())
	origin = y.ReadOrCreateMemberCallVariable(origin, fieldName)
	if i.ActualArguments() != nil {
		y.VisitActualArguments(i.ActualArguments())
	}

	return origin
}

func (y *builder) VisitKeyedFieldName(raw phpparser.IKeyedFieldNameContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*phpparser.KeyedFieldNameContext)

	if i.KeyedSimpleFieldName() != nil {
		return y.VisitKeyedSimpleFieldName(i.KeyedSimpleFieldName())
	} else if i.KeyedVariable() != nil {
		return y.VisitKeyedVariable(i.KeyedVariable())
	}
	return y.EmitUndefined(raw.GetText())
}

func (y *builder) VisitKeyedVariable(raw phpparser.IKeyedVariableContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.KeyedVariableContext)
	if i == nil {
		return nil
	}

	dollarCount := len(i.AllDollar())
	var varMain ssa.Value
	if i.VarName() != nil {
		// ($*)$a
		//// {} as index [] as sliceCall
		variable := y.ReadOrCreateVariable(i.VarName().GetText()).GetLastVariable()
		if variable == nil {
			variable = y.CreateVariable(i.VarName().GetText())
		}
		varMain = variable.GetValue()
		if varMain == nil {
			varMain = y.EmitUndefined(i.VarName().GetText())
		}
		if dollarCount > 1 {
			for i := 0; i < dollarCount-1; i++ {
				// 处理变量的变量
			}
		}

	} else {
		// {} as name [] as sliceCall
		varMain = y.VisitExpression(i.Expression())
	}

	for _, a := range i.AllSquareCurlyExpression() {
		v := y.VisitSquareCurlyExpression(a)
		if v != nil {
			varMain = y.ReadOrCreateMemberCallVariable(varMain, v)
		}
	}

	return varMain
}

func (y *builder) VisitKeyedSimpleFieldName(raw phpparser.IKeyedSimpleFieldNameContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.KeyedSimpleFieldNameContext)
	if i == nil {
		return nil
	}

	var val ssa.Value
	if i.Identifier() != nil {
		val = y.EmitConstInst(y.VisitIdentifier(i.Identifier()))
	} else if i.Expression() != nil {
		val = y.VisitExpression(i.Expression())
	} else {
		val = y.EmitEmptyContainer()
	}

	for _, sce := range i.AllSquareCurlyExpression() {
		v := y.VisitSquareCurlyExpression(sce)
		if v != nil {
			val = y.ReadOrCreateMemberCallVariable(val, v)
		}
	}

	return val
}

func (y *builder) VisitSquareCurlyExpression(raw phpparser.ISquareCurlyExpressionContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.SquareCurlyExpressionContext)
	if i == nil {
		return nil
	}

	if i.OpenSquareBracket() != nil {
		if i.Expression() != nil {
			return y.VisitExpression(i.Expression())
		} else {
			/*
				$a = array("apple", "banana");
				$a[] = "cherry";

				// 现在，$a 包含 "apple", "banana", "cherry"
			*/
			// call len
			log.Errorf("UNIMPLEMTED SquareCurlyExpressionContext like a[] = ...: %v", raw.GetText())
			log.Errorf("UNIMPLEMTED SquareCurlyExpressionContext like a[] = ...: %v", raw.GetText())
			log.Errorf("UNIMPLEMTED SquareCurlyExpressionContext like a[] = ...: %v", raw.GetText())
			log.Errorf("UNIMPLEMTED SquareCurlyExpressionContext like a[] = ...: %v", raw.GetText())
			log.Error("PHP $a[...] call empty")
			return y.EmitUndefined("$var[]")
		}
	} else {
		return y.VisitExpression(i.Expression())
	}
}

func (y *builder) VisitFunctionCall(raw phpparser.IFunctionCallContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FunctionCallContext)
	if i == nil {
		return nil
	}

	v := y.VisitFunctionCallName(i.FunctionCallName())
	var c *ssa.Call
	args, ellipsis := y.VisitActualArguments(i.ActualArguments())
	if _, exit := y.GetProgram().ExternInstance[strings.ToLower(v.String())]; exit {
		c = y.NewCall(y.EmitConstInst(strings.ToLower(v.String())), args)
	} else {
		c = y.NewCall(v, args)
	}
	c.IsEllipsis = ellipsis
	return y.EmitCall(c)
}

func (y *builder) VisitFunctionCallName(raw phpparser.IFunctionCallNameContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FunctionCallNameContext)
	if i == nil {
		return nil
	}

	if ret := i.QualifiedNamespaceName(); ret != nil {
		text := y.VisitQualifiedNamespaceName(ret)
		_ = text
		//return y.ReadValue(text)
	} else if ret := i.ChainBase(); ret != nil {
		return y.VisitChainBase(ret)
	} else if ret := i.ClassConstant(); ret != nil {
		return y.VisitClassConstant(ret)
	} else if ret := i.Parentheses(); ret != nil {
		return y.VisitParentheses(ret)
	} else if ret := i.Label(); ret != nil {
		return y.ReadValue(i.Label().GetText())
	}
	log.Errorf("BUG: unknown function call name: %v", i.GetText())
	return nil
}

func (y *builder) VisitChainOrigin(raw phpparser.IChainOriginContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainOriginContext)
	if i == nil {
		return nil
	}

	if ret := i.NewExpr(); ret != nil {
		return y.VisitNewExpr(ret)
	} else if ret := i.FunctionCall(); ret != nil {
		return y.VisitFunctionCall(ret)
	} else if ret := i.ChainBase(); ret != nil {
		return y.VisitChainBase(ret)
	} else {
		log.Errorf("BUG: unknown chain origin: %v", i.GetText())
	}

	return y.EmitUndefined(i.GetText())
}

func (y *builder) VisitChainBase(raw phpparser.IChainBaseContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainBaseContext)
	if i == nil {
		return nil
	}
	if ret := i.QualifiedStaticTypeRef(); ret != nil {
		log.Error(`QualifiedStaticTypeRef unfinished`)
		log.Error(`QualifiedStaticTypeRef unfinished`)
		log.Error(`QualifiedStaticTypeRef unfinished`)
	} else {
		var ret ssa.Value
		for _, i := range i.AllKeyedVariable() {
			if ret == nil {
				ret = y.VisitKeyedVariable(i)
				continue
			}
			ret = y.CreateMemberCallVariable(ret, y.VisitKeyedVariable(i)).GetValue()
		}
		return ret
	}
	return nil
}

func (y *builder) VisitArrayCreation(raw phpparser.IArrayCreationContext) ([][2]ssa.Value, int) {
	if y == nil || raw == nil {
		return [][2]ssa.Value{}, 0
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayCreationContext)
	if i == nil {
		return [][2]ssa.Value{}, 0
	}
	return y.VisitArrayItemList(i.ArrayItemList())
}

func (y *builder) VisitArrayItemList(raw phpparser.IArrayItemListContext) ([][2]ssa.Value, int) {
	if y == nil || raw == nil {
		return [][2]ssa.Value{}, 0
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayItemListContext)
	if i == nil {
		return [][2]ssa.Value{}, 0
	}

	countIndex := 0
	var results [][2]ssa.Value
	for _, a := range i.AllArrayItem() {

		k, v := y.VisitArrayItem(a)
		if k == nil {
			k = y.EmitConstInst(countIndex)
			countIndex++
		}
		kv := [2]ssa.Value{k, v}
		results = append(results, kv)
	}
	return results, countIndex
}

func (y *builder) VisitArrayItem(raw phpparser.IArrayItemContext) (ssa.Value, ssa.Value) {
	if y == nil || raw == nil {
		return nil, nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.ArrayItemContext)
	if i == nil {
		return nil, nil
	}

	if i.Chain() != nil {
		// (expression '=>')? '&' chain
		var v ssa.Value
		if i.Expression(0) != nil {
			v = y.VisitExpression(i.Expression(0))
		}
		return v, y.VisitChain(i.Chain())
	} else {
		// expression ('=>' expression)?
		k := y.VisitExpression(i.Expression(0))
		var v ssa.Value
		if ret := i.Expression(1); ret != nil {
			v = y.VisitExpression(ret)
		} else {
			return nil, k
		}
		return k, v
	}
}

func (y *builder) VisitAttributes(raw phpparser.IAttributesContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AttributesContext)
	if i == nil {
		return nil
	}

	for _, g := range i.AllAttributeGroup() {
		y.VisitAttributeGroup(g)
	}

	return nil
}

func (y *builder) VisitAttributeGroup(raw phpparser.IAttributeGroupContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AttributeGroupContext)
	if i == nil {
		return nil
	}

	y.VisitIdentifier(i.Identifier())

	for _, a := range i.AllAttribute() {
		y.VisitAttribute(a)
	}

	return nil
}

func (y *builder) VisitAttribute(raw phpparser.IAttributeContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AttributeContext)
	if i == nil {
		return nil
	}

	y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
	if i.Arguments() != nil {
		y.VisitArguments(i.Arguments())
	}

	return nil
}

func (y *builder) VisitLeftArrayCreation(raw phpparser.ILeftArrayCreationContext) []*ssa.Variable {
	if y == nil || raw == nil {
		return []*ssa.Variable{}
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	arraycreation := raw.(*phpparser.LeftArrayCreationContext)
	return y.VisitArrayDestructuring(arraycreation.ArrayDestructuring())
}
func (y *builder) VisitArrayDestructuring(raw phpparser.IArrayDestructuringContext) []*ssa.Variable {
	if y == nil || raw == nil {
		return []*ssa.Variable{}
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	arrayDest, ok := raw.(*phpparser.ArrayDestructuringContext)
	if !ok {
		return []*ssa.Variable{}
	}
	var result = make([]*ssa.Variable, 0)
	for _, itemContext := range arrayDest.AllIndexedDestructItem() {
		item := y.VisitIndexDestructItem(itemContext)
		result = append(result, item)
	}
	return result
}

func (y *builder) VisitIndexDestructItem(raw phpparser.IIndexedDestructItemContext) *ssa.Variable {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	arrayDest, ok := raw.(*phpparser.IndexedDestructItemContext)
	if !ok {
		return nil
	}
	return y.VisitChainLeft(arrayDest.Chain())
}

type arrayKeyValuePair struct {
	Key   ssa.Value
	Value ssa.Value
}

func (y *builder) VisitConstantInitializer(raw phpparser.IConstantInitializerContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ConstantInitializerContext)
	if i == nil {
		return nil
	}

	if ret := i.ArrayItemList(); ret != nil {
		//    | Array '(' (arrayItemList ','?)? ')'
		//    | '[' (arrayItemList ','?)? ']'
		results, l := y.VisitArrayItemList(ret)
		array := y.EmitMakeWithoutType(y.EmitConstInst(l), y.EmitConstInst(l))
		obj := y.EmitMakeWithoutType(y.EmitConstInst(i), y.EmitConstInst(i))
		for _, values := range results {
			k, v := values[0], values[1]
			variable := y.CreateMemberCallVariable(obj, k)
			y.AssignVariable(variable, v)
		}
		for _, v := range results {
			key, value := v[0], v[1]
			variable := y.ReadOrCreateMemberCallVariable(array, key).GetLastVariable()
			y.AssignVariable(variable, value)
		}
		return array
	} else if ret := i.ConstantInitializer(); ret != nil {
		// op = ('+' | '-') constantInitializer
		val := y.VisitConstantInitializer(ret)
		if i.Minus() != nil {
			return y.EmitUnOp(ssa.OpNeg, val)
		}
		return y.EmitUnOp(ssa.OpPlus, val)
	} else if ret := i.Expression(); ret != nil {
		return y.VisitExpression(ret)
	} else {
		var initVal ssa.Value
		for _, c := range i.AllConstantString() {
			if initVal == nil {
				initVal = y.VisitConstantString(c)
				continue
			}
			initVal = y.EmitBinOp(ssa.OpAdd, initVal, y.VisitConstantString(c))
		}
		if initVal == nil {
			if i.GetText() == "array()" {
				log.Warnf("create an emtpy make via `array()`")
				return y.EmitMakeWithoutType(y.EmitConstInst(0), y.EmitConstInst(0))
			}
			log.Errorf("unhandled constant initializer: %v", i.GetText())
			return y.EmitConstInstNil()
		}
		return initVal
	}
}

func (y *builder) VisitExpressionList(raw phpparser.IExpressionListContext) []ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ExpressionListContext)
	if i == nil {
		return nil
	}
	value := make([]ssa.Value, 0, len(i.AllExpression()))
	for _, expressionContext := range i.AllExpression() {
		value = append(value, y.VisitExpression(expressionContext))
	}
	return value
}

func (y *builder) VisitConstantString(raw phpparser.IConstantStringContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ConstantStringContext)
	if i == nil {
		return nil
	}

	if r := i.String_(); r != nil {
		return y.VisitString_(r)
	}
	return y.VisitConstant(i.Constant())
}

func (y *builder) reduceAssignCalcExpression(operator string, variable *ssa.Variable, value ssa.Value) ssa.Value {
	if operator == "=" {
		return value
	}
	// y.GetReferenceFiles()
	return y.reduceAssignCalcExpressionEx(operator, y.ReadValueByVariable(variable), value)
}

func (y *builder) reduceAssignCalcExpressionEx(operator string, leftValues ssa.Value, rightValue ssa.Value) ssa.Value {
	switch operator {
	case "=":
		return rightValue
	case "+=":
		rightValue = y.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
	case "-=":
		rightValue = y.EmitBinOp(ssa.OpSub, leftValues, rightValue)
	case "*=":
		rightValue = y.EmitBinOp(ssa.OpMul, leftValues, rightValue)
	case "**=":
		rightValue = y.EmitBinOp(ssa.OpPow, leftValues, rightValue)
		// rightValue = ssa.CalcConstBinary(y.c, rightValue, ssa.OpPow)
	case "/=":
		rightValue = y.EmitBinOp(ssa.OpDiv, leftValues, rightValue)
	case "%=":
		rightValue = y.EmitBinOp(ssa.OpMod, leftValues, rightValue)
	case ".=":
		rightValue = y.EmitConstInst(leftValues.String() + rightValue.String())
		// rightValue = y.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
	case "&=":
		rightValue = y.EmitBinOp(ssa.OpAnd, leftValues, rightValue)
	case "|=":
		rightValue = y.EmitBinOp(ssa.OpOr, leftValues, rightValue)
	case "^=":
		rightValue = y.EmitBinOp(ssa.OpXor, leftValues, rightValue)
	case "<<=":
		rightValue = y.EmitBinOp(ssa.OpShl, leftValues, rightValue)
	case ">>=":
		rightValue = y.EmitBinOp(ssa.OpShr, leftValues, rightValue)
	case "??=":
		if leftValues == nil || leftValues.IsUndefined() {
			return rightValue
		} else {
			return leftValues
		}
	default:
		log.Errorf("unhandled assignment operator: %v", operator)
	}
	return rightValue
}

// VisitLeftVariable
func (y *builder) VisitLeftVariable(raw phpparser.IFlexiVariableContext) *ssa.Variable {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	//todo $_POST{}、$_POST[] 这两种情况
	switch i := raw.(type) {
	case *phpparser.CustomVariableContext:
		return y.CreateVariable(
			y.VisitVariable(i.Variable()),
		)
	case *phpparser.IndexVariableContext:
		value := y.VisitRightValue(i.FlexiVariable())
		if key := y.VisitIndexMemberCallKey(i.IndexMemberCallKey()); key != nil {
			return y.CreateMemberCallVariable(value, key)
		} else {
			return y.VisitLeftVariable(i.FlexiVariable())
		}
	case *phpparser.IndexLegacyCallVariableContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		if key := y.VisitIndexMemberCallKey(i.IndexMemberCallKey()); key != nil {
			return y.VisitLeftVariable(i.FlexiVariable())
		} else {
			return y.CreateMemberCallVariable(obj, key)
		}
	case *phpparser.MemberVariableContext:
		value := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitMemberCallKey(i.MemberCallKey())
		member := y.CreateMemberCallVariable(value, key)
		return member
	default:
		return nil
	}
}

// flexivariable读右值
func (y *builder) VisitRightValue(raw phpparser.IFlexiVariableContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch i := raw.(type) {
	case *phpparser.CustomVariableContext:
		variable := y.VisitVariable(i.Variable())
		return y.ReadValue(variable)
	case *phpparser.IndexVariableContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitIndexMemberCallKey(i.IndexMemberCallKey())
		return y.ReadMemberCallVariable(obj, key)
	case *phpparser.IndexLegacyCallVariableContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitIndexMemberCallKey(i.IndexMemberCallKey())
		return y.ReadMemberCallVariable(obj, key)
	case *phpparser.MemberVariableContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitMemberCallKey(i.MemberCallKey())
		return y.ReadMemberCallVariable(obj, key)
	default:
		return nil
	}
}

func (y *builder) VisitVariable(raw phpparser.IVariableContext) string {
	if y == nil || raw == nil {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch ret := raw.(type) {
	case *phpparser.NormalVariableContext:
		return ret.VarName().GetText()

	case *phpparser.DynamicVariableContext:
		id := ret.VarName().GetText()
		var value ssa.Value
		for i := range ret.AllDollar() {
			_ = i
			value = y.ReadValue(id)
			id = "$" + value.String()
		}
		return "$" + value.String()
	case *phpparser.MemberCallVariableContext:
		value := y.VisitExpression(ret.Expression())
		// TODO: handler this
		return value.String()

	default:
		raw.GetText()
		log.Errorf("unhandled expression: %v(T: %T)", raw.GetText(), raw)
		log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
		return ""
	}
}

func (y *builder) VisitIncludeExpression(raw phpparser.IIncludeContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, ok := raw.(*phpparser.IncludeContext)
	if !ok {
		return nil
	}
	/*
			不支持
		<?php
		$b = "231.php";
		$a = include("$b");
		var_dump($a);
	*/
	var flag, once bool
	if i.IncludeOnce() != nil || i.RequireOnce() != nil {
		once = true
	}
	expr := i.Expression()
	value := y.VisitExpression(expr)
	if utils.IsNil(value) {
		log.Errorf("_________________BUG___EXPR IS NIL: %v________________", expr.GetText())
		log.Errorf("_________________BUG___EXPR IS NIL: %v________________", expr.GetText())
		log.Errorf("_________________BUG___EXPR IS NIL: %v________________", expr.GetText())
		log.Errorf("_________________BUG___EXPR IS NIL: %v________________", expr.GetText())
		return y.EmitUndefined(expr.GetText())
	}
	if value.IsUndefined() {
	} else {
		file := value.String()
		if err := y.BuildFilePackage(file, once); err != nil {
			log.Errorf("include: %v failed: %v", file, err)
		} else {
			flag = true
		}
	}
	return y.EmitConstInst(flag)
}

func (y *builder) VisitDefineExpr(raw phpparser.IDefineExprContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, ok := raw.(*phpparser.DefineExprContext)
	if !ok {
		return nil
	}
	var flag bool
	if i.Defined() != nil {
		if value := y.PeekValue(i.ConstantString().GetText()); value == nil || value.IsUndefined() {
			flag = false
		} else {
			flag = true
		}
	}
	if i.Define() != nil {
		value := y.VisitExpression(i.Expression())
		constantString := y.VisitConstantString(i.ConstantString())
		if y.AssignConst(constantString.String(), value) {
			flag = true
		}
	}
	return y.EmitConstInst(flag)
}
