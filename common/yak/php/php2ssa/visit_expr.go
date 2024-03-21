package php2ssa

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
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
		return nil
	}

	switch ret := raw.(type) {
	case *phpparser.CloneExpressionContext:
		// 浅拷贝，一个对象
		// 如果类定义了 __clone，就执行 __clone
		target := y.VisitExpression(ret.Expression())
		y.ir.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.ir.EmitBinOp(
				ssa.OpNotEq,
				y.ir.ReadOrCreateMemberCallVariable(target, y.ir.EmitConstInst("__clone")),
				y.ir.EmitConstInstNil(),
			)
		}, func() {
			// have __clone
			calling := y.ir.NewCall(
				y.ir.ReadOrCreateMemberCallVariable(target, y.ir.EmitConstInst("__clone")),
				nil,
			)
			y.ir.EmitCall(calling)
		}).Build()
		return nil
	case *phpparser.VariableExpressionContext:
		id := y.VisitVariable(ret.Variable())
		return y.ir.ReadValue(id)

	case *phpparser.KeywordNewExpressionContext:
		return y.VisitNewExpr(ret.NewExpr())
	case *phpparser.IndexCallExpressionContext: // $a[1]
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitIndexMemberCallKey(ret.IndexMemberCallKey())
		return y.ir.ReadMemberCallVariable(obj, key)
	case *phpparser.MemberCallExpressionContext: // $a->b
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitMemberCallKey(ret.MemberCallKey())
		return y.ir.ReadMemberCallVariable(obj, key)
	case *phpparser.SliceCallAssignmentExpressionContext: // $a[1] = expr
		// build left
		object := y.VisitExpression(ret.Expression(0))
		key := y.VisitIndexMemberCallKey(ret.IndexMemberCallKey())
		member := y.ir.CreateMemberCallVariable(object, key)
		// right
		rightValue := y.VisitExpression(ret.Expression(1))
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), member, rightValue)
		y.ir.AssignVariable(member, rightValue)
		return rightValue
	case *phpparser.FieldMemberCallAssignmentExpressionContext: // $a->b = expr
		// build left
		object := y.VisitExpression(ret.Expression(0))
		key := y.VisitMemberCallKey(ret.MemberCallKey())
		member := y.ir.CreateMemberCallVariable(object, key)
		// right
		rightValue := y.VisitExpression(ret.Expression(1))
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), member, rightValue)
		y.ir.AssignVariable(member, rightValue)
		return rightValue
	case *phpparser.FunctionCallExpressionContext:
		callee := y.VisitExpression(ret.Expression())
		args, ellipsis := y.VisitArguments(ret.Arguments())
		callInst := y.ir.NewCall(callee, args)
		if ellipsis {
			callInst.IsEllipsis = true
		}
		return y.ir.EmitCall(callInst)
	case *phpparser.CastExpressionContext:
		target := y.VisitExpression(ret.Expression())
		return y.ir.EmitTypeCast(target, y.VisitCastOperation(ret.CastOperation()))
	case *phpparser.UnaryOperatorExpressionContext:
		/*
			| ('~' | '@') expression                                      # UnaryOperatorExpression
			| ('!' | '+' | '-') expression                                # UnaryOperatorExpression
		*/
		val := y.VisitExpression(ret.Expression())
		switch {
		case ret.Bang() != nil:
			return y.ir.EmitUnOp(ssa.OpNot, val)
		case ret.Plus() != nil:
			return y.ir.EmitUnOp(ssa.OpPlus, val)
		case ret.Minus() != nil:
			return y.ir.EmitUnOp(ssa.OpNeg, val)
		case ret.Tilde() != nil:
			return y.ir.EmitUnOp(ssa.OpBitwiseNot, val)
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
		variable := y.VisitLeftVariable(ret.LeftVariable())
		val := y.ir.ReadValueByVariable(variable)
		if ret.Inc() != nil {
			after := y.ir.EmitBinOp(ssa.OpAdd, val, y.ir.EmitConstInst(1))
			y.ir.AssignVariable(variable, after)
			// y.ir.EmitUpdate(val, after)
			return after
		} else if ret.Dec() != nil {
			after := y.ir.EmitBinOp(ssa.OpSub, val, y.ir.EmitConstInst(1))
			y.ir.AssignVariable(variable, after)
			return after
		}
		return y.ir.EmitConstInstNil()
	case *phpparser.PostfixIncDecExpressionContext:
		variable := y.VisitLeftVariable(ret.LeftVariable())
		val := y.ir.ReadValueByVariable(variable)
		if ret.Inc() != nil {
			after := y.ir.EmitBinOp(ssa.OpAdd, val, y.ir.EmitConstInst(1))
			y.ir.AssignVariable(variable, after)
			return val
		} else if ret.Dec() != nil {
			after := y.ir.EmitBinOp(ssa.OpSub, val, y.ir.EmitConstInst(1))
			y.ir.AssignVariable(variable, after)
			return val
		}
		return y.ir.EmitConstInstNil()
	case *phpparser.PrintExpressionContext:
		caller := y.ir.ReadValue("print")
		args := y.VisitExpression(ret.Expression())
		callInst := y.ir.NewCall(caller, []ssa.Value{args})
		return y.ir.EmitCall(callInst)
	case *phpparser.ArrayCreationExpressionContext:
		// arrayCreation
		return y.VisitArrayCreation(ret.ArrayCreation())
	case *phpparser.ScalarExpressionContext: // constant / string / label / php literal
		if i := ret.Constant(); i != nil {
			return y.VisitConstant(i)
		} else if i := ret.String_(); i != nil {
			return y.VisitString_(i)
		} else if ret.Label() != nil {
			return y.ir.EmitConstInst(i.GetText())
		} else {
			log.Warnf("PHP Scalar Expr Failed: %s", ret.GetText())
		}
	case *phpparser.BackQuoteStringExpressionContext:
		r := ret.GetText()
		if len(r) >= 2 {
			r = r[1 : len(r)-1]
		}
		return y.ir.EmitConstInst(r)
	case *phpparser.ParenthesisExpressionContext:
		return y.VisitExpression(ret.Expression())
	case *phpparser.SpecialWordExpressionContext:
		if i := ret.Yield(); i != nil {
			return y.ir.EmitConstInstNil()
		} else if i := ret.List(); i != nil {

		} else if i := ret.IsSet(); i != nil {

		} else if i := ret.Empty(); i != nil {

		} else if i := ret.Eval(); i != nil {

		} else if i := ret.Exit(); i != nil {

		} else if i := ret.Include(); i != nil {

		} else if i := ret.IncludeOnce(); i != nil {

		} else if i := ret.Require(); i != nil {

		} else if i := ret.RequireOnce(); i != nil {

		} else if i := ret.Throw(); i != nil {

		} else {
			log.Errorf("unhandled special word: %v", ret.GetText())
		}
		return y.ir.EmitConstInstNil()
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
			return y.ir.EmitConstInstAny()
		}
		return y.ir.EmitBinOp(o, op1, op2)
	case *phpparser.InstanceOfExpressionContext:
		// instanceof
		panic("NOT IMPL")
	case *phpparser.ComparisonExpressionContext:
		switch ret.GetOp().GetText() {
		case "<<":
			return y.ir.EmitBinOp(ssa.OpShl, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">>":
			return y.ir.EmitBinOp(ssa.OpShr, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "<":
			return y.ir.EmitBinOp(ssa.OpLt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">":
			return y.ir.EmitBinOp(ssa.OpGt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "<=":
			return y.ir.EmitBinOp(ssa.OpLtEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">=":
			return y.ir.EmitBinOp(ssa.OpGtEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "==":
			return y.ir.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "===":
			return y.ir.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "!=":
			return y.ir.EmitBinOp(ssa.OpNotEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "!==":
			return y.ir.EmitBinOp(ssa.OpNotEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		default:
			log.Errorf("unhandled comparison expression: %v", ret.GetText())
		}
		return y.ir.EmitConstInstNil()
	case *phpparser.BitwiseExpressionContext:
		switch ret.GetOp().GetText() {
		case "&&":
			var id string
			v1 := y.VisitExpression(ret.Expression(0))
			y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInstAny())
			y.ir.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.ir.EmitBinOp(ssa.OpEq, v1, y.ir.EmitConstInst(true))
			}, func() {
				v2 := y.VisitExpression(ret.Expression(1))
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitBinOp(ssa.OpEq, v2, y.ir.EmitConstInst(true)))
			}).SetElse(func() {
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInst(false))
			}).Build()
			return y.ir.ReadValue(id)
		case "||":
			var id string
			v1 := y.VisitExpression(ret.Expression(0))
			y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInstAny())
			y.ir.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.ir.EmitBinOp(ssa.OpEq, v1, y.ir.EmitConstInst(true))
			}, func() {
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInst(true))
			}).SetElse(func() {
				v2 := y.VisitExpression(ret.Expression(1))
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitBinOp(ssa.OpEq, v2, y.ir.EmitConstInst(true)))
			}).Build()
			return y.ir.ReadValue(id)
		case "|":
			return y.ir.EmitBinOp(ssa.OpOr, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "^":
			return y.ir.EmitBinOp(ssa.OpXor, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "&":
			return y.ir.EmitBinOp(ssa.OpAnd, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		default:
			return y.ir.EmitConstInstNil()
		}
	case *phpparser.ConditionalExpressionContext:
		v1 := y.VisitExpression(ret.Expression(0))
		exprCount := len(ret.AllExpression())
		var result ssa.Value
		y.ir.CreateIfBuilder().AppendItem(func() ssa.Value {
			t1 := y.ir.EmitBinOp(ssa.OpNotEq, v1, y.ir.EmitConstInstNil())
			t2 := y.ir.EmitBinOp(ssa.OpNotEq, v1, y.ir.EmitConstInst(0))
			t3 := y.ir.EmitBinOp(ssa.OpNotEq, v1, y.ir.EmitConstInst(false))
			return y.ir.EmitBinOp(ssa.OpLogicAnd, t1, y.ir.EmitBinOp(ssa.OpLogicAnd, t2, t3))
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
			return y.VisitExpression(ret.Expression(1)) //如果是undefined就返回1
		} else {
			return nil
		}
	case *phpparser.SpaceshipExpressionContext:
		var result ssa.Value
		y.ir.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.ir.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		}, func() {
			result = y.ir.EmitConstInst(0)
		}).SetElse(func() {
			y.ir.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.ir.EmitBinOp(ssa.OpLt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
			}, func() {
				result = y.ir.EmitConstInst(-1)
			}).SetElse(func() {
				result = y.ir.EmitConstInst(1)
			})
		})
		return result
	case *phpparser.ArrayCreationUnpackExpressionContext:
		// [$1, $2, $3] = $arr;
		// unpacking
		log.Errorf("unpack unfinished")
		return nil

	case *phpparser.OrdinaryAssignmentExpressionContext:
		variable := y.VisitLeftVariable(ret.LeftVariable())
		rightValue := y.VisitExpression(ret.Expression())
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), variable, rightValue)
		y.ir.AssignVariable(variable, rightValue)
		return rightValue

	case *phpparser.LogicalExpressionContext:
		var id = uuid.NewString()
		y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInstAny())
		if ret.LogicalXor() != nil {
			v1 := y.VisitExpression(ret.Expression(0))
			v2 := y.VisitExpression(ret.Expression(1))
			y.ir.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.ir.EmitBinOp(ssa.OpEq, v1, v2)
			}, func() {
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInst(true))
			}).SetElse(func() {
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInst(false))
			}).Build()
		}
		if ret.LogicalOr() != nil {
			value := y.VisitExpression(ret.Expression(0))
			y.ir.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.ir.EmitBinOp(ssa.OpEq, value, y.ir.EmitConstInst(true))
			}, func() {
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInst(true))
			}).SetElse(func() {
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(1)), y.ir.EmitConstInst(true)))
			}).Build()
		}
		if ret.LogicalAnd() != nil {
			value := y.VisitExpression(ret.Expression(0))
			y.ir.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.ir.EmitBinOp(ssa.OpEq, value, y.ir.EmitConstInst(true))
			}, func() {
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(1)), y.ir.EmitConstInst(true)))
			}).SetElse(func() {
				y.ir.AssignVariable(y.ir.CreateVariable(id), y.ir.EmitConstInst(false))
			}).Build()
		}
		return y.ir.ReadValue(id)

	case *phpparser.ShortQualifiedNameExpressionContext:
		//因为涉及到函数，先peek 如果没有读取到说明是一个常量 （define定义的常量会出现问题）
		identifier := y.VisitIdentifier(ret.Identifier())
		if value := y.ir.PeekValue(identifier); value != nil {
			return value
		} else {
			return y.ir.EmitConstInst(identifier)
		}
		//return y.ir.ReadOrCreateVariable(y.VisitIdentifier(ret.Identifier()))

	case *phpparser.StaticClassAccessExpressionContext:
		class, key := y.VisitStaticClassExpr(ret.StaticClassExpr())
		return y.ir.GetStaticMember(class, key)

	case *phpparser.StaticClassMemberCallAssignmentExpressionContext:
		class, key := y.VisitStaticClassExpr(ret.StaticClassExpr())
		leftValue := y.ir.GetStaticMember(class, key)
		rightValue := y.VisitExpression(ret.Expression())
		rightValue = y.reduceAssignCalcExpressionEx(ret.AssignmentOperator().GetText(), leftValue, rightValue)
		y.ir.SetStaticMember(class, key, rightValue)
		return rightValue
	}
	raw.GetText()
	log.Errorf("unhandled expression: %v(T: %T)", raw.GetText(), raw)
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	return y.ir.EmitConstInstAny()
}

