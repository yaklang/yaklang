package php2ssa

import (
	"strconv"

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
		return nil
	}

	switch ret := raw.(type) {
	case *phpparser.CloneExpressionContext:
		// 浅拷贝，一个对象
		// 如果类定义了 __clone，就执行 __clone
		target := y.VisitExpression(ret.Expression())
		y.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.EmitBinOp(
				ssa.OpNotEq,
				y.ReadOrCreateMemberCallVariable(target, y.EmitConstInst("__clone")),
				y.EmitConstInstNil(),
			)
		}, func() {
			// have __clone
			calling := y.NewCall(
				y.ReadOrCreateMemberCallVariable(target, y.EmitConstInst("__clone")),
				nil,
			)
			y.EmitCall(calling)
		}).Build()
		return nil
	case *phpparser.VariableExpressionContext:
		id := y.VisitVariable(ret.Variable())
		return y.ReadValue(id)
	case *phpparser.CodeExecExpressionContext:
		var code string
		value := y.VisitExpression(ret.Expression())
		if value.GetType().GetTypeKind() == ssa.StringTypeKind {
			if unquote, err := strconv.Unquote(value.String()); err != nil {
				code = value.String()
			} else {
				code = unquote
			}
			//应该考虑更多情况
			code = `<?php ` + code + ";"
			if err := Build(code, false, y.FunctionBuilder); err != nil {
				log.Errorf("execute code %v failed", code)
			}
		} else {
			var execFunction string
			if ret.Assert() != nil {
				execFunction = "assert"
			} else {
				execFunction = "eval"
			}
			readValue := y.ReadValue(execFunction)
			call := y.NewCall(readValue, []ssa.Value{value})
			return y.EmitCall(call)
		}
		return y.EmitConstInstNil()
	case *phpparser.KeywordNewExpressionContext:
		return y.VisitNewExpr(ret.NewExpr())
	case *phpparser.IndexCallExpressionContext: // $a[1]
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitIndexMemberCallKey(ret.IndexMemberCallKey())
		return y.ReadMemberCallVariable(obj, key)
	case *phpparser.MemberCallExpressionContext: // $a->b
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitMemberCallKey(ret.MemberCallKey())
		return y.ReadMemberCallVariable(obj, key)
	case *phpparser.SliceCallAssignmentExpressionContext: // $a[1] = expr
		// build left
		object := y.VisitExpression(ret.Expression(0))
		key := y.VisitIndexMemberCallKey(ret.IndexMemberCallKey())
		member := y.CreateMemberCallVariable(object, key)
		// right
		rightValue := y.VisitExpression(ret.Expression(1))
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), member, rightValue)
		y.AssignVariable(member, rightValue)
		return rightValue
	case *phpparser.FieldMemberCallAssignmentExpressionContext: // $a->b = expr
		// build left
		object := y.VisitExpression(ret.Expression(0))
		key := y.VisitMemberCallKey(ret.MemberCallKey())
		member := y.CreateMemberCallVariable(object, key)
		// right
		rightValue := y.VisitExpression(ret.Expression(1))
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), member, rightValue)
		y.AssignVariable(member, rightValue)
		return rightValue
	case *phpparser.FunctionCallExpressionContext:
		tmp := y.isFunction
		y.isFunction = true
		defer func() {
			y.isFunction = tmp
		}()
		callee := y.VisitExpression(ret.Expression())
		//for _, callKeyContext := range ret.AllMemberCallKey() {
		//	_ = callKeyContext
		//doSomethings
		//}
		args, ellipsis := y.VisitArguments(ret.Arguments())
		callInst := y.NewCall(callee, args)
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
		variable := y.VisitLeftVariable(ret.LeftVariable())
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
		variable := y.VisitLeftVariable(ret.LeftVariable())
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
		// arrayCreation
		return y.VisitArrayCreation(ret.ArrayCreation())
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
				if visitChain := y.VisitChain(chain); visitChain.IsUndefined() {
					return y.EmitConstInstAny()
					//return y.EmitConstInst(false)
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
	case *phpparser.IncludeExpreesionContext:
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
			return y.EmitConstInstAny()
		}
		return y.EmitBinOp(o, op1, op2)
	case *phpparser.InstanceOfExpressionContext:
		// instanceof
		panic("NOT IMPL")
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
			var id string
			v1 := y.VisitExpression(ret.Expression(0))
			y.AssignVariable(y.CreateVariable(id), y.EmitConstInstAny())
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
			var id string
			v1 := y.VisitExpression(ret.Expression(0))
			y.AssignVariable(y.CreateVariable(id), y.EmitConstInstAny())
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
			return y.VisitExpression(ret.Expression(1)) //如果是undefined就返回1
		} else {
			return nil
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
		log.Errorf("unpack unfinished")
		return nil

	case *phpparser.OrdinaryAssignmentExpressionContext:
		variable := y.VisitLeftVariable(ret.LeftVariable())
		rightValue := y.VisitExpression(ret.Expression())
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), variable, rightValue)
		y.AssignVariable(variable, rightValue)
		return rightValue

	case *phpparser.LogicalExpressionContext:
		var id = uuid.NewString()
		y.AssignVariable(y.CreateVariable(id), y.EmitConstInstAny())
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
		//因为涉及到函数，先peek 如果没有读取到说明是一个常量 （define定义的常量会出现问题）
		var unquote string
		_unquote, err := yakunquote.Unquote(ret.Identifier().GetText())
		if err != nil {
			unquote = ret.Identifier().GetText()
		} else {
			unquote = _unquote
		}
		//先在常量表中查询
		if !y.isFunction {
			if s, ok := y.ReadConst(unquote); ok {
				return s
			}
			log.Warnf("const map not found %v", unquote)
		}
		if value := y.PeekValue(y.VisitIdentifier(ret.Identifier())); value != nil {
			return value
		} else {
			return y.EmitConstInst(y.VisitIdentifier(ret.Identifier()))
		}

	// TODO: static class member
	case *phpparser.StaticClassAccessExpressionContext:
		return y.VisitStaticClassExpr(ret.StaticClassExpr())

	case *phpparser.StaticClassMemberCallAssignmentExpressionContext:
		variable := y.VisitStaticClassExprVariableMember(ret.StaticClassExprVariableMember())
		rightValue := y.VisitExpression(ret.Expression())
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), variable, rightValue)
		y.AssignVariable(variable, rightValue)
		return rightValue
	}
	raw.GetText()
	log.Errorf("unhandled expression: %v(T: %T)", raw.GetText(), raw)
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	return y.EmitConstInstAny()
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
		if v == nil {
			varMain = y.ReadOrCreateMemberCallVariable(varMain, v)
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

	args, ellipsis := y.VisitActualArguments(i.ActualArguments())
	c := y.NewCall(v, args)
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
		return y.ReadValue(text)
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
		panic("NOT IMPL")
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

	obj := y.EmitMakeWithoutType(y.EmitConstInst(l), y.EmitConstInst(l))

	if ret := i.ArrayItemList(); ret != nil {
		for _, kv := range y.VisitArrayItemList(ret.(*phpparser.ArrayItemListContext)) {
			k, v := kv[0], kv[1]
			variable := y.ReadOrCreateMemberCallVariable(obj, k).GetLastVariable()
			y.AssignVariable(variable, v)
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
			k = y.EmitConstInst(countIndex)
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

	//ret := y.ReadVariable(i.GetText(), false)
	//if ret == nil {
	//	return y.EmitConstInst(i.GetText())
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
		array := y.EmitMakeWithoutType(y.EmitConstInst(lnc), y.EmitConstInst(lnc))
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
	//y.GetReferenceFiles()
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
		//rightValue = ssa.CalcConstBinary(y.c, rightValue, ssa.OpPow)
	case "/=":
		rightValue = y.EmitBinOp(ssa.OpDiv, leftValues, rightValue)
	case "%=":
		rightValue = y.EmitBinOp(ssa.OpMod, leftValues, rightValue)
	case ".=":
		rightValue = y.EmitConstInst(leftValues.String() + rightValue.String())
		//rightValue = y.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
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

	return y.CreateVariable(
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
	if value := y.VisitExpression(i.Expression()); value.IsUndefined() {
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
