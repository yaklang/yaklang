package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitExpressionStatement(raw phpparser.IExpressionStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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

	i, _ := raw.(*phpparser.ParenthesesContext)
	if i == nil {
		return nil
	}

	if i.Expression() != nil {
		return y.VisitExpression(i.Expression())
	} else if i.YieldExpression() != nil {
		y.VisitYieldExpression(i.YieldExpression())
	}

	return nil
}

func (y *builder) VisitExpression(raw phpparser.IExpressionContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	if raw.GetText() == "" {
		return nil
	}

	switch ret := raw.(type) {
	case *phpparser.CloneExpressionContext:
		// 浅拷贝，一个对象
		// 如果类定义了 __clone，就执行 __clone
		target := y.VisitExpression(ret.Expression())
		checkCloneBuildin := y.ir.BuildIf()
		checkCloneBuildin.BuildCondition(func() ssa.Value {
			return y.ir.EmitBinOp(
				ssa.OpNotEq,
				y.ir.EmitField(target, y.ir.EmitConstInst("__clone")),
				y.ir.EmitConstInstNil(),
			)
		})

		checkCloneBuildin.BuildTrue(func() {
			// have __clone
			calling := y.ir.NewCall(
				y.ir.EmitField(target, y.ir.EmitConstInst("__clone")),
				nil,
			)
			y.ir.EmitCall(calling)
		})

		//
		return nil
	case *phpparser.KeywordNewExpressionContext:
		return y.VisitNewExpr(ret.NewExpr())
	case *phpparser.IndexerExpressionContext:
		v1 := y.VisitStringConstant(ret.StringConstant())
		indexKey := y.VisitExpression(ret.Expression())
		return y.ir.EmitField(v1, indexKey)
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
		val := y.VisitChain(ret.Chain())
		if ret.Inc() != nil {
			after := y.ir.EmitBinOp(ssa.OpAdd, val, y.ir.EmitConstInst(1))
			y.ir.EmitUpdate(val, after)
			return after
		} else if ret.Dec() != nil {
			after := y.ir.EmitBinOp(ssa.OpSub, val, y.ir.EmitConstInst(1))
			y.ir.EmitUpdate(val, after)
			return after
		}
		return y.ir.EmitConstInstNil()
	case *phpparser.PostfixIncDecExpressionContext:
		val := y.VisitChain(ret.Chain())
		if ret.Inc() != nil {
			after := y.ir.EmitBinOp(ssa.OpAdd, val, y.ir.EmitConstInst(1))
			y.ir.EmitUpdate(val, after)
			return val
		} else if ret.Dec() != nil {
			after := y.ir.EmitBinOp(ssa.OpSub, val, y.ir.EmitConstInst(1))
			y.ir.EmitUpdate(val, after)
			return val
		}
		return y.ir.EmitConstInstNil()
	case *phpparser.PrintExpressionContext:
		return y.ir.EmitConstInst(1)
	case *phpparser.ArrayCreationExpressionContext:
		// arrayCreation
		return y.VisitArrayCreation(ret.ArrayCreation())
	case *phpparser.ChainExpressionContext:
		return y.VisitChain(ret.Chain())
	case *phpparser.ScalarExpressionContext: // constant / string / label
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
		return y.VisitParentheses(ret.Parentheses())
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
	case *phpparser.MatchExpressionContext:
	case *phpparser.ArithmeticExpressionContext:
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
			return y.ir.EmitFieldMust(op1, op2)
		default:
			log.Errorf("unhandled arithmetic expression: %v", ret.GetText())
			return nil
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
			ifStmt := y.ir.BuildIf()
			var v1, v2 ssa.Value
			var result ssa.Value
			ifStmt.BuildCondition(func() ssa.Value {
				v1 = y.VisitExpression(ret.Expression(0))
				return v1
			}).BuildTrue(func() {
				v2 = y.VisitExpression(ret.Expression(1))
				result = y.ir.EmitBinOp(ssa.OpEq, v2, y.ir.EmitConstInst(true))
			}).BuildFalse(func() {
				result = y.ir.EmitConstInst(false)
			})
			ifStmt.Finish()
			return result
		case "||":
			var v1, v2 ssa.Value
			var result ssa.Value
			y.ir.BuildIf().BuildCondition(func() ssa.Value {
				v1 = y.VisitExpression(ret.Expression(0))
				return v1
			}).BuildTrue(func() {
				result = y.ir.EmitConstInst(true)
			}).BuildFalse(func() {
				v2 = y.VisitExpression(ret.Expression(1))
				result = y.ir.EmitBinOp(ssa.OpEq, v2, y.ir.EmitConstInst(true))
			}).Finish()
			return result
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
		ifb := y.ir.BuildIf().BuildCondition(func() ssa.Value {
			// false 0 nil
			t1 := y.ir.EmitBinOp(ssa.OpNotEq, v1, y.ir.EmitConstInstNil())
			t2 := y.ir.EmitBinOp(ssa.OpNotEq, v1, y.ir.EmitConstInst(0))
			t3 := y.ir.EmitBinOp(ssa.OpNotEq, v1, y.ir.EmitConstInst(false))
			return y.ir.EmitBinOp(ssa.OpLogicAnd, t1, y.ir.EmitBinOp(ssa.OpLogicAnd, t2, t3))
		})
		switch exprCount {
		case 2:
			ifb.BuildTrue(func() {
				result = v1
			}).BuildFalse(func() {
				result = y.VisitExpression(ret.Expression(1))
			}).Finish()
			return result
		case 3:
			ifb.BuildTrue(func() {
				result = y.VisitExpression(ret.Expression(1))
			}).BuildFalse(func() {
				result = y.VisitExpression(ret.Expression(2))
			}).Finish()
			return result
		default:
			log.Errorf("unhandled conditional expression: %v", ret.GetText())
			return y.ir.EmitConstInstNil()
		}
	case *phpparser.NullCoalescingExpressionContext:
		v1 := y.VisitExpression(ret.Expression(0))
		var result ssa.Value
		y.ir.BuildIf().BuildCondition(func() ssa.Value {
			return y.ir.EmitBinOp(ssa.OpEq, v1, y.ir.EmitConstInstNil())
		}).BuildTrue(func() {
			result = v1
		}).BuildFalse(func() {
			result = y.VisitExpression(ret.Expression(1))
		})
		return result
	case *phpparser.SpaceshipExpressionContext:
		var result ssa.Value
		var v1, v2 = y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1))
		y.ir.BuildIf().BuildCondition(func() ssa.Value {
			return y.ir.EmitBinOp(ssa.OpEq, v1, v2)
		}).BuildTrue(func() {
			result = y.ir.EmitConstInst(0)
		}).BuildElif(func() ssa.Value {
			return y.ir.EmitBinOp(ssa.OpLt, v1, v2)
		}, func() {
			result = y.ir.EmitConstInst(-1)
		}).BuildFalse(func() {
			result = y.ir.EmitConstInst(1)
		}).Finish()
		return result
	case *phpparser.ArrayDestructExpressionContext:
		// [$1, $2, $3] = $arr;
		// unpacking
	case *phpparser.AssignmentExpressionContext:
		if ret.AssignmentOperator() != nil {
			// assignable assignmentOperator attributes? expression        # AssignmentExpression

			// left value: chain array creation
			leftValues := y.VisitAssignable(ret.Assignable())

			var annotation any
			if ret.Attributes() != nil {
				annotation = y.VisitAttributes(ret.Attributes())
				_ = annotation
			}

			rightValue := y.VisitExpression(ret.Expression())
			operator := ret.AssignmentOperator()
			switch operator.GetText() {
			case "=":
				break
			case "+=":
				rightValue = y.ir.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
			case "-=":
				rightValue = y.ir.EmitBinOp(ssa.OpSub, leftValues, rightValue)
			case "*=":
				rightValue = y.ir.EmitBinOp(ssa.OpMul, leftValues, rightValue)
			case "**=":
				rightValue = y.ir.EmitBinOp(ssa.OpPow, leftValues, rightValue)
			case "/=":
				rightValue = y.ir.EmitBinOp(ssa.OpDiv, leftValues, rightValue)
			case "%=":
				rightValue = y.ir.EmitBinOp(ssa.OpMod, leftValues, rightValue)
			case ".=":
				rightValue = y.ir.EmitFieldMust(leftValues, rightValue)
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
				// 左值为空的时候，才会赋值
				var returnVal = leftValues
				var leftValueIsEmpty ssa.Value
				y.ir.BuildIf().BuildCondition(func() ssa.Value {
					leftValueIsEmpty = y.ir.EmitBinOp(ssa.OpEq, leftValues, y.ir.EmitConstInstNil())
					return leftValueIsEmpty
				}).BuildTrue(func() {
					y.ir.EmitUpdate(leftValues, rightValue)
					returnVal = rightValue
				}).Finish()
				return returnVal
			default:
				log.Errorf("unhandled assignment operator: %v", operator.GetText())
			}
			updateVal := y.ir.EmitUpdate(leftValues, rightValue)
			_ = updateVal
			return rightValue
		} else if ret.Ampersand() != nil {
			// assignable Eq attributes? '&' (chain | newExpr)
			leftValues := y.VisitAssignable(ret.Assignable())
			if ret.Attributes() != nil {
				y.VisitAttributes(ret.Attributes())
			}

			// right val
			if i := ret.Chain(); i != nil {
				y.VisitChain(i)
			} else if i := ret.NewExpr(); i != nil {
				y.VisitNewExpr(i)
			}
			_ = leftValues
		}

	case *phpparser.LogicalExpressionContext:
		if ret.LogicalAnd() != nil {
			ifStmt := y.ir.BuildIf()
			var v1, v2 ssa.Value
			var result ssa.Value
			ifStmt.BuildCondition(func() ssa.Value {
				v1 = y.VisitExpression(ret.Expression(0))
				return v1
			}).BuildTrue(func() {
				v2 = y.VisitExpression(ret.Expression(1))
				result = y.ir.EmitBinOp(ssa.OpEq, v2, y.ir.EmitConstInst(true))
			}).BuildFalse(func() {
				result = y.ir.EmitConstInst(false)
			})
			ifStmt.Finish()
			return result
		} else if ret.LogicalOr() != nil {
			var v1, v2 ssa.Value
			var result ssa.Value
			y.ir.BuildIf().BuildCondition(func() ssa.Value {
				v1 = y.VisitExpression(ret.Expression(0))
				return v1
			}).BuildTrue(func() {
				result = y.ir.EmitConstInst(true)
			}).BuildFalse(func() {
				v2 = y.VisitExpression(ret.Expression(1))
				result = y.ir.EmitBinOp(ssa.OpEq, v2, y.ir.EmitConstInst(true))
			}).Finish()
			return result
		} else if ret.LogicalXor() != nil {
			var v1, v2 ssa.Value
			var result ssa.Value
			v1 = y.VisitExpression(ret.Expression(0))
			v2 = y.VisitExpression(ret.Expression(1))
			y.ir.BuildIf().BuildCondition(func() ssa.Value {
				return v1
			}).BuildTrue(func() {
				result = y.ir.EmitBinOp(ssa.OpEq, v2, y.ir.EmitConstInst(false))
			}).BuildFalse(func() {
				result = y.ir.EmitBinOp(ssa.OpEq, v2, y.ir.EmitConstInst(true))
			}).Finish()
			return result
		} else {
			log.Errorf("unhandled logical expression: %v", ret.GetText())
			return nil
		}
	default:
		ret.GetText()
		log.Errorf("unhandled expression: %v(T: %T)", ret.GetText(), ret)
		log.Errorf("-------------unhandled expression: %v(%T)", ret.GetText(), ret)
		_ = ret
	}

	return nil
}