func (y *builder) VisitAssignable(raw phpparser.IAssignableContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AssignableContext)
	if i == nil {
		return nil
	}

	if i.Chain() != nil {
		return y.VisitChain(i.Chain())
	} else if i.ArrayCreation() != nil {
		return y.VisitArrayCreation(i.ArrayCreation())
	} else {
		log.Errorf("cannot build leftValue Assignable with: %v", i.Chain().GetText())
		return nil
	}
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

	origin := y.VisitChainOrigin(i.ChainOrigin())

	for _, m := range i.AllMemberAccess() {
		origin = y.VisitMemberAccess(origin, m)
	}
	return origin
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
	origin = y.ir.ReadOrCreateMemberCallVariable(origin, fieldName)
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

	i, _ := raw.(*phpparser.KeyedFieldNameContext)
	if i == nil {
		return nil
	}

	if i.KeyedSimpleFieldName() != nil {
		y.VisitKeyedSimpleFieldName(i.KeyedSimpleFieldName())
	} else if i.KeyedVariable() != nil {
		y.VisitKeyedVariable(i.KeyedVariable())
	}

	return nil
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
		variable := y.ir.ReadOrCreateVariable(i.VarName().GetText()).GetLastVariable()
		if variable == nil {
			variable = y.ir.CreateVariable(i.VarName().GetText())
		}
		varMain = variable.GetValue()
		if varMain == nil {
			varMain = y.ir.EmitUndefined(i.VarName().GetText())
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
		if v == nil {
			varMain = y.ir.ReadOrCreateMemberCallVariable(varMain, v)
		}
	}

	return varMain
}

func (y *builder) VisitKeyedSimpleFieldName(raw phpparser.IKeyedSimpleFieldNameContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.KeyedSimpleFieldNameContext)
	if i == nil {
		return nil
	}

	if i.Identifier() != nil {
		v := y.VisitIdentifier(i.Identifier())
		_ = v
	} else if i.Expression() != nil {
		v := y.VisitExpression(i.Expression())
		_ = v
	}

	for _, sce := range i.AllSquareCurlyExpression() {
		y.VisitSquareCurlyExpression(sce)
	}

	return nil
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
			log.Warnf("PHP $a[...] call empty")
			return y.ir.EmitUndefined("$var[]")
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

	args, ellipsis := y.VisitActualArguments(i.ActualArguments())
	c := y.ir.NewCall(v, args)
	c.IsEllipsis = ellipsis
	return y.ir.EmitCall(c)
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
		return y.ir.ReadValue(text)
	} else if ret := i.ChainBase(); ret != nil {
		return y.VisitChainBase(ret)
	} else if ret := i.ClassConstant(); ret != nil {
		return y.VisitClassConstant(ret)
	} else if ret := i.Parentheses(); ret != nil {
		return y.VisitParentheses(ret)
	} else if ret := i.Label(); ret != nil {
		return y.ir.ReadValue(i.Label().GetText())
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

	return y.ir.EmitUndefined(i.GetText())
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
		panic("NOT IMPL")
	} else {
		var ret ssa.Value
		for _, i := range i.AllKeyedVariable() {
			if ret == nil {
				ret = y.VisitKeyedVariable(i)
				continue
			}
			ret = y.ir.CreateMemberCallVariable(ret, y.VisitKeyedVariable(i)).GetValue()
		}
		return ret
	}
	return nil
}