func (y *builder) VisitAssignable(raw phpparser.IAssignableContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

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

	i, _ := raw.(*phpparser.ChainContext)
	if i == nil {
		return nil
	}

	origin := y.VisitChainOrigin(i.ChainOrigin())

	for _, m := range i.AllMemberAccess() {
		origin = y.ir.EmitField(origin, y.VisitMemberAccess(m))
	}
	return origin
}

func (y *builder) VisitMemberAccess(raw phpparser.IMemberAccessContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.MemberAccessContext)
	if i == nil {
		return nil
	}

	y.VisitKeyedFieldName(i.KeyedFieldName())
	if i.ActualArguments() != nil {
		y.VisitActualArguments(i.ActualArguments())
	}

	return nil
}

func (y *builder) VisitActualArguments(raw phpparser.IActualArgumentsContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ActualArgumentsContext)
	if i == nil {
		return nil
	}

	// PHP8 annotation
	for _, a := range i.AllArguments() {
		y.VisitArguments(a)
	}

	for _, a := range i.AllSquareCurlyExpression() {
		y.VisitSquareCurlyExpression(a)
	}

	return nil
}

func (y *builder) VisitKeyedFieldName(raw phpparser.IKeyedFieldNameContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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

	i, _ := raw.(*phpparser.KeyedVariableContext)
	if i == nil {
		return nil
	}

	dollarCount := len(i.AllDollar())
	var varMain ssa.Value
	if i.VarName() != nil {
		// ($*)$a
		//// {} as index [] as sliceCall
		varMain = y.ir.ReadVariable(i.VarName().GetText(), true)
		for i := 0; i < dollarCount; i++ {
			//TODO: val = y.ir.ReadDynamicVariable(val)
		}
	} else {
		// {} as name [] as sliceCall
		varMain = nil // TODO: y.ir.ReadDynamicVariable()
		for i := 0; i < dollarCount; i++ {
			// val = y.ir.ReadDynamicVariable(val)
		}
	}

	for _, a := range i.AllSquareCurlyExpression() {
		v := y.VisitSquareCurlyExpression(a)
		if v == nil {
			count := y.ir.ReadVariable("count", true)
			calling := y.ir.NewCall(count, nil)
			y.ir.EmitCall(calling)
			v = calling
		}
		varMain = y.ir.EmitField(varMain, v)
	}

	return varMain
}