func (y *builder) VisitArrayCreation(raw phpparser.IArrayCreationContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayCreationContext)
	if i == nil {
		return nil
	}

	var l int

	if ret := i.ArrayItemList(); ret != nil {
		itemList := ret.(*phpparser.ArrayItemListContext)
		l = len(itemList.AllArrayItem())
	}

	obj := y.ir.EmitMakeWithoutType(y.ir.EmitConstInst(l), y.ir.EmitConstInst(l))

	if ret := i.ArrayItemList(); ret != nil {
		for _, kv := range y.VisitArrayItemList(ret.(*phpparser.ArrayItemListContext)) {
			k, v := kv[0], kv[1]
			variable := y.ir.ReadOrCreateMemberCallVariable(obj, k).GetLastVariable()
			y.ir.AssignVariable(variable, v)
		}
	}
	return obj
}

func (y *builder) VisitArrayItemList(raw phpparser.IArrayItemListContext) [][2]ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayItemListContext)
	if i == nil {
		return nil
	}

	countIndex := 0
	var results [][2]ssa.Value
	for _, a := range i.AllArrayItem() {

		k, v := y.VisitArrayItem(a)
		if k == nil {
			k = y.ir.EmitConstInst(countIndex)
			countIndex++
		}
		kv := [2]ssa.Value{k, v}
		results = append(results, kv)
	}
	return results
}