func (y *builder) VisitKeyedSimpleFieldName(raw phpparser.IKeyedSimpleFieldNameContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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
			return nil
		}
	} else {
		return y.VisitExpression(i.Expression())
	}
}

func (y *builder) VisitFunctionCall(raw phpparser.IFunctionCallContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.FunctionCallContext)
	if i == nil {
		return nil
	}

	v := y.VisitFunctionCallName(i.FunctionCallName())
	c := y.ir.NewCall(v, nil)
	return y.ir.EmitCall(c)
}

func (y *builder) VisitFunctionCallName(raw phpparser.IFunctionCallNameContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.FunctionCallNameContext)
	if i == nil {
		return nil
	}

	if ret := i.QualifiedNamespaceName(); ret != nil {
		return y.VisitQualifiedNamespaceName(ret)
	} else if ret := i.ChainBase(); ret != nil {
		return y.VisitChainBase(ret)
	} else if ret := i.ClassConstant(); ret != nil {
		return y.VisitClassConstant(ret)
	} else if ret := i.Parentheses(); ret != nil {
		return y.VisitParentheses(ret)
	} else if ret := i.Label(); ret != nil {
		return y.ir.ReadVariable(i.Label().GetText(), true)
	}
	log.Errorf("BUG: unknown function call name: %v", i.GetText())
	return nil
}

func (y *builder) VisitChainOrigin(raw phpparser.IChainOriginContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

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

	return nil
}

func (y *builder) VisitChainBase(raw phpparser.IChainBaseContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ChainBaseContext)
	if i == nil {
		return nil
	}

	/*
		$hello = "world";
		$a = "hello";
		echo $$a; // world
	*/
	if ret := i.QualifiedStaticTypeRef(); ret != nil {
		panic("NOT IMPL")
	} else {
		var ret ssa.Value
		for _, i := range i.AllKeyedVariable() {
			if ret == nil {
				ret = y.VisitKeyedVariable(i)
				continue
			}
			ret = y.ir.EmitField(ret, y.VisitKeyedVariable(i))
		}
		return ret
	}
	return nil
}

func (y *builder) VisitArrayCreation(raw phpparser.IArrayCreationContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ArrayCreationContext)
	if i == nil {
		return nil
	}

	val := y.ir.EmitInterfaceMake(func(feed func(key ssa.Value, val ssa.Value)) {
		for _, kv := range y.VisitArrayItemList(i.ArrayItemList()) {
			feed(kv[0], kv[1])
		}
	})

	if i := i.Expression(); i != nil {
		return y.ir.EmitField(val, y.VisitExpression(i))
	}
	return val
}

func (y *builder) VisitArrayItemList(raw phpparser.IArrayItemListContext) [][2]ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

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

	i, _ := raw.(*phpparser.StringConstantContext)
	if i == nil {
		return nil
	}

	ret := y.ir.ReadVariable(i.GetText(), false)
	if ret == nil {
		return y.ir.EmitConstInst(i.GetText())
	}

	return ret
}

func (y *builder) VisitConstantInitializer(raw phpparser.IConstantInitializerContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ConstantInitializerContext)
	if i == nil {
		return nil
	}

	if ret := i.ArrayItemList(); ret != nil {
		return y.ir.EmitInterfaceMake(func(feed func(key ssa.Value, val ssa.Value)) {
			for _, kv := range y.VisitArrayItemList(ret) {
				feed(kv[0], kv[1])
			}
		})
	} else if ret := i.ConstantInitializer(); ret != nil {
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
			initVal = y.ir.EmitField(initVal, y.VisitConstantString(c))
		}
		if initVal == nil {
			log.Errorf("unhandled constant initializer: %v", i.GetText())
			return y.ir.EmitConstInstNil()
		}
		return initVal
	}
}

func (y *builder) VisitConstantString(raw phpparser.IConstantStringContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ConstantStringContext)
	if i == nil {
		return nil
	}

	if r := i.String_(); r != nil {
		return y.VisitString_(r)
	}
	return y.VisitConstant(i.Constant())
}