func (y *builder) VisitArrayItem(raw phpparser.IArrayItemContext) (ssa.Value, ssa.Value) {
	if y == nil || raw == nil {
		return nil, nil
	}

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

func (y *builder) VisitStringConstant(raw phpparser.IStringConstantContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.StringConstantContext)
	if i == nil {
		return nil
	}

	//ret := y.ir.ReadVariable(i.GetText(), false)
	//if ret == nil {
	//	return y.ir.EmitConstInst(i.GetText())
	//}

	//return ret
	return nil
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
		results := y.VisitArrayItemList(ret)
		lnc := len(results)
		array := y.ir.EmitMakeWithoutType(y.ir.EmitConstInst(lnc), y.ir.EmitConstInst(lnc))
		for _, v := range results {
			key, value := v[0], v[1]
			variable := y.ir.ReadOrCreateMemberCallVariable(array, key).GetLastVariable()
			y.ir.AssignVariable(variable, value)
		}
		return array
	} else if ret := i.ConstantInitializer(); ret != nil {
		// op = ('+' | '-') constantInitializer
		val := y.VisitConstantInitializer(ret)
		if i.Minus() != nil {
			return y.ir.EmitUnOp(ssa.OpNeg, val)
		}
		return y.ir.EmitUnOp(ssa.OpPlus, val)
	} else {
		var initVal ssa.Value
		for _, c := range i.AllConstantString() {
			if initVal == nil {
				initVal = y.VisitConstantString(c)
				continue
			}
			initVal = y.ir.EmitBinOp(ssa.OpAdd, initVal, y.VisitConstantString(c))
		}
		if initVal == nil {
			log.Errorf("unhandled constant initializer: %v", i.GetText())
			return y.ir.EmitConstInstNil()
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
	var value = make([]ssa.Value, 0, len(i.AllExpression()))
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
	return y.reduceAssignCalcExpressionEx(operator, y.ir.ReadValueByVariable(variable), value)
}

func (y *builder) reduceAssignCalcExpressionEx(operator string, leftValues ssa.Value, rightValue ssa.Value) ssa.Value {
	switch operator {
	case "=":
		return rightValue
	case "+=":
		rightValue = y.ir.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
	case "-=":
		rightValue = y.ir.EmitBinOp(ssa.OpSub, leftValues, rightValue)
	case "*=":
		rightValue = y.ir.EmitBinOp(ssa.OpMul, leftValues, rightValue)
	case "**=":
		rightValue = y.ir.EmitBinOp(ssa.OpPow, leftValues, rightValue)
		//rightValue = ssa.CalcConstBinary(y.ir.c, rightValue, ssa.OpPow)
	case "/=":
		rightValue = y.ir.EmitBinOp(ssa.OpDiv, leftValues, rightValue)
	case "%=":
		rightValue = y.ir.EmitBinOp(ssa.OpMod, leftValues, rightValue)
	case ".=":
		rightValue = y.ir.EmitConstInst(leftValues.String() + rightValue.String())
		//rightValue = y.ir.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
	case "&=":
		rightValue = y.ir.EmitBinOp(ssa.OpAnd, leftValues, rightValue)
	case "|=":
		rightValue = y.ir.EmitBinOp(ssa.OpOr, leftValues, rightValue)
	case "^=":
		rightValue = y.ir.EmitBinOp(ssa.OpXor, leftValues, rightValue)
	case "<<=":
		rightValue = y.ir.EmitBinOp(ssa.OpShl, leftValues, rightValue)
	case ">>=":
		rightValue = y.ir.EmitBinOp(ssa.OpShr, leftValues, rightValue)
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

func (y *builder) VisitLeftVariable(raw phpparser.ILeftVariableContext) *ssa.Variable {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, ok := raw.(*phpparser.LeftVariableContext)
	if !ok {
		return nil
	}

	return y.ir.CreateVariable(
		y.VisitVariable(i.Variable()),
	)
}

func (y *builder) VisitVariable(raw phpparser.IVariableContext) string {
	if y == nil || raw == nil {
		return ""
	}
	switch ret := raw.(type) {

	case *phpparser.NormalVariableContext:
		return ret.VarName().GetText()
		// log.Infof("NormalVariable: %v", name)

	case *phpparser.DynamicVariableContext:
		id := ret.VarName().GetText()
		var value ssa.Value
		for i := range ret.AllDollar() {
			_ = i
			value = y.ir.ReadValue(id)
			id = "$" + value.String()
		}
		// log.Infof("DynamicVariable: %v", value)
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
